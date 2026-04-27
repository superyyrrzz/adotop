// Package ui hosts the Bubble Tea application shell.
package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ado"
	"github.com/renzeyu/adotop/internal/cache"
	"github.com/renzeyu/adotop/internal/config"
	"github.com/renzeyu/adotop/internal/gitlocal"
)

type screen int

const (
	screenList screen = iota
	screenDetail
)

type detailFocus int

const (
	focusFiles detailFocus = iota
	focusDiff
)

type Model struct {
	cfg    config.Config
	client *ado.Client
	git    *gitlocal.Finder
	cache  *cache.Store
	keys   KeyMap

	user   string
	myID   string
	screen screen

	list    ListModel
	detail  DetailModel
	preview DiffModel

	width, height int
	footerErr     string
	showHelp      bool
	useDelta      bool
	previewReqID  int
	previewKey    string
	detailFocus   detailFocus
	scrollMem     map[string]int
	previewCache  *diffBodyCache

	pendingAction pendingAction // empty action == no prompt

	threads        []ado.Thread
	showResolved   bool
	expandedThread map[int]bool
}

// pendingAction is a destructive operation awaiting y/n confirmation.
// `kind` is "" when no prompt is active.
type pendingAction struct {
	kind   string // "abandon" (extend later: "complete", etc.)
	prompt string // text shown to the user, e.g. "Abandon PR #123? (y/N)"
	run    func(m Model) tea.Cmd
}

func New(cfg config.Config, client *ado.Client) Model {
	keys := DefaultKeys()
	m := Model{
		cfg:      cfg,
		client:   client,
		git:      gitlocal.New(cfg.RepoRoots),
		keys:     keys,
		list:          NewList(keys),
		detail:        NewDetail(keys),
		preview:       NewDiff(keys),
		user:          "loading…",
		useDelta:      gitlocal.HasDelta(),
		scrollMem:     map[string]int{},
		previewCache:  newDiffBodyCache(5),
		expandedThread: map[int]bool{},
	}
	st, err := cache.New()
	if err != nil {
		slog.Warn("cache disabled", "err", err)
		return m
	}
	m.cache = st
	if id, ok := st.LoadIdentity(); ok {
		m.user = id.DisplayName
		m.myID = id.UserID
		m.detail = m.detail.SetMyID(m.myID)
	}
	if cfg.Org != "" && cfg.Project != "" {
		for _, tab := range []ado.Tab{ado.TabAssigned, ado.TabCreated, ado.TabReviewRequested} {
			if prs, ok := st.LoadList(cfg.Org, cfg.Project, tab); ok {
				m.list, _ = m.list.Update(prsLoadedMsg{tab: tab, prs: prs})
			}
		}
	}
	if prs, ok := st.LoadRecents(); ok {
		m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})
	}
	return m
}

type connDataMsg struct {
	data *ado.ConnectionData
	err  error
}

type tickMsg time.Time

// actionDoneMsg is the result of a write action (approve, abandon, etc.).
type actionDoneMsg struct {
	kind  string // "approve" | "abandon"
	prID  int
	err   error
	notes string // optional human note shown on success ("voted approve")
}

// threadsLoadedMsg is the response from GetPullRequestThreads.
type threadsLoadedMsg struct {
	threads []ado.Thread
	err     error
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchConnectionData(), tick(m.cfg.RefreshInterval.Duration))
}

func (m Model) fetchConnectionData() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := m.client.GetConnectionData(ctx)
		return connDataMsg{data: d, err: err}
	}
}

func (m Model) loadList(tab ado.Tab) tea.Cmd {
	if tab == ado.TabRecents {
		var prs []ado.PRSummary
		if m.cache != nil {
			prs, _ = m.cache.LoadRecents()
		}
		return func() tea.Msg {
			return prsLoadedMsg{tab: ado.TabRecents, prs: prs}
		}
	}
	if m.myID == "" || m.cfg.Project == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		prs, err := m.client.ListPullRequests(ctx, ado.ListPRFilter{
			Project: m.cfg.Project, Tab: tab, MyID: m.myID,
		})
		return prsLoadedMsg{tab: tab, prs: prs, err: err}
	}
}

func (m Model) loadDetail(s ado.PRSummary) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			d, err := m.client.GetPullRequest(ctx, s.RepoID, s.ID, m.myID)
			return detailLoadedMsg{detail: d, err: err}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			files, err := m.client.GetIterationChanges(ctx, s.RepoID, s.ID)
			return filesLoadedMsg{files: files, err: err}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			st, err := m.client.GetStatuses(ctx, s.RepoID, s.ID)
			return statusesLoadedMsg{statuses: st, err: err}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			threads, err := m.client.GetPullRequestThreads(ctx, s.RepoID, s.ID)
			return threadsLoadedMsg{threads: threads, err: err}
		},
	)
}

// approveCurrent issues a vote=10 against the current PR. Safe to call
// repeatedly — ADO treats it as setting the vote to 10.
func (m Model) approveCurrent() tea.Cmd {
	s := m.detail.Summary()
	if s.ID == 0 || s.RepoID == "" || m.myID == "" {
		return func() tea.Msg {
			return actionDoneMsg{kind: "approve", prID: s.ID, err: fmt.Errorf("approve: missing PR/repo/identity")}
		}
	}
	repoID, prID := s.RepoID, s.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.SetReviewerVote(ctx, repoID, prID, m.myID, 10)
		return actionDoneMsg{kind: "approve", prID: prID, err: err, notes: "voted approve"}
	}
}

// abandonCurrent flips the PR status to abandoned.
func (m Model) abandonCurrent() tea.Cmd {
	s := m.detail.Summary()
	if s.ID == 0 || s.RepoID == "" {
		return func() tea.Msg {
			return actionDoneMsg{kind: "abandon", prID: s.ID, err: fmt.Errorf("abandon: missing PR/repo")}
		}
	}
	repoID, prID := s.RepoID, s.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.AbandonPullRequest(ctx, repoID, prID)
		return actionDoneMsg{kind: "abandon", prID: prID, err: err, notes: "abandoned"}
	}
}

func (m Model) loadDiff(target diffTarget, current DiffModel, requestID int, s ado.PRSummary, file ado.FileChange, sourceSha, targetSha string) (DiffModel, tea.Cmd) {
	renderer := "rest"
	var clonePath string
	if p, ok := m.git.Find(s.Repo, m.cfg.Org); ok {
		clonePath = p
		if m.useDelta {
			renderer = "local+delta"
		} else {
			renderer = "local"
		}
	}
	dm := m.sizeDiffModel(current.SetHeader(file.Path, renderer), target)
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), m.useDelta)
			return diffLoadedMsg{content: out, err: err, target: target, requestID: requestID}
		}
		src, tgt, err := m.client.GetFileContents(ctx, s.RepoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return diffLoadedMsg{err: err, target: target, requestID: requestID}
		}
		return diffLoadedMsg{content: simpleDiff(tgt, src, file.Path), target: target, requestID: requestID}
	}
	return dm, cmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var c1, c2 tea.Cmd
		m.list, c1 = m.list.Update(msg)
		m.detail, c2 = m.detail.Update(msg)
		m.preview = m.sizeDiffModel(m.preview, diffTargetPreview)
		return m, tea.Batch(c1, c2)
	case connDataMsg:
		if msg.err != nil {
			m.footerErr = "auth: " + msg.err.Error()
			return m, nil
		}
		m.user = msg.data.DisplayName()
		m.myID = msg.data.AuthenticatedUser.ID
		m.detail = m.detail.SetMyID(m.myID)
		if m.cache != nil {
			if err := m.cache.SaveIdentity(m.myID, m.user); err != nil {
				slog.Warn("cache: save identity", "err", err)
			}
		}
		return m, m.loadList(m.list.Tab())
	case tickMsg:
		var cmd tea.Cmd
		if m.screen == screenList {
			cmd = m.loadList(m.list.Tab())
		}
		return m, tea.Batch(cmd, tick(m.cfg.RefreshInterval.Duration))
	case tabSwitchedMsg:
		return m, m.loadList(msg.Tab)
	case prsLoadedMsg:
		if msg.err == nil && m.cache != nil && m.cfg.Org != "" && m.cfg.Project != "" && msg.tab != ado.TabRecents {
			if err := m.cache.SaveList(m.cfg.Org, m.cfg.Project, msg.tab, msg.prs); err != nil {
				slog.Warn("cache: save list", "tab", msg.tab, "err", err)
			}
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case detailLoadedMsg:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	case filesLoadedMsg:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	case statusesLoadedMsg:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	case threadsLoadedMsg:
		if msg.err == nil {
			m.threads = msg.threads
			m = m.refreshPreview()
		}
		return m, nil
	case diffLoadedMsg:
		if msg.target != diffTargetPreview {
			return m, nil
		}
		if msg.requestID != m.previewReqID {
			return m, nil
		}
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		if msg.err == nil && m.previewKey != "" {
			m.previewCache.Set(m.detail.Summary().ID, m.previewKey, msg.content)
		}
		if off, ok := m.scrollMem[m.previewKey]; ok {
			m.preview.vp.SetYOffset(off)
		}
		m = m.refreshPreview()
		return m, cmd
	case prefetchLoadedMsg:
		if msg.err == nil {
			m.previewCache.Set(m.detail.Summary().ID, msg.key, msg.content)
		} else {
			m.previewCache.Drop(msg.key)
		}
		return m, nil
	case jumpRequestedMsg:
		prID := msg.ID
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			d, err := m.client.GetPullRequestByID(ctx, prID, m.myID)
			if err != nil {
				return jumpResultMsg{prID: prID, err: err}
			}
			return jumpResultMsg{prID: prID, summary: d.PRSummary}
		}
	case jumpResultMsg:
		if msg.err != nil {
			m.footerErr = fmt.Sprintf("jump #%d: %v", msg.prID, msg.err)
			return m, nil
		}
		mm, cmd := m.openDetail(msg.summary)
		return mm, cmd
	case actionDoneMsg:
		if msg.err != nil {
			m.footerErr = fmt.Sprintf("%s PR #%d: %s", msg.kind, msg.prID, friendlyErr(msg.err))
			return m, nil
		}
		m.footerErr = fmt.Sprintf("PR #%d %s", msg.prID, msg.notes)
		// Refresh the detail in place so vote/status update; also bust the
		// list cache for the active tab so the row reflects the change.
		var cmds []tea.Cmd
		if m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
			cmds = append(cmds, m.loadDetail(m.detail.Summary()))
		}
		cmds = append(cmds, m.loadList(m.list.Tab()))
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		// Confirmation prompt swallows all keys until resolved.
		if m.pendingAction.kind != "" {
			if keyMatches(msg, m.keys.ConfirmYes) {
				run := m.pendingAction.run
				m.pendingAction = pendingAction{}
				return m, run(m)
			}
			if keyMatches(msg, m.keys.ConfirmNo) {
				m.pendingAction = pendingAction{}
				return m, nil
			}
			return m, nil
		}
		if keyMatches(msg, m.keys.QuitForce) {
			return m, tea.Quit
		}
		if keyMatches(msg, m.keys.Help) {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if m.showHelp {
			if msg.Type == tea.KeyEsc {
				m.showHelp = false
			}
			return m, nil
		}
		switch m.screen {
		case screenList:
			return m.updateListScreen(msg)
		case screenDetail:
			return m.updateDetailScreen(msg)
		}
	}
	switch m.screen {
	case screenList:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case screenDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	return m, nil
}

// openDetail switches to the Detail screen for s, records the visit in the
// recents cache, and returns the load command. Idempotent: caller may call
// from list, recents tab, or ID-jump.
func (m Model) openDetail(s ado.PRSummary) (Model, tea.Cmd) {
	m.detail = m.detail.SetSummary(s)
	m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
	m.previewKey = ""
	m.detailFocus = focusFiles
	m.scrollMem = map[string]int{}
	m.threads = nil
	m.expandedThread = map[int]bool{}
	m.screen = screenDetail
	// NOTE: previewCache survives PR re-open so bouncing list↔detail
	// stays snappy. Refresh (R) explicitly clears the current PR below.
	if m.cache != nil {
		if err := m.cache.RecordVisit(s); err != nil {
			slog.Warn("cache: record visit", "pr", s.ID, "err", err)
		}
		if recents, ok := m.cache.LoadRecents(); ok {
			m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: recents})
		}
	}
	return m, m.loadDetail(s)
}

func (m Model) updateListScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a prompt (filter or jump-to-ID) is active, route all keys to the
	// list so its prompt handlers see Enter/Esc/Backspace before the global
	// Open/Quit/Browser shortcuts intercept them.
	if m.list.filtering || m.list.jumping {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	switch {
	case keyMatches(msg, m.keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.Open):
		if s, ok := m.list.Selected(); ok {
			mm, cmd := m.openDetail(s)
			return mm, cmd
		}
	case keyMatches(msg, m.keys.Refresh):
		return m, m.loadList(m.list.Tab())
	case keyMatches(msg, m.keys.Browser):
		if s, ok := m.list.Selected(); ok {
			if s.URL == "" {
				slog.Warn("open in browser: PR has no URL", "pr", s.ID)
			} else if err := OpenInBrowser(s.URL); err != nil {
				slog.Warn("open in browser failed", "url", s.URL, "err", err)
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDetailScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyTab:
		if m.detailFocus == focusFiles {
			m.detailFocus = focusDiff
		} else {
			m.detailFocus = focusFiles
		}
		return m, nil
	case msg.Type == tea.KeyShiftTab:
		if m.detailFocus == focusDiff {
			m.detailFocus = focusFiles
		} else {
			m.detailFocus = focusDiff
		}
		return m, nil
	case keyMatches(msg, m.keys.Back):
		m.screen = screenList
		m.detailFocus = focusFiles
		return m, nil
	case keyMatches(msg, m.keys.Quit):
		// On Detail, q acts like Back so a stray keystroke doesn't kill
		// the program. ctrl+c still quits unconditionally.
		m.screen = screenList
		m.detailFocus = focusFiles
		return m, nil
	case keyMatches(msg, m.keys.Refresh):
		m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
		m.previewKey = ""
		m.scrollMem = map[string]int{}
		m.previewCache.ClearPR(m.detail.Summary().ID)
		return m, m.loadDetail(m.detail.Summary())
	case keyMatches(msg, m.keys.Browser):
		u := m.detail.Summary().URL
		if u == "" {
			slog.Warn("open in browser: PR has no URL", "pr", m.detail.Summary().ID)
		} else if err := OpenInBrowser(u); err != nil {
			slog.Warn("open in browser failed", "url", u, "err", err)
		}
		return m, nil
	case keyMatches(msg, m.keys.Approve):
		return m, m.approveCurrent()
	case keyMatches(msg, m.keys.ShowResolved):
		m.showResolved = !m.showResolved
		m = m.refreshPreview()
		return m, nil
	case keyMatches(msg, m.keys.Open):
		if m.detailFocus == focusDiff {
			// Toggle expansion of every (visible) thread on the focused file.
			f, ok := m.detail.SelectedFile()
			if ok {
				m = m.toggleThreadsForFile(f.Path)
				m = m.refreshPreview()
			}
			return m, nil
		}
		return m, nil
	case keyMatches(msg, m.keys.Abandon):
		s := m.detail.Summary()
		if s.ID == 0 {
			return m, nil
		}
		m.pendingAction = pendingAction{
			kind:   "abandon",
			prompt: fmt.Sprintf("Abandon PR #%d? (y/esc)", s.ID),
			run:    func(mm Model) tea.Cmd { return mm.abandonCurrent() },
		}
		return m, nil
	case keyMatches(msg, m.keys.NextFile):
		if m.detail.cursor < len(m.detail.files)-1 {
			m.detail.cursor++
		}
		mm, previewCmd := m.queuePreviewForSelection()
		return mm, previewCmd
	case keyMatches(msg, m.keys.PrevFile):
		if m.detail.cursor > 0 {
			m.detail.cursor--
		}
		mm, previewCmd := m.queuePreviewForSelection()
		return mm, previewCmd
	}
	if m.detailFocus == focusDiff {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}
	before, beforeOK := m.detail.SelectedFile()
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	after, afterOK := m.detail.SelectedFile()
	if afterOK && (!beforeOK || before.Path != after.Path) {
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	}
	return m, cmd
}

func (m Model) View() string {
	header := Header.Render(fmt.Sprintf("adotop  %s/%s  user=%s", orPlaceholder(m.cfg.Org, "(no org)"), orPlaceholder(m.cfg.Project, "(no project)"), m.user))
	var body string
	switch m.screen {
	case screenList:
		body = m.list.View()
	case screenDetail:
		body = m.detailPreviewView()
	}
	if m.showHelp {
		body = HelpBox.Render(strings.Join([]string{
			"Help",
			"",
			"  ?           toggle this help",
			"  q           quit (list) / back (detail); ctrl+c always quits",
			"  r           refresh current screen",
			"  o           open in browser",
			"  /           filter (list)",
			"  #           jump to PR by ID (list)",
			"  tab/shift+tab  switch focus (Detail)",
			"  n / N       next / prev file (Detail)",
			"  ↑↓ pgup/pgdn g/G  scroll focused pane",
			"  a           approve PR (Detail)",
			"  X           abandon PR (Detail, confirms)",
			"  enter       expand comment threads on focused file (Diff focus)",
			"  R           toggle showing resolved comments",
			"  esc         back",
		}, "\n"))
	}
	footer := Footer.Render(footerHints(m.screen))
	if m.pendingAction.kind != "" {
		footer = ErrLine.Render(m.pendingAction.prompt)
	} else if m.footerErr != "" {
		footer = ErrLine.Render(m.footerErr)
	}
	// Pad the body so the footer sticks to the bottom of the terminal,
	// regardless of how much content the current screen produced.
	if m.height > 0 {
		headerH := lipgloss.Height(header)
		footerH := lipgloss.Height(footer)
		bodyH := lipgloss.Height(body)
		// 2 blank-line spacers between header/body and body/footer.
		used := headerH + footerH + bodyH + 2
		if pad := m.height - used; pad > 0 {
			body = body + strings.Repeat("\n", pad)
		}
	}
	return strings.Join([]string{header, "", body, "", footer}, "\n")
}

func footerHints(s screen) string {
	switch s {
	case screenList:
		return "/:filter  #:goto  enter:open  o:browser  r:refresh  tab:next  ?:help  q:quit"
	case screenDetail:
		return "tab:focus  n/N:file  ↑↓ pgup/pgdn g/G:scroll  enter:expand-comments  R:show-resolved  a:approve  X:abandon  o:browser  esc/q:back  r:refresh  ?:help"
	}
	return ""
}

func (m Model) queuePreviewForSelection() (Model, tea.Cmd) {
	if m.screen != screenDetail || m.detail.Detail() == nil {
		return m, nil
	}
	f, ok := m.detail.SelectedFile()
	if !ok {
		return m, nil
	}
	key := diffSelectionKey(m.detail.Detail().SourceSha, m.detail.Detail().TargetSha, f.Path)
	if key == m.previewKey {
		return m, nil
	}
	if m.previewKey != "" {
		m.scrollMem[m.previewKey] = m.preview.vp.YOffset
	}
	m.previewKey = key

	// Cache hit: render immediately (no async msg) and prefetch neighbors.
	if body, ok := m.previewCache.Get(key); ok && body != nil {
		renderer := "rest"
		if p, ok := m.git.Find(m.detail.Summary().Repo, m.cfg.Org); ok && p != "" {
			if m.useDelta {
				renderer = "local+delta"
			} else {
				renderer = "local"
			}
		}
		m.preview = m.sizeDiffModel(m.preview.SetHeader(f.Path, renderer), diffTargetPreview)
		m.previewReqID++
		// Pull the precolorized render so we skip Colorize on every j/k.
		rendered, _ := m.previewCache.Rendered(key)
		m.preview, _ = m.preview.Update(diffLoadedMsg{
			content: body, rendered: rendered, target: diffTargetPreview, requestID: m.previewReqID,
		})
		if off, ok := m.scrollMem[key]; ok {
			m.preview.vp.SetYOffset(off)
		}
		m = m.refreshPreview()
		mm, prefetchCmd := m.prefetchNeighbors()
		return mm, prefetchCmd
	}

	m.previewReqID++
	pm, cmd := m.loadDiff(diffTargetPreview, m.preview, m.previewReqID, m.detail.Summary(), f, m.detail.Detail().SourceSha, m.detail.Detail().TargetSha)
	m.preview = pm
	mm, prefetchCmd := m.prefetchNeighbors()
	m = mm
	return m, tea.Batch(cmd, prefetchCmd)
}

type prefetchLoadedMsg struct {
	key     string
	content []byte
	err     error
}

type jumpResultMsg struct {
	prID    int
	summary ado.PRSummary
	err     error
}

func (m Model) prefetchNeighbors() (Model, tea.Cmd) {
	if m.detail.Detail() == nil {
		return m, nil
	}
	d := m.detail.Detail()
	files := m.detail.files
	prID := m.detail.Summary().ID
	// Prefetch the ±3 nearest files in DISPLAY order so the cache warms
	// the rows the user will actually navigate to with j/k, not the
	// API-order neighbors that rarely match the sorted tree.
	const radius = 3
	idxs := m.detail.DisplayNeighbors(radius)
	var cmds []tea.Cmd
	for _, idx := range idxs {
		if idx < 0 || idx >= len(files) {
			continue
		}
		f := files[idx]
		key := diffSelectionKey(d.SourceSha, d.TargetSha, f.Path)
		if _, ok := m.previewCache.Get(key); ok {
			continue
		}
		// Reserve cache slot (nil) so we don't re-issue while in flight.
		m.previewCache.Reserve(prID, key)
		cmds = append(cmds, m.prefetchOne(f, key, d.SourceSha, d.TargetSha))
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m Model) prefetchOne(file ado.FileChange, key, sourceSha, targetSha string) tea.Cmd {
	s := m.detail.Summary()
	var clonePath string
	if p, ok := m.git.Find(s.Repo, m.cfg.Org); ok {
		clonePath = p
	}
	useDelta := m.useDelta
	repoID := s.RepoID
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), useDelta)
			return prefetchLoadedMsg{key: key, content: out, err: err}
		}
		if client == nil {
			return prefetchLoadedMsg{key: key, err: nil, content: nil}
		}
		src, tgt, err := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return prefetchLoadedMsg{key: key, err: err}
		}
		return prefetchLoadedMsg{key: key, content: simpleDiff(tgt, src, file.Path)}
	}
}

func (m Model) detailPreviewView() string {
	layout := m.detailLayout()
	detail := m.detail.SetPaneSize(layout.leftWidth, layout.bodyHeight)
	left := detail.ViewWithFocus(m.detailFocus == focusFiles)
	right := m.previewPaneView()
	if !layout.split {
		return strings.Join([]string{left, "", right}, "\n")
	}
	leftPane := lipgloss.NewStyle().
		Width(layout.leftWidth).
		MaxWidth(layout.leftWidth).
		Height(layout.bodyHeight).
		Render(left)
	rightPane := lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(lipgloss.Color("8")).
		PaddingLeft(1).
		Width(layout.rightWidth).
		MaxWidth(layout.rightWidth).
		Height(layout.bodyHeight).
		Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m Model) previewPaneView() string {
	dot := "○ "
	if m.detailFocus == focusDiff {
		dot = "● "
	}
	title := Header.Render(dot + "Diff Preview")
	if m.preview.file == "" {
		if _, ok := m.detail.SelectedFile(); !ok {
			return title + "\n" + Faint.Render("No changed files available.")
		}
		return title + "\n" + Faint.Render("Loading selected file diff…")
	}
	return title + "\n" + m.preview.View()
}

type previewLayout struct {
	split      bool
	bodyHeight int
	leftWidth  int
	rightWidth int
}

func (m Model) detailLayout() previewLayout {
	layout := previewLayout{
		bodyHeight: maxInt(10, m.height-4),
		leftWidth:  80,
		rightWidth: 80,
	}
	if m.width <= 0 {
		return layout
	}
	if m.width < 100 {
		layout.leftWidth = m.width
		layout.rightWidth = m.width
		return layout
	}
	left := m.width * 2 / 5
	left = maxInt(36, minInt(left, m.width-40))
	right := maxInt(30, m.width-left-1)
	if right < 30 {
		layout.leftWidth = m.width
		layout.rightWidth = m.width
		return layout
	}
	layout.split = true
	layout.leftWidth = left
	layout.rightWidth = right
	return layout
}

func (m Model) sizeDiffModel(dm DiffModel, target diffTarget) DiffModel {
	width, height := m.diffViewportSize(target)
	return dm.SetSize(width, height)
}

func (m Model) diffViewportSize(target diffTarget) (int, int) {
	switch target {
	case diffTargetPreview:
		layout := m.detailLayout()
		if layout.split {
			return maxInt(20, layout.rightWidth-2), maxInt(3, layout.bodyHeight-2)
		}
		return maxInt(20, layout.rightWidth), maxInt(6, layout.bodyHeight/2-2)
	default:
		return maxInt(20, m.width), maxInt(3, m.height-5)
	}
}

func diffSelectionKey(sourceSha, targetSha, path string) string {
	return sourceSha + "\x00" + targetSha + "\x00" + path
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func orPlaceholder(s, p string) string {
	if s == "" {
		return p
	}
	return s
}

// friendlyErr unwraps an ADO APIError to its server-supplied "message" field
// so the footer shows e.g. "Pull request is completed" instead of a raw 400
// dump. Falls back to err.Error() for non-API errors.
func friendlyErr(err error) string {
	var apiErr *ado.APIError
	if errors.As(err, &apiErr) {
		return apiErr.FriendlyMessage()
	}
	return err.Error()
}

// simpleDiff produces a unified diff (target → source) for the REST
// fallback when no local clone is available. Uses an LCS-based line
// comparison so the output is line-accurate (not a wholesale "everything
// removed, everything added" dump). Applies the same whitespace
// normalization as the local-git path so it matches the ADO web UI:
//   - CRLF → LF
//   - trailing whitespace stripped
// Set ADOTOP_DIFF_STRICT=1 to skip normalization.
func simpleDiff(target, source []byte, path string) []byte {
	t := normalizeForDiff(string(target))
	s := normalizeForDiff(string(source))
	tl := strings.Split(t, "\n")
	sl := strings.Split(s, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "--- a%s\n+++ b%s\n", path, path)
	hunks := lcsUnifiedHunks(tl, sl, 3)
	for _, h := range hunks {
		b.WriteString(h)
	}
	return []byte(b.String())
}

func normalizeForDiff(s string) string {
	if v := strings.TrimSpace(os.Getenv("ADOTOP_DIFF_STRICT")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// lcsUnifiedHunks computes a Myers-style LCS between two line slices and
// emits unified-diff hunks with the given context size. Good enough to
// match what `git diff` would produce for typical PR-sized files.
func lcsUnifiedHunks(a, b []string, ctx int) []string {
	n, m := len(a), len(b)
	// Build LCS length table.
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	// Walk the table to emit ops: " " (equal), "-" (a-only), "+" (b-only).
	type op struct {
		kind byte
		text string
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for i < n {
		ops = append(ops, op{'-', a[i]})
		i++
	}
	for j < m {
		ops = append(ops, op{'+', b[j]})
		j++
	}

	// Group ops into hunks with `ctx` lines of surrounding context.
	var hunks []string
	k := 0
	for k < len(ops) {
		// Find the next change.
		for k < len(ops) && ops[k].kind == ' ' {
			k++
		}
		if k >= len(ops) {
			break
		}
		// Hunk start = max(0, k-ctx).
		start := k - ctx
		if start < 0 {
			start = 0
		}
		// Find hunk end: walk forward including changes; allow up to 2*ctx
		// equal lines between change blocks before splitting.
		end := k
		gap := 0
		for end < len(ops) {
			if ops[end].kind == ' ' {
				gap++
				if gap > 2*ctx {
					end -= gap - ctx
					break
				}
			} else {
				gap = 0
			}
			end++
		}
		if end > len(ops) {
			end = len(ops)
		}
		// Compute line numbers for header.
		var aStart, bStart, aLen, bLen int
		for x := 0; x < start; x++ {
			if ops[x].kind != '+' {
				aStart++
			}
			if ops[x].kind != '-' {
				bStart++
			}
		}
		aStart++
		bStart++
		var body strings.Builder
		for x := start; x < end; x++ {
			body.WriteByte(ops[x].kind)
			body.WriteString(ops[x].text)
			body.WriteByte('\n')
			switch ops[x].kind {
			case ' ':
				aLen++
				bLen++
			case '-':
				aLen++
			case '+':
				bLen++
			}
		}
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
		hunks = append(hunks, header+body.String())
		k = end
	}
	return hunks
}

// Run starts the Bubble Tea program with the alt screen.
func Run(cfg config.Config, client *ado.Client) error {
	p := tea.NewProgram(New(cfg, client), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

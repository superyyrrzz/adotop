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

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/cache"
	"github.com/superyyrrzz/adotop/internal/config"
	"github.com/superyyrrzz/adotop/internal/gitlocal"
	"github.com/superyyrrzz/adotop/internal/ui/theme"
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
	// footerErr is the sticky red banner for write-action failures
	// (vote/abandon errors, jump-to-PR misses). Cleared on next
	// keypress; "press any key to dismiss" is shown alongside.
	footerErr string
	// footerOK is the sticky neutral banner for write-action successes
	// ("PR #N abandoned"). Cleared on next keypress; no dismiss cue
	// because success is informational, not blocking.
	footerOK string
	showHelp      bool
	useDelta      bool
	previewReqID  int
	previewKey    string
	detailFocus   detailFocus
	scrollMem     map[string]int
	previewCache  *diffBodyCache

	pendingAction pendingAction // empty action == no prompt

	// voteMenu is true when the `v` overlay is open and waiting for a
	// vote selection (a/s/w/r/c/esc). Modal: while open, all keypresses
	// route to the menu and global bindings (Refresh, ShowResolved, etc.)
	// are ignored.
	voteMenu bool

	// pendingG is set when the user pressed `g` and we're waiting for
	// the second key of the vim-style `gg` sequence. Any other key
	// cancels the pending state. Reset on screen change.
	pendingG bool

	threads        []ado.Thread
	showResolved   bool
	expandedThread map[int]bool
	// wrapDiff toggles soft-wrap on the diff preview viewport. Off by
	// default — most lines fit and visual line counts matter for
	// scanning large diffs. Press `w` to enable when reading a file
	// with long lines (minified JS, generated code, long URLs).
	wrapDiff bool

	// diffCtx is the unified-diff context size for the preview pane.
	// 0 = "use the default" (3 lines, matches `git diff` and ADO web).
	// Positive = explicit line count. -1 = "all" (full file, no folding).
	// `+`/`-` cycle through ctxLadder. The cache key includes this value
	// so revisiting a level is instant.
	diffCtx int

	// detailInflight counts how many of the four detail-screen background
	// fetches (detail, files, statuses, threads) are currently outstanding.
	// Bumped by loadDetail, decremented by each *LoadedMsg handler. When >0
	// the statusline shows a small ↻ glyph so the user knows the cached
	// snapshot they're looking at is being verified against the server.
	detailInflight int
}

// pendingAction is a destructive operation awaiting y/n confirmation.
// `kind` is "" when no prompt is active.
type pendingAction struct {
	kind   string // "abandon" (extend later: "complete", etc.)
	prompt string // text shown to the user, e.g. "Abandon PR #123? (y/N)"
	run    func(m Model) tea.Cmd
}

func New(cfg config.Config, client *ado.Client) Model {
	// Resolve theme once at startup. ADOTOP_THEME can be "light",
	// "dark", "auto", or empty (auto-detect from terminal background).
	th := theme.New(os.Getenv("ADOTOP_THEME"))
	applyStyles(th)
	applyDiffTheme(th)
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
	kind  string // "vote" | "abandon"
	prID  int
	err   error
	notes string // optional human note shown on success ("voted approve")

	// vote, when kind=="vote", is the vote value we just wrote (10/-10
	// etc.). The detail screen uses this for an optimistic update
	// because the round-trip GetPullRequest sometimes echoes the
	// reviewer back under a different ID descriptor (e.g., the AAD
	// group instead of the user) and our `rv.ID == myID` match misses.
	vote int
}

// threadsLoadedMsg is the response from GetPullRequestThreads.
type threadsLoadedMsg struct {
	threads   []ado.Thread
	err       error
	fromCache bool
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

// loadDetail fans out the four detail-screen fetches (PR, files,
// statuses, threads) and — when a cached snapshot exists for this PR —
// also dispatches synthetic *LoadedMsg events with the cached payload
// so the screen paints instantly. The fresh fetches still run in
// parallel and overwrite cached values via the existing Update path
// when they return.
//
// detailInflight tracks how many fetches are outstanding so the
// statusline can show a refresh indicator while the cached view is
// being verified against the server.
func (m Model) loadDetail(s ado.PRSummary) (Model, tea.Cmd) {
	m.detailInflight = 4

	cmds := make([]tea.Cmd, 0, 8)
	if m.cache != nil {
		if snap, ok := m.cache.LoadDetail(s.ID); ok {
			// Each cached field comes back as the same *LoadedMsg the
			// network path uses. Empty-field omission keeps the existing
			// "loading" placeholders if a previous session never managed
			// to fetch that endpoint.
			if snap.Detail != nil {
				d := snap.Detail
				cmds = append(cmds, func() tea.Msg { return detailLoadedMsg{detail: d, fromCache: true} })
			}
			if snap.Files != nil {
				files := snap.Files
				cmds = append(cmds, func() tea.Msg { return filesLoadedMsg{files: files, fromCache: true} })
			}
			if snap.Statuses != nil {
				st := snap.Statuses
				cmds = append(cmds, func() tea.Msg { return statusesLoadedMsg{statuses: st, fromCache: true} })
			}
			if snap.Threads != nil {
				th := snap.Threads
				cmds = append(cmds, func() tea.Msg { return threadsLoadedMsg{threads: th, fromCache: true} })
			}
		}
	}

	cmds = append(cmds,
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
	return m, tea.Batch(cmds...)
}

// markDetailFetchDone decrements the in-flight counter that powers the
// statusline refresh indicator. Bottoms out at 0 so an extra arrival
// (cached + fresh both for the same field) doesn't underflow. We rely
// on counter==0 as the "all four fetches accounted for" signal.
func (m Model) markDetailFetchDone() Model {
	if m.detailInflight > 0 {
		m.detailInflight--
	}
	return m
}

// persistDetailField writes the current detail snapshot to the
// per-PR cache file, applying mutate to whatever is on disk first
// (read-modify-write so a parallel fetch's prior write isn't lost).
// Best-effort: cache failures are logged but don't surface to the user.
func (m Model) persistDetailField(mutate func(snap *cache.DetailSnapshot)) {
	if m.cache == nil {
		return
	}
	prID := m.detail.Summary().ID
	if prID == 0 {
		return
	}
	snap, _ := m.cache.LoadDetail(prID)
	if snap == nil {
		snap = &cache.DetailSnapshot{PRID: prID}
	}
	snap.PRID = prID
	mutate(snap)
	if err := m.cache.SaveDetail(*snap); err != nil {
		slog.Warn("cache: save detail", "pr", prID, "err", err)
	}
}

// maybeDropTerminalCache removes the cached snapshot when the PR has
// transitioned to a terminal state (completed/abandoned). Those PRs
// don't change again, so the disk slot is better spent on active ones.
func (m Model) maybeDropTerminalCache(d *ado.PRDetail) {
	if m.cache == nil || d == nil {
		return
	}
	switch strings.ToLower(d.Status) {
	case "completed", "abandoned":
		if err := m.cache.DropDetail(d.ID); err != nil {
			slog.Warn("cache: drop terminal", "pr", d.ID, "err", err)
		}
	}
}

// approveCurrent issues a vote=10 against the current PR. Safe to call
// repeatedly — ADO treats it as setting the vote to 10.
func (m Model) approveCurrent() tea.Cmd {
	return m.setVoteCurrent(10, "voted approve")
}

// setVoteCurrent sets the caller's vote on the active PR. vote uses
// ADO's scale: 10=approved, 5=approved-with-suggestions, 0=no-vote,
// -5=waiting-for-author, -10=rejected. label is the success message
// shown in the footer.
//
// Logs the request to the file-only slog at Info level so we have a
// record of what was attempted; the result lands as actionDoneMsg and
// is logged by that handler at Error level if it fails.
func (m Model) setVoteCurrent(vote int, label string) tea.Cmd {
	s := m.detail.Summary()
	if s.ID == 0 || s.RepoID == "" || m.myID == "" {
		slog.Info("action skipped", "kind", "vote", "pr", s.ID, "reason", "missing PR/repo/identity")
		return func() tea.Msg {
			return actionDoneMsg{kind: "vote", prID: s.ID, err: fmt.Errorf("vote: missing PR/repo/identity")}
		}
	}
	repoID, prID := s.RepoID, s.ID
	slog.Info("action requested", "kind", "vote", "pr", prID, "vote", vote, "label", label)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.SetReviewerVote(ctx, repoID, prID, m.myID, vote)
		return actionDoneMsg{kind: "vote", prID: prID, err: err, notes: label, vote: vote}
	}
}

// abandonCurrent flips the PR status to abandoned.
func (m Model) abandonCurrent() tea.Cmd {
	s := m.detail.Summary()
	if s.ID == 0 || s.RepoID == "" {
		slog.Info("action skipped", "kind", "abandon", "pr", s.ID, "reason", "missing PR/repo")
		return func() tea.Msg {
			return actionDoneMsg{kind: "abandon", prID: s.ID, err: fmt.Errorf("abandon: missing PR/repo")}
		}
	}
	repoID, prID := s.RepoID, s.ID
	slog.Info("action requested", "kind", "abandon", "pr", prID)
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
	ctxN := ctxLines(m.diffCtx)
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), m.useDelta, ctxN)
			return diffLoadedMsg{content: out, err: err, target: target, requestID: requestID}
		}
		src, tgt, err := m.client.GetFileContents(ctx, s.RepoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return diffLoadedMsg{err: err, target: target, requestID: requestID}
		}
		return diffLoadedMsg{content: simpleDiff(tgt, src, file.Path, ctxN), target: target, requestID: requestID}
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
		// Refresh all three server-backed tabs regardless of screen so
		// when the user returns from detail view the list is current.
		// Recents is a local-only synthesized view (no server endpoint),
		// so it's skipped — UpdatePR keeps it fresh from detail loads
		// and PatchRecents persists it across restarts.
		var cmds []tea.Cmd
		for _, tab := range []ado.Tab{ado.TabAssigned, ado.TabCreated, ado.TabReviewRequested} {
			if c := m.loadList(tab); c != nil {
				cmds = append(cmds, c)
			}
		}
		cmds = append(cmds, tick(m.cfg.RefreshInterval.Duration))
		return m, tea.Batch(cmds...)
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
		if !msg.fromCache {
			m = m.markDetailFetchDone()
			if msg.err == nil {
				m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Detail = msg.detail })
				m.maybeDropTerminalCache(msg.detail)
				// Propagate fresh PR state to the list and recents cache so
				// the row the user came from updates without a list refetch.
				if msg.detail != nil {
					m.list = m.list.UpdatePR(msg.detail.PRSummary)
					if m.cache != nil {
						if err := m.cache.PatchRecents(msg.detail.PRSummary); err != nil {
							slog.Warn("cache: patch recents", "pr", msg.detail.ID, "err", err)
						}
					}
				}
			}
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	case filesLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
			if msg.err == nil {
				m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Files = msg.files })
			}
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	case statusesLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
			if msg.err == nil {
				m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Statuses = msg.statuses })
			}
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	case threadsLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
		}
		if msg.err != nil {
			slog.Warn("threads: fetch failed", "err", msg.err)
			return m, nil
		}
		m.threads = msg.threads
		m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
		m = m.refreshPreview()
		if !msg.fromCache {
			m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Threads = msg.threads })
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
			// File-only log: the footer banner is transient so an
			// after-the-fact debug needs the structured record. Prefer
			// the raw err.Error() over friendlyErr — the friendly form
			// strips the ADO-side reason which is exactly what we
			// want when reading the log.
			slog.Error("action failed", "kind", msg.kind, "pr", msg.prID, "err", msg.err.Error())
			m.footerErr = fmt.Sprintf("%s PR #%d: %s", msg.kind, msg.prID, friendlyErr(msg.err))
			return m, nil
		}
		slog.Info("action succeeded", "kind", msg.kind, "pr", msg.prID, "notes", msg.notes)
		m.footerOK = fmt.Sprintf("PR #%d %s", msg.prID, msg.notes)
		// Optimistic local update for votes: ADO sometimes echoes the
		// reviewer back under a different identity descriptor (group vs
		// user) so the GET-side `rv.ID == myID` match misses and the
		// reviewer line still shows "No vote" after refresh. We KNOW the
		// vote we just wrote succeeded, so write it locally too.
		if msg.kind == "vote" && m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
			m.detail = m.detail.SetMyVote(msg.vote, m.myID, m.user)
		}
		// Refresh the detail in place so vote/status update; also bust the
		// list cache for the active tab so the row reflects the change.
		var cmds []tea.Cmd
		if m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
			var dCmd tea.Cmd
			m, dCmd = m.loadDetail(m.detail.Summary())
			cmds = append(cmds, dCmd)
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
		// Any key dismisses a sticky error or success banner. The
		// "press any key to dismiss" cue in the statusline tells the
		// user this is what'll happen for errors. Successes also
		// clear so the banner doesn't follow you across screens.
		// We do NOT swallow the key — the dismiss is a side effect of
		// whatever the user typed next, so nav stays fluid.
		if m.footerErr != "" {
			m.footerErr = ""
		}
		if m.footerOK != "" {
			m.footerOK = ""
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
	return m.loadDetail(s)
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
	// Vote menu is modal: while open, only its own letters and esc are
	// recognized. This lets us reuse single chars like `r` for reject
	// without conflicting with the global Refresh binding.
	if m.voteMenu {
		return m.handleVoteMenuKey(msg)
	}
	// Vim-style gg/G: `g` is a lead-in that arms pendingG, the next
	// keystroke completes (or cancels) the sequence. `G` is a one-shot
	// jump-to-end. Behavior depends on which pane is focused:
	//   - Files focus: jump file cursor to first/last in display order.
	//   - Diff focus:  jump preview viewport to top/bottom.
	if m.pendingG {
		m.pendingG = false
		if keyMatches(msg, m.keys.GotoTop) {
			return m.gotoTop()
		}
		// Any other key cancels and falls through to normal handling.
	}
	if keyMatches(msg, m.keys.GotoTop) {
		// First `g`: arm pending and wait for the second.
		m.pendingG = true
		return m, nil
	}
	if keyMatches(msg, m.keys.GotoEnd) {
		return m.gotoEnd()
	}

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
		return m.loadDetail(m.detail.Summary())
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
	case keyMatches(msg, m.keys.VoteMenu):
		// Open the vote overlay; next key picks the vote (a/s/w/r/c/esc).
		m.voteMenu = true
		return m, nil
	case keyMatches(msg, m.keys.ShowResolved):
		m.showResolved = !m.showResolved
		m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
		m = m.refreshPreview()
		return m, nil
	case keyMatches(msg, m.keys.WrapDiff):
		// Soft-wrap toggle for the diff preview. Off by default so
		// large diffs stay scannable; on for files with long lines.
		// refreshPreview re-feeds the viewport from the cached
		// unwrapped render, applying wrapDiffLines when wrapDiff=on.
		m.wrapDiff = !m.wrapDiff
		// Reset scroll so the user lands at the top of the wrapped
		// view — the visual line numbering changed under them.
		m.preview.vp.GotoTop()
		m = m.refreshPreview()
		return m, nil
	case keyMatches(msg, m.keys.CtxMore), keyMatches(msg, m.keys.CtxLess):
		return m.cycleDiffCtx(keyMatches(msg, m.keys.CtxMore))
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
		// Files focus: enter drills into the diff for the selected file.
		// Mirrors the "enter to open" idiom from the PR list. tab still
		// works as the symmetric focus toggle for users who learned it.
		m.detailFocus = focusDiff
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
		// Walk in display (tree) order so n/N matches j/k. Without
		// neighborFile this would step through API order, which can
		// jump across the file tree in non-obvious directions.
		next := m.detail.neighborFile(+1)
		if next == m.detail.cursor {
			return m, nil
		}
		m.detail.cursor = next
		mm, previewCmd := m.queuePreviewForSelection()
		return mm, previewCmd
	case keyMatches(msg, m.keys.PrevFile):
		prev := m.detail.neighborFile(-1)
		if prev == m.detail.cursor {
			return m, nil
		}
		m.detail.cursor = prev
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

// gotoTop implements the gg sequence: in Files focus, jump cursor to
// the first file in display order and reload its preview; in Diff
// focus, scroll the preview viewport to the top.
func (m Model) gotoTop() (tea.Model, tea.Cmd) {
	if m.detailFocus == focusDiff {
		m.preview.vp.GotoTop()
		return m, nil
	}
	idx := m.detail.FirstDisplayFile()
	if idx < 0 || idx == m.detail.cursor {
		return m, nil
	}
	m.detail.cursor = idx
	mm, cmd := m.queuePreviewForSelection()
	return mm, cmd
}

// gotoEnd implements G: in Files focus, jump cursor to the last file
// in display order; in Diff focus, scroll viewport to the bottom.
func (m Model) gotoEnd() (tea.Model, tea.Cmd) {
	if m.detailFocus == focusDiff {
		m.preview.vp.GotoBottom()
		return m, nil
	}
	idx := m.detail.LastDisplayFile()
	if idx < 0 || idx == m.detail.cursor {
		return m, nil
	}
	m.detail.cursor = idx
	mm, cmd := m.queuePreviewForSelection()
	return mm, cmd
}

// handleVoteMenuKey resolves the open vote overlay. It always closes
// the menu (success or cancel) so the user can never get stuck in it.
// Recognized keys:
//
//	a -> approve (+10)
//	s -> approve with suggestions (+5)
//	w -> wait for author (-5)
//	r -> reject (-10)
//	c -> clear vote (0)
//	esc / any other key -> cancel, no API call
func (m Model) handleVoteMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.voteMenu = false
	if msg.Type == tea.KeyEsc {
		return m, nil
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return m, nil
	}
	switch msg.Runes[0] {
	case 'a':
		return m, m.setVoteCurrent(10, "voted approve")
	case 's':
		return m, m.setVoteCurrent(5, "voted approve w/ suggestions")
	case 'w':
		return m, m.setVoteCurrent(-5, "voted wait for author")
	case 'r':
		return m, m.setVoteCurrent(-10, "voted reject")
	case 'c':
		return m, m.setVoteCurrent(0, "cleared vote")
	}
	return m, nil
}

func (m Model) View() string {
	header := renderTopbar(m)
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
			"  ↑↓ pgup/pgdn  scroll focused pane",
			"  gg / G       jump to first / last (file list or diff, depending on focus)",
			"  a           approve PR (Detail)",
			"  v           open vote menu: a/s/w/r/c (Detail)",
			"  X           abandon PR (Detail, confirms)",
			"  enter       expand comment threads on focused file (Diff focus)",
			"  R           toggle showing resolved comments",
			"  esc         back",
		}, "\n"))
	}
	footer := renderStatusline(m)
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


// cycleDiffCtx advances (or rewinds) the diff context level and reloads
// the preview at the new level. Each level has its own cache slot, so
// flipping back to a previously-visited level is instant.
//
// We clear previewKey so queuePreviewForSelection treats this as a
// selection change — that's the existing path that handles cache lookup
// or fresh fetch + neighbor prefetch.
func (m Model) cycleDiffCtx(forward bool) (Model, tea.Cmd) {
	if m.screen != screenDetail || m.detail.Detail() == nil {
		return m, nil
	}
	if forward {
		m.diffCtx = nextCtx(m.diffCtx)
	} else {
		m.diffCtx = prevCtx(m.diffCtx)
	}
	m.previewKey = ""
	return m.queuePreviewForSelection()
}

func (m Model) queuePreviewForSelection() (Model, tea.Cmd) {
	if m.screen != screenDetail || m.detail.Detail() == nil {
		return m, nil
	}
	f, ok := m.detail.SelectedFile()
	if !ok {
		return m, nil
	}
	key := diffSelectionKey(m.detail.Detail().SourceSha, m.detail.Detail().TargetSha, f.Path, m.diffCtx)
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
		key := diffSelectionKey(d.SourceSha, d.TargetSha, f.Path, m.diffCtx)
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
	ctxN := ctxLines(m.diffCtx)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), useDelta, ctxN)
			return prefetchLoadedMsg{key: key, content: out, err: err}
		}
		if client == nil {
			return prefetchLoadedMsg{key: key, err: nil, content: nil}
		}
		src, tgt, err := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return prefetchLoadedMsg{key: key, err: err}
		}
		return prefetchLoadedMsg{key: key, content: simpleDiff(tgt, src, file.Path, ctxN)}
	}
}

func (m Model) detailPreviewView() string {
	layout := m.detailLayout()
	detail := m.detail.SetPaneSize(layout.leftWidth, layout.bodyHeight)
	left := detail.ViewWithFocus(m.detailFocus == focusFiles)
	if !layout.split {
		// Single-column layout: stack the two panes vertically. The
		// preview still gets the bordered chrome so the user sees it
		// as a discrete unit, just full-width.
		previewBody := m.previewPaneBody()
		previewTitle := m.previewPaneTitle()
		right := borderedPane(previewTitle, previewBody, layout.rightWidth, maxInt(6, layout.bodyHeight/2), m.detailFocus == focusDiff)
		return strings.Join([]string{left, "", right}, "\n")
	}
	leftPane := lipgloss.NewStyle().
		Width(layout.leftWidth).
		MaxWidth(layout.leftWidth).
		Height(layout.bodyHeight).
		Render(left)
	previewBody := m.previewPaneBody()
	previewTitle := m.previewPaneTitle()
	rightPane := borderedPane(previewTitle, previewBody, layout.rightWidth, layout.bodyHeight, m.detailFocus == focusDiff)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

// previewPaneTitle returns the label spliced into the right pane's
// border. The selected file path is what's actually useful — the
// previous "Diff Preview" label was redundant chrome since the user
// already knows they're looking at a diff.
func (m Model) previewPaneTitle() string {
	if m.preview.file != "" {
		return m.preview.file
	}
	if f, ok := m.detail.SelectedFile(); ok {
		return f.Path
	}
	return "Diff"
}

// previewPaneBody is what goes inside the bordered preview pane. No
// title prefix — the border carries that — just the diff viewport, or
// a placeholder when no file is loaded.
func (m Model) previewPaneBody() string {
	if m.preview.file == "" {
		if _, ok := m.detail.SelectedFile(); !ok {
			return Faint.Render("No changed files available.")
		}
		return Faint.Render("Loading selected file diff…")
	}
	return m.preview.View()
}

// previewPaneView is retained as a thin shim for any caller that still
// wants the "title + body" string form (currently only tests).
func (m Model) previewPaneView() string {
	return m.previewPaneTitle() + "\n" + m.previewPaneBody()
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
		// borderedPane wraps the preview in a rounded border + 1col
		// inner padding, eating paneChromeWidth horizontally and
		// paneChromeHeight vertically. The viewport sees the inside.
		if layout.split {
			return maxInt(20, layout.rightWidth-paneChromeWidth), maxInt(3, layout.bodyHeight-paneChromeHeight)
		}
		return maxInt(20, layout.rightWidth-paneChromeWidth), maxInt(6, layout.bodyHeight/2-paneChromeHeight)
	default:
		return maxInt(20, m.width), maxInt(3, m.height-5)
	}
}

func diffSelectionKey(sourceSha, targetSha, path string, ctxLevel int) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00c%d", sourceSha, targetSha, path, ctxLevel)
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
//
// ctxLines controls how many surrounding lines each hunk emits; pass 3
// for the standard view, larger for the user's "expand context" mode.
// Set ADOTOP_DIFF_STRICT=1 to skip normalization.
func simpleDiff(target, source []byte, path string, ctxLines int) []byte {
	t := normalizeForDiff(string(target))
	s := normalizeForDiff(string(source))
	tl := strings.Split(t, "\n")
	sl := strings.Split(s, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "--- a%s\n+++ b%s\n", path, path)
	hunks := lcsUnifiedHunks(tl, sl, ctxLines)
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

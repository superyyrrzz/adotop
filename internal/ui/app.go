// Package ui hosts the Bubble Tea application shell.
package ui

import (
	"context"
	"fmt"
	"log/slog"
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
	screenDiff
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
	diff    DiffModel
	preview DiffModel

	width, height int
	footerErr     string
	showHelp      bool
	useDelta      bool
	previewReqID  int
	diffReqID     int
	previewKey    string
	detailFocus   detailFocus
	scrollMem     map[string]int
	previewBodies map[string][]byte
}

func New(cfg config.Config, client *ado.Client) Model {
	keys := DefaultKeys()
	m := Model{
		cfg:      cfg,
		client:   client,
		git:      gitlocal.New(cfg.RepoRoots),
		keys:     keys,
		list:     NewList(keys),
		detail:   NewDetail(keys),
		diff:     NewDiff(keys),
		preview:  NewDiff(keys),
		user:          "loading…",
		useDelta:      gitlocal.HasDelta(),
		scrollMem:     map[string]int{},
		previewBodies: map[string][]byte{},
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
	}
	if cfg.Org != "" && cfg.Project != "" {
		for _, tab := range []ado.Tab{ado.TabAssigned, ado.TabCreated, ado.TabReviewRequested} {
			if prs, ok := st.LoadList(cfg.Org, cfg.Project, tab); ok {
				m.list, _ = m.list.Update(prsLoadedMsg{tab: tab, prs: prs})
			}
		}
	}
	return m
}

type connDataMsg struct {
	data *ado.ConnectionData
	err  error
}

type tickMsg time.Time

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
	)
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
		m.diff = m.sizeDiffModel(m.diff, diffTargetFull)
		m.preview = m.sizeDiffModel(m.preview, diffTargetPreview)
		return m, tea.Batch(c1, c2)
	case connDataMsg:
		if msg.err != nil {
			m.footerErr = "auth: " + msg.err.Error()
			return m, nil
		}
		m.user = msg.data.DisplayName()
		m.myID = msg.data.AuthenticatedUser.ID
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
		if msg.err == nil && m.cache != nil && m.cfg.Org != "" && m.cfg.Project != "" {
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
	case diffLoadedMsg:
		switch msg.target {
		case diffTargetPreview:
			if msg.requestID != m.previewReqID {
				return m, nil
			}
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		default:
			if msg.requestID != m.diffReqID {
				return m, nil
			}
			var cmd tea.Cmd
			m.diff, cmd = m.diff.Update(msg)
			return m, cmd
		}
	case tea.KeyMsg:
		if keyMatches(msg, m.keys.Quit) {
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
		case screenDiff:
			return m.updateDiffScreen(msg)
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
	case screenDiff:
		var cmd tea.Cmd
		m.diff, cmd = m.diff.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateListScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Open):
		if s, ok := m.list.Selected(); ok {
			m.detail = m.detail.SetSummary(s)
			m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
			m.previewKey = ""
			m.detailFocus = focusFiles
			m.scrollMem = map[string]int{}
			m.previewBodies = map[string][]byte{}
			m.screen = screenDetail
			return m, m.loadDetail(s)
		}
	case keyMatches(msg, m.keys.Refresh):
		return m, m.loadList(m.list.Tab())
	case keyMatches(msg, m.keys.Browser):
		if s, ok := m.list.Selected(); ok {
			OpenInBrowser(s.URL)
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
	case keyMatches(msg, m.keys.Open):
		f, ok := m.detail.SelectedFile()
		if !ok || m.detail.Detail() == nil {
			return m, nil
		}
		key := diffSelectionKey(m.detail.Detail().SourceSha, m.detail.Detail().TargetSha, f.Path)
		if key == m.previewKey && m.preview.loaded {
			m.diff = m.sizeDiffModel(m.preview, diffTargetFull)
			m.diff.vp.GotoTop()
			m.screen = screenDiff
			return m, nil
		}
		m.diffReqID++
		dm, cmd := m.loadDiff(diffTargetFull, m.diff, m.diffReqID, m.detail.Summary(), f, m.detail.Detail().SourceSha, m.detail.Detail().TargetSha)
		m.diff = dm
		m.screen = screenDiff
		return m, cmd
	case keyMatches(msg, m.keys.Refresh):
		m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
		m.previewKey = ""
		m.scrollMem = map[string]int{}
		m.previewBodies = map[string][]byte{}
		return m, m.loadDetail(m.detail.Summary())
	case keyMatches(msg, m.keys.Browser):
		OpenInBrowser(m.detail.Summary().URL)
		return m, nil
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

func (m Model) updateDiffScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Back):
		m.screen = screenDetail
		return m, nil
	case keyMatches(msg, m.keys.Browser):
		OpenInBrowser(m.detail.Summary().URL)
		return m, nil
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(msg)
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
	case screenDiff:
		body = m.diff.View()
	}
	if m.showHelp {
		body = HelpBox.Render(strings.Join([]string{
			"Help",
			"",
			"  ?           toggle this help",
			"  q / ctrl+c  quit",
			"  r           refresh current screen",
			"  o           open in browser",
			"  /           filter (list)",
			"  tab / l     next tab",
			"  shift+tab/h prev tab",
			"  enter       open selected / full diff",
			"  esc         back",
			"  ↑↓          move files + auto-preview",
			"  pgup/pgdn   scroll preview/diff",
			"  g / G       top / bottom (preview/diff)",
		}, "\n"))
	}
	footer := Footer.Render(footerHints(m.screen))
	if m.footerErr != "" {
		footer = ErrLine.Render(m.footerErr)
	}
	return strings.Join([]string{header, "", body, "", footer}, "\n")
}

func footerHints(s screen) string {
	switch s {
	case screenList:
		return "/:filter  enter:open  o:browser  r:refresh  tab:next  ?:help  q:quit"
	case screenDetail:
		return "↑↓:files  pgup/pgdn g/G:preview  enter:full diff  o:browser  esc:back  r:refresh  ?:help  q:quit"
	case screenDiff:
		return "↑↓ pgup/pgdn g/G:scroll  esc:back  o:browser  ?:help  q:quit"
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
	m.previewKey = key
	m.previewReqID++
	pm, cmd := m.loadDiff(diffTargetPreview, m.preview, m.previewReqID, m.detail.Summary(), f, m.detail.Detail().SourceSha, m.detail.Detail().TargetSha)
	m.preview = pm
	return m, cmd
}

func (m Model) detailPreviewView() string {
	layout := m.detailLayout()
	left := m.detail.View()
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
	title := Header.Render("Diff Preview")
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

func simpleDiff(target, source []byte, path string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a%s\n+++ b%s\n", path, path)
	t := strings.Split(string(target), "\n")
	s := strings.Split(string(source), "\n")
	for _, line := range t {
		b.WriteString("- " + line + "\n")
	}
	for _, line := range s {
		b.WriteString("+ " + line + "\n")
	}
	return []byte(b.String())
}

// Run starts the Bubble Tea program with the alt screen.
func Run(cfg config.Config, client *ado.Client) error {
	p := tea.NewProgram(New(cfg, client), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Package ui hosts the Bubble Tea application shell.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/renzeyu/adotop/internal/ado"
	"github.com/renzeyu/adotop/internal/config"
	"github.com/renzeyu/adotop/internal/gitlocal"
)

type screen int

const (
	screenList screen = iota
	screenDetail
	screenDiff
)

type Model struct {
	cfg    config.Config
	client *ado.Client
	git    *gitlocal.Finder
	keys   KeyMap

	user   string
	myID   string
	screen screen

	list   ListModel
	detail DetailModel
	diff   DiffModel

	width, height int
	footerErr     string
	showHelp      bool
	useDelta      bool
}

func New(cfg config.Config, client *ado.Client) Model {
	keys := DefaultKeys()
	return Model{
		cfg:      cfg,
		client:   client,
		git:      gitlocal.New(cfg.RepoRoots),
		keys:     keys,
		list:     NewList(keys),
		detail:   NewDetail(keys),
		diff:     NewDiff(keys),
		user:     "loading…",
		useDelta: gitlocal.HasDelta(),
	}
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

func (m Model) loadDiff(s ado.PRSummary, file ado.FileChange, sourceSha, targetSha string) (DiffModel, tea.Cmd) {
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
	dm := m.diff.SetHeader(file.Path, renderer)
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), m.useDelta)
			return diffLoadedMsg{content: out, err: err}
		}
		src, tgt, err := m.client.GetFileContents(ctx, s.RepoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return diffLoadedMsg{err: err}
		}
		return diffLoadedMsg{content: simpleDiff(tgt, src, file.Path)}
	}
	return dm, cmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var c1, c2, c3 tea.Cmd
		m.list, c1 = m.list.Update(msg)
		m.detail, c2 = m.detail.Update(msg)
		m.diff, c3 = m.diff.Update(msg)
		return m, tea.Batch(c1, c2, c3)
	case connDataMsg:
		if msg.err != nil {
			m.footerErr = "auth: " + msg.err.Error()
			return m, nil
		}
		m.user = msg.data.DisplayName()
		m.myID = msg.data.AuthenticatedUser.ID
		return m, m.loadList(m.list.Tab())
	case tickMsg:
		var cmd tea.Cmd
		if m.screen == screenList {
			cmd = m.loadList(m.list.Tab())
		}
		return m, tea.Batch(cmd, tick(m.cfg.RefreshInterval.Duration))
	case tabSwitchedMsg:
		return m, m.loadList(msg.Tab)
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
	case keyMatches(msg, m.keys.Back):
		m.screen = screenList
		return m, nil
	case keyMatches(msg, m.keys.Open):
		f, ok := m.detail.SelectedFile()
		if !ok || m.detail.Detail() == nil {
			return m, nil
		}
		dm, cmd := m.loadDiff(m.detail.Summary(), f, m.detail.Detail().SourceSha, m.detail.Detail().TargetSha)
		m.diff = dm
		m.screen = screenDiff
		return m, cmd
	case keyMatches(msg, m.keys.Refresh):
		return m, m.loadDetail(m.detail.Summary())
	case keyMatches(msg, m.keys.Browser):
		OpenInBrowser(m.detail.Summary().URL)
		return m, nil
	}
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
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
		body = m.detail.View()
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
			"  enter       open selected",
			"  esc         back",
			"  g / G       top / bottom (diff)",
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
		return "↑↓:files  enter:diff  o:browser  esc:back  r:refresh  ?:help  q:quit"
	case screenDiff:
		return "↑↓ pgup/pgdn g/G:scroll  esc:back  o:browser  ?:help  q:quit"
	}
	return ""
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

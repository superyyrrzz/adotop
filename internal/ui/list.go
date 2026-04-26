package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ado"
)

type prsLoadedMsg struct {
	tab ado.Tab
	prs []ado.PRSummary
	err error
}

type ListModel struct {
	keys      KeyMap
	tab       ado.Tab
	prs       map[ado.Tab][]ado.PRSummary
	cursor    int
	filter    string
	filtering bool
	width     int
	height    int
	loadErr   string
}

func NewList(keys KeyMap) ListModel {
	return ListModel{keys: keys, prs: map[ado.Tab][]ado.PRSummary{}}
}

func (m ListModel) Tab() ado.Tab { return m.tab }

// window returns [start, end) indices of rows that fit in the current view,
// keeping m.cursor visible. Each row uses 2 lines; tabs+blank reserve 2,
// filter/footer reserve 2.
func (m ListModel) window(total int) (int, int) {
	const rowLines = 2
	const chrome = 4
	if m.height <= 0 {
		return 0, total
	}
	cap := (m.height - chrome) / rowLines
	if cap <= 0 {
		cap = 1
	}
	if cap >= total {
		return 0, total
	}
	start := m.cursor - cap/2
	if start < 0 {
		start = 0
	}
	if start+cap > total {
		start = total - cap
	}
	if m.cursor < start {
		start = m.cursor
	}
	if m.cursor >= start+cap {
		start = m.cursor - cap + 1
	}
	return start, start + cap
}

func (m ListModel) Selected() (ado.PRSummary, bool) {
	rows := m.visible()
	if len(rows) == 0 {
		return ado.PRSummary{}, false
	}
	if m.cursor >= len(rows) {
		return rows[len(rows)-1], true
	}
	return rows[m.cursor], true
}

func (m ListModel) visible() []ado.PRSummary {
	all := m.prs[m.tab]
	if m.filter == "" {
		return all
	}
	q := strings.ToLower(m.filter)
	out := make([]ado.PRSummary, 0, len(all))
	for _, p := range all {
		hay := strings.ToLower(p.Title + " " + p.Author + " " + p.SourceBranch + " " + p.TargetBranch)
		if strings.Contains(hay, q) {
			out = append(out, p)
		}
	}
	return out
}

type tabSwitchedMsg struct{ Tab ado.Tab }

func tabSwitchCmd(t ado.Tab) tea.Cmd {
	return func() tea.Msg { return tabSwitchedMsg{Tab: t} }
}

func keyMatches(msg tea.KeyMsg, b interface{ Keys() []string }) bool {
	got := msg.String()
	for _, k := range b.Keys() {
		if k == got {
			return true
		}
	}
	return false
}

func (m ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case prsLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else {
			m.loadErr = ""
			m.prs[msg.tab] = msg.prs
			if msg.tab == m.tab && m.cursor >= len(msg.prs) {
				m.cursor = 0
			}
		}
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		switch {
		case keyMatches(msg, m.keys.NextTab):
			m.tab = (m.tab + 1) % 3
			m.cursor = 0
			return m, tabSwitchCmd(m.tab)
		case keyMatches(msg, m.keys.PrevTab):
			m.tab = (m.tab + 2) % 3
			m.cursor = 0
			return m, tabSwitchCmd(m.tab)
		case keyMatches(msg, m.keys.Down):
			rows := m.visible()
			if m.cursor < len(rows)-1 {
				m.cursor++
			}
		case keyMatches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case keyMatches(msg, m.keys.Filter):
			m.filtering = true
			m.filter = ""
		}
	}
	return m, nil
}

func (m ListModel) updateFiltering(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filter = ""
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
	}
	m.cursor = 0
	return m, nil
}

func (m ListModel) View() string {
	var b strings.Builder
	tabs := []string{ado.TabAssigned.String(), ado.TabCreated.String(), ado.TabReviewRequested.String()}
	for i, name := range tabs {
		count := len(m.prs[ado.Tab(i)])
		label := fmt.Sprintf(" %s (%d) ", name, count)
		if ado.Tab(i) == m.tab {
			b.WriteString(TabOn.Render(label))
		} else {
			b.WriteString(TabOff.Render(label))
		}
	}
	b.WriteString("\n\n")

	rows := m.visible()
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render("error: " + m.loadErr))
		b.WriteString("\n")
	} else if len(rows) == 0 {
		b.WriteString(Faint.Render("No PRs in this tab.\n"))
	} else {
		start, end := m.window(len(rows))
		for i := start; i < end; i++ {
			p := rows[i]
			line := fmt.Sprintf("#%-5d %-40s %-12s %s → %s   %s",
				p.ID, truncate(p.Title, 40), truncate(p.Author, 12),
				p.SourceBranch, p.TargetBranch, age(p.CreatedAt))
			if p.Draft {
				line += "  [DRAFT]"
			}
			if i == m.cursor {
				line = Selected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
			b.WriteString("    ")
			b.WriteString(voteGlyphs(p.Reviewers))
			b.WriteString("\n")
		}
		if start > 0 || end < len(rows) {
			b.WriteString(Faint.Render(fmt.Sprintf("  [%d-%d of %d]\n", start+1, end, len(rows))))
		}
	}

	if m.filtering {
		b.WriteString("\n/" + m.filter + lipgloss.NewStyle().Faint(true).Render("█"))
	}
	return b.String()
}

func voteGlyphs(rs []ado.ReviewerVote) string {
	var b strings.Builder
	for _, r := range rs {
		var g string
		switch {
		case r.Vote >= 10:
			g = Approve.Render("✓")
		case r.Vote >= 5:
			g = Approve.Render("✓~")
		case r.Vote <= -10:
			g = Reject.Render("✗")
		case r.Vote <= -5:
			g = Wait.Render("⏳")
		default:
			g = None.Render("·")
		}
		if r.IsRequired {
			g = lipgloss.NewStyle().Bold(true).Render(g)
		}
		b.WriteString(g)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}

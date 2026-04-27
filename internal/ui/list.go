package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

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
	jumping   bool
	jumpInput string
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
		if m.jumping {
			return m.updateJumping(msg)
		}
		switch {
		case keyMatches(msg, m.keys.NextTab):
			m.tab = (m.tab + 1) % 4
			m.cursor = 0
			return m, tabSwitchCmd(m.tab)
		case keyMatches(msg, m.keys.PrevTab):
			m.tab = (m.tab + 3) % 4
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
		case keyMatches(msg, m.keys.JumpToID):
			m.jumping = true
			m.jumpInput = ""
		}
	}
	return m, nil
}

type jumpRequestedMsg struct{ ID int }

func (m ListModel) updateJumping(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.jumping = false
		m.jumpInput = ""
	case tea.KeyEnter:
		id, _ := strconv.Atoi(m.jumpInput)
		m.jumping = false
		m.jumpInput = ""
		if id <= 0 {
			return m, nil
		}
		return m, func() tea.Msg { return jumpRequestedMsg{ID: id} }
	case tea.KeyBackspace:
		if len(m.jumpInput) > 0 {
			m.jumpInput = m.jumpInput[:len(m.jumpInput)-1]
		}
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if r >= '0' && r <= '9' {
				m.jumpInput += string(r)
			}
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
	tabs := []string{
		ado.TabRecents.String(),
		ado.TabAssigned.String(),
		ado.TabCreated.String(),
		ado.TabReviewRequested.String(),
	}
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
	cols := m.colWidths()
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render("error: " + m.loadErr))
		b.WriteString("\n")
	} else if len(rows) == 0 {
		b.WriteString(Faint.Render("No PRs in this tab.\n"))
	} else {
		header := fmt.Sprintf("%s %s %s %s %s   %s   %s",
			padCols("ID", cols.id),
			padCols("State", 10),
			padCols("Title", cols.title),
			padCols("Author", cols.author),
			padCols("Source", cols.source),
			padCols("Target", cols.target),
			padCols("Age", cols.age))
		b.WriteString(Faint.Render(header))
		b.WriteString("\n")
		start, end := m.window(len(rows))
		for i := start; i < end; i++ {
			p := rows[i]
			stateText, stateStyle := prStateBadgeCompact(p)
			line := fmt.Sprintf("%s %s %s %s %s → %s   %s",
				padCols(fmt.Sprintf("#%d", p.ID), cols.id),
				stateStyle.Render(padCols(stateText, 10)),
				truncCols(p.Title, cols.title),
				truncCols(p.Author, cols.author),
				truncCols(p.SourceBranch, cols.source),
				truncCols(p.TargetBranch, cols.target),
				padCols(age(p.CreatedAt), cols.age))
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
		b.WriteString("\n/" + m.filter + Faint.Render("█"))
	}
	if m.jumping {
		b.WriteString("\n#" + m.jumpInput + Faint.Render("█"))
	}
	return b.String()
}

// listCols holds the rendered widths of each column in the PR list.
// Title is the elastic column — it absorbs slack space when the terminal
// is wider than the minimum layout, and shrinks (down to a floor) when
// the terminal is narrow.
type listCols struct {
	id, title, author, source, target, age int
}

func (m ListModel) colWidths() listCols {
	// Defaults that work in narrow terminals (sum ~= 108, ignoring arrow/separators).
	c := listCols{id: 8, title: 40, author: 14, source: 22, target: 18, age: 6}
	if m.width <= 0 {
		return c
	}
	// Account for the literal separators in the row format string:
	// "%s %s %s %s → %s   %s" -> 1+1+1+3+3 = 9 chars of glue. Use 10 for safety.
	const glue = 10
	const minTitle = 20
	avail := m.width - (c.id + c.author + c.source + c.target + c.age + glue)
	if avail < minTitle {
		// Squeeze in this order: target, source, author. Keep ID/age fixed.
		need := minTitle - avail
		for _, cap := range []struct {
			pField *int
			floor  int
		}{
			{&c.target, 10},
			{&c.source, 12},
			{&c.author, 8},
		} {
			if need <= 0 {
				break
			}
			room := *cap.pField - cap.floor
			if room <= 0 {
				continue
			}
			cut := room
			if cut > need {
				cut = need
			}
			*cap.pField -= cut
			need -= cut
		}
		avail = m.width - (c.id + c.author + c.source + c.target + c.age + glue)
		if avail < 8 {
			avail = 8
		}
		c.title = avail
		return c
	}
	// Wide terminal: give title the slack, but also let source/target grow
	// modestly so long branch names aren't always truncated.
	c.title = avail
	if c.title > 60 && c.source < 32 {
		extra := (c.title - 60) / 4
		if extra > 10 {
			extra = 10
		}
		c.source += extra
		c.target += extra
		c.title -= 2 * extra
	}
	return c
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
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// truncCols truncates s so its display width fits in cols (appending "…"
// when shortened) and right-pads with spaces to exactly cols columns.
func truncCols(s string, cols int) string {
	if cols <= 0 {
		return ""
	}
	w := runewidth.StringWidth(s)
	if w <= cols {
		return s + strings.Repeat(" ", cols-w)
	}
	out := runewidth.Truncate(s, cols-1, "") + "…"
	pad := cols - runewidth.StringWidth(out)
	if pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

// padCols right-pads s with spaces to exactly cols display columns,
// truncating with "…" if it overflows. Use for fields where overflow
// (long PR IDs, etc.) must not push downstream columns right.
func padCols(s string, cols int) string {
	return truncCols(s, cols)
}

// padRunes is kept for callers that want simple rune-count padding.
func padRunes(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
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

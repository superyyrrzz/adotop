package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/superyyrrzz/adotop/internal/ado"
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
	// refreshingPRID tracks which PR (if any) the background recents
	// sweep is currently re-fetching. Kept on the model so handlers
	// can ask "is a sweep active?" without consulting the parent.
	// Not surfaced visually — refreshes are silent because each fetch
	// is fast enough that any indicator just reads as a flash.
	refreshingPRID int
}

func NewList(keys KeyMap) ListModel {
	return ListModel{keys: keys, prs: map[ado.Tab][]ado.PRSummary{}}
}

func (m ListModel) Tab() ado.Tab { return m.tab }

// window returns [start, end) indices of rows that fit in the current view,
// keeping m.cursor visible.
//
// Per-row vertical budget:
//   - non-compact (two-line) row: 1 data line + 1 meta line + 1 separator = 3
//   - compact (one-line) row:     1 data line + 1 separator           = 2
//
// The selected row uses a 1-col left-edge accent stripe instead of a
// box border, so it occupies the same height as any other row — no
// extra-line bookkeeping needed.
//
// Chrome above/below the list rows we have to leave room for:
//   - topbar bar + rule                  = 2
//   - blank between header and body      = 1
//   - tab strip + blank-blank gap        = 2 (renderTabs writes "\n\n")
//   - column header                      = 1
//   - blank between body and footer      = 1
//   - statusline                         = 1
//   - "[start-end of total]" pager line  = 1 (only when scrolling, but
//                                            we always reserve it so the
//                                            pager doesn't shove rows
//                                            out of view at the moment
//                                            it appears)
//
// Total chrome = 9. Underestimating chrome scrolls the topbar and
// tab strip off the top of the alt-screen, which is how the "topbar
// disappears" bug manifests in tall lists.
func (m ListModel) window(total int) (int, int) {
	rowLines := 3
	if m.width > 0 && m.width < 90 {
		rowLines = 2
	}
	const chrome = 9
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

// Rows returns the count of currently-visible rows on the active tab,
// after the filter (if any) is applied. Used by the statusline to
// gate hints — there's no point advertising `/:filter` on an empty
// tab where it has nothing to filter.
func (m ListModel) Rows() int { return len(m.visible()) }

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

// SetRefreshing records (or clears) the PR ID currently being re-
// fetched by the background sweep. Kept as state so IsRefreshing can
// gate parent-side decisions; not used for any visual indicator.
func (m ListModel) SetRefreshing(prID int) ListModel {
	m.refreshingPRID = prID
	return m
}

// IsRefreshing reports whether a sweep is in flight, so the parent can
// decide whether further work is needed.
func (m ListModel) IsRefreshing() bool { return m.refreshingPRID != 0 }

// UpdatePR finds rows matching s.ID across every tab and replaces them
// with s. Used when fresh data arrives via the detail view (votes,
// status, etc.) — without this the list keeps showing the row as it
// looked the moment the user opened the PR.
//
// No-op when s.ID is 0 (defensive) or no row matches in any tab.
func (m ListModel) UpdatePR(s ado.PRSummary) ListModel {
	if s.ID == 0 {
		return m
	}
	for tab, rows := range m.prs {
		for i := range rows {
			if rows[i].ID == s.ID {
				rows[i] = s
			}
		}
		m.prs[tab] = rows
	}
	return m
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
			if len(rows) > 0 {
				m.cursor = (m.cursor + 1) % len(rows)
			}
		case keyMatches(msg, m.keys.Up):
			rows := m.visible()
			if len(rows) > 0 {
				m.cursor = (m.cursor - 1 + len(rows)) % len(rows)
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

// renderTabs draws the four-tab top bar with a width-aware fallback
// chain so the strip never wraps and pushes the table off-screen. The
// invariant: lipgloss.Width(strip) <= m.width.
//
// Tiers, widest to narrowest:
//
//	A. " Recents (12) "   (full labels + counts)
//	B. " Recents (12) "   (short labels + counts — "Assigned to me" → "Assigned")
//	C. " Recents "        (short labels, no count)
//	D. " R  A  C  R "     (single-letter initials; selection still styled)
//	E. " Recents "        (active tab only — narrowest possible signal)
//
// The active tab uses the mauve pill style so it reads as a chip on the
// strip. Inactive tabs are plain faint text with a single-space gutter
// between them — no leading marker, since horizontal selection is
// signalled by the chip itself.
func (m ListModel) renderTabs() string {
	full := []string{"Recents", "Assigned to me", "Created by me", "All reviewing"}
	short := []string{"Recents", "Assigned", "Created", "Reviewing"}
	initials := []string{"R", "A", "C", "V"} // Recents/Assigned/Created/reViewing
	w := m.width

	build := func(labels []string, withCount bool) string {
		var b strings.Builder
		// Tier D (initials) brackets the active label so selection
		// survives terminals (and tests) that strip ANSI styling. The
		// other tiers carry it via the pill background, which is enough
		// at sane widths but disappears in the bracketless one-letter
		// form.
		isInitials := len(labels) > 0 && len(labels[0]) == 1
		for i, name := range labels {
			label := " " + name
			if withCount {
				label += fmt.Sprintf(" (%d)", len(m.prs[ado.Tab(i)]))
			}
			label += " "
			if ado.Tab(i) == m.tab {
				if isInitials {
					b.WriteString(TabOn.Render("[" + name + "]"))
					b.WriteString(" ")
				} else {
					b.WriteString(TabOn.Render(label))
				}
			} else {
				if isInitials {
					b.WriteString(TabOff.Render(" " + name + " "))
					b.WriteString(" ")
				} else {
					b.WriteString(TabOff.Render(label))
				}
			}
		}
		return b.String()
	}

	candidates := []struct {
		labels    []string
		withCount bool
	}{
		{full, true},
		{short, true},
		{short, false},
		{initials, false},
	}
	if w <= 0 {
		return build(full, true)
	}
	for _, c := range candidates {
		out := build(c.labels, c.withCount)
		if lipgloss.Width(out) <= w {
			return out
		}
	}
	// Tier E: even initials don't fit. Render only the active tab so
	// the user keeps the "where am I" signal even on absurdly narrow
	// terminals. Brackets carry the selection signal in case ANSI is
	// stripped (tests, dumb terminals).
	active := short[int(m.tab)]
	maxInner := w - 2 // [ + ]
	if maxInner < 1 {
		maxInner = 1
	}
	if len(active) > maxInner {
		active = active[:maxInner]
	}
	return TabOn.Render("[" + active + "]")
}

func (m ListModel) View() string {
	var b strings.Builder
	b.WriteString(m.renderTabs())
	b.WriteString("\n\n")

	rows := m.visible()
	cols := m.colWidths()
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render("error: " + m.loadErr))
		b.WriteString("\n")
	} else if len(rows) == 0 {
		b.WriteString(Faint.Render("No PRs in this tab.\n"))
	} else {
		header := fmt.Sprintf("%s%s %s %s %s %s   %s   %s",
			rowIndent,
			padCols("ID", cols.id),
			padCols("State", stateColWidth),
			padCols("Title", cols.title),
			padCols("Author", cols.author),
			padCols("Source", cols.source),
			padCols("Target", cols.target),
			padCols("Age", cols.age))
		b.WriteString(Faint.Render(header))
		b.WriteString("\n")
		start, end := m.window(len(rows))
		// compactRows is true on narrow terminals where the second-line
		// metadata band would wrap. We fall back to the original glyph
		// strip in that case so users on small windows aren't punished.
		compactRows := m.width > 0 && m.width < 90
		// Bracket the selected row in a mauve rounded frame. To keep
		// columns aligned across selected/unselected rows, every row
		// gets the same 1-char left+right inset; the bracket border
		// occupies that inset on the selected row, plain spaces on the
		// others. The frame adds 2 lines (top/bottom border) to the
		// selected row — window() compensates so the visible window
		// still fits.
		for i := start; i < end; i++ {
			p := rows[i]
			stateText, stateStyle := prStateBadgeCompact(p)
			pill := stateStyle.Render(stateText)
			pad := stateColWidth - lipgloss.Width(pill)
			if pad > 0 {
				pill += strings.Repeat(" ", pad)
			}
			line := fmt.Sprintf("%s %s %s %s %s → %s   %s",
				padCols(fmt.Sprintf("#%d", p.ID), cols.id),
				pill,
				truncCols(p.Title, cols.title),
				truncCols(p.Author, cols.author),
				truncCols(p.SourceBranch, cols.source),
				truncCols(p.TargetBranch, cols.target),
				padCols(age(p.CreatedAt), cols.age))
			var meta string
			if compactRows {
				meta = "    " + voteGlyphs(p.Reviewers)
			} else {
				meta = "    " + voteChips(p.Reviewers)
			}
			block := line + "\n" + meta
			if i == m.cursor {
				b.WriteString(renderSelectedRow(block, m.rowSeparatorWidth()))
			} else {
				// Indent unselected rows by one column on each side so
				// they sit under the bracket's interior.
				b.WriteString(indentRowBlock(block))
			}
			b.WriteString("\n")
			// Inter-row separator after every row except the last visible
			// one. The selected row no longer adds a border, so it gets a
			// separator like everything else.
			if i < end-1 {
				b.WriteString(Faint.Render(strings.Repeat("─", m.rowSeparatorWidth())))
				b.WriteString("\n")
			}
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

// stateColWidth is the visible-width budget for the State column. It
// must accommodate the widest pill label plus its 1-char horizontal
// padding on each side. Current widest: " CHECKING " = 10. We give a
// 2-char cushion so neighbouring columns don't kiss the badge.
const stateColWidth = 12

// rowIndent is the leftmost two columns reserved as a uniform inset
// for every PR row. On the selected row those two columns are taken
// up by the mauve bracket frame ("│ "); on every other row they're
// plain spaces. Defined once so the column header alignment lines up
// with the data rows under either path.
const rowIndent = "  "

// renderSelectedRow paints a 1-col mauve accent stripe down the left
// edge of the row's two-line block — a glow-style "you are here" cue
// that doesn't enclose the row in a box. The accent occupies the same
// 2-col rowIndent as unselected rows ("▌ "), so columns line up
// across the whole list. No top/bottom border means the row's height
// stays equal to non-selected rows, simplifying the window math.
func renderSelectedRow(block string, sepWidth int) string {
	stripe := Cursor.Render("▌") + " "
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		lines[i] = stripe + ln
	}
	return strings.Join(lines, "\n")
}

// indentRowBlock prefixes both lines of a non-selected row with the
// uniform rowIndent so its columns line up under the bracketed row's
// interior.
func indentRowBlock(block string) string {
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		lines[i] = rowIndent + ln
	}
	return strings.Join(lines, "\n")
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

// voteChips renders one chip per reviewer in the form "[ <glyph> <name> ]"
// using the same per-vote color scheme as voteGlyphs but with the
// reviewer's display name attached so the user knows *who* voted what.
// Required reviewers get a trailing "*" inside the chip.
//
// Names are truncated to 12 chars to keep the second line bounded; the
// caller is expected to fall back to voteGlyphs on narrow terminals.
func voteChips(rs []ado.ReviewerVote) string {
	if len(rs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range rs {
		var glyph string
		var style lipgloss.Style
		switch {
		case r.Vote >= 10:
			glyph, style = "✓", Approve
		case r.Vote >= 5:
			glyph, style = "✓~", Approve
		case r.Vote <= -10:
			glyph, style = "✗", Reject
		case r.Vote <= -5:
			glyph, style = "⏳", Wait
		default:
			glyph, style = "·", None
		}
		name := truncate(firstName(r.DisplayName), 12)
		if r.IsRequired {
			name += "*"
		}
		chip := style.Render(fmt.Sprintf("[ %s %s ]", glyph, name))
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(chip)
	}
	return b.String()
}

// firstName trims a "Last, First" or "First Last" display name down to
// the first token so chips stay readable on narrow rows.
func firstName(displayName string) string {
	if displayName == "" {
		return ""
	}
	// "Last, First" form: take the part after the comma.
	if i := strings.Index(displayName, ","); i >= 0 {
		rest := strings.TrimSpace(displayName[i+1:])
		if rest != "" {
			displayName = rest
		}
	}
	if i := strings.Index(displayName, " "); i >= 0 {
		return displayName[:i]
	}
	return displayName
}

// rowSeparatorWidth returns how wide the inter-row rule should be. Uses
// the available terminal width minus a small inset so it visually
// belongs to the table rather than the screen edge.
func (m ListModel) rowSeparatorWidth() int {
	if m.width <= 0 {
		return 60
	}
	w := m.width - 4
	if w < 20 {
		return 20
	}
	return w
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

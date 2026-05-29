package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// paneChromeWidth is the total horizontal cells consumed by a
// borderedPane wrapper: 1 left border + 1 left pad + 1 right pad + 1
// right border. Diff sizing math subtracts this from the parent width
// before handing space to the viewport.
const paneChromeWidth = 4

// paneChromeHeight is the rows consumed by a borderedPane: 1 top edge
// (which carries the title) + 1 bottom edge.
const paneChromeHeight = 2

// borderedPane wraps body in a rounded border and splices title into
// the top edge — lazygit-style "the border IS the chrome."
//
//	╭─ <title> ─────────────────╮
//	│ <body line 1>             │
//	│ <body line 2>             │
//	╰───────────────────────────╯
//
// focused selects the border color: mauve when true, the regular pane
// border grey when false. That single bit is the "you are here" signal
// — no extra dot or label needed.
//
// width and height are the OUTSIDE dimensions of the box (including the
// border). Body is rendered at width-2 cols inside; if the body is too
// short to fill height-2 rows it is padded with blank lines so the
// bottom border lands where the caller expects it.
//
// title is truncated with an ellipsis when it would push the top edge
// past width. An empty title produces a plain top edge with no
// title-marker characters at all (just `╭───────╮`).
func borderedPane(title, body string, width, height int, focused bool) string {
	if width < paneChromeWidth+1 {
		// Box too small to be useful; fall back to a plain string so
		// callers don't have to special-case tiny terminals.
		return body
	}
	color := lipgloss.TerminalColor(PaneBorder)
	if focused {
		color = focusedPaneBorder()
	}

	// Build the top edge: corner + title-marker + corner. We render the
	// title separately and slice it into a row of horizontal-line glyphs
	// so the marker text gets the right styling without the border
	// itself being styled twice.
	top := composePaneTop(title, width, color)

	// Body: budget width-2 inside the border, height-2 inside top/bottom.
	innerW := width - 2
	innerH := height - paneChromeHeight
	bodyBlock := fitPaneBody(body, innerW, innerH)

	border := lipgloss.NormalBorder()
	leftEdge := lipgloss.NewStyle().Foreground(color).Render(border.Left)
	rightEdge := lipgloss.NewStyle().Foreground(color).Render(border.Right)
	bottom := composePaneBottom(width, color)

	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	for _, line := range strings.Split(bodyBlock, "\n") {
		// Pad the body line to exactly innerW cells so the right edge
		// lines up. lipgloss.Width handles ANSI; pad with spaces.
		w := lipgloss.Width(line)
		if w < innerW {
			line += strings.Repeat(" ", innerW-w)
		}
		b.WriteString(leftEdge)
		b.WriteString(line)
		b.WriteString(rightEdge)
		b.WriteString("\n")
	}
	b.WriteString(bottom)
	return b.String()
}

// composePaneTop renders `╭─ title ──...──╮` at exactly width cells.
// title is rendered in the body Header style so it reads as a label,
// not as part of the border.
func composePaneTop(title string, width int, color lipgloss.TerminalColor) string {
	border := lipgloss.RoundedBorder()
	tl := lipgloss.NewStyle().Foreground(color).Render(border.TopLeft)
	tr := lipgloss.NewStyle().Foreground(color).Render(border.TopRight)
	bar := lipgloss.NewStyle().Foreground(color).Render(border.Top)

	// inner = width - 2 corners
	inner := width - 2
	if inner < 1 {
		inner = 1
	}

	if title == "" {
		return tl + strings.Repeat(bar, inner) + tr
	}

	// Layout: "─ <title> " + remaining bars. Reserve cells for the
	// space and the leading dash explicitly so a 1-cell title still has
	// breathing room.
	const lead = "─ "
	const trail = " "
	overhead := lipgloss.Width(lead) + lipgloss.Width(trail)
	titleBudget := inner - overhead
	if titleBudget < 1 {
		// Not enough room for any titled top — fall back to plain.
		return tl + strings.Repeat(bar, inner) + tr
	}
	t := title
	if lipgloss.Width(t) > titleBudget {
		// Truncate with ellipsis. lipgloss.Width approximates rune
		// width; we slice by bytes which is good enough for ASCII
		// paths. Non-ASCII titles get a slightly tighter cut.
		if titleBudget < 1 {
			t = "…"
		} else {
			t = t[:max(1, titleBudget-1)] + "…"
		}
	}
	titled := lipgloss.NewStyle().Foreground(color).Render(lead) +
		Header.Render(t) +
		lipgloss.NewStyle().Foreground(color).Render(trail)

	used := lipgloss.Width(titled)
	if used > inner {
		// Defensive: refit. Should not happen because we budgeted above.
		return tl + strings.Repeat(bar, inner) + tr
	}
	return tl + titled + strings.Repeat(bar, inner-used) + tr
}

// composePaneBottom renders the closing edge. No title here so it's
// just `╰───╯` at exactly width cells.
func composePaneBottom(width int, color lipgloss.TerminalColor) string {
	border := lipgloss.RoundedBorder()
	bl := lipgloss.NewStyle().Foreground(color).Render(border.BottomLeft)
	br := lipgloss.NewStyle().Foreground(color).Render(border.BottomRight)
	bar := lipgloss.NewStyle().Foreground(color).Render(border.Bottom)
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	return bl + strings.Repeat(bar, inner) + br
}

// fitPaneBody trims/pads the body string to exactly innerH rows. Long
// lines are not wrapped here — the caller (the diff viewport) already
// sized its content to innerW. Truncating tall content to fit is
// preferred over scrolling because the viewport itself is the
// scrolling primitive.
func fitPaneBody(body string, innerW, innerH int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// focusedPaneBorder is the active-pane border tint — mauve so it
// matches the rest of the "you are here" vocabulary (cursor, active
// tab, active breadcrumb crumb).
//
// Pulled from Cursor's foreground, which is the canonical mauve
// already set per theme by NewStyles. Recomputed every read so theme
// switches at runtime are picked up.
func focusedPaneBorder() lipgloss.TerminalColor {
	if c, ok := Cursor.GetForeground().(lipgloss.Color); ok {
		return c
	}
	return PaneBorder
}

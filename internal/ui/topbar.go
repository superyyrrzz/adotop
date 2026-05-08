package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderTopbar produces the application's persistent top chrome:
//
//	  myorg  ›  myproject  ›  Reviewing                       alice  14:32
//	  ────────────────────────────────────────────────────────────────────────
//
// Layout zones:
//
//	left:  breadcrumb crumbs (org → project → view), current crumb in
//	       mauve, earlier crumbs faint, chevrons even fainter.
//	right: identity + clock, both faint.
//	rule:  a faint horizontal line spanning the full terminal width,
//	       grounding the bar without a heavy box.
//
// The bar is one line of crumbs/identity plus one line of rule — two
// lines total. It replaces the previous flat header so the user always
// sees "where am I" at a glance.
//
// Width handling: the right zone is rendered first; the left zone
// truncates from the deepest-but-one crumb (org/project) inward when
// space is tight. The deepest crumb is always preserved because that's
// the dynamic "you are here" signal — losing it would defeat the bar.
func renderTopbar(m Model) string {
	crumbs := topbarCrumbs(m)
	right := topbarRightZone(m)

	w := m.width
	if w <= 0 {
		w = 80
	}

	left := composeCrumbs(crumbs, w-lipgloss.Width(right)-2)
	bar := joinTopbar(left, right, w)
	rule := topbarRule(w)
	return bar + "\n" + rule
}

// topbarCrumbs assembles the breadcrumb segments from the current
// model state. The last segment is always the "active view"; earlier
// segments are static context (org, project).
//
// Empty org/project are surfaced as placeholders so the user can see
// what's missing at a glance instead of a phantom-empty crumb.
func topbarCrumbs(m Model) []string {
	out := []string{
		orPlaceholder(m.cfg.Org, "(no org)"),
		orPlaceholder(m.cfg.Project, "(no project)"),
	}
	switch m.screen {
	case screenList:
		// Use the short tab label so the deepest crumb stays compact —
		// "All reviewing" was bumping into the right-zone identity at
		// every realistic terminal width.
		out = append(out, m.list.Tab().Short())
	case screenDetail:
		s := m.detail.Summary()
		if s.ID > 0 {
			out = append(out, fmt.Sprintf("#%d", s.ID))
		} else {
			out = append(out, "PR")
		}
	}
	return out
}

// composeCrumbs renders the breadcrumb string under a width budget.
// The deepest crumb (active view) is always rendered in mauve; earlier
// crumbs are faint. When the budget is too tight, leading crumbs are
// dropped one at a time (org first, then project) until it fits.
func composeCrumbs(crumbs []string, maxWidth int) string {
	if len(crumbs) == 0 {
		return ""
	}
	if maxWidth < 1 {
		maxWidth = 1
	}
	const chevron = " › "
	for start := 0; start < len(crumbs); start++ {
		visible := crumbs[start:]
		var b strings.Builder
		for i, c := range visible {
			if i > 0 {
				b.WriteString(Faint.Render(chevron))
			}
			if i == len(visible)-1 {
				b.WriteString(Cursor.Render(c))
			} else {
				b.WriteString(Faint.Render(c))
			}
		}
		out := b.String()
		if lipgloss.Width(out) <= maxWidth {
			return out
		}
	}
	// Even the active crumb alone is too wide. Truncate it.
	last := crumbs[len(crumbs)-1]
	if len(last) > maxWidth {
		last = last[:maxInt(1, maxWidth-1)] + "…"
	}
	return Cursor.Render(last)
}

// topbarRightZone renders identity + clock. Empty when neither is
// available so the bar collapses gracefully.
func topbarRightZone(m Model) string {
	parts := []string{}
	if m.user != "" {
		parts = append(parts, m.user)
	}
	if !isClockHidden() {
		parts = append(parts, time.Now().Format("15:04"))
	}
	if len(parts) == 0 {
		return ""
	}
	return Faint.Render(strings.Join(parts, "  "))
}

// joinTopbar pads between the left and right zones so right is
// flush against the terminal edge.
func joinTopbar(left, right string, width int) string {
	if width <= 0 {
		if right != "" {
			return left + "  " + right
		}
		return left
	}
	used := lipgloss.Width(left) + lipgloss.Width(right)
	pad := width - used
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// topbarRule draws the faint underline below the bar that gives the
// header weight without a box. Uses the box-drawing horizontal so it
// looks like a separator, not text.
func topbarRule(width int) string {
	if width <= 0 {
		width = 80
	}
	return Faint.Render(strings.Repeat("─", width))
}

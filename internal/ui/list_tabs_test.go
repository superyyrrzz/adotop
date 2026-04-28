package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestTabsFitWithinWidth ensures the four-tab top bar never wraps,
// regardless of terminal width. When the full labels won't fit we
// fall back to short labels, then drop the per-tab counts. The
// invariant: lipgloss.Width(tabStrip) <= m.width.
//
// This is the regression guard for the bug where "All reviewing"
// pushed the tab strip past narrow terminals and ate the top bar.
func TestTabsFitWithinWidth(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: samplePRs()})

	for _, w := range []int{15, 20, 25, 30, 40, 60, 80, 100, 120} {
		m2, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		strip := m2.renderTabs()
		if got := lipgloss.Width(strip); got > w {
			t.Fatalf("width=%d: tab strip width %d exceeds terminal width\nstrip=%q", w, got, strip)
		}
	}
}

// TestTabsActiveStyleVisible asserts the active tab is visually
// distinguishable from the inactive ones at every width tier. Without
// this guard a future "make it fit" tweak could collapse the strip
// to plain text, leaving the user with no idea which tab is selected.
//
// Robust check: render the same list state twice with different
// active tabs and require the output to differ. Comparing for ANSI
// bytes is unreliable because lipgloss strips styling in non-TTY
// test environments — bracketing/text differences carry the signal.
func TestTabsActiveStyleVisible(t *testing.T) {
	for _, w := range []int{15, 20, 25, 30, 40, 80, 120} {
		a := NewList(DefaultKeys())
		a, _ = a.Update(prsLoadedMsg{tab: ado.TabRecents, prs: samplePRs()})
		a, _ = a.Update(tea.WindowSizeMsg{Width: w, Height: 30})

		b := NewList(DefaultKeys())
		b, _ = b.Update(prsLoadedMsg{tab: ado.TabReviewRequested, prs: samplePRs()})
		b, _ = b.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		// Advance b's active tab three times so it lands on TabReviewRequested.
		for i := 0; i < 3; i++ {
			b, _ = b.Update(tea.KeyMsg{Type: tea.KeyTab})
		}

		if a.renderTabs() == b.renderTabs() {
			t.Fatalf("width=%d: same strip for two different active tabs — selection invisible\n%q",
				w, a.renderTabs())
		}
	}
}

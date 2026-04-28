package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ado"
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

	for _, w := range []int{40, 60, 80, 100, 120} {
		m2, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		strip := m2.renderTabs()
		if got := lipgloss.Width(strip); got > w {
			t.Fatalf("width=%d: tab strip width %d exceeds terminal width\nstrip=%q", w, got, strip)
		}
	}
}

// TestTabsAlwaysShowSelected verifies that no matter how narrow the
// fallback gets, the active tab is still rendered (so the user can
// tell where they are).
func TestTabsAlwaysShowSelected(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabReviewRequested, prs: samplePRs()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	strip := m.renderTabs()
	// "Reviewing" is the short label for TabReviewRequested. At width=30
	// the count tier should be dropped so the active short label still
	// appears in the strip.
	if !strings.Contains(strip, "Reviewing") {
		t.Fatalf("narrow strip should still contain active 'Reviewing' label, got %q", strip)
	}
}

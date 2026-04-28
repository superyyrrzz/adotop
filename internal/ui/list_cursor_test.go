package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/renzeyu/adotop/internal/ado"
)

// TestCursorRowHasBarMarker is the regression guard for the cursor
// styling refresh: the highlighted row must carry a "▌" gutter and the
// non-highlighted rows must not. Before this change the cursor row was
// rendered with Selected.Reverse(true), inverting the entire two-line
// block and producing a flat sea of grey that fought with the vote
// chips and pill backgrounds.
func TestCursorRowHasBarMarker(t *testing.T) {
	now := time.Now()
	prs := []ado.PRSummary{
		{ID: 1, Title: "first", Author: "a", SourceBranch: "f", TargetBranch: "main", CreatedAt: now},
		{ID: 2, Title: "second", Author: "b", SourceBranch: "g", TargetBranch: "main", CreatedAt: now},
		{ID: 3, Title: "third", Author: "c", SourceBranch: "h", TargetBranch: "main", CreatedAt: now},
	}
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})

	// Move the cursor to row 1 (the middle PR).
	m.cursor = 1

	out := m.View()
	lines := strings.Split(out, "\n")

	// Build a flat list of "PR rows" — both halves of the 2-line block
	// for each PR. We detect them by looking for "#<id>" or by the meta
	// strip immediately after.
	var firstRowIdx, secondRowIdx, thirdRowIdx = -1, -1, -1
	for i, ln := range lines {
		switch {
		case strings.Contains(ln, "#1") && strings.Contains(ln, "first"):
			firstRowIdx = i
		case strings.Contains(ln, "#2") && strings.Contains(ln, "second"):
			secondRowIdx = i
		case strings.Contains(ln, "#3") && strings.Contains(ln, "third"):
			thirdRowIdx = i
		}
	}
	for name, idx := range map[string]int{"first": firstRowIdx, "second": secondRowIdx, "third": thirdRowIdx} {
		if idx < 0 {
			t.Fatalf("could not locate %s row in:\n%s", name, out)
		}
	}

	// Cursor row (second PR) must carry the ▌ marker on both lines.
	if !strings.Contains(lines[secondRowIdx], "▌") {
		t.Fatalf("cursor row first line missing ▌ marker:\n%s", lines[secondRowIdx])
	}
	if secondRowIdx+1 >= len(lines) || !strings.Contains(lines[secondRowIdx+1], "▌") {
		t.Fatalf("cursor row second line missing ▌ marker:\n%s", lines[secondRowIdx+1])
	}

	// Non-cursor rows must NOT carry the marker.
	if strings.Contains(lines[firstRowIdx], "▌") {
		t.Fatalf("non-cursor row #1 should not have ▌ marker:\n%s", lines[firstRowIdx])
	}
	if strings.Contains(lines[thirdRowIdx], "▌") {
		t.Fatalf("non-cursor row #3 should not have ▌ marker:\n%s", lines[thirdRowIdx])
	}
}

// TestCursorRowHasNoReverseStyle ensures the old Selected.Reverse(true)
// code path is gone — the cursor row should NOT carry the SGR 7
// (reverse video) escape, which was the source of the heavy grey block
// look that fought with chip backgrounds.
func TestCursorRowHasNoReverseStyle(t *testing.T) {
	now := time.Now()
	prs := []ado.PRSummary{
		{ID: 1, Title: "x", Author: "a", SourceBranch: "f", TargetBranch: "main", CreatedAt: now},
	}
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})

	out := m.View()
	// SGR reverse is "\x1b[7m" (sometimes joined with other params, e.g.
	// "\x1b[1;7m"). Either form would have appeared from the old code.
	if strings.Contains(out, "\x1b[7m") || strings.Contains(out, ";7m") {
		t.Fatalf("rendered list still contains SGR reverse — Selected.Reverse leak:\n%q", out)
	}
}

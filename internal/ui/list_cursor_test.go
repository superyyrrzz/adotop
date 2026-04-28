package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/renzeyu/adotop/internal/ado"
)

// TestCursorRowIsBracketed is the regression guard for the cursor
// styling: the highlighted row must be wrapped in a mauve rounded
// frame (top corner ╭, side rails │, bottom corner ╰) and the
// non-highlighted rows must not carry frame characters.
//
// History: this replaced an earlier "▌" gutter marker, which itself
// replaced a Selected.Reverse(true) approach. The bracket is the
// strongest signal of the three because it visually contains the row
// instead of just decorating its left edge — important on wide
// terminals where the leftmost column is far from where the user's
// eyes track.
func TestCursorRowIsBracketed(t *testing.T) {
	now := time.Now()
	prs := []ado.PRSummary{
		{ID: 1, Title: "first", Author: "a", SourceBranch: "f", TargetBranch: "main", CreatedAt: now},
		{ID: 2, Title: "second", Author: "b", SourceBranch: "g", TargetBranch: "main", CreatedAt: now},
		{ID: 3, Title: "third", Author: "c", SourceBranch: "h", TargetBranch: "main", CreatedAt: now},
	}
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})

	m.cursor = 1

	out := m.View()
	lines := strings.Split(out, "\n")

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

	// Cursor row (second PR) must carry the side rail │ on both
	// lines, and the line above must be the top corner ╭, the line
	// below the bottom corner ╰.
	if !strings.Contains(lines[secondRowIdx], "│") {
		t.Fatalf("cursor row first line missing │ side rail:\n%s", lines[secondRowIdx])
	}
	if secondRowIdx+1 >= len(lines) || !strings.Contains(lines[secondRowIdx+1], "│") {
		t.Fatalf("cursor row second line missing │ side rail:\n%s", lines[secondRowIdx+1])
	}
	if secondRowIdx-1 < 0 || !strings.Contains(lines[secondRowIdx-1], "╭") {
		t.Fatalf("cursor row should be preceded by ╭ top border:\n%s", lines[secondRowIdx-1])
	}
	if secondRowIdx+2 >= len(lines) || !strings.Contains(lines[secondRowIdx+2], "╰") {
		t.Fatalf("cursor row should be followed by ╰ bottom border:\n%s", lines[secondRowIdx+2])
	}

	// Non-cursor rows must NOT carry frame side rails.
	if strings.Contains(lines[firstRowIdx], "│") {
		t.Fatalf("non-cursor row #1 should not have │ frame:\n%s", lines[firstRowIdx])
	}
	if strings.Contains(lines[thirdRowIdx], "│") {
		t.Fatalf("non-cursor row #3 should not have │ frame:\n%s", lines[thirdRowIdx])
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
	if strings.Contains(out, "\x1b[7m") || strings.Contains(out, ";7m") {
		t.Fatalf("rendered list still contains SGR reverse — Selected.Reverse leak:\n%q", out)
	}
}

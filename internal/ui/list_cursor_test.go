package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestCursorRowIsBracketed is the regression guard for the cursor
// styling: the highlighted row must carry a left-edge mauve accent
// stripe (▌) on both of its lines, and non-highlighted rows must
// not carry the stripe glyph.
//
// History: this is the third iteration. Started as Selected.Reverse(true)
// (heavy grey block, fought with chip bgs); evolved to a full rounded
// frame ╭──╮│ │╰──╯ (visually contained the row but added 2 lines of
// chrome and a heavy "boxed" look); now a 1-col left stripe — borrowed
// from glow — gives the same "you are here" cue with no extra height
// and a lighter overall feel.
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

	// Cursor row (second PR) must carry the left-stripe ▌ on both its
	// data line and its meta line below.
	if !strings.Contains(lines[secondRowIdx], "▌") {
		t.Fatalf("cursor row data line missing ▌ stripe:\n%s", lines[secondRowIdx])
	}
	if secondRowIdx+1 >= len(lines) || !strings.Contains(lines[secondRowIdx+1], "▌") {
		t.Fatalf("cursor row meta line missing ▌ stripe:\n%s", lines[secondRowIdx+1])
	}

	// Non-cursor rows must NOT carry the stripe glyph anywhere on their
	// rendered lines (data line + meta line each).
	for _, label := range []struct {
		name string
		idx  int
	}{{"#1", firstRowIdx}, {"#3", thirdRowIdx}} {
		if strings.Contains(lines[label.idx], "▌") {
			t.Fatalf("non-cursor row %s should not have ▌ stripe:\n%s", label.name, lines[label.idx])
		}
		if label.idx+1 < len(lines) && strings.Contains(lines[label.idx+1], "▌") {
			t.Fatalf("non-cursor row %s meta line should not have ▌ stripe:\n%s", label.name, lines[label.idx+1])
		}
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

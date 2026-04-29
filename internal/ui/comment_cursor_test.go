package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// The thread cursor is the user's anchor for C (reply) and x
// (resolve/reactivate). Without a visible, addressable cursor those keys
// would silently no-op or guess. Contract: starts unset (currentThreadID
// returns 0); first move lands on index 0; subsequent moves clamp at
// both ends.
func TestThreadCursorClampsAndCycles(t *testing.T) {
	m := newDetailModel(t)
	m.threadCursor = map[string]int{}
	m.threads = []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "first"}}},
		{ID: 2, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "B", Content: "second"}}},
	}
	d, _ := m.detail.Update(filesLoadedMsg{files: []ado.FileChange{{Path: "/a.go", ChangeType: "edit"}}})
	m.detail = d

	if got := m.currentThreadID(); got != 0 {
		t.Fatalf("default cursor should yield no thread (0), got %d", got)
	}

	m = m.moveThreadCursor(+1)
	if got := m.currentThreadID(); got != 1 {
		t.Fatalf("after first advance, expected thread 1, got %d", got)
	}
	m = m.moveThreadCursor(+1)
	if got := m.currentThreadID(); got != 2 {
		t.Fatalf("expected thread 2, got %d", got)
	}
	m = m.moveThreadCursor(+1)
	if got := m.currentThreadID(); got != 2 {
		t.Fatalf("clamp to last expected, got %d", got)
	}
	m = m.moveThreadCursor(-1)
	m = m.moveThreadCursor(-1)
	m = m.moveThreadCursor(-1)
	if got := m.currentThreadID(); got != 1 {
		t.Fatalf("clamp to first expected, got %d", got)
	}
}

// The selected thread must be visually distinguishable from unselected
// threads. Without a gutter mark, [/] navigation is invisible — the
// user can't tell which thread C/x will act on.
func TestRenderCommentsBlockHighlightsSelected(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "first"}}},
		{ID: 2, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "B", Content: "second"}}},
	}
	out := renderCommentsBlockWithCursor(threads, map[int]bool{}, false, threads, "/a.go", 80, 2)
	plain := stripANSI(out)
	lines := strings.Split(plain, "\n")
	var firstLine, secondLine string
	for _, l := range lines {
		if strings.Contains(l, "first") && firstLine == "" {
			firstLine = l
		}
		if strings.Contains(l, "second") && secondLine == "" {
			secondLine = l
		}
	}
	if firstLine == "" || secondLine == "" {
		t.Fatalf("missing thread lines:\n%s", plain)
	}
	// We can't assert on ANSI escapes (notty profile strips them). The
	// observable contract is that the two lines start with different
	// glyph sequences so the cursor is visible.
	if firstLine == secondLine {
		t.Fatalf("selected thread line equals unselected:\nfirst:  %q\nsecond: %q", firstLine, secondLine)
	}
	if !strings.HasPrefix(strings.TrimLeft(secondLine, " "), "▎") {
		// gutter glyph must lead the selected line so the eye finds it
		t.Fatalf("selected thread should start with ▎ gutter:\n%q", secondLine)
	}
}

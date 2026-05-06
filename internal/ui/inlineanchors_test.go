package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestRightLineNumbersTracksHunks: the line-number map must advance the
// counter on context (' ') and added ('+') lines, hold steady on
// deleted ('-') lines, and reset to the hunk's starting right-line on
// each `@@`. File headers and blank trailing lines map to 0 so the
// splice walker never injects there.
func TestRightLineNumbersTracksHunks(t *testing.T) {
	raw := []byte("--- a/foo.go\n+++ b/foo.go\n@@ -10,3 +20,4 @@\n keep1\n-removed\n+added1\n+added2\n")
	got := rightLineNumbers(raw)
	want := []int{
		0, // ---
		0, // +++
		0, // @@
		20, // ' keep1' (right-side starts at 20)
		0,  // '-removed' (no right-side)
		21, // '+added1'
		22, // '+added2'
		0,  // trailing blank from SplitAfter
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestRightLineNumbersHandlesSecondHunk: a second `@@` resets the
// counter; the lockstep walk must not carry the previous hunk's
// counter across the gap.
func TestRightLineNumbersHandlesSecondHunk(t *testing.T) {
	raw := []byte("@@ -1 +5 @@\n+a\n@@ -10 +30 @@\n+b\n")
	got := rightLineNumbers(raw)
	// Indexes: 0=@@ -> 0, 1=+a -> 5, 2=@@ -> 0, 3=+b -> 30, 4=trailing -> 0.
	want := []int{0, 5, 0, 30, 0}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d want %d", i, got[i], want[i])
		}
	}
}

// TestSpliceInlineCommentsAttachesToTargetLine: after Colorize the
// rendered string still has one output line per input line, so the
// splice should inject a comment block immediately after the diff line
// matching its anchor. Lines without anchors pass through unchanged.
func TestSpliceInlineCommentsAttachesToTargetLine(t *testing.T) {
	raw := []byte("@@ -1 +1 @@\n+line1\n+line2\n+line3\n")
	rendered := string(Colorize(raw))
	lineNums := rightLineNumbers(raw)

	threads := []ado.Thread{{
		ID:        42,
		FilePath:  "/foo.go",
		Status:    "active",
		RightLine: 2,
		Comments: []ado.Comment{
			{ID: 1, Author: "alice", Content: "needs nil check"},
		},
	}}
	byLine := inlineThreadsByLine(threads)
	out, lineMap := spliceInlineComments(rendered, lineNums, byLine, map[int]bool{}, 80, 0)

	plain := stripANSI(out)
	if !strings.Contains(plain, "needs nil check") {
		t.Fatalf("inlined comment text missing:\n%s", plain)
	}
	// The comment must appear AFTER line2 but BEFORE line3.
	idxLine2 := strings.Index(plain, "line2")
	idxComment := strings.Index(plain, "needs nil check")
	idxLine3 := strings.Index(plain, "line3")
	if !(idxLine2 < idxComment && idxComment < idxLine3) {
		t.Fatalf("ordering wrong: line2=%d comment=%d line3=%d\n%s",
			idxLine2, idxComment, idxLine3, plain)
	}
	// The map must report the line index where the inline thread was
	// spliced — used by [/]'s scroll-into-view path.
	if _, ok := lineMap[42]; !ok {
		t.Fatalf("expected lineMap[42] to be set, got %v", lineMap)
	}
}

// TestSpliceInlineCommentsSkipsUnanchored: threads without RightLine
// must NOT appear in the spliced output — those are footer-block
// territory.
func TestSpliceInlineCommentsSkipsUnanchored(t *testing.T) {
	raw := []byte("@@ -1 +1 @@\n+only line\n")
	rendered := string(Colorize(raw))
	lineNums := rightLineNumbers(raw)
	threads := []ado.Thread{{
		ID:       7,
		FilePath: "/foo.go",
		Status:   "active",
		// RightLine left at 0 — unanchored.
		Comments: []ado.Comment{{ID: 1, Author: "bob", Content: "file-level note"}},
	}}
	byLine := inlineThreadsByLine(threads)
	out, _ := spliceInlineComments(rendered, lineNums, byLine, map[int]bool{}, 80, 0)
	plain := stripANSI(out)
	if strings.Contains(plain, "file-level note") {
		t.Fatalf("unanchored thread should NOT inline-render:\n%s", plain)
	}
}

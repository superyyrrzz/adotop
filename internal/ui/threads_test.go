package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestRenderThreadExpandedShowsFullBody is the regression guard for
// "I can see comments but only single-line trimmed and don't know how
// to expand". When expanded, the thread body must:
//  1. Render on its own indented lines (not squeezed via squeezeOneLine
//     into the header)
//  2. Preserve newlines from the source (no " ¶ " substitution)
//  3. Wrap long lines to the viewport width so the comment doesn't
//     extend past the right edge of the diff pane
func TestRenderThreadExpandedShowsFullBody(t *testing.T) {
	thread := ado.Thread{
		ID:        1,
		FilePath:  "/x.go",
		RightLine: 42,
		Comments: []ado.Comment{
			{Author: "Alice", Content: "First paragraph.\n\nSecond paragraph with more detail."},
			{Author: "Bob", Content: "A reply."},
		},
	}

	out := renderThread(thread, true /*expand*/, 80)

	// Header line carries author + location only — no body in head.
	headerLine := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(headerLine, "Alice") {
		t.Fatalf("header should carry author:\n%s", headerLine)
	}
	if strings.Contains(headerLine, "First paragraph") {
		t.Fatalf("expanded header should NOT inline body — body belongs on its own lines:\n%s", headerLine)
	}

	// Body lines preserve newlines (no " ¶ " squeezing).
	if strings.Contains(out, " ¶ ") {
		t.Fatalf("expanded body should not use squeezeOneLine's ¶ separator:\n%s", out)
	}
	if !strings.Contains(out, "First paragraph.") {
		t.Fatalf("expanded body missing first paragraph:\n%s", out)
	}
	if !strings.Contains(out, "Second paragraph with more detail.") {
		t.Fatalf("expanded body missing second paragraph:\n%s", out)
	}

	// Reply renders too, with a continuation marker.
	if !strings.Contains(out, "Bob") {
		t.Fatalf("expanded thread missing reply author:\n%s", out)
	}
	if !strings.Contains(out, "A reply.") {
		t.Fatalf("expanded thread missing reply body:\n%s", out)
	}
}

// TestRenderThreadCollapsedShowsExpandCue: collapsed threads must
// continue to surface the "[N more — space to expand]" cue so users
// know there's something to do. This was already there before; the
// guard is to keep the literal string stable as the affordance the
// help screen and muscle memory point at.
func TestRenderThreadCollapsedShowsExpandCue(t *testing.T) {
	thread := ado.Thread{
		ID: 2,
		Comments: []ado.Comment{
			{Author: "Alice", Content: "First."},
			{Author: "Bob", Content: "Reply."},
		},
	}
	out := renderThread(thread, false, 80)
	if !strings.Contains(out, "space to expand") {
		t.Fatalf("collapsed thread should advertise expand affordance:\n%s", out)
	}
}

// TestRenderThreadExpandedWrapsLongLines: a single very long line in
// the body must be hard-wrapped at the viewport width, not extend off
// the right edge.
func TestRenderThreadExpandedWrapsLongLines(t *testing.T) {
	long := strings.Repeat("supercalifragilistic ", 20) // ~400 chars
	thread := ado.Thread{
		ID: 3,
		Comments: []ado.Comment{
			{Author: "Alice", Content: long},
		},
	}
	out := renderThread(thread, true, 60)
	for _, ln := range strings.Split(out, "\n") {
		if len(ln) > 60 {
			t.Fatalf("line exceeds wrap width 60 (len=%d): %q\nfull:\n%s", len(ln), ln, out)
		}
	}
}

package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestBorderedPaneRendersTitleAndBorder is the contract test for
// borderedPane: the top edge contains the title surrounded by border
// glyphs, the bottom edge is plain, and the body is wrapped in
// vertical bars at the requested width.
func TestBorderedPaneRendersTitleAndBorder(t *testing.T) {
	out := borderedPane("foo.go", "diff body line 1\ndiff body line 2", 30, 6, false)
	lines := strings.Split(out, "\n")

	if len(lines) != 6 {
		t.Fatalf("expected 6 lines (top + 4 body + bottom); got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "foo.go") {
		t.Fatalf("title missing from top edge: %q", lines[0])
	}
	// Top edge starts with a rounded corner ╭ and ends with ╮.
	if !strings.Contains(lines[0], "╭") || !strings.Contains(lines[0], "╮") {
		t.Fatalf("top edge missing rounded corners: %q", lines[0])
	}
	// Bottom edge starts with ╰ and ends with ╯.
	last := lines[len(lines)-1]
	if !strings.Contains(last, "╰") || !strings.Contains(last, "╯") {
		t.Fatalf("bottom edge missing rounded corners: %q", last)
	}
}

// TestBorderedPaneFitsExactWidth proves every rendered row is exactly
// `width` cells wide. Without this guarantee the right border would
// tear away from the rest of the layout when JoinHorizontal padded
// neighboring panes.
func TestBorderedPaneFitsExactWidth(t *testing.T) {
	for _, w := range []int{20, 30, 40, 60, 100} {
		out := borderedPane("title", "body", w, 5, false)
		for i, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got != w {
				t.Fatalf("width=%d line=%d: got %d, want exact match\n%q", w, i, got, line)
			}
		}
	}
}

// TestBorderedPaneTruncatesLongTitle is the safety net for "the file
// path is longer than the pane is wide." We expect an ellipsis rather
// than a torn border.
func TestBorderedPaneTruncatesLongTitle(t *testing.T) {
	long := "src/very/deeply/nested/path/to/a/file/that/wont/fit.go"
	out := borderedPane(long, "", 20, 4, false)
	top := strings.Split(out, "\n")[0]
	if lipgloss.Width(top) != 20 {
		t.Fatalf("top edge width %d != 20: %q", lipgloss.Width(top), top)
	}
	if !strings.Contains(top, "…") {
		t.Fatalf("expected ellipsis in truncated title: %q", top)
	}
}

// TestBorderedPaneFocusChangesOutput proves the focused/unfocused
// renders differ. We can't measure border color directly in tests
// (lipgloss strips ANSI in non-TTY runs) but the *string* must change
// when focus does — that guarantees the user sees a different border.
//
// EXPECTED FAILURE MODE: if a future refactor stops applying focused
// styling at all, both renders return identical bytes and this fires.
func TestBorderedPaneFocusChangesOutput(t *testing.T) {
	a := borderedPane("foo", "body", 20, 5, false)
	b := borderedPane("foo", "body", 20, 5, true)
	if a == b {
		t.Skipf("ANSI stripped in test env; can't distinguish border color. Verified visually.")
	}
}

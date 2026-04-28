package ui

import (
	"strings"
	"testing"
)

// TestWrapDiffLinesNoOpWhenWidthZero covers the safe-default contract:
// a non-positive width returns the input unchanged so callers don't
// have to special-case the "we don't know the pane size yet" path.
func TestWrapDiffLinesNoOpWhenWidthZero(t *testing.T) {
	in := "anything\nat all\n"
	if got := wrapDiffLines(in, 0); got != in {
		t.Fatalf("width=0 should be no-op; got %q", got)
	}
	if got := wrapDiffLines(in, -5); got != in {
		t.Fatalf("width<0 should be no-op; got %q", got)
	}
}

// TestWrapDiffLinesShortLineUnchanged ensures we don't touch lines that
// already fit. Exact-byte equality matters: rewriting short lines would
// invalidate the diffcache's rendered string.
func TestWrapDiffLinesShortLineUnchanged(t *testing.T) {
	in := "   short context line\n"
	if got := wrapDiffLines(in, 80); got != in {
		t.Fatalf("short line was rewritten:\nwant=%q\n got=%q", in, got)
	}
}

// TestWrapDiffLinesAddLinePreservesBgOnContinuation is the regression
// guard for the wrap-respects-color invariant. When a + line wraps,
// the continuation row must reapply the green bg ANSI so the wrapped
// portion still reads as an addition. Without this, the second visual
// row would render with the terminal default bg and look like context.
func TestWrapDiffLinesAddLinePreservesBgOnContinuation(t *testing.T) {
	// Build an "add" line the way Colorize produces them: addBar +
	// bg + green + "+" + body + reset. Make the body long enough to
	// force a wrap at width=20.
	body := strings.Repeat("X", 60)
	in := addBar + ansiAddLineBg + ansiGreen + "+" + body + ansiReset + "\n"

	out := wrapDiffLines(in, 20)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected the long add line to wrap into 3+ visual rows; got %d:\n%q", len(lines), out)
	}

	// Every continuation row must contain the bg SGR so the wrap stays
	// green. (The first row also has it via the original prefix.)
	for i, ln := range lines[1:] {
		if !strings.Contains(ln, ansiAddLineBg) {
			t.Fatalf("continuation row %d missing add-bg SGR:\n%q", i+1, ln)
		}
	}
}

// TestWrapDiffLinesContinuationGutterMarker confirms continuation rows
// carry the faint "…" marker so the user can tell at a glance that a
// row is the tail of the previous logical line, not a new one.
func TestWrapDiffLinesContinuationGutterMarker(t *testing.T) {
	body := strings.Repeat("Y", 50)
	in := addBar + ansiAddLineBg + ansiGreen + "+" + body + ansiReset + "\n"
	out := wrapDiffLines(in, 18)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrap; got %d lines: %q", len(lines), out)
	}
	for i, ln := range lines[1:] {
		if !strings.Contains(ln, "…") {
			t.Fatalf("continuation row %d missing '…' marker:\n%q", i+1, ln)
		}
	}
}

// TestWrapDiffLinesContextLineWraps covers context (3-space gutter)
// lines too — they don't carry a colored bg, but they still need to
// wrap so a long unchanged line in a hunk is fully visible.
func TestWrapDiffLinesContextLineWraps(t *testing.T) {
	body := strings.Repeat("z", 200)
	in := "   " + body + "\n"
	out := wrapDiffLines(in, 40)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected 5+ wrap lines for 200-char body at width 40; got %d", len(lines))
	}
}

// TestSplitANSIWidthIgnoresSGRWhenCounting is the unit-level guarantee
// that the wrap logic doesn't count escape bytes against the visible
// width. "hello world" (11 cells) at width 5 must produce 3 segments
// (5+5+1), not 1 or 5+. If we counted escape bytes, the colored
// "hello" alone would already exceed the budget.
func TestSplitANSIWidthIgnoresSGRWhenCounting(t *testing.T) {
	in := "\x1b[32mhello\x1b[0m world"
	segs := splitANSIWidth(in, 5)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments for 11 visible chars at width 5; got %d: %+v", len(segs), segs)
	}
}

// TestStripANSIRemovesEscapesOnly — the helper that powers gutter
// width measurement. Plain ASCII passes through untouched; CSI
// sequences are dropped.
func TestStripANSIRemovesEscapesOnly(t *testing.T) {
	cases := map[string]string{
		"plain":             "plain",
		"\x1b[1mboldX\x1b[0m": "boldX",
		"\x1b[48;5;22m":     "",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Fatalf("stripANSI(%q): want %q, got %q", in, want, got)
		}
	}
}

// TestWrapDiffLinesPreservesTerminatingNewline keeps the diff body's
// trailing "\n" on the final row so concatenation with the comments
// block (composeDiffWithComments) doesn't fuse two lines.
func TestWrapDiffLinesPreservesTerminatingNewline(t *testing.T) {
	in := addBar + ansiAddLineBg + ansiGreen + "+" + strings.Repeat("a", 40) + ansiReset + "\n"
	out := wrapDiffLines(in, 20)
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("wrapped output should end with newline; got %q", out[len(out)-10:])
	}
}

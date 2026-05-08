package ui

import (
	"bytes"
	"strings"
	"testing"
)

// TestSimpleDiffEmitsPlaceholderForBinaryInput: a JPEG (or any byte
// stream containing NULs) must NOT be split on LF and emitted as
// + lines — that's the bug that filled PR 1145102's diff pane with
// terminal garbage. Asserts the unified-diff header is preserved
// (so downstream Colorize/splice still walks it) and the body
// reduces to a single human-readable placeholder.
func TestSimpleDiffEmitsPlaceholderForBinaryInput(t *testing.T) {
	// Synthesize a tiny "JPEG-ish" buffer: a NUL byte in the first
	// few bytes is enough to trigger the binary heuristic, matching
	// what `git diff` checks.
	src := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46}, bytes.Repeat([]byte{0xab}, 200)...)
	tgt := []byte{} // simulate "added" file (no prior version)

	out := string(simpleDiff(tgt, src, "/images/foo.jpg", 3))
	if !strings.Contains(out, "--- a/images/foo.jpg") {
		t.Fatalf("missing unified-diff header in:\n%s", out)
	}
	if !strings.Contains(out, "Binary files differ") {
		t.Fatalf("expected binary placeholder line, got:\n%s", out)
	}
	if strings.Contains(out, "\xab") {
		t.Fatalf("raw binary bytes leaked into diff body:\n%q", out)
	}
}

// TestSimpleDiffStillWorksForTextInput: the binary guard must NOT
// trip on plain UTF-8 text — locks in the regression-bait scenario
// where the heuristic accidentally flags valid source code (e.g.,
// because of a stray non-ASCII byte) as binary and we'd lose the
// real diff.
func TestSimpleDiffStillWorksForTextInput(t *testing.T) {
	src := []byte("line one\nline two with emoji 🔒\nline three\n")
	tgt := []byte("line one\nline two\nline three\n")
	out := string(simpleDiff(tgt, src, "/foo.go", 3))
	if strings.Contains(out, "Binary files differ") {
		t.Fatalf("text input falsely flagged as binary:\n%s", out)
	}
	if !strings.Contains(out, "+line two with emoji") {
		t.Fatalf("expected line-level diff for text input:\n%s", out)
	}
}

// TestIsBinaryDiffInput exercises the heuristic directly: NUL bytes
// in the probe window flag binary; pure text or empty buffers do
// not. Probe is capped at 8 KB so a NUL hidden past that boundary
// is treated as text — matches git's behavior and keeps the check
// O(1) in file size.
func TestIsBinaryDiffInput(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", []byte{}, false},
		{"plain text", []byte("hello\nworld\n"), false},
		{"nul at start", []byte{0xff, 0x00, 0xab}, true},
		{"nul deep in probe", append(bytes.Repeat([]byte{'a'}, 4000), 0x00), true},
		{"utf-8 emoji", []byte("🔒 stays text"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isBinaryDiffInput(c.in); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestFormatByteSize spot-checks the human-readable size formatter
// used by the binary placeholder. Verifies all three branches —
// bytes, KB, MB — so the placeholder line stays readable across
// asset sizes.
func TestFormatByteSize(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0 bytes"},
		{500, "500 bytes"},
		{2048, "2.0 KB"},
		{1500000, "1.4 MB"},
	}
	for _, c := range cases {
		if got := formatByteSize(c.in); got != c.want {
			t.Fatalf("formatByteSize(%d)=%q, want %q", c.in, got, c.want)
		}
	}
}

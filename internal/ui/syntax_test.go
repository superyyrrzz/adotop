package ui

import (
	"strings"
	"testing"
)

func TestHighlightLineAddsAnsiForKnownExtension(t *testing.T) {
	out := HighlightLine("foo.go", "func main() {}")
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escapes for .go input, got %q", out)
	}
	if !strings.Contains(out, "func") {
		t.Fatalf("expected literal text preserved, got %q", out)
	}
}

func TestHighlightLineUnknownExtensionIsPassthrough(t *testing.T) {
	in := "anything goes"
	out := HighlightLine("notes.zzz", in)
	if out != in {
		t.Fatalf("expected unknown extension to pass through, got %q", out)
	}
}

func TestHighlightLineEmptyInputSafe(t *testing.T) {
	if HighlightLine("foo.go", "") != "" {
		t.Fatalf("empty input should round-trip empty")
	}
}

func TestHighlightLineStripsTrailingResetNewline(t *testing.T) {
	// chroma appends a newline by default; we want a single line for inline use.
	out := HighlightLine("foo.go", "x := 1")
	if strings.HasSuffix(out, "\n") {
		t.Fatalf("expected no trailing newline, got %q", out)
	}
}

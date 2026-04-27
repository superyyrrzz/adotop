package ui

import (
	"strings"
	"testing"
)

func TestColorizeDiffMarksAddDelete(t *testing.T) {
	in := []byte("--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,2 @@\n-old line\n+new line\n context\n")
	out := string(Colorize(in))
	// + line should carry green ANSI (32) and a bar glyph; - line should carry red (31).
	plus := indexLine(out, "new")
	minus := indexLine(out, "old")
	hunk := indexLine(out, "@@")
	if plus == "" || minus == "" || hunk == "" {
		t.Fatalf("missing lines:\n%s", out)
	}
	if !strings.Contains(plus, "\x1b[32") {
		t.Fatalf("+ line missing green:\n%q", plus)
	}
	if !strings.Contains(minus, "\x1b[31") {
		t.Fatalf("- line missing red:\n%q", minus)
	}
	if !strings.Contains(plus, "\x1b[42m") || !strings.Contains(minus, "\x1b[41m") {
		t.Fatalf("missing highlighted gutter bar:\n%q\n%q", plus, minus)
	}
	if !strings.Contains(plus, "▌") || !strings.Contains(minus, "▌") {
		t.Fatalf("missing bar glyph:\n%q\n%q", plus, minus)
	}
	if !strings.Contains(hunk, "\x1b[36") {
		t.Fatalf("hunk header missing cyan:\n%q", hunk)
	}
}

func TestColorizeLeavesContextUntouched(t *testing.T) {
	in := []byte(" plain context line\n")
	out := string(Colorize(in))
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("context line should not be colored:\n%q", out)
	}
}

func TestColorizeSkipsIfAlreadyColored(t *testing.T) {
	// delta output already contains ANSI escapes — don't double-color.
	in := []byte("\x1b[32m+already green\x1b[0m\n")
	out := Colorize(in)
	if string(out) != string(in) {
		t.Fatalf("colorizer should pass-through pre-colored input")
	}
}

func TestColorizeFileHeadersAreBold(t *testing.T) {
	in := []byte("--- a/x\n+++ b/x\n")
	out := string(Colorize(in))
	if !strings.Contains(out, "\x1b[1m") {
		t.Fatalf("file headers should be bold:\n%q", out)
	}
}

func TestColorizeHighlightsCodeBodiesByPath(t *testing.T) {
	in := []byte("--- a/foo.go\n+++ b/foo.go\n@@ -1,2 +1,2 @@\n-old := 1\n+new := 2\n")
	out := string(Colorize(in))
	plus := indexLine(out, "new")
	if plus == "" {
		t.Fatalf("missing plus line:\n%s", out)
	}
	// HighlightLine inserts at least one extra ANSI escape *inside* the body
	// (besides the existing red/green wrapping). Look for an SGR foreground
	// color sequence after the `+` marker.
	body := plus[strings.Index(plus, "+"):]
	if !strings.Contains(body, "\x1b[38;5;") {
		t.Fatalf("expected 256-color SGR inside .go body:\n%q", body)
	}
}

func TestColorizeUnknownExtensionStillWorks(t *testing.T) {
	in := []byte("--- a/notes.zzz\n+++ b/notes.zzz\n@@ -1 +1 @@\n+plain text body\n")
	out := string(Colorize(in))
	if !strings.Contains(out, "plain text body") {
		t.Fatalf("body missing for unknown extension:\n%s", out)
	}
	// Should still have the green gutter bar from the + line.
	if !strings.Contains(out, "\x1b[32") {
		t.Fatalf("missing green wrap on + line:\n%s", out)
	}
}

func indexLine(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

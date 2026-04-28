package ui

import (
	"strings"
	"testing"
)

// TestDetectCommentFormat: format sniffer must correctly distinguish
// HTML, markdown, and plain text bodies. Misclassifying HTML as
// markdown would feed raw `<p>` into glamour, defeating the whole
// point of the conversion step.
func TestDetectCommentFormat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want commentFormat
	}{
		{"empty", "", formatPlain},
		{"plain", "Just a short comment.", formatPlain},
		{"html_paragraph", "<p>Some text</p>", formatHTML},
		{"html_link", `Click <a href="https://x.com">here</a>`, formatHTML},
		{"html_list", "<ul><li>one</li><li>two</li></ul>", formatHTML},
		{"md_bold", "This is **bold** text.", formatMarkdown},
		{"md_fence", "before\n```\ncode\n```\nafter", formatMarkdown},
		{"md_list_dash", "- item 1\n- item 2", formatMarkdown},
		{"md_list_num", "1. first\n2. second", formatMarkdown},
		{"md_heading", "# Title\nbody", formatMarkdown},
		{"md_link", "see [docs](https://x.com)", formatMarkdown},
		{"plain_with_lt", "x < y && y < z, all good", formatPlain},
		{"plain_with_emoji_colon", "looks :good: to me", formatPlain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectCommentFormat(c.in); got != c.want {
				t.Fatalf("detectCommentFormat(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestRenderCommentBodyHTML: an HTML body must come out without
// literal angle-bracketed tags. The exact rendered form depends on
// glamour styling — we don't assert on ANSI codes — but the words and
// link target must survive.
func TestRenderCommentBodyHTML(t *testing.T) {
	in := `<p>This change <strong>looks good</strong> but check <a href="https://x.com/docs">the docs</a>.</p>`
	out := renderCommentBody(in, 80, "      ")
	plain := stripANSI(out)
	if strings.Contains(plain, "<p>") || strings.Contains(plain, "</p>") || strings.Contains(plain, "<a ") {
		t.Fatalf("HTML tags leaked into rendered body:\n%s", plain)
	}
	for _, want := range []string{"This change", "looks good", "the docs", "x.com/docs"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, plain)
		}
	}
}

// TestRenderCommentBodyMarkdown: markdown bot output (the GitOps /
// Copilot reviewer / ownership-enforcer case) must NOT show literal
// `**` or backtick-fence syntax in the rendered output. The whole
// reason markdown comments hurt today is users see the raw syntax.
func TestRenderCommentBodyMarkdown(t *testing.T) {
	in := "Found a bug in **server.go**:\n\n- Missing nil check on `req`\n- Race on close\n\n```go\nclose(ch)\n```"
	out := renderCommentBody(in, 80, "      ")
	plain := stripANSI(out)
	if strings.Contains(plain, "**") {
		t.Fatalf("raw markdown bold markers leaked:\n%s", plain)
	}
	if strings.Contains(plain, "```") {
		t.Fatalf("raw fence markers leaked:\n%s", plain)
	}
	for _, want := range []string{"server.go", "Missing nil check", "close(ch)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered markdown body missing %q:\n%s", want, plain)
		}
	}
}

// TestRenderCommentBodyPlain: plain text bypasses glamour entirely so
// we don't pay a parse for short replies like "LGTM". Output should
// equal the indented passthrough.
func TestRenderCommentBodyPlain(t *testing.T) {
	in := "LGTM, thanks!"
	out := renderCommentBody(in, 80, "      ")
	if !strings.Contains(out, "      LGTM, thanks!") {
		t.Fatalf("plain body should round-trip indented; got:\n%s", out)
	}
	// And no ANSI escape codes — plain text doesn't need styling.
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain body should not contain ANSI escapes:\n%q", out)
	}
}

// TestSqueezeCommentOneLineStripsHTML: the collapsed preview row must
// not show literal HTML tags. The user's #1145743 complaint started
// here — long HTML bodies squeezed into "<p>The change…" with the tag
// still visible.
func TestSqueezeCommentOneLineStripsHTML(t *testing.T) {
	in := `<p>This is the <strong>important</strong> bit you should read.</p>`
	out := squeezeCommentOneLine(in, 80)
	if strings.Contains(out, "<p>") || strings.Contains(out, "<strong>") {
		t.Fatalf("HTML tags leaked into one-line preview: %q", out)
	}
	if !strings.Contains(out, "important") {
		t.Fatalf("preview lost the body text: %q", out)
	}
}

// TestSqueezeCommentOneLineStripsMarkdownNoise: bot markdown should
// preview cleanly. `**bold**` should not survive in the squeezed form;
// `- item` should become `• item` for legibility.
func TestSqueezeCommentOneLineStripsMarkdownNoise(t *testing.T) {
	in := "Issues:\n- **Crash** in handler\n- Missing test"
	out := squeezeCommentOneLine(in, 80)
	if strings.Contains(out, "**") {
		t.Fatalf("bold markers leaked into preview: %q", out)
	}
	if !strings.Contains(out, "Crash") {
		t.Fatalf("preview lost body text: %q", out)
	}
}

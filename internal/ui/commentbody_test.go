package ui

import (
	"os"
	"regexp"
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

// TestRenderCommentBodyGitOpsAssistant uses a real GitOps PR
// Assistant comment (testdata/gitops_pr_assistant.html) — the case
// reported on PR #1145743. The comment is HTML wrapping markdown
// inside a <details> block. Two specific things have to render:
//  1. Markdown headings inside the <details> (`#### About`,
//     `#### Skills marketplace`, `##### Example skills:`) must come
//     out as glamour-styled headings, NOT as literal `####` text.
//  2. Markdown link text `[aka.ms/prassistant]` must NOT appear as
//     literal `\[aka.ms/prassistant]` (escaped square bracket) — the
//     html-to-markdown converter escapes `[` defensively, defeating
//     glamour's link rendering.
func TestRenderCommentBodyGitOpsAssistant(t *testing.T) {
	data, err := os.ReadFile("testdata/gitops_pr_assistant.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	out := renderCommentBody(string(data), 100, "      ")
	plain := stripANSI(out)

	// No leftover escape sequences for `\[`. The links should be
	// rendered as link text.
	if strings.Contains(plain, `\[`) {
		t.Fatalf("escaped \\[ leaked into output:\n%s", plain)
	}
	// Headings must be glamour-styled (the dark style keeps `####` as a
	// visual marker but wraps the whole heading line in ANSI bold+color).
	// We assert the styled form by looking for the ANSI-stripped heading
	// text on its own line — glamour separates headings with blank lines,
	// so `About` should be on a line by itself.
	if !regexp.MustCompile(`(?m)^\s*#### About\s*$`).MatchString(plain) {
		t.Fatalf("expected `#### About` on its own line (glamour-rendered heading):\n%s", plain)
	}
	if !regexp.MustCompile(`(?m)^\s*#### Skills marketplace\s*$`).MatchString(plain) {
		t.Fatalf("expected `#### Skills marketplace` on its own line:\n%s", plain)
	}
	// And the heading text must be ANSI-styled in the raw output (not
	// just plain text). Headings now inherit the theme's Identifier
	// color (Catppuccin Blue, #89b4fa) via the custom glamour spec —
	// look for ANY truecolor SGR (`\x1b[38;2;` for RGB foreground)
	// so the assertion survives palette tweaks. We only care that the
	// heading text was rendered through glamour, not the exact hex.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected glamour ANSI styling in output (heading should be colorized)")
	}
	// Sanity: headings' text should still be present.
	for _, want := range []string{"About", "Skills marketplace", "Example skills"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered output missing %q:\n%s", want, plain)
		}
	}
}

// TestSanitizeCommentStripsGitOpsRateThis: GitOps PR Assistant comments
// trail off with a "Rate this:" feedback table. The block is most of
// the rendered height for zero signal, so for GitOps authors we cut
// from the marker (and any wrapping HTML tag) onwards.
func TestSanitizeCommentStripsGitOpsRateThis(t *testing.T) {
	in := `<p>About: this PR was reviewed by the assistant.</p><table><tr><td><p>Rate this:</p></td><td>★★★★★</td></tr></table>`
	out := sanitizeComment("GitOps PR Assistant", in)
	if strings.Contains(strings.ToLower(out), "rate this") {
		t.Fatalf("expected Rate this tail removed:\n%s", out)
	}
	if !strings.Contains(out, "About: this PR was reviewed by the assistant.") {
		t.Fatalf("body content was lost:\n%s", out)
	}
}

// TestSanitizeCommentLeavesHumanCommentsAlone: a real reviewer who
// happens to write "Please rate this:" must NOT lose their content.
// The strip is author-gated by isGitOpsAuthor for exactly this reason.
func TestSanitizeCommentLeavesHumanCommentsAlone(t *testing.T) {
	in := "Please rate this: how confident are you in the rollout plan?"
	out := sanitizeComment("Alice Reviewer", in)
	if out != in {
		t.Fatalf("human comment was modified — got %q, want %q", out, in)
	}
}

// TestSanitizeCommentMatchesGitOpsVariants: the bot's display name
// varies by org ("GitOps", "GitOps PR Assistant", "gitops-bot", etc.).
// The classifier is prefix + case-insensitive so all of those still hit
// the strip path.
func TestSanitizeCommentMatchesGitOpsVariants(t *testing.T) {
	for _, name := range []string{"GitOps", "GitOps PR Assistant", "gitops-bot", "  GitOps  "} {
		out := sanitizeComment(name, "body\nRate this: yes")
		if strings.Contains(strings.ToLower(out), "rate this") {
			t.Fatalf("author %q should match GitOps strip path:\n%s", name, out)
		}
	}
}

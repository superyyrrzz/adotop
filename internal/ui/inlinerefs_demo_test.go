package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestInlineRefsDemoOutput is a manual-inspection test: it renders a
// realistic comment body two ways — with and without the inline-ref
// highlighter — and prints both. Run with `-v -run InlineRefsDemo` to
// eyeball the difference; this is the artifact that justifies the
// feature visually.
//
// We strip glamour's trailing-whitespace padding (long runs of
// document-fg spaces) before printing so the colored tokens aren't
// drowned in noise when viewed in a normal terminal.
func TestInlineRefsDemoOutput(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	body := strings.Join([]string{
		"Thanks for the patch! A few notes:",
		"",
		"- @alice the retry math here looks off — see !4567 for the previous fix",
		"- This closes #890123 and supersedes AB#1234567",
		"- @bob can you double-check the `ctx` plumbing in `server.go`?",
		"",
		"```go",
		"// inside code @nobody and #999 must stay plain",
		"return doStuff()",
		"```",
		"",
		"Otherwise LGTM.",
	}, "\n")

	withRefs := renderCommentBody(body, 76, "")
	r := glamourRenderer(76)
	rawGlamour, err := r.Render(body)
	if err != nil {
		t.Fatalf("glamour render failed: %v", err)
	}
	withoutRefs := strings.TrimRight(rawGlamour, "\n") + "\n"

	fmt.Println()
	fmt.Println("══════ WITHOUT highlighter (baseline glamour) ══════")
	fmt.Println(stripTrailingPadding(withoutRefs))
	fmt.Println("══════ WITH inline-ref highlighter ══════")
	fmt.Println(stripTrailingPadding(withRefs))
}

// stripTrailingPadding removes the long runs of document-fg styled
// spaces that glamour appends to each line to fill the wrap width.
// They're invisible on a real terminal but make the test output
// unreadable when piped through Bash. We collapse each
// `\x1b[…m \x1b[0m` per-space stanza on the right edge of a line into
// nothing, then re-trim.
func stripTrailingPadding(s string) string {
	// One stanza: ESC [ ... m <space> ESC [0m
	stanza := regexp.MustCompile(`\x1b\[[0-9;:]*m \x1b\[0m`)
	out := stanza.ReplaceAllString(s, " ")
	// Collapse runs of trailing whitespace on each line.
	lines := strings.Split(out, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " ")
	}
	return strings.Join(lines, "\n")
}

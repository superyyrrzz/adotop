package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestRenderCommentBodyRespectsWidthBudget locks in the contract that
// no rendered line exceeds the requested width — including the indent.
// User report: in narrow split-pane mode, comment text gets hard-cut at
// the viewport's right edge instead of wrapping.
func TestRenderCommentBodyRespectsWidthBudget(t *testing.T) {
	cases := []struct {
		name  string
		width int
		body  string
	}{
		{"narrow_plain", 30, "this is a fairly long plain text comment that needs to wrap several times to fit"},
		{"narrow_md", 30, "## Heading\n\nthis is a fairly long markdown comment with **bold** and a [link](https://example.com) that should wrap"},
		{"narrow_html", 30, "<p>HTML comment with a <strong>strong</strong> tag and some longer text that must wrap properly within the budget</p>"},
		{"medium_plain", 60, "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."},
	}
	const indent = "      " // 6 spaces, matches renderThread's bodyIndent
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := renderCommentBody(c.body, c.width, indent)
			plain := stripANSI(out)
			for i, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
				w := lipgloss.Width(line)
				if w > c.width {
					t.Fatalf("line %d exceeds width %d (got %d): %q", i, c.width, w, line)
				}
			}
		})
	}
}

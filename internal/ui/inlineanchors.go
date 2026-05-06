package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// rightLineNumbers walks raw unified-diff bytes the same way Colorize
// does and returns one entry per output line giving the 1-based right-
// side file line number, or 0 when the line doesn't represent a line
// that exists on the right side (— deletes, hunk headers, file headers,
// trailing blank). The returned slice indexes 1:1 with the output of
// Colorize, so callers can walk both in lockstep to splice inline
// content keyed by line number.
//
// Hunk headers shape: `@@ -a,b +c,d @@` or `@@ -a +c @@` (b/d default 1).
// We track only the right-side counter — comments anchor to right-side
// line numbers in the ADO API.
func rightLineNumbers(raw []byte) []int {
	lines := strings.SplitAfter(string(raw), "\n")
	out := make([]int, 0, len(lines))
	rightLine := 0 // becomes 1-based the moment we hit the first hunk
	for _, line := range lines {
		body := line
		if strings.HasSuffix(body, "\n") {
			body = body[:len(body)-1]
		}
		switch {
		case strings.HasPrefix(body, "@@"):
			if start, ok := parseHunkRightStart(body); ok {
				// The next "+" or " " line will be at `start`, so prime
				// the counter to start-1 and let the increment land it.
				rightLine = start - 1
			}
			out = append(out, 0)
		case strings.HasPrefix(body, "+++ "),
			strings.HasPrefix(body, "--- "),
			strings.HasPrefix(body, "diff "),
			strings.HasPrefix(body, "index "):
			out = append(out, 0)
		case strings.HasPrefix(body, "+"):
			rightLine++
			out = append(out, rightLine)
		case strings.HasPrefix(body, "-"):
			out = append(out, 0)
		default:
			// Context line (leading space) or a trailing blank/junk line.
			// Context advances the right-side counter; everything else
			// gets 0. We treat any non-empty default line as context to
			// match Colorize's "default" branch.
			if body == "" {
				out = append(out, 0)
				continue
			}
			rightLine++
			out = append(out, rightLine)
		}
	}
	return out
}

// hunkHeaderRE matches `@@ -a[,b] +c[,d] @@`. We only need the c (right
// side start), but we still match the rest to anchor the regex.
var hunkHeaderRE = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func parseHunkRightStart(s string) (int, bool) {
	m := hunkHeaderRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// inlineThreadsByLine groups anchored threads by their right-side line
// number for fast lookup during the splice walk. Threads without a
// right-side anchor are skipped — they'll continue to render in the
// file-level footer block.
//
// Multiple threads on the same line preserve their order; the splice
// renders them stacked.
func inlineThreadsByLine(threads []ado.Thread) map[int][]ado.Thread {
	out := map[int][]ado.Thread{}
	for _, t := range threads {
		if t.RightLine <= 0 {
			continue
		}
		out[t.RightLine] = append(out[t.RightLine], t)
	}
	return out
}

// spliceInlineComments injects rendered comment blocks into a colorized
// diff body, immediately after each line whose right-side line number
// has anchored threads. Lines without anchored threads pass through.
//
// rendered must be the Colorize output of the same raw bytes used to
// build lineNums — they are walked in lockstep, so any divergence in
// line counts will silently misalign comments. (Empirically Colorize
// always emits exactly one output line per input line.)
//
// Returns the spliced body AND a map of thread ID → 0-based output line
// index where each inline thread starts. Callers use the map to scroll
// the viewport to a selected thread without re-walking the body.
func spliceInlineComments(rendered string, lineNums []int, byLine map[int][]ado.Thread, expanded map[int]bool, width int, selectedThreadID int) (string, map[int]int) {
	threadLines := map[int]int{}
	if len(byLine) == 0 {
		return rendered, threadLines
	}
	lines := strings.SplitAfter(rendered, "\n")
	var b strings.Builder
	b.Grow(len(rendered) + 256)
	out := 0 // running output line index
	for i, line := range lines {
		b.WriteString(line)
		out++
		if i >= len(lineNums) {
			continue
		}
		ln := lineNums[i]
		if ln == 0 {
			continue
		}
		threads, ok := byLine[ln]
		if !ok {
			continue
		}
		for _, t := range threads {
			threadLines[t.ID] = out
			block := renderInlineThread(t, expanded[t.ID], width, selectedThreadID == t.ID)
			b.WriteString(block)
			out += strings.Count(block, "\n")
		}
	}
	return b.String(), threadLines
}

// renderInlineThread formats one thread for inline display under a diff
// line. Always trails with a newline so the next diff line starts on its
// own row. The "└─" connector + indent visually attach it to the line
// above; selection state flips both the gutter (thicker glyph) AND the
// head row's background so the user can spot the cursor in a long diff
// without hunting for a single-glyph mark.
func renderInlineThread(t ado.Thread, expand bool, width int, selected bool) string {
	const baseIndent = "      " // 6 cols: 3 for the diff gutter, 3 for the connector area
	const wrapIndent = "      "
	gutter := "    "
	if selected {
		// Thicker block glyph (▌ vs ▎) makes the cursor visible at a
		// glance even when the thread sits between many diff lines.
		gutter = "  " + Cursor.Render("▌") + " "
	}
	connector := Faint.Render("└─ ")
	rendered := renderThread(t, expand, maxInt(20, width-len(baseIndent)-2))
	lines := strings.Split(rendered, "\n")
	var b strings.Builder
	for idx, ln := range lines {
		if idx == len(lines)-1 && ln == "" {
			continue
		}
		b.WriteString(gutter)
		if idx == 0 {
			b.WriteString(connector)
			if selected {
				// Full-row accent bg on the head: pad to pane width
				// minus the gutter+connector we already wrote (≈7 cols)
				// so the rectangle reads as a band rather than a chip
				// hugging the text. inline pane width can be ≤ 0 in
				// degenerate test cases — guard against that.
				prefix := lipgloss.Width(gutter) + lipgloss.Width(connector)
				w := width - prefix
				if w < lipgloss.Width(ln) {
					w = lipgloss.Width(ln)
				}
				ln = inlineSelectedHeadStyle().Width(w).Render(ln)
			}
		} else {
			b.WriteString(wrapIndent[:3])
		}
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}

// inlineSelectedHeadStyle returns the soft accent bg used to highlight
// the selected inline thread's head row. Bg is the Cursor color at low
// opacity (here approximated as the Surface0 color which is already a
// muted accent in every theme); fg is the theme's normal foreground via
// lipgloss inheritance — we don't override it so chips and chroma keep
// their colors readable on top.
//
// Caller is responsible for stretching the style to a full-row width
// via .Width(N) so the background reads as a band rather than a chip
// that just hugs the text.
func inlineSelectedHeadStyle() lipgloss.Style {
	bg := lipgloss.AdaptiveColor{Light: "#dce0e8", Dark: "#313244"}
	return lipgloss.NewStyle().Background(bg)
}

// fmtLineLabel is a tiny helper used by callers that want to print a
// line number alongside other text. Centralized here so the formatting
// stays consistent across renderers.
func fmtLineLabel(n int) string {
	return fmt.Sprintf("ln %d", n)
}

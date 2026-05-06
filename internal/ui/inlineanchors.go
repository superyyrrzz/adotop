package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
// Comment blocks are indented and prefixed with a tree connector so
// the eye reads them as "child of the line above" rather than as
// peer content.
func spliceInlineComments(rendered string, lineNums []int, byLine map[int][]ado.Thread, expanded map[int]bool, width int, selectedThreadID int) string {
	if len(byLine) == 0 {
		return rendered
	}
	lines := strings.SplitAfter(rendered, "\n")
	var b strings.Builder
	b.Grow(len(rendered) + 256)
	for i, line := range lines {
		b.WriteString(line)
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
			b.WriteString(renderInlineThread(t, expanded[t.ID], width, selectedThreadID == t.ID))
		}
	}
	return b.String()
}

// renderInlineThread formats one thread for inline display under a diff
// line. Always trails with a newline so the next diff line starts on its
// own row. The "└─" connector + 4-col indent visually attaches it to the
// line above; the gutter chip flips on selection (cursor) state.
func renderInlineThread(t ado.Thread, expand bool, width int, selected bool) string {
	const baseIndent = "      " // 6 cols: 3 for the diff gutter, 3 for the connector area
	const wrapIndent = "      "
	gutter := "    "
	if selected {
		gutter = "  " + Cursor.Render("▎") + " "
	}
	// "└─" connector on the first line so the inline block reads as a
	// child of the diff line above. Subsequent lines (replies, body
	// continuation) get a plain space indent so the connector doesn't
	// repeat — that would look like a separate thread each time.
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
		} else {
			b.WriteString(wrapIndent[:3])
		}
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}

// fmtLineLabel is a tiny helper used by callers that want to print a
// line number alongside other text. Centralized here so the formatting
// stays consistent across renderers.
func fmtLineLabel(n int) string {
	return fmt.Sprintf("ln %d", n)
}

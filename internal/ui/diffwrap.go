package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// wrapDiffLines breaks each line of pre-colorized diff content at the
// given visible width, preserving the gutter + line-bg styling on
// continuation rows so wrapped + / - lines still read as additions /
// deletions all the way across.
//
// The function is ANSI-aware: SGR escape sequences (`\x1b[...m`) are
// passed through verbatim and excluded from the width count. Wrapping
// a line in the middle of a colored span re-emits the open SGR
// sequence at the start of the next continuation row so colors don't
// bleed.
//
// width <= 0 returns the input unchanged. Empty input returns "".
//
// Continuation rows are prefixed with a faint version of the original
// gutter ("…") so the user can tell at a glance "this is the same
// logical diff line, wrapped" without losing the add/delete signal.
func wrapDiffLines(content string, width int) string {
	if width <= 0 || content == "" {
		return content
	}
	var out strings.Builder
	out.Grow(len(content) + len(content)/8)
	for _, line := range splitLinesKeepNL(content) {
		body, nl := stripTrailingNL(line)
		gutter, rest := splitGutter(body)
		// gutterWidth is the visible cell count consumed by the
		// gutter. Continuation rows reserve the same width for their
		// faint marker so the body columns align with the first row.
		gutterWidth := runewidth.StringWidth(stripANSI(gutter))
		bodyWidth := width - gutterWidth
		if bodyWidth < 4 {
			// Pane is comically narrow — fall back to no wrap rather
			// than producing 1-char-per-line output.
			out.WriteString(line)
			continue
		}
		segments := splitANSIWidth(rest, bodyWidth)
		if len(segments) <= 1 {
			out.WriteString(line)
			continue
		}
		// First segment keeps the original gutter.
		out.WriteString(gutter)
		out.WriteString(segments[0].text)
		// Reset before newline so the bg doesn't leak into the
		// terminal's view of the continuation gutter prefix.
		if segments[0].openBg != "" {
			out.WriteString(ansiReset)
		}
		out.WriteString("\n")
		// Continuation rows: faint marker + reapplied bg + tail.
		contMarker := faintContinuationGutter(gutterWidth)
		for i := 1; i < len(segments); i++ {
			seg := segments[i]
			out.WriteString(contMarker)
			if seg.openBg != "" {
				out.WriteString(seg.openBg)
			}
			out.WriteString(seg.text)
			if seg.openBg != "" {
				out.WriteString(ansiReset)
			}
			if i < len(segments)-1 {
				out.WriteString("\n")
			}
		}
		out.WriteString(nl)
	}
	return out.String()
}

// faintContinuationGutter returns a width-padded marker that makes
// continuation rows visually distinct from fresh diff lines without
// stealing attention from the wrapped content.
//
// Width is the visible cell count of the original gutter (typically 3,
// matching addBar/deleteBar/contextBar). We use a single "…" followed
// by spaces so the eye reads it as "more of the previous line" without
// looking like a new add/delete bar.
func faintContinuationGutter(width int) string {
	if width <= 0 {
		return ""
	}
	pad := width - 1
	if pad < 0 {
		pad = 0
	}
	return ansiDim + "…" + ansiReset + strings.Repeat(" ", pad)
}

// splitGutter pulls the leading 3-cell gutter off a Colorize-d diff
// line. Recognizes the four bar shapes Colorize emits:
//
//   - addBar:     ANSI bg-space + reset + green + ▌ + reset + space
//   - deleteBar:  same shape with red
//   - hunkBar:    same shape with cyan
//   - contextBar: three plain spaces
//
// Returns (gutter, rest). For lines without a recognizable gutter
// (file headers, "diff "/"index " lines, blank lines), returns ("", body).
func splitGutter(line string) (string, string) {
	// All three colored bars share the suffix "▌\x1b[0m " after their
	// bg-space prefix — split on that and take everything up through
	// the trailing space.
	const colorBarSuffix = "▌" + ansiReset + " "
	if i := strings.Index(line, colorBarSuffix); i >= 0 {
		end := i + len(colorBarSuffix)
		// Sanity: the prefix must be at the very start (modulo ANSI).
		head := line[:end]
		if strings.Contains(head, "\x1b[") {
			return head, line[end:]
		}
	}
	// Plain context bar: exactly three leading spaces. We require
	// length >= 4 so we don't strip a leading-space hunk header.
	if len(line) >= 4 && line[0] == ' ' && line[1] == ' ' && line[2] == ' ' {
		return "   ", line[3:]
	}
	return "", line
}

// wrapSegment is one chunk of a wrapped line: the visible text (with
// ANSI escapes still embedded) plus the open-bg sequence that was
// active when the chunk started — needed to re-arm the bg on
// continuation rows.
type wrapSegment struct {
	text   string
	openBg string
}

// splitANSIWidth walks s and breaks it whenever the visible width of
// the current chunk reaches maxWidth. ANSI escape sequences (CSI ...m)
// are passed through and don't contribute to width. The most recent bg
// SGR is tracked so the caller can reapply it on continuation rows.
//
// Returns one segment per resulting visual line. Segments preserve the
// in-line ANSI verbatim so chroma's foreground colors keep working.
func splitANSIWidth(s string, maxWidth int) []wrapSegment {
	if maxWidth <= 0 {
		return []wrapSegment{{text: s}}
	}
	var (
		out   []wrapSegment
		buf   strings.Builder
		w     int
		curBg string // most recent bg SGR (e.g. "\x1b[48;5;22m")
		segBg string // bg active at start of current segment
	)
	flush := func() {
		out = append(out, wrapSegment{text: buf.String(), openBg: segBg})
		buf.Reset()
		w = 0
		segBg = curBg
	}
	i := 0
	for i < len(s) {
		// ANSI CSI: \x1b[ ... m — copy verbatim, update curBg if it's
		// a background SGR or a reset.
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				// Malformed — emit the rest as-is and stop.
				buf.WriteString(s[i:])
				break
			}
			seq := s[i : i+end+1]
			buf.WriteString(seq)
			updateBg(&curBg, seq)
			if w == 0 && segBg == "" {
				segBg = curBg
			}
			i += end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		if w+rw > maxWidth {
			flush()
		}
		buf.WriteRune(r)
		w += rw
		i += size
	}
	if buf.Len() > 0 {
		out = append(out, wrapSegment{text: buf.String(), openBg: segBg})
	}
	if len(out) == 0 {
		out = append(out, wrapSegment{text: ""})
	}
	return out
}

// updateBg interprets one CSI ...m sequence and updates *bg.
//   - "\x1b[0m" (reset)         → clear bg
//   - "\x1b[48;...m" (bg color) → set bg
//   - anything else             → leave bg as-is
//
// We also clear bg when we see "\x1b[49m" (default bg).
func updateBg(bg *string, seq string) {
	switch {
	case seq == ansiReset:
		*bg = ""
	case strings.HasPrefix(seq, "\x1b[48"):
		*bg = seq
	case seq == "\x1b[49m":
		*bg = ""
	}
}

// stripANSI returns s with all CSI escape sequences removed. Used to
// measure visible width.
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				b.WriteString(s[i:])
				break
			}
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// splitLinesKeepNL splits s into lines but keeps the trailing "\n" on
// each (matching strings.SplitAfter, but compatible with our existing
// helpers).
func splitLinesKeepNL(s string) []string {
	return strings.SplitAfter(s, "\n")
}

func stripTrailingNL(s string) (string, string) {
	if strings.HasSuffix(s, "\n") {
		return s[:len(s)-1], "\n"
	}
	return s, ""
}

package ui

import (
	"bytes"
	"strings"
)

const (
	ansiReset    = "\x1b[0m"
	ansiBold     = "\x1b[1m"
	ansiRed      = "\x1b[31m"
	ansiGreen    = "\x1b[32m"
	ansiCyan     = "\x1b[36m"
	ansiDim      = "\x1b[2m"
	addBar       = "\x1b[32m▌\x1b[0m "
	deleteBar    = "\x1b[31m▌\x1b[0m "
	contextBar   = "  "
	hunkBar      = "\x1b[36m▌\x1b[0m "
	headerEscape = "\x1b["
)

// Colorize takes raw unified-diff bytes and returns ANSI-colored bytes
// suitable for a terminal. + lines are green, - lines are red, hunk
// headers are cyan, file headers are bold; each line gets a colored
// gutter bar so changes stand out at a glance.
//
// If the input already contains ANSI escapes (e.g. delta output),
// it is returned unchanged.
func Colorize(in []byte) []byte {
	if bytes.Contains(in, []byte(headerEscape)) {
		return in
	}
	var out bytes.Buffer
	out.Grow(len(in) + len(in)/4)
	lines := strings.SplitAfter(string(in), "\n")
	for _, line := range lines {
		// Trim trailing newline for inspection but re-add at end.
		body := line
		nl := ""
		if strings.HasSuffix(body, "\n") {
			body = body[:len(body)-1]
			nl = "\n"
		}
		switch {
		case strings.HasPrefix(body, "+++ ") || strings.HasPrefix(body, "--- "):
			out.WriteString(ansiBold)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "@@"):
			out.WriteString(hunkBar)
			out.WriteString(ansiCyan)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "+"):
			out.WriteString(addBar)
			out.WriteString(ansiGreen)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "-"):
			out.WriteString(deleteBar)
			out.WriteString(ansiRed)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "diff ") || strings.HasPrefix(body, "index "):
			out.WriteString(ansiDim)
			out.WriteString(body)
			out.WriteString(ansiReset)
		default:
			out.WriteString(contextBar)
			out.WriteString(body)
		}
		out.WriteString(nl)
	}
	return out.Bytes()
}

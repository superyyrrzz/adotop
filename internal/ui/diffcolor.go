package ui

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/renzeyu/adotop/internal/ui/theme"
)

const (
	ansiReset    = "\x1b[0m"
	ansiBold     = "\x1b[1m"
	ansiRed      = "\x1b[31m"
	ansiGreen    = "\x1b[32m"
	ansiCyan     = "\x1b[36m"
	ansiDim      = "\x1b[2m"
	ansiRedBg    = "\x1b[41m"
	ansiGreenBg  = "\x1b[42m"
	ansiCyanBg   = "\x1b[46m"
	addBar       = ansiGreenBg + " " + ansiReset + ansiGreen + "▌" + ansiReset + " "
	deleteBar    = ansiRedBg + " " + ansiReset + ansiRed + "▌" + ansiReset + " "
	contextBar   = "   "
	hunkBar      = ansiCyanBg + " " + ansiReset + ansiCyan + "▌" + ansiReset + " "
	headerEscape = "\x1b["
)

// Theme-derived diff line backgrounds. Defaulted to xterm-256 22 (dark
// green) and 52 (dark red) so package-level use without a theme still
// produces a sensible render. applyDiffTheme overwrites these at app
// startup (see New() in app.go) once the active Theme is known.
var (
	ansiAddLineBg = "\x1b[48;5;22m"
	ansiDelLineBg = "\x1b[48;5;52m"
)

// Colorize takes raw unified-diff bytes and returns ANSI-colored bytes
// suitable for a terminal. + lines are green, - lines are red, hunk
// headers are cyan, file headers are bold; each line gets a colored
// gutter bar so changes stand out at a glance.
//
// Added/removed lines also get a dim full-line background so the change
// is visible even after syntax highlighting recolors the foreground.
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
	currentPath := ""
	for _, line := range lines {
		body := line
		nl := ""
		if strings.HasSuffix(body, "\n") {
			body = body[:len(body)-1]
			nl = "\n"
		}
		switch {
		case strings.HasPrefix(body, "+++ "):
			currentPath = stripDiffPathPrefix(body[4:])
			out.WriteString(ansiBold)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "--- "):
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
			out.WriteString(ansiAddLineBg)
			out.WriteString(ansiGreen)
			out.WriteString("+")
			out.WriteString(persistBg(HighlightLine(currentPath, body[1:]), ansiAddLineBg))
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "-"):
			out.WriteString(deleteBar)
			out.WriteString(ansiDelLineBg)
			out.WriteString(ansiRed)
			out.WriteString("-")
			out.WriteString(persistBg(HighlightLine(currentPath, body[1:]), ansiDelLineBg))
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "diff ") || strings.HasPrefix(body, "index "):
			out.WriteString(ansiDim)
			out.WriteString(body)
			out.WriteString(ansiReset)
		default:
			out.WriteString(contextBar)
			if len(body) > 0 && body[0] == ' ' {
				out.WriteString(" ")
				out.WriteString(HighlightLine(currentPath, body[1:]))
			} else {
				out.WriteString(HighlightLine(currentPath, body))
			}
		}
		out.WriteString(nl)
	}
	return out.Bytes()
}

// persistBg keeps a background color alive across the SGR resets that
// chroma emits at the end of every token. Without this, the first token
// boundary (e.g. a keyword → space) clears the diff line's background and
// the rest of the line renders with terminal default bg.
//
// We replace every full reset with `reset + bg` so the background is
// re-armed immediately. The caller is responsible for the final reset
// at end-of-line.
func persistBg(s, bg string) string {
	if s == "" {
		return s
	}
	if !strings.Contains(s, ansiReset) {
		return s
	}
	return strings.ReplaceAll(s, ansiReset, ansiReset+bg)
}

// stripDiffPathPrefix removes a leading "a/" or "b/" if present and trims
// trailing whitespace, leaving something filepath.Ext can use.
func stripDiffPathPrefix(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "a/") || strings.HasPrefix(s, "b/") {
		return s[2:]
	}
	return s
}

// applyDiffTheme repoints the package-level diff backgrounds at the
// active theme's DiffAddBg/DiffDelBg colors. lipgloss handles the
// truecolor → 256-color downgrade based on termenv.ColorProfile().
//
// This is package-global state — there's only one diff renderer and
// only one theme per process, so the indirection isn't worth a struct.
func applyDiffTheme(t theme.Theme) {
	// Use a dedicated TrueColor renderer so we can compute ANSI sequences
	// even when stdout is not a TTY (e.g., in tests). The diff bytes we
	// emit feed into the bubble tea viewport, which has its own profile,
	// so producing TrueColor here is safe — the terminal multiplexer at
	// the very end normalizes it.
	r := lipgloss.NewRenderer(nil)
	r.SetColorProfile(termenv.TrueColor)
	add := r.NewStyle().Background(t.DiffAddBg)
	del := r.NewStyle().Background(t.DiffDelBg)
	if seq := openSequence(add.Render("")); seq != "" {
		ansiAddLineBg = seq
	}
	if seq := openSequence(del.Render("")); seq != "" {
		ansiDelLineBg = seq
	}
}

// openSequence pulls the leading "\x1b[...m" out of a lipgloss-rendered
// string. lipgloss wraps content as "<open><content><reset>"; we want
// just <open>. Returns "" if the input doesn't start with a CSI.
func openSequence(s string) string {
	if !strings.HasPrefix(s, "\x1b[") {
		return ""
	}
	end := strings.Index(s, "m")
	if end < 0 {
		return ""
	}
	return s[:end+1]
}

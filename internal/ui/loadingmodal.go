package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// spinnerFrames is the standard Braille-dot spinner cycle (10 frames,
// rotating ~every 100ms feels like a smooth rotation to the eye).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// loadingTickMsg drives the spinner animation. The Model handler advances
// loadingFrame and re-emits a tick if the modal is still up.
type loadingTickMsg struct{}

// scheduleLoadingTick returns a Cmd that fires the next spinner advance
// after a short delay. Caller is responsible for only scheduling when
// the modal is active.
func scheduleLoadingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return loadingTickMsg{}
	})
}

// overlayLoadingModal composites a styled "LOADING / PR #N" box over
// the existing body so the user can still see the underlying screen
// behind/around the modal. The box has a thick mauve border, accented
// header text, and an animated spinner driven by `frame`.
func overlayLoadingModal(body string, prID, frame, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	box := renderLoadingBox(prID, frame)
	if strings.TrimSpace(body) == "" {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}
	return overlayBox(body, box, width, height)
}

// renderLoadingBox builds the two-line modal content: an accent-colored
// "LOADING" header on top and the PR identifier with spinner below.
// Width is fixed so the box doesn't shimmer as the spinner glyph
// changes pixel width across frames.
func renderLoadingBox(prID, frame int) string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#cba6f7")). // Mauve, matches selection frame
		Render("LOADING")
	spinner := spinnerFrames[frame%len(spinnerFrames)]
	body := fmt.Sprintf("%s  PR #%d", spinner, prID)
	content := lipgloss.JoinVertical(lipgloss.Center, header, body)
	return ModalBox.Width(28).Align(lipgloss.Center).Render(content)
}

// overlayBox splices each line of fg into the corresponding line of bg
// at the centered offset, returning the composed string. ANSI escapes
// in bg are preserved using ansi.Cut so colors don't bleed past the
// box on either side.
//
// If fg is taller than bg (or wider than termW) it's clipped — the box
// is meant to be a small overlay, not a takeover.
func overlayBox(bg, fg string, termW, termH int) string {
	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < termH {
		bgLines = append(bgLines, "")
	}
	fgLines := strings.Split(strings.TrimRight(fg, "\n"), "\n")
	boxH := len(fgLines)
	boxW := 0
	for _, l := range fgLines {
		if w := lipgloss.Width(l); w > boxW {
			boxW = w
		}
	}
	if boxH > termH {
		boxH = termH
		fgLines = fgLines[:boxH]
	}
	if boxW > termW {
		boxW = termW
	}
	rowOffset := (termH - boxH) / 2
	colOffset := (termW - boxW) / 2
	if colOffset < 0 {
		colOffset = 0
	}
	for i, fl := range fgLines {
		row := rowOffset + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bgLines[row] = spliceLine(bgLines[row], fl, colOffset, boxW)
	}
	return strings.Join(bgLines, "\n")
}

// spliceLine inserts fg starting at column `at` into bg, preserving the
// portion of bg to the left of `at` and to the right of `at+boxW`. bg
// may contain ANSI escape sequences; ansi.Cut handles them correctly.
func spliceLine(bg, fg string, at, boxW int) string {
	bgW := lipgloss.Width(bg)
	left := ansi.Cut(bg, 0, at)
	if bgW < at {
		left += strings.Repeat(" ", at-bgW)
	}
	right := ""
	if bgW > at+boxW {
		right = ansi.Cut(bg, at+boxW, bgW)
	}
	return left + fg + right
}

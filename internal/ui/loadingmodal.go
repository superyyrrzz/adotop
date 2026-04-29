package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// overlayLoadingModal composites a small "Loading PR #N…" box over the
// existing body so the user can still see the underlying screen
// (typically the PR list) faintly behind/around the modal. Each box
// row replaces a horizontal slice of the corresponding body row at the
// computed centered offset; the rest of the body row is preserved with
// its ANSI styling intact.
//
// width and height describe the body's full pane dimensions in cells.
// When either is non-positive (e.g., before the first WindowSizeMsg)
// we fall back to printing the box on a blank canvas so the affordance
// still appears.
func overlayLoadingModal(body string, prID, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	box := ModalBox.Render(fmt.Sprintf("Loading PR #%d…", prID))
	if strings.TrimSpace(body) == "" {
		// No background to composite onto — just center the box on a
		// blank canvas. lipgloss.Place handles the geometry.
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}
	return overlayBox(body, box, width, height)
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
	// Pad bg to termH so we can place the box even when the screen
	// hasn't filled its area yet. Empty padding lines splice cleanly.
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
		bgLines[row] = spliceLine(bgLines[row], fl, colOffset, boxW, termW)
	}
	return strings.Join(bgLines, "\n")
}

// spliceLine inserts fg starting at column `at` into bg, preserving the
// portion of bg to the left of `at` and to the right of `at+boxW`. bg
// may contain ANSI escape sequences; ansi.Cut handles them correctly.
// termW is the total cell width — needed so we can pad bg up to the
// splice point when bg is shorter than `at`.
func spliceLine(bg, fg string, at, boxW, termW int) string {
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

package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// overlayLoadingModal renders a small centered "Loading PR #N…" box on
// top of the body. Uses lipgloss.Place to center within the body's
// bounding box; the underlying body is replaced rather than blended
// because terminal cells can't truly composite. The Help-style border
// keeps the visual vocabulary consistent with the existing help modal.
//
// width and height are the body's available dimensions. When either is
// non-positive (the very first frame before WindowSizeMsg lands), we
// fall back to a fixed reasonable size so the modal still appears —
// the alternative was returning the body unchanged, which made the
// modal silently invisible during the auth-faster-than-resize window.
func overlayLoadingModal(body string, prID, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	box := HelpBox.Render(fmt.Sprintf("Loading PR #%d…", prID))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

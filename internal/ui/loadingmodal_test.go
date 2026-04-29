package ui

import (
	"strings"
	"testing"
)

// TestOverlayLoadingModalShowsPRID: the rendered overlay must contain
// the literal "Loading PR #<id>…" so the user can confirm the launch
// resolved to the PR they pasted. The exact centering offsets aren't
// asserted — lipgloss is responsible for those — only the text and the
// fact that it sits inside the bounding box.
func TestOverlayLoadingModalShowsPRID(t *testing.T) {
	out := overlayLoadingModal("ignored body", 1145743, 80, 24)
	if !strings.Contains(out, "Loading PR #1145743") {
		t.Fatalf("overlay should name the PR ID; got:\n%s", out)
	}
}

// TestOverlayLoadingModalCompositesOverBackground: with a real body
// the modal must replace only the centered slice — text outside the
// box's column range survives so the user can see the screen behind.
func TestOverlayLoadingModalCompositesOverBackground(t *testing.T) {
	bg := strings.Repeat("LEFT-EDGE-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX-RIGHT-EDGE\n", 10)
	out := overlayLoadingModal(bg, 7, 60, 10)
	if !strings.Contains(out, "Loading PR #7") {
		t.Fatalf("modal text missing:\n%s", out)
	}
	// Both the left and right ends of at least one bg row must survive
	// — the box is centered so it shouldn't reach the column-0 or last
	// column of a 60-wide canvas.
	if !strings.Contains(out, "LEFT-EDGE") {
		t.Fatalf("bg left edge clobbered:\n%s", out)
	}
	if !strings.Contains(out, "RIGHT-EDGE") {
		t.Fatalf("bg right edge clobbered:\n%s", out)
	}
}

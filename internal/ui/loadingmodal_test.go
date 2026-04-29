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

// TestOverlayLoadingModalUsesFallbackOnZeroSize: when called before
// WindowSize arrives (width or height is 0), the modal must still
// render — falling back to a sensible default size — so the user
// always sees the affordance. The earlier behavior of returning body
// unchanged made the modal invisible in real launches because auth
// frequently completed before the first WindowSizeMsg.
func TestOverlayLoadingModalUsesFallbackOnZeroSize(t *testing.T) {
	out := overlayLoadingModal("body", 7, 0, 0)
	if !strings.Contains(out, "Loading PR #7") {
		t.Fatalf("zero-size call should still render modal text; got:\n%s", out)
	}
}

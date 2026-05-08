package ui

import (
	"strings"
	"testing"
)

// TestHelpModalRendersGroupedSections: the rendered help overlay
// must include section titles (so it reads as a reference card, not
// a flat list) AND at least one key from each section. Catches the
// common breakage where a refactor accidentally drops a whole
// section or its rows.
func TestHelpModalRendersGroupedSections(t *testing.T) {
	out := stripANSI(renderHelpModal(120))

	wantSections := []string{"Navigation", "Threads", "Actions", "Views & Modals", "List screen", "Chrome"}
	for _, sec := range wantSections {
		if !strings.Contains(out, sec) {
			t.Fatalf("help modal missing section %q:\n%s", sec, out)
		}
	}

	// Spot-check one key per section to confirm the rows render and
	// the keys aren't silently dropped by the column-width logic.
	wantKeys := []string{"tab / shift+tab", "space", "v", "M", "/", "?"}
	for _, k := range wantKeys {
		if !strings.Contains(out, k) {
			t.Fatalf("help modal missing key %q:\n%s", k, out)
		}
	}
}

// TestHelpModalHasFooterCloseHint: the footer must remind the user
// of the close keys. Without this hint the modal looks self-
// contained but the user has to remember `?` toggles it off.
func TestHelpModalHasFooterCloseHint(t *testing.T) {
	out := stripANSI(renderHelpModal(120))
	if !strings.Contains(out, "? or esc to close") {
		t.Fatalf("help modal missing close-key hint:\n%s", out)
	}
}

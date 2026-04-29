package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestCommentsHeaderHighlightsHiddenResolved is the regression guard
// for "I missed the (N resolved hidden — press R) hint and didn't
// know I could expand them". The affordance must:
//  1. Carry a non-faint, attention-grabbing style (Wait/yellow + bold)
//     so it stands out from the surrounding faint header
//  2. Spell out "press R to show" so the binding is discoverable from
//     the comment block itself, not just from the global help screen
func TestCommentsHeaderHighlightsHiddenResolved(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "/x.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "open one"}}},
		{ID: 2, FilePath: "/x.go", Status: "fixed", Comments: []ado.Comment{{Author: "B", Content: "resolved"}}},
	}
	out := renderCommentsBlock(threads[:1], map[int]bool{}, false /*showResolved*/, threads, "/x.go", 80)
	plain := stripANSI(out)
	if !strings.Contains(plain, "1 resolved hidden") {
		t.Fatalf("header should advertise hidden count:\n%s", plain)
	}
	if !strings.Contains(plain, "press R to show") {
		t.Fatalf("header should spell out the R binding:\n%s", plain)
	}
	// We can't assert on ANSI escapes in test mode (lipgloss notty
	// profile strips them), but the text presence and ordering are
	// the contract: hint is OUTSIDE the parenthesized count, not
	// crammed inside it where it would scan as part of the header.
	idxClose := strings.Index(plain, ")")
	idxHint := strings.Index(plain, "1 resolved hidden")
	if idxHint < idxClose {
		t.Fatalf("hint should appear after the (open) count, not inside the parens:\n%s", plain)
	}
}

// TestCommentsHeaderShowsToggleOnState mirrors the affordance: when
// resolved threads ARE visible, the header reminds the user the
// toggle is on so they can flip it off.
func TestCommentsHeaderShowsToggleOnState(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "/x.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "open"}}},
		{ID: 2, FilePath: "/x.go", Status: "fixed", Comments: []ado.Comment{{Author: "B", Content: "resolved"}}},
	}
	// All threads passed through (showResolved=true means the caller
	// already included resolved ones in the visible list).
	out := renderCommentsBlock(threads, map[int]bool{}, true /*showResolved*/, threads, "/x.go", 80)
	plain := stripANSI(out)
	if !strings.Contains(plain, "showing resolved") {
		t.Fatalf("header should remind user the toggle is on:\n%s", plain)
	}
	if !strings.Contains(plain, "press R to hide") {
		t.Fatalf("header should advertise the reverse binding:\n%s", plain)
	}
}

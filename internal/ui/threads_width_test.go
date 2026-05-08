package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestRenderThreadCollapsedFitsWidthBudget locks in the fix for the
// narrow-pane "comment trimmed at right edge" bug. The collapsed
// single-line form was previously emitting a line up to ~200+
// characters wide regardless of the pane's width — the viewport
// then hard-clipped it with no indication, hiding the more-comments
// affordance and breaking the visual frame.
//
// Contract: the rendered line (head + optional more-hint) must not
// exceed the supplied width budget for any reasonable pane size.
func TestRenderThreadCollapsedFitsWidthBudget(t *testing.T) {
	long := strings.Repeat("very-long-comment-text ", 20) // ~440 chars
	thread := ado.Thread{
		ID: 1, FilePath: "/x.go", RightLine: 12, Status: "active",
		Comments: []ado.Comment{
			{Author: "Alice", Content: long},
			{Author: "Bob", Content: "follow up"},
		},
	}
	for _, w := range []int{30, 40, 60, 80, 120} {
		out := renderThread(thread, false /*collapsed*/, w)
		plain := strings.TrimRight(stripANSI(out), "\n")
		got := lipgloss.Width(plain)
		if got > w {
			t.Fatalf("width=%d: line is %d wide, exceeds budget:\n%q", w, got, plain)
		}
		// The expand affordance must still be visible — that's the
		// whole point of the budgeting (without it the viewport
		// silently clips it off). Either the long form or the compact
		// "+N" fallback counts.
		if !strings.Contains(plain, "more — space to expand") && !strings.Contains(plain, "+1") {
			t.Fatalf("width=%d: expand cue missing from:\n%q", w, plain)
		}
	}
}

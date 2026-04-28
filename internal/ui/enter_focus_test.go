package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEnterFromFilesFocusSwitchesToDiff is the regression guard for the
// "enter as drill-in" idiom. From Files focus, enter must move the user
// to Diff focus on the same file. From Diff focus, enter retains its
// existing behavior (toggle thread expansion) and must NOT bounce back
// to Files — drilling in once is a single deliberate keystroke.
func TestEnterFromFilesFocusSwitchesToDiff(t *testing.T) {
	m := newDetailModel(t)
	if m.detailFocus != focusFiles {
		t.Fatalf("detail screen should open in Files focus; got %v", m.detailFocus)
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.detailFocus != focusDiff {
		t.Fatalf("enter from Files focus should land on Diff focus; got %v", m.detailFocus)
	}

	// Second enter on Diff focus should keep us on Diff (it's the
	// thread-expand toggle, not a focus toggle).
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.detailFocus != focusDiff {
		t.Fatalf("enter on Diff focus must not change focus; got %v", m.detailFocus)
	}
}

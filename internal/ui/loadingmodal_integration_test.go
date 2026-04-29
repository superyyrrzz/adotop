package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestLoadingModalClearedOnJumpResultError: error case dismisses the
// modal immediately (no point holding a "Loading…" affordance when
// the load failed — the footer carries the error message instead).
func TestLoadingModalClearedOnJumpResultError(t *testing.T) {
	m := Model{}
	m.loadingPRModal = 42
	updated, _ := m.Update(jumpResultMsg{prID: 42, err: errFakeJump})
	mm := updated.(Model)
	if mm.loadingPRModal != 0 {
		t.Fatalf("error path should clear loadingPRModal; got %d", mm.loadingPRModal)
	}
}

// TestLoadingModalClearedOnJumpResult: success path dismisses the
// modal as soon as the screen flips from list to detail (jumpResultMsg
// triggers openDetail). The detail screen renders a brief skeleton
// while files/diff stream in — that's preferable to a modal lingering
// over a screen that's already taken over.
func TestLoadingModalClearedOnJumpResult(t *testing.T) {
	m := newDetailModel(t)
	m.loadingPRModal = 42
	updated, _ := m.Update(jumpResultMsg{prID: 42, summary: m.detail.Summary()})
	mm := updated.(Model)
	if mm.loadingPRModal != 0 {
		t.Fatalf("jumpResultMsg success should clear modal; got %d", mm.loadingPRModal)
	}
}

// TestLoadingModalRendersInView: with the flag set, View() must
// produce output containing the "Loading PR #N…" text.
func TestLoadingModalRendersInView(t *testing.T) {
	m := Model{}
	m.screen = screenList
	m.loadingPRModal = 42
	m.width = 80
	m.height = 24
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mm := updated.(Model)
	mm.loadingPRModal = 42
	out := mm.View()
	if !strings.Contains(out, "PR #42") {
		t.Fatalf("View should render the loading modal; got:\n%s", out)
	}
}

var errFakeJump = &fakeErr{msg: "boom"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }


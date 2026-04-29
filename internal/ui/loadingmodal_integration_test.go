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

// TestLoadingModalClearedByDelayedMsg: the success path keeps the
// modal up until clearLoadingModalMsg arrives (fired by a 350ms
// timer), so the modal stays visible through fast jump fetches.
func TestLoadingModalClearedByDelayedMsg(t *testing.T) {
	m := Model{}
	m.loadingPRModal = 42
	updated, _ := m.Update(clearLoadingModalMsg{prID: 42})
	mm := updated.(Model)
	if mm.loadingPRModal != 0 {
		t.Fatalf("clearLoadingModalMsg should clear modal; got %d", mm.loadingPRModal)
	}
}

// TestLoadingModalClearGuardsByPRID: a clear timer fired for an old
// PR must not dismiss a freshly-armed modal for a different PR.
func TestLoadingModalClearGuardsByPRID(t *testing.T) {
	m := Model{}
	m.loadingPRModal = 999
	updated, _ := m.Update(clearLoadingModalMsg{prID: 42}) // old timer
	mm := updated.(Model)
	if mm.loadingPRModal != 999 {
		t.Fatalf("stale clear should not dismiss current modal; got %d", mm.loadingPRModal)
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
	if !strings.Contains(out, "Loading PR #42") {
		t.Fatalf("View should render the loading modal; got:\n%s", out)
	}
}

var errFakeJump = &fakeErr{msg: "boom"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }


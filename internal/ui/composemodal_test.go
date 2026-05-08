package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestComposeModalEscCancelsCleanly: esc inside the modal closes it
// without dispatching a post — the in-progress draft is intentionally
// dropped. Asserts the modal pointer goes back to nil so the next
// keystroke acts on the underlying screen.
func TestComposeModalEscCancelsCleanly(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m = m.openComposeNewModal()
	if !m.composeModalOpen() {
		t.Fatalf("openComposeNewModal did not open the modal")
	}
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.composeModalOpen() {
		t.Fatalf("esc must close the compose modal")
	}
	if cmd != nil {
		t.Fatalf("esc must not dispatch a post cmd; got %v", cmd)
	}
}

// TestComposeModalCtrlSDispatchesPost: ctrl+s with a non-empty buffer
// must close the modal AND dispatch the post cmd. Empty buffers are a
// silent no-op (cancel-by-typing-nothing-then-submitting reads as
// cancel, not as an empty comment we'd reject server-side).
func TestComposeModalCtrlSDispatchesPost(t *testing.T) {
	m := newDetailModel(t)
	d := m.detail.SetSummary(ado.PRSummary{ID: 1, RepoID: "r", Title: "x"})
	d, _ = d.Update(filesLoadedMsg{files: []ado.FileChange{{Path: "/a.go", ChangeType: "edit"}}})
	m.detail = d
	m.detailFocus = focusDiff
	m = m.openComposeNewModal()
	// Type a few characters by feeding rune key messages — exercises
	// the textarea delegation path so we don't just assert on a
	// directly-set buffer.
	for _, r := range "hi" {
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = mm.(Model)
	if m.composeModalOpen() {
		t.Fatalf("ctrl+s should close the modal after submit")
	}
	if cmd == nil {
		t.Fatalf("ctrl+s with non-empty buffer must dispatch a post cmd")
	}
}

// TestComposeModalCtrlSEmptyBufferNoops: an empty buffer + ctrl+s
// closes the modal but dispatches nothing — same contract as the old
// "save empty file in $EDITOR" cancel gesture. Prevents posting empty
// threads to ADO when the user just wanted out of the modal.
func TestComposeModalCtrlSEmptyBufferNoops(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m = m.openComposeNewModal()
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = mm.(Model)
	if m.composeModalOpen() {
		t.Fatalf("ctrl+s should close the modal even with empty buffer")
	}
	if cmd != nil {
		t.Fatalf("ctrl+s with empty buffer must NOT dispatch a post; got cmd=%v", cmd)
	}
}

// TestComposeModalRendersOverlay: with the modal open, the rendered
// View must contain the modal title and hint footer. Confirms the
// overlay path is wired so the user actually sees the compose UI
// (not just state on the model that never paints).
func TestComposeModalRendersOverlay(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.width, m.height = 120, 40
	m = m.openComposeNewModal()
	out := stripANSI(m.View())
	if !strings.Contains(out, "New PR Comment") {
		t.Fatalf("compose modal title missing from rendered View:\n%s", out)
	}
	if !strings.Contains(out, "ctrl+s") {
		t.Fatalf("compose modal hint missing from rendered View:\n%s", out)
	}
}

// TestComposeModalSwallowsKeysAndBlocksUnderlyingScreen: while the
// modal is open, keys like 'q' and 'X' that would normally trigger
// quit/abandon must NOT fire. Otherwise a stray keystroke in a
// composer can have catastrophic side effects.
func TestComposeModalSwallowsKeysAndBlocksUnderlyingScreen(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m = m.openComposeNewModal()

	// q would normally try to leave the screen. With the modal open
	// it must just be typed into the textarea.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = mm.(Model)
	if m.screen != screenDetail {
		t.Fatalf("q during compose modal should not change screen; got %v", m.screen)
	}
	if !m.composeModalOpen() {
		t.Fatalf("q during compose modal should keep the modal open")
	}
}

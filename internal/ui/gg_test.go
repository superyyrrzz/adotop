package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestDetailGGJumpsCursorToFirstFile covers the vim-style `gg` two-key
// sequence in Files focus: it should walk the file cursor to the first
// file in display order. A single `g` must NOT move the cursor — the
// pendingG state has to wait for the second key.
func TestDetailGGJumpsCursorToFirstFile(t *testing.T) {
	m := newDetailModel(t)
	// Move off the first file so we can detect the jump.
	m.detail.cursor = 2
	if got := m.detail.cursor; got != 2 {
		t.Fatalf("setup: cursor want 2 got %d", got)
	}

	// First g arms pendingG; cursor must not move.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if !m.pendingG {
		t.Fatalf("first g should arm pendingG")
	}
	if m.detail.cursor != 2 {
		t.Fatalf("first g must not move cursor, got %d", m.detail.cursor)
	}

	// Second g completes the sequence and jumps to the first file.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if m.pendingG {
		t.Fatalf("pendingG should clear after second g")
	}
	want := m.detail.FirstDisplayFile()
	if m.detail.cursor != want {
		t.Fatalf("gg should jump to first display file %d, got %d", want, m.detail.cursor)
	}
}

// TestDetailGCancelledByOtherKey: after the first g, any non-g key
// cancels the pending sequence and is processed normally.
func TestDetailGCancelledByOtherKey(t *testing.T) {
	m := newDetailModel(t)
	m.detail.cursor = 0

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if !m.pendingG {
		t.Fatalf("g should arm pendingG")
	}

	// `n` advances file cursor — but only as the cancelling key, the
	// jump must not also fire.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	if m.pendingG {
		t.Fatalf("pendingG should be cleared by intervening key")
	}
	if m.detail.cursor != 1 {
		t.Fatalf("n after g should still advance cursor by 1, got %d", m.detail.cursor)
	}
}

// TestDetailCapitalGJumpsCursorToLastFile: G is a one-shot end-jump.
func TestDetailCapitalGJumpsCursorToLastFile(t *testing.T) {
	m := newDetailModel(t)
	m.detail.cursor = 0

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = mm.(Model)
	want := m.detail.LastDisplayFile()
	if m.detail.cursor != want {
		t.Fatalf("G should jump to last display file %d, got %d", want, m.detail.cursor)
	}
}

// TestDetailGGScrollsDiffViewportInDiffFocus: when focus is on the
// diff pane, gg/G must scroll the viewport instead of moving the file
// cursor.
func TestDetailGGScrollsDiffViewportInDiffFocus(t *testing.T) {
	m := newDetailModel(t)
	m.preview = m.preview.SetSize(40, 5)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   []byte(strings.Repeat("ctx\n", 200)),
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})
	// Switch to diff focus.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	cursorBefore := m.detail.cursor

	// Scroll to bottom with G.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = mm.(Model)
	if m.detail.cursor != cursorBefore {
		t.Fatalf("G in diff focus must not move file cursor")
	}
	if m.preview.vp.YOffset == 0 {
		t.Fatalf("G in diff focus should scroll viewport to bottom; YOffset still 0")
	}

	bottom := m.preview.vp.YOffset
	// gg should scroll back to top.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = mm.(Model)
	if m.preview.vp.YOffset == bottom {
		t.Fatalf("gg in diff focus should scroll viewport to top")
	}
	if m.preview.vp.YOffset != 0 {
		t.Fatalf("gg should put YOffset at 0, got %d", m.preview.vp.YOffset)
	}
}

// Ensure ado import isn't dropped when somebody trims the file later.
var _ = ado.PRSummary{}

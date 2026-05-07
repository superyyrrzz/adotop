package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestListCursorWrapsAtTopAndBottom: pressing k on the first PR jumps
// to the last; pressing j on the last jumps to the first. Walking off
// either edge should return to the opposite end so the user can cycle
// through a small list without lifting their hand.
func TestListCursorWrapsAtTopAndBottom(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: samplePRs()})
	n := len(m.visible())
	if n < 2 {
		t.Fatalf("samplePRs returned %d rows; need ≥2 to test wrap", n)
	}

	// Cursor starts at 0. k once → last index.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.cursor != n-1 {
		t.Fatalf("k at top: cursor=%d, want %d (wrap to last)", m.cursor, n-1)
	}

	// j once from the last → 0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.cursor != 0 {
		t.Fatalf("j at bottom: cursor=%d, want 0 (wrap to first)", m.cursor)
	}
}

// TestDetailFileCursorWrapsAtTopAndBottom: same wrap contract on the
// detail-screen file list. neighborFile is shared by j/k and n/N so a
// single change covers both navigation surfaces.
func TestDetailFileCursorWrapsAtTopAndBottom(t *testing.T) {
	m := newDetailModel(t)
	order := m.detail.displayOrder()
	if len(order) < 2 {
		t.Fatalf("need ≥2 display rows; got %d", len(order))
	}

	// Cursor starts at file index 0. k → last entry in display order.
	wantLast := order[len(order)-1]
	got := m.detail.neighborFile(-1)
	if got != wantLast {
		t.Fatalf("k from first file: cursor=%d, want %d (last in display order)", got, wantLast)
	}

	// Park on the last entry then j → first.
	m.detail.cursor = wantLast
	wantFirst := order[0]
	got = m.detail.neighborFile(+1)
	if got != wantFirst {
		t.Fatalf("j from last file: cursor=%d, want %d (first in display order)", got, wantFirst)
	}
}

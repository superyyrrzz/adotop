package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestShowResolvedTogglesFilterOnly verifies R is now a pure filter
// toggle: it flips showResolved, rebuilds the preview so resolved
// threads appear, but does NOT expand or scroll. Earlier R bundled
// expand+scroll as a side effect; that role moved to J so each key
// has one responsibility.
func TestShowResolvedTogglesFilterOnly(t *testing.T) {
	m := newDetailModel(t)
	m.preview = m.preview.SetSize(40, 10)

	body := []byte(strings.Repeat("ctx-line\n", 80))
	key := diffSelectionKey("src", "tgt", "/a.go", 0)
	m.previewKey = key
	m.previewCache.Set(1, key, body)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   body,
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})

	m.threads = []ado.Thread{{
		ID: 100, FilePath: "/a.go", Status: "fixed",
		Comments: []ado.Comment{
			{ID: 1, Author: "alice", Content: "ship it"},
			{ID: 2, Author: "bob", Content: "+1"},
		},
	}}
	m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = mm.(Model)

	if !m.showResolved {
		t.Fatalf("R should toggle showResolved on")
	}
	if m.expandedThread[100] {
		t.Fatalf("R must NOT auto-expand any thread (that's J's job)")
	}
	if off := m.preview.vp.YOffset; off != 0 {
		t.Fatalf("R must NOT auto-scroll (that's J's job); viewport YOffset=%d", off)
	}
}

// TestJumpToCommentsExpandsAndScrolls covers the new J handler. It
// must:
//
//  1. expand all threads on the selected file
//  2. move the viewport down toward the comments block (YOffset > 0)
//  3. when the only comments on the file are resolved AND showResolved
//     is off, also flip showResolved on so the user actually lands on
//     readable content (not "(no open comments)").
func TestJumpToCommentsExpandsAndScrolls(t *testing.T) {
	m := newDetailModel(t)
	m.preview = m.preview.SetSize(40, 10)

	body := []byte(strings.Repeat("ctx-line\n", 80))
	key := diffSelectionKey("src", "tgt", "/a.go", 0)
	m.previewKey = key
	m.previewCache.Set(1, key, body)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   body,
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})

	// One resolved thread, no open ones — the J auto-flip case.
	m.threads = []ado.Thread{{
		ID: 100, FilePath: "/a.go", Status: "fixed",
		Comments: []ado.Comment{
			{ID: 1, Author: "alice", Content: "ship it"},
			{ID: 2, Author: "bob", Content: "+1"},
		},
	}}
	m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = mm.(Model)

	if !m.showResolved {
		t.Fatalf("J should auto-flip showResolved on when only resolved comments exist")
	}
	if !m.expandedThread[100] {
		t.Fatalf("J should expand all threads on the file; expandedThread[100]=false")
	}
	if off := m.preview.vp.YOffset; off == 0 {
		t.Fatalf("J should scroll toward the comments block; viewport YOffset still 0")
	}
	m.width, m.height = 120, 30
	m.preview = m.preview.SetSize(40, 10)
	view := m.preview.View()
	if !strings.Contains(view, "alice") {
		t.Fatalf("preview View should show the revealed comment author after J; got:\n%s", view)
	}
}

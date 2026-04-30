package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestShowResolvedExpandsAndScrolls reproduces the user-reported bug: pressing
// R to reveal resolved comments expands the threads on the focused file but
// the viewport stays parked at the top of the diff instead of scrolling down
// to the comments block. Both behaviors must hold:
//
//  1. every visible thread on the selected file is expanded
//  2. the viewport YOffset is non-zero (it scrolled toward the comments)
func TestShowResolvedExpandsAndScrolls(t *testing.T) {
	m := newDetailModel(t)
	m.preview = m.preview.SetSize(40, 10) // small viewport so scroll is meaningful

	// Seed the preview cache with a diff for /a.go so refreshPreview can
	// rebuild the viewport content. Without a cached body, scrollPreviewToComments
	// and refreshPreview both early-return.
	body := []byte(strings.Repeat("ctx-line\n", 80))
	key := diffSelectionKey("src", "tgt", "/a.go", 0)
	m.previewKey = key
	m.previewCache.Set(1, key, body)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   body,
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})

	// One resolved thread on /a.go that R will reveal.
	m.threads = []ado.Thread{{
		ID: 100, FilePath: "/a.go", Status: "fixed",
		Comments: []ado.Comment{
			{ID: 1, Author: "alice", Content: "ship it"},
			{ID: 2, Author: "bob", Content: "+1"},
		},
	}}
	m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)

	// Sanity: thread starts collapsed and resolved is hidden.
	if m.showResolved {
		t.Fatalf("precondition: showResolved must start false")
	}
	if m.expandedThread[100] {
		t.Fatalf("precondition: thread should start collapsed")
	}
	if off := m.preview.vp.YOffset; off != 0 {
		t.Fatalf("precondition: viewport should start at top, got offset %d", off)
	}

	// Press R.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = mm.(Model)

	// (1) showResolved flipped on.
	if !m.showResolved {
		t.Fatalf("R should toggle showResolved on, got false")
	}
	// (2) The thread on /a.go is now expanded.
	if !m.expandedThread[100] {
		t.Fatalf("R should auto-expand resolved threads on the focused file; expandedThread[100]=false")
	}
	// (3) The viewport scrolled down toward the comments block.
	if off := m.preview.vp.YOffset; off == 0 {
		t.Fatalf("R should auto-scroll toward comments; viewport YOffset still 0")
	}
	// (4) End-to-end: the rendered preview pane must actually show
	// the resolved comment author/body. YOffset moving isn't enough
	// if View() resets sizing or rebuilds content somewhere downstream.
	m.width, m.height = 120, 30
	m.preview = m.preview.SetSize(40, 10)
	view := m.preview.View()
	if !strings.Contains(view, "alice") {
		t.Fatalf("preview View should show the revealed comment author after R; got:\n%s", view)
	}
}

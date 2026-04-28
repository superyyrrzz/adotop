package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestNextPrevFileMatchesJK verifies that n/N walks the file cursor in
// the same display (tree) order as j/k. Before the fix, n/N walked
// API order (m.files slice) while j/k walked the sorted/grouped tree,
// so the two could diverge once we group files into directories.
//
// The test uses a file set whose API order != display order: API order
// here is /z/a, /a/b, /a/c, /z/d (mixed dirs); display order after
// buildFileTree groups by directory: /a/b, /a/c, /z/a, /z/d.
func TestNextPrevFileMatchesJK(t *testing.T) {
	files := []ado.FileChange{
		{Path: "/z/a.go", ChangeType: "edit"},
		{Path: "/a/b.go", ChangeType: "edit"},
		{Path: "/a/c.go", ChangeType: "edit"},
		{Path: "/z/d.go", ChangeType: "edit"},
	}

	walkN := walkCursor(t, files, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	walkJ := walkCursor(t, files, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	if !equalInts(walkN, walkJ) {
		t.Fatalf("n and j should produce same cursor sequence.\n n: %v\n j: %v", walkN, walkJ)
	}

	walkBigN := walkCursor(t, files, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	walkK := walkCursor(t, files, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Reverse walks start from the same cursor too. We start at file 0
	// for both, then press the key 4 times. The sequence should match.
	if !equalInts(walkBigN, walkK) {
		t.Fatalf("N and k should produce same cursor sequence.\n N: %v\n k: %v", walkBigN, walkK)
	}
}

// walkCursor builds a fresh detail model with `files`, presses `key`
// 4 times, and returns the cursor position observed at each step
// (including the starting position).
func walkCursor(t *testing.T, files []ado.FileChange, key tea.KeyMsg) []int {
	t.Helper()
	m := newTestModel()
	m.screen = screenDetail
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: 1, Title: "x"})
	d, _ := m.detail.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary: ado.PRSummary{ID: 1, Title: "x"},
		SourceSha: "src", TargetSha: "tgt",
	}})
	m.detail = d
	d, _ = m.detail.Update(filesLoadedMsg{files: files})
	m.detail = d
	// Start cursor at the first display-order file so forward walks
	// from j and n start identically and reverse walks from k and N
	// start identically.
	m.detail.cursor = m.detail.FirstDisplayFile()

	out := []int{m.detail.cursor}
	for i := 0; i < len(files); i++ {
		mm, _ := m.Update(key)
		m = mm.(Model)
		out = append(out, m.detail.cursor)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

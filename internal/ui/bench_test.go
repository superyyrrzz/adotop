package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/gitlocal"
)

// BenchmarkJKCycleCacheHit measures the cost of a single n/N keystroke
// when the target diff is already in the preview cache. This is the
// hot path the user feels while flicking up/down a long PR's file
// list — every regression in DetailModel.Update, queuePreviewForSelection,
// refreshPreview, or buildFileTree shows up here.
//
// Setup: 50 files, each with a ~2k-line diff body, all preloaded into
// previewCache. The benchmark alternates n/N so the cursor bounces
// between the same two cache-hit slots.
func BenchmarkJKCycleCacheHit(b *testing.B) {
	keys := DefaultKeys()
	m := Model{
		keys:           keys,
		git:            gitlocal.New(nil),
		list:           NewList(keys),
		detail:         NewDetail(keys),
		preview:        NewDiff(keys),
		scrollMem:     map[string]int{},
		previewCache:  newDiffBodyCache(5),
		expandedThread: map[int]bool{},
		width:          140,
		height:         40,
		screen:         screenDetail,
	}
	summary := ado.PRSummary{ID: 9001, Title: "bench", RepoID: "r"}
	m.detail = m.detail.SetSummary(summary)
	d, _ := m.detail.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary: summary, SourceSha: "src", TargetSha: "tgt",
	}})
	m.detail = d
	const nFiles = 50
	files := make([]ado.FileChange, nFiles)
	for i := range files {
		files[i] = ado.FileChange{Path: fmt.Sprintf("/src/file_%03d.go", i), ChangeType: "edit"}
	}
	d, _ = m.detail.Update(filesLoadedMsg{files: files})
	m.detail = d

	// Build a synthetic ~2k-line diff body and pre-warm the cache.
	body := make([]byte, 0, 2000*40)
	body = append(body, []byte("--- a/x\n+++ b/x\n@@ -1,2000 +1,2000 @@\n")...)
	for i := 0; i < 2000; i++ {
		body = append(body, []byte(fmt.Sprintf(" line %d kept context\n", i))...)
	}
	for _, f := range files {
		key := diffSelectionKey("src", "tgt", f.Path, 0)
		m.previewCache.Set(summary.ID, key, body)
	}

	m.preview = m.preview.SetSize(60, 20)
	// Prime the preview by selecting the first file so we're measuring
	// steady-state navigation cost, not first-render cost.
	mm, _ := m.queuePreviewForSelection()
	m = mm

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key := tea.KeyRunes
		r := []rune{'n'}
		if i%2 == 1 {
			r = []rune{'N'}
		}
		next, _ := m.Update(tea.KeyMsg{Type: key, Runes: r})
		m = next.(Model)
	}
}

// BenchmarkBuildFileTree measures the per-keypress cost of computing
// the sorted/grouped tree from scratch. This is what the memo cache
// avoids — useful for spotting accidental regressions in the memo path.
func BenchmarkBuildFileTree(b *testing.B) {
	const nFiles = 200
	files := make([]ado.FileChange, nFiles)
	for i := range files {
		files[i] = ado.FileChange{
			Path:       fmt.Sprintf("/src/pkg_%02d/file_%03d.go", i%10, i),
			ChangeType: "edit",
		}
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = buildFileTree(files)
	}
}

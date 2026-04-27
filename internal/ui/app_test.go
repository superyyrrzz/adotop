package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/renzeyu/adotop/internal/ado"
	"github.com/renzeyu/adotop/internal/gitlocal"
)

func TestDetailScreenAutoPreviewsFirstFile(t *testing.T) {
	m := newTestModel()
	m.screen = screenDetail
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: 42, Title: "Preview me"})

	next, _ := m.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary: ado.PRSummary{ID: 42, Title: "Preview me"},
		SourceSha: "source-sha",
		TargetSha: "target-sha",
	}})
	m = next.(Model)

	next, cmd := m.Update(filesLoadedMsg{files: []ado.FileChange{
		{Path: "/a.go", ChangeType: "edit"},
		{Path: "/b.go", ChangeType: "edit"},
	}})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected preview diff load command")
	}
	if got := m.preview.file; got != "/a.go" {
		t.Fatalf("expected first file to auto-preview, got %q", got)
	}
}

func TestDetailScreenMovesPreviewWithCursor(t *testing.T) {
	m := newTestModel()
	m.screen = screenDetail
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: 7, Title: "Cursor preview"})
	m.detail, _ = m.detail.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary: ado.PRSummary{ID: 7, Title: "Cursor preview"},
		SourceSha: "source-sha",
		TargetSha: "target-sha",
	}})
	m.detail, _ = m.detail.Update(filesLoadedMsg{files: []ado.FileChange{
		{Path: "/a.go", ChangeType: "edit"},
		{Path: "/b.go", ChangeType: "edit"},
	}})
	m, _ = m.queuePreviewForSelection()

	next, cmd := m.updateDetailScreen(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected preview reload command after cursor move")
	}
	if got := m.preview.file; got != "/b.go" {
		t.Fatalf("expected preview to follow cursor, got %q", got)
	}
}

func TestDetailViewShowsPreviewPane(t *testing.T) {
	m := newTestModel()
	m.screen = screenDetail
	m.width = 140
	m.height = 30
	m.detail = m.detail.SetSummary(ado.PRSummary{
		ID:           1234,
		Title:        "Fix login bug",
		Author:       "alice",
		SourceBranch: "feat/login",
		TargetBranch: "main",
	})
	m.detail, _ = m.detail.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary:     ado.PRSummary{ID: 1234, Title: "Fix login bug"},
		DescriptionMD: "Fixes the issue where session tokens were not refreshed.",
		SourceSha:     "source-sha",
		TargetSha:     "target-sha",
	}})
	m.detail, _ = m.detail.Update(filesLoadedMsg{files: []ado.FileChange{{Path: "/src/login.go", ChangeType: "edit"}}})
	m.preview = m.sizeDiffModel(m.preview.SetHeader("/src/login.go", "rest"), diffTargetPreview)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		target:    diffTargetPreview,
		requestID: 1,
		content:   []byte("--- a/src/login.go\n+++ b/src/login.go\n-old\n+new\n"),
	})

	out := m.View()
	if !strings.Contains(out, "Diff Preview") {
		t.Fatalf("missing preview pane title:\n%s", out)
	}
	if !strings.Contains(out, "/src/login.go") || !strings.Contains(out, "+new") {
		t.Fatalf("missing preview diff content:\n%s", out)
	}
}

func TestModelIgnoresStalePreviewDiffLoads(t *testing.T) {
	m := newTestModel()
	m.screen = screenDetail
	m.previewReqID = 2
	m.preview = m.preview.SetHeader("/fresh.go", "rest")

	next, _ := m.Update(diffLoadedMsg{
		target:    diffTargetPreview,
		requestID: 1,
		content:   []byte("--- a/stale.go\n+++ b/stale.go\n-old\n+stale\n"),
	})
	m = next.(Model)

	out := m.preview.View()
	if strings.Contains(out, "stale") {
		t.Fatalf("stale preview load should be ignored:\n%s", out)
	}
	if !strings.Contains(out, "loading") {
		t.Fatalf("preview should remain unchanged:\n%s", out)
	}
}

func newTestModel() Model {
	keys := DefaultKeys()
	return Model{
		keys:          keys,
		git:           gitlocal.New(nil),
		list:          NewList(keys),
		detail:        NewDetail(keys),
		diff:          NewDiff(keys),
		preview:       NewDiff(keys),
		scrollMem:     map[string]int{},
		previewBodies: map[string][]byte{},
	}
}

func newDetailModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel()
	m.screen = screenDetail
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: 1, Title: "x"})
	d, _ := m.detail.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary: ado.PRSummary{ID: 1, Title: "x"},
		SourceSha: "src", TargetSha: "tgt",
	}})
	m.detail = d
	d, _ = m.detail.Update(filesLoadedMsg{files: []ado.FileChange{
		{Path: "/a.go", ChangeType: "edit"},
		{Path: "/b.go", ChangeType: "edit"},
		{Path: "/c.go", ChangeType: "edit"},
	}})
	m.detail = d
	return m
}

func TestDetailTabTogglesFocus(t *testing.T) {
	m := newDetailModel(t)
	if m.detailFocus != focusFiles {
		t.Fatalf("expected default focusFiles, got %v", m.detailFocus)
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	if m.detailFocus != focusDiff {
		t.Fatalf("tab should switch to focusDiff, got %v", m.detailFocus)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = mm.(Model)
	if m.detailFocus != focusFiles {
		t.Fatalf("shift+tab should switch back to focusFiles, got %v", m.detailFocus)
	}
}

func TestDetailDiffFocusRoutesScrollKeys(t *testing.T) {
	m := newDetailModel(t)
	// Pre-fill preview with multi-line content so PgDn moves the offset.
	m.preview = m.preview.SetSize(40, 5)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   []byte(strings.Repeat("ctx\n", 200)),
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})

	// In files focus, PgDn must NOT scroll the preview.
	beforeOffset := m.preview.vp.YOffset
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = mm.(Model)
	if m.preview.vp.YOffset != beforeOffset {
		t.Fatalf("preview should not scroll in files focus; offset went %d -> %d", beforeOffset, m.preview.vp.YOffset)
	}

	// Switch to diff focus; PgDn must scroll preview.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	cursorAfterTab := m.detail.cursor
	offsetBefore := m.preview.vp.YOffset
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = mm.(Model)
	if m.detail.cursor != cursorAfterTab {
		t.Fatalf("file cursor must not move when diff has focus")
	}
	if m.preview.vp.YOffset == offsetBefore {
		t.Fatalf("preview should have scrolled in diff focus")
	}
}

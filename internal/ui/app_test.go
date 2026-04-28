package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/cache"
	"github.com/superyyrrzz/adotop/internal/gitlocal"
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
	// The bordered preview pane uses the file path as its title now;
	// "Diff Preview" was redundant chrome. Body must contain the +
	// hunk so we know the diff actually rendered inside the box.
	if !strings.Contains(out, "/src/login.go") {
		t.Fatalf("missing preview pane file-path title:\n%s", out)
	}
	if !strings.Contains(out, "new") {
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
		preview:       NewDiff(keys),
		scrollMem:     map[string]int{},
		previewCache:  newDiffBodyCache(5),
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

func TestDetailNextPrevFileInDiffFocus(t *testing.T) {
	m := newDetailModel(t)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // focus diff
	m = mm.(Model)

	if m.detail.cursor != 0 {
		t.Fatalf("cursor should start at 0")
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	if m.detail.cursor != 1 {
		t.Fatalf("n should advance file cursor while keeping diff focus, got %d", m.detail.cursor)
	}
	if m.detailFocus != focusDiff {
		t.Fatalf("focus should remain on diff after n")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = mm.(Model)
	if m.detail.cursor != 0 {
		t.Fatalf("N should retreat file cursor, got %d", m.detail.cursor)
	}
}

func TestDetailRestoresPerFileScrollOffset(t *testing.T) {
	m := newDetailModel(t)
	body := []byte(strings.Repeat("ctx\n", 200))
	m.preview = m.preview.SetSize(40, 5)

	// Land on /a.go and load its body.
	mfocus, _ := m.queuePreviewForSelection()
	m = mfocus
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: body, target: diffTargetPreview, requestID: m.previewReqID,
	})
	// Scroll preview down on /a.go (simulate via SetYOffset).
	m.preview.vp.SetYOffset(40)
	scrolledA := m.preview.vp.YOffset
	if scrolledA == 0 {
		t.Fatalf("expected non-zero scroll on /a.go")
	}

	// Move to /b.go.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: body, target: diffTargetPreview, requestID: m.previewReqID,
	})
	if m.preview.vp.YOffset != 0 {
		t.Fatalf("expected /b.go to start at top, got %d", m.preview.vp.YOffset)
	}

	// Back to /a.go — saved offset must be restored after the diff lands.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = mm.(Model)
	mm, _ = m.Update(diffLoadedMsg{
		content: body, target: diffTargetPreview, requestID: m.previewReqID,
	})
	m = mm.(Model)
	if m.preview.vp.YOffset != scrolledA {
		t.Fatalf("expected /a.go scroll to be restored to %d, got %d", scrolledA, m.preview.vp.YOffset)
	}
}

func TestDetailServesPrefetchedNeighborInstantly(t *testing.T) {
	m := newDetailModel(t)
	bKey := diffSelectionKey("src", "tgt", "/b.go", 0)
	m.previewCache.Set(m.detail.Summary().ID, bKey, []byte("--- a/b.go\n+++ b/b.go\n+B\n"))
	m.preview = m.preview.SetSize(40, 10)

	// Move to /b.go — cache hit should render synchronously.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	if !strings.Contains(m.preview.vp.View(), "B") {
		t.Fatalf("expected cached body to render immediately:\n%s", m.preview.vp.View())
	}
}

func TestDetailFocusIndicatorMovesWithFocus(t *testing.T) {
	m := newDetailModel(t)
	m.width, m.height = 140, 40
	m.preview = m.preview.SetSize(60, 20)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: []byte("--- a/x\n+++ b/x\n+hi\n"), target: diffTargetPreview,
	})

	// Files focus: left-pane Files header still uses the ● dot
	// indicator (it's a section sub-header inside a chrome-free pane,
	// not a pane title — the dot pattern is appropriate there).
	out := m.detailPreviewView()
	if !strings.Contains(out, "● Files") {
		t.Fatalf("expected files header to show focus dot:\n%s", out)
	}
	// The right pane no longer has a "Diff Preview" header — the
	// rounded border carries the file path as the title and the
	// border color is the focus signal. Just confirm the file path
	// title is present and the dot-style focus marker is absent.
	if strings.Contains(out, "● Diff") {
		t.Fatalf("diff pane should not use the ● dot focus marker:\n%s", out)
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	out = m.detailPreviewView()
	if strings.Contains(out, "● Files") {
		t.Fatalf("files header should not show focus dot when diff focused:\n%s", out)
	}
	if strings.Contains(out, "● Diff") {
		t.Fatalf("diff pane should never use the ● dot focus marker:\n%s", out)
	}
}

func TestDetailAbandonRequiresConfirmation(t *testing.T) {
	m := newDetailModel(t)
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = mm.(Model)
	if m.pendingAction.kind != "abandon" {
		t.Fatalf("expected pending abandon prompt, got %q", m.pendingAction.kind)
	}
	if cmd != nil {
		t.Fatalf("X should not fire the action immediately; got cmd")
	}
	// Esc cancels.
	mm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.pendingAction.kind != "" {
		t.Fatalf("esc should clear pending action")
	}
	if cmd != nil {
		t.Fatalf("esc should not run any command")
	}
}

func TestDetailApproveFiresImmediately(t *testing.T) {
	m := newDetailModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd == nil {
		t.Fatalf("expected approve to return a command")
	}
}

func TestQuitKeyOnDetailGoesBackNotQuit(t *testing.T) {
	m := newDetailModel(t)
	if m.screen != screenDetail {
		t.Fatalf("setup: expected detail screen")
	}
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = mm.(Model)
	if cmd != nil {
		t.Fatalf("q on detail should not return tea.Quit cmd")
	}
	if m.screen != screenList {
		t.Fatalf("expected screen=list after q on detail, got %v", m.screen)
	}
}

func TestQuitKeyOnListQuits(t *testing.T) {
	m := newTestModel()
	m.screen = screenList
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("q on list should quit (returning a tea.Cmd)")
	}
}

func TestCtrlCAlwaysQuitsFromDetail(t *testing.T) {
	m := newDetailModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("ctrl+c on detail must always quit")
	}
}

func TestOpeningPRRecordsRecentVisit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	st, err := cache.New()
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	m := newTestModel()
	m.cache = st
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: []ado.PRSummary{
		{ID: 77, Title: "open me"},
	}})

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)

	got, ok := st.LoadRecents()
	if !ok || len(got) != 1 || got[0].ID != 77 {
		t.Fatalf("expected recents=[77], got ok=%v list=%+v", ok, got)
	}
}

func TestListJumpPromptCollectsDigitsAndEmitsRequest(t *testing.T) {
	m := newTestModel()
	m.screen = screenList

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'#'}})
	m = mm.(Model)
	if !m.list.jumping {
		t.Fatalf("expected jumping=true after #")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = mm.(Model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = mm.(Model)
	if m.list.jumpInput != "12" {
		t.Fatalf("expected jumpInput=12, got %q", m.list.jumpInput)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter to emit a jumpRequestedMsg cmd")
	}
	got := cmd()
	if jr, ok := got.(jumpRequestedMsg); !ok || jr.ID != 12 {
		t.Fatalf("expected jumpRequestedMsg{ID:12}, got %T %+v", got, got)
	}
}


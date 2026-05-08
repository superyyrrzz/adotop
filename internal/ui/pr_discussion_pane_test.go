package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// seedPRThreads attaches two PR-level (unanchored) threads and refreshes
// the detail model's prThreads cache so HasDiscussion/IsDiscussionSelected
// behave as in the live app.
func seedPRThreads(m Model) Model {
	m.threads = []ado.Thread{
		{ID: 11, FilePath: "", Status: "active", Comments: []ado.Comment{
			{ID: 1, Author: "alice", Content: "first PR comment"},
			{ID: 2, Author: "bob", Content: "first reply"},
		}},
		{ID: 22, FilePath: "", Status: "active", Comments: []ado.Comment{
			{ID: 3, Author: "carol", Content: "second PR comment"},
		}},
	}
	m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
	return m
}

// TestDiscussionEntryAppearsAndIsFirst: with PR-level threads present,
// the synthetic Discussion row sits at the top of the file list and is
// the first entry visited by FirstDisplayFile.
func TestDiscussionEntryAppearsAndIsFirst(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	if !m.detail.HasDiscussion() {
		t.Fatalf("HasDiscussion=false after seeding PR threads")
	}
	if first := m.detail.FirstDisplayFile(); first != discussionRowIdx {
		t.Fatalf("FirstDisplayFile=%d, want discussionRowIdx (%d)", first, discussionRowIdx)
	}
}

// TestNoDiscussionEntryWithoutPRThreads: when the PR has only file-
// anchored threads, the synthetic row must NOT appear — otherwise we
// add chrome that does nothing.
func TestNoDiscussionEntryWithoutPRThreads(t *testing.T) {
	m := newDetailModel(t)
	m.threads = []ado.Thread{{ID: 99, FilePath: "/a.go", Status: "active",
		Comments: []ado.Comment{{ID: 1, Author: "a", Content: "x"}}}}
	m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
	if m.detail.HasDiscussion() {
		t.Fatalf("HasDiscussion=true with only file-anchored threads")
	}
}

// TestBracketKeysWalkPRThreadsInFilesFocus: with Discussion selected
// and Files focus active, [/] cycle through the PR-level thread list
// via prThreadCursor (not threadCursor, which is per-file).
func TestBracketKeysWalkPRThreadsInFilesFocus(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	m.detail = m.detail.SelectDiscussion()
	if !m.detail.IsDiscussionSelected() {
		t.Fatalf("SelectDiscussion did not stick")
	}

	// First ] lands on index 0 — the contract from moveThreadCursor's
	// "unset → 0 regardless of direction" rule.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if m.prThreadCursor != 0 {
		t.Fatalf("after first ]: prThreadCursor=%d, want 0", m.prThreadCursor)
	}
	if got := m.currentThreadID(); got != 11 {
		t.Fatalf("currentThreadID after first ]: got %d, want 11", got)
	}

	// Second ] advances to the second PR thread.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if got := m.currentThreadID(); got != 22 {
		t.Fatalf("currentThreadID after second ]: got %d, want 22", got)
	}

	// [ steps back.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m = mm.(Model)
	if got := m.currentThreadID(); got != 11 {
		t.Fatalf("currentThreadID after [: got %d, want 11", got)
	}
}

// TestDiscussionPaneRendersThreadsWithSelection: refreshPreview swaps
// the viewport contents to the PR-thread list, the selected thread
// gets the ▌ gutter, and the line-map is populated so [/] can scroll.
func TestDiscussionPaneRendersThreadsWithSelection(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	m.preview = m.preview.SetSize(60, 20)
	m.detail = m.detail.SelectDiscussion()
	m.prThreadCursor = 1 // pick second thread so we can assert it's selected

	m = m.refreshPreview()
	view := m.preview.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "Discussion") {
		t.Fatalf("preview View missing 'Discussion' header:\n%s", plain)
	}
	if !strings.Contains(plain, "carol") {
		t.Fatalf("preview View missing second-thread author 'carol':\n%s", plain)
	}
	if !strings.Contains(plain, "▌") {
		t.Fatalf("preview View missing selection gutter (▌):\n%s", plain)
	}
	if _, ok := m.inlineThreadLines[22]; !ok {
		t.Fatalf("inlineThreadLines missing entry for selected thread 22: %v", m.inlineThreadLines)
	}
}

// TestSpaceTogglesPRThreadExpandFromDiffFocus: with Discussion selected
// and Diff focus active (user dropped in to read the pane), space must
// toggle the selected PR thread's expand state. enter is reserved for
// the focus-drill path now, so this exercises the new ExpandThread key.
func TestSpaceTogglesPRThreadExpandFromDiffFocus(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	m.preview = m.preview.SetSize(60, 20)
	m.detail = m.detail.SelectDiscussion()
	m.prThreadCursor = 0 // cursor on thread 11
	m.detailFocus = focusDiff

	if m.expandedThread[11] {
		t.Fatalf("expandedThread[11]=true before space; expected false")
	}
	mm, _ := m.updateDetailScreen(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = mm.(Model)
	if !m.expandedThread[11] {
		t.Fatalf("expandedThread[11]=false after space in Diff focus; expected true")
	}
}

// TestSpaceTogglesPRThreadExpand: in Files focus + Discussion selected,
// pressing space on the cursor's PR thread flips its expandedThread
// entry. enter no longer carries this — it's now strictly the
// Files→Diff drill key.
func TestSpaceTogglesPRThreadExpand(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	m.preview = m.preview.SetSize(60, 20)
	m.detail = m.detail.SelectDiscussion()
	m.prThreadCursor = 0 // cursor on thread 11

	if m.expandedThread[11] {
		t.Fatalf("expandedThread[11]=true before space; expected false")
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = mm.(Model)
	if !m.expandedThread[11] {
		t.Fatalf("expandedThread[11]=false after space; expected true")
	}
	// Reply text only renders when the thread is expanded — confirm
	// the rendered pane reflects the new state.
	view := stripANSI(m.preview.View())
	if !strings.Contains(view, "first reply") {
		t.Fatalf("expanded thread should show second comment 'first reply':\n%s", view)
	}
}

// TestEnterDoesNotExpandPRThread: locks down the inverse contract —
// enter on a Discussion-selected PR thread must NOT toggle expansion.
// enter is now the Files→Diff drill key only. Catches a regression
// that re-overloads enter.
func TestEnterDoesNotExpandPRThread(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	m.preview = m.preview.SetSize(60, 20)
	m.detail = m.detail.SelectDiscussion()
	m.prThreadCursor = 0
	// Files focus → enter should drill into Diff, not expand.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.expandedThread[11] {
		t.Fatalf("enter must not expand PR thread; expandedThread[11]=true")
	}
	if m.detailFocus != focusDiff {
		t.Fatalf("enter from Files focus must drill into Diff; got focus=%v", m.detailFocus)
	}
}

// TestSwitchingFromDiscussionRestoresFilePreview: moving the cursor
// off Discussion (j/k or n) must trigger a preview re-fetch for the
// newly-selected file, not leave the Discussion pane stale on screen.
// We assert the indirect signal: previewKey is reset to the new
// selection's key (cache lookup will then drive content).
func TestSwitchingFromDiscussionRestoresFilePreview(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	m.preview = m.preview.SetSize(60, 20)
	m.detail = m.detail.SelectDiscussion()
	m = m.refreshPreview() // pane now shows Discussion

	// Press n to advance to the first real file.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	if m.detail.IsDiscussionSelected() {
		t.Fatalf("n from Discussion should advance to first file")
	}
	// previewKey must now match the new selection — proves
	// syncPreviewForSelection routed to queuePreviewForSelection
	// instead of leaving the Discussion pane in place.
	f, ok := m.detail.SelectedFile()
	if !ok {
		t.Fatalf("expected a file selection after n")
	}
	wantKey := diffSelectionKey("src", "tgt", f.Path, 0)
	if m.previewKey != wantKey {
		t.Fatalf("previewKey=%q, want %q", m.previewKey, wantKey)
	}
}

// TestDiscussionFiltersSystemNoise: ADO emits audit threads (policy
// status, branch ref updates, abandon notifications) with status="" and
// every comment marked commentType=system. Those swamp the human review
// signal in the Discussion list, so prLevelThreads must drop them. Real
// bots that file an actionable thread (Status set OR a text comment)
// must survive — they carry actual review feedback.
func TestDiscussionFiltersSystemNoise(t *testing.T) {
	m := newDetailModel(t)
	m.threads = []ado.Thread{
		// Pure noise — should be filtered.
		{ID: 1, FilePath: "", Status: "", Comments: []ado.Comment{
			{Author: "Microsoft.VisualStudio.Services.TFS", Content: "Policy status updated", Type: "system"},
		}},
		{ID: 2, FilePath: "", Status: "", Comments: []ado.Comment{
			{Author: "Microsoft.VisualStudio.Services.TFS", Content: "Branch updated", Type: "system"},
		}},
		// Real human comment — must survive.
		{ID: 3, FilePath: "", Status: "active", Comments: []ado.Comment{
			{Author: "Renze Yu", Content: "thoughts here", Type: "text"},
		}},
		// System bot with an actionable status — must survive too.
		{ID: 4, FilePath: "", Status: "active", Comments: []ado.Comment{
			{Author: "GitOps PR Assistant", Content: "found a violation", Type: "system"},
		}},
	}
	m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
	got := m.prLevelThreads()
	if len(got) != 2 {
		t.Fatalf("expected 2 surviving threads (3 and 4), got %d: %+v", len(got), got)
	}
	wantIDs := map[int]bool{3: true, 4: true}
	for _, th := range got {
		if !wantIDs[th.ID] {
			t.Fatalf("unexpected thread id=%d in result", th.ID)
		}
	}
	if m.detail.PRThreadCount() != 2 {
		t.Fatalf("PRThreadCount=%d, want 2", m.detail.PRThreadCount())
	}
}

// TestDiscussionPaneShowsPositionIndicator: with cursor on the second of
// three threads, the header must include "[2/3]" so the user knows
// where they are. With no selection (prThreadCursor == -1), the
// indicator is omitted to keep the header tidy.
func TestDiscussionPaneShowsPositionIndicator(t *testing.T) {
	threads := []ado.Thread{
		{ID: 10, FilePath: "", Status: "active", Comments: []ado.Comment{{Author: "a", Content: "x", Type: "text"}}},
		{ID: 20, FilePath: "", Status: "active", Comments: []ado.Comment{{Author: "b", Content: "y", Type: "text"}}},
		{ID: 30, FilePath: "", Status: "active", Comments: []ado.Comment{{Author: "c", Content: "z", Type: "text"}}},
	}
	body, _ := renderDiscussionPane(threads, map[int]bool{}, 60, 20)
	plain := stripANSI(body)
	if !strings.Contains(plain, "[2/3]") {
		t.Fatalf("expected [2/3] indicator with cursor on thread 20:\n%s", plain)
	}

	// No selection — no indicator.
	body, _ = renderDiscussionPane(threads, map[int]bool{}, 60, 0)
	plain = stripANSI(body)
	if strings.Contains(plain, "[") && strings.Contains(plain, "/") && strings.Contains(plain, "]") {
		// Be more precise — the chip rendering can introduce brackets.
		if strings.Contains(plain, "[1/3]") || strings.Contains(plain, "[0/3]") {
			t.Fatalf("expected no indicator without selection:\n%s", plain)
		}
	}
}

// TestDiscussionRowSurvivesPaneSizes runs the live geometry matrix from
// CLAUDE.md: across realistic pane sizes, the synthetic Discussion row
// must appear in the rendered file pane when PR-level threads exist.
// This catches view bugs where a tight window budget would clip the
// always-on top row of the file list.
func TestDiscussionRowSurvivesPaneSizes(t *testing.T) {
	m := newDetailModel(t)
	m = seedPRThreads(m)
	forEachPaneSize(t, func(t *testing.T, w, h int) {
		out := renderDetailInLayout(t, m.detail, w, h)
		plain := stripANSI(out)
		if !strings.Contains(plain, "Discussion") {
			t.Fatalf("Discussion row missing at %dx%d:\n%s", w, h, plain)
		}
	})
}

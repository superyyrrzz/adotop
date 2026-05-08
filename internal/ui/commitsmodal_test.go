package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestCommitsModalOpensAndShowsLoading: pressing M opens the picker
// and renders the loading state. The actual fetch happens via the
// returned cmd; we don't run it (no client wiring in tests), but
// the modal pointer being set + loading flag is the public contract.
func TestCommitsModalOpensAndShowsLoading(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.width, m.height = 120, 40
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	m = mm.(Model)
	if !m.commitsModalOpen() {
		t.Fatalf("M should open the commits modal")
	}
	if !m.commitsModal.loading {
		t.Fatalf("modal should start in loading state")
	}
	if cmd == nil {
		t.Fatalf("M should dispatch the commits-fetch cmd")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "Commits in this PR") {
		t.Fatalf("modal title missing from rendered View:\n%s", out)
	}
	if !strings.Contains(out, "loading") {
		t.Fatalf("loading state missing from rendered View:\n%s", out)
	}
}

// TestCommitsModalRendersListAfterLoad: once commitsLoadedMsg arrives
// the loading state clears and each commit row appears. Cursor
// defaults to 0 (first commit), highlighted with the selection
// marker.
func TestCommitsModalRendersListAfterLoad(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.width, m.height = 120, 40
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	m = mm.(Model)
	mm, _ = m.Update(commitsLoadedMsg{commits: []ado.Commit{
		{ID: "abc1234567", Author: "Alice", Subject: "first commit", CommitDate: time.Now()},
		{ID: "def8901234", ParentID: "abc1234567", Author: "Bob", Subject: "second commit", CommitDate: time.Now()},
	}})
	m = mm.(Model)
	if m.commitsModal.loading {
		t.Fatalf("loading flag should clear after commitsLoadedMsg")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "first commit") || !strings.Contains(out, "second commit") {
		t.Fatalf("commit subjects missing from list:\n%s", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Fatalf("short SHA missing from list:\n%s", out)
	}
}

// TestCommitsModalEnterSelectsAndExitsPicker: enter on the cursor
// closes the picker AND dispatches the GetCommitChanges fetch. The
// picked commit is staged so the subsequent commitChangesLoadedMsg
// can wire it into viewingCommit.
func TestCommitsModalEnterSelectsAndExitsPicker(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.width, m.height = 120, 40
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	m = mm.(Model)
	mm, _ = m.Update(commitsLoadedMsg{commits: []ado.Commit{
		{ID: "abc1234567", Subject: "first"},
	}})
	m = mm.(Model)
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if m.commitsModalOpen() {
		t.Fatalf("enter should close the picker after select")
	}
	if cmd == nil {
		t.Fatalf("enter should dispatch the commit-changes fetch")
	}
}

// TestCommitChangesLoadedSwapsFilesAndStashes: when the per-commit
// file list arrives, the detail screen swaps to it AND the original
// PR file list is stashed for restore. viewingCommit gets set so
// effectiveSourceSha/effectiveTargetSha route through the commit.
func TestCommitChangesLoadedSwapsFilesAndStashes(t *testing.T) {
	m := newDetailModel(t)
	originalFiles := m.detail.files
	if len(originalFiles) < 2 {
		t.Fatalf("test setup expected ≥2 files, got %d", len(originalFiles))
	}
	c := ado.Commit{ID: "abc123", ParentID: "ppp999", Subject: "x"}
	mm, _ := m.Update(commitChangesLoadedMsg{
		commit: c,
		files:  []ado.FileChange{{Path: "/just-one.go", ChangeType: "edit"}},
	})
	m = mm.(Model)
	if m.viewingCommit == nil || m.viewingCommit.ID != "abc123" {
		t.Fatalf("viewingCommit not set: %+v", m.viewingCommit)
	}
	if len(m.detail.files) != 1 || m.detail.files[0].Path != "/just-one.go" {
		t.Fatalf("file list not swapped: %+v", m.detail.files)
	}
	if len(m.prFiles) != len(originalFiles) {
		t.Fatalf("prFiles stash should hold the original list; got %d", len(m.prFiles))
	}
	if m.effectiveSourceSha() != "abc123" {
		t.Fatalf("effectiveSourceSha=%q, want commit SHA abc123", m.effectiveSourceSha())
	}
	if m.effectiveTargetSha() != "ppp999" {
		t.Fatalf("effectiveTargetSha=%q, want parent SHA ppp999", m.effectiveTargetSha())
	}
}

// TestSecondMReturnsToPRView: pressing M again while viewingCommit
// is set restores the PR file list and clears the commit pointer.
// Effective SHAs revert to the PR's source/target.
func TestSecondMReturnsToPRView(t *testing.T) {
	m := newDetailModel(t)
	originalFiles := m.detail.files
	mm, _ := m.Update(commitChangesLoadedMsg{
		commit: ado.Commit{ID: "abc", ParentID: "ppp"},
		files:  []ado.FileChange{{Path: "/just-one.go", ChangeType: "edit"}},
	})
	m = mm.(Model)
	if m.viewingCommit == nil {
		t.Fatalf("setup: viewingCommit should be set after commit changes load")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	m = mm.(Model)
	if m.viewingCommit != nil {
		t.Fatalf("second M should exit commit view")
	}
	if len(m.detail.files) != len(originalFiles) {
		t.Fatalf("file list should be restored to PR-wide list; got %d", len(m.detail.files))
	}
	if m.effectiveSourceSha() != "src" {
		t.Fatalf("effectiveSourceSha after exit=%q, want PR sourceSha 'src'", m.effectiveSourceSha())
	}
}

// TestEffectiveSHAFallbackForRootCommit: a commit with no parent
// (root, or first commit when ADO omitted parents) falls back to
// the PR's target SHA so the diff still renders against something
// meaningful instead of failing.
func TestEffectiveSHAFallbackForRootCommit(t *testing.T) {
	m := newDetailModel(t)
	m.viewingCommit = &ado.Commit{ID: "abc", ParentID: ""}
	if got := m.effectiveTargetSha(); got != "tgt" {
		t.Fatalf("effectiveTargetSha for parentless commit=%q, want PR target 'tgt'", got)
	}
}

// TestCommitViewHidesThreads: while viewingCommit is set,
// refreshPreview must skip both inline-anchored splices and the
// footer comments block. PR threads anchor to iteration line numbers
// that don't align with arbitrary per-commit diffs, so showing them
// would point at the wrong lines.
func TestCommitViewHidesThreads(t *testing.T) {
	m := newDetailModel(t)
	m.preview = m.preview.SetSize(60, 20)
	m.threads = []ado.Thread{{
		ID: 99, FilePath: "/a.go", Status: "active", RightLine: 1,
		Comments: []ado.Comment{{ID: 1, Author: "alice", Content: "should not appear in commit view"}},
	}}
	body := []byte("@@ -1 +1 @@\n+line\n")
	key := diffSelectionKey("abc", "ppp", "/a.go", 0)
	m.previewKey = key
	m.previewCache.Set(1, key, body)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   body,
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})
	m.viewingCommit = &ado.Commit{ID: "abc", ParentID: "ppp"}
	m = m.refreshPreview()
	view := stripANSI(m.preview.View())
	if strings.Contains(view, "should not appear in commit view") {
		t.Fatalf("threads must be hidden in commit view:\n%s", view)
	}
}

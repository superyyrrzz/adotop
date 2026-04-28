package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestFileListShowsCommentBadge confirms that files with anchored
// threads get a "💬 N" badge in the rendered file list, and files
// without comments don't. Regression guard for Task 2 of the
// surface-comments plan.
func TestFileListShowsCommentBadge(t *testing.T) {
	files := []ado.FileChange{
		{Path: "/a.go", ChangeType: "edit"},
		{Path: "/b.go", ChangeType: "edit"},
	}
	threads := []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "x", Content: "c1"}}},
		{ID: 2, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "y", Content: "c2"}}},
		{ID: 3, FilePath: "", Status: "active", Comments: []ado.Comment{{Author: "z", Content: "pr-level"}}},
	}

	m := NewDetail(KeyMap{})
	m = m.SetSummary(ado.PRSummary{ID: 1, Title: "x"})
	m, _ = m.Update(filesLoadedMsg{files: files})
	m = m.SetPRThreads(threads, false)
	m = m.SetPaneSize(60, 40)

	out := m.renderFilesBlock(true, 0)
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "💬 2") {
		t.Fatalf("expected a.go row with '💬 2' badge, got:\n%s", out)
	}
	// b.go has no comments — must not have a badge.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "b.go") && strings.Contains(line, "💬") {
			t.Fatalf("b.go has no comments but got badge in line: %q", line)
		}
	}
}

// TestSetPRThreadsCountsPerFileRespectResolvedFilter ensures the
// per-file counts honour the includeResolved flag the same way
// prThreads does.
func TestSetPRThreadsCountsPerFileRespectResolvedFilter(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active"},
		{ID: 2, FilePath: "/a.go", Status: "fixed"},
		{ID: 3, FilePath: "/b.go", Status: "fixed"},
	}
	d := DetailModel{}
	d = d.SetPRThreads(threads, false)
	if got := d.fileThreadCounts["/a.go"]; got != 1 {
		t.Fatalf("includeResolved=false: want 1 open thread for /a.go, got %d", got)
	}
	if got := d.fileThreadCounts["/b.go"]; got != 0 {
		t.Fatalf("includeResolved=false: want 0 for /b.go (only resolved), got %d", got)
	}
	d = d.SetPRThreads(threads, true)
	if got := d.fileThreadCounts["/a.go"]; got != 2 {
		t.Fatalf("includeResolved=true: want 2 for /a.go, got %d", got)
	}
	if got := d.fileThreadCounts["/b.go"]; got != 1 {
		t.Fatalf("includeResolved=true: want 1 for /b.go, got %d", got)
	}
}

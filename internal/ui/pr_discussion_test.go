package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestSetPRThreadsFiltersFileAnchored confirms SetPRThreads keeps only
// the threads with empty FilePath, regardless of resolved state when
// includeResolved is true.
func TestSetPRThreadsFiltersFileAnchored(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "", Status: "active", Comments: []ado.Comment{{Author: "alice", Content: "PR-level comment"}}},
		{ID: 2, FilePath: "/foo.go", Status: "active", Comments: []ado.Comment{{Author: "bob", Content: "anchored"}}},
		{ID: 3, FilePath: "", Status: "fixed", Comments: []ado.Comment{{Author: "carol", Content: "resolved PR-level"}}},
		{ID: 4, FilePath: "/bar.go", Status: "fixed", Comments: []ado.Comment{{Author: "dave", Content: "resolved anchored"}}},
	}
	d := DetailModel{}
	d = d.SetPRThreads(threads, false)
	if got := len(d.prThreads); got != 1 {
		t.Fatalf("includeResolved=false: want 1 PR-level open thread, got %d (%+v)", got, d.prThreads)
	}
	if d.prThreads[0].ID != 1 {
		t.Fatalf("includeResolved=false: want thread ID 1, got %+v", d.prThreads[0])
	}
	d = d.SetPRThreads(threads, true)
	if got := len(d.prThreads); got != 2 {
		t.Fatalf("includeResolved=true: want 2 PR-level threads, got %d (%+v)", got, d.prThreads)
	}
}

// TestRenderPRDiscussionSurfacesContent ensures the renderer actually
// emits the comment author and a snippet of the body. This is the
// regression guard for the bug we just fixed: comments existed but
// were never displayed because nothing rendered PR-level threads.
func TestRenderPRDiscussionSurfacesContent(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "", Status: "active", Comments: []ado.Comment{
			{Author: "MerlinBot", Content: "please consider adding a test"},
		}},
		{ID: 2, FilePath: "", Status: "active", Comments: []ado.Comment{
			{Author: "Alice", Content: "lgtm"},
		}},
	}
	out := renderPRDiscussion(threads)
	for _, want := range []string{"Discussion", "(2)", "MerlinBot", "please consider", "Alice", "lgtm"} {
		if !strings.Contains(out, want) {
			t.Fatalf("renderPRDiscussion missing %q in %q", want, out)
		}
	}
}

// TestRenderPRDiscussionEmpty: empty input yields empty string so the
// caller can skip the section without checking length.
func TestRenderPRDiscussionEmpty(t *testing.T) {
	if got := renderPRDiscussion(nil); got != "" {
		t.Fatalf("nil threads should yield empty string, got %q", got)
	}
}

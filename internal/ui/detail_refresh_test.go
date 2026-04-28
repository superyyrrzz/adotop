package ui

import (
	"strings"
	"testing"

	"github.com/renzeyu/adotop/internal/ado"
)

// TestDetailLoadedRefreshesReviewerVotes is the regression guard for
// the "I see no approvals after r" bug. The list-side summary may be
// stale (someone approved between the cache write and now). When the
// fresh PRDetail arrives, the detail header — which reads m.summary,
// not m.detail — must show the new vote count, not the old one.
func TestDetailLoadedRefreshesReviewerVotes(t *testing.T) {
	staleSummary := ado.PRSummary{
		ID: 1, Title: "x", Status: "active",
		Reviewers: []ado.ReviewerVote{
			{ID: "u1", DisplayName: "Alice", Vote: 0},
			{ID: "u2", DisplayName: "Bob", Vote: 0},
		},
	}
	freshDetail := &ado.PRDetail{
		PRSummary: ado.PRSummary{
			ID: 1, Title: "x", Status: "active",
			Reviewers: []ado.ReviewerVote{
				{ID: "u1", DisplayName: "Alice", Vote: 10},
				{ID: "u2", DisplayName: "Bob", Vote: 0},
			},
		},
	}

	m := NewDetail(KeyMap{})
	m = m.SetSummary(staleSummary)
	m, _ = m.Update(detailLoadedMsg{detail: freshDetail})

	// The summary stored on the detail model must reflect Alice's
	// approval. Header rendering reads from this, so failing this
	// invariant means the user can't see fresh approvals.
	got := m.Summary().Reviewers[0]
	if got.Vote != 10 {
		t.Fatalf("Alice's vote not refreshed: got %d, want 10", got.Vote)
	}

	// And the rendered header should actually surface it.
	header := m.renderHeader(true)
	if !strings.Contains(header, "Alice") {
		t.Fatalf("header missing Alice; got:\n%s", header)
	}
	// Approve glyph "✓" must appear because Alice's vote is 10.
	if !strings.Contains(header, "✓") {
		t.Fatalf("header missing approval glyph; got:\n%s", header)
	}
}

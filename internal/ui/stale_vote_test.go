package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestComputeMyStaleApprovalLatestPushAfterApprove: the canonical
// stale case — user approved at iteration 1, author pushed iteration
// 2, vote should be flagged stale.
func TestComputeMyStaleApprovalLatestPushAfterApprove(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	its := []ado.Iteration{
		{ID: 1, CreatedDate: t0},
		{ID: 2, CreatedDate: t0.Add(2 * time.Hour)},
	}
	events := []ado.VoteEvent{
		{ReviewerID: "renze", Vote: 10, PublishedAt: t0.Add(30 * time.Minute)},
	}
	if !computeMyStaleApproval(its, events, "renze") {
		t.Fatalf("expected stale=true (approved at it1, latest is it2)")
	}
}

// TestComputeMyStaleApprovalCurrentIterationApprove: user approved
// after the latest push — not stale, even with multiple iterations.
func TestComputeMyStaleApprovalCurrentIterationApprove(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	its := []ado.Iteration{
		{ID: 1, CreatedDate: t0},
		{ID: 2, CreatedDate: t0.Add(2 * time.Hour)},
	}
	events := []ado.VoteEvent{
		// Old vote on it1 — superseded.
		{ReviewerID: "renze", Vote: 10, PublishedAt: t0.Add(30 * time.Minute)},
		// Fresh vote on it2 — what counts.
		{ReviewerID: "renze", Vote: 10, PublishedAt: t0.Add(3 * time.Hour)},
	}
	if computeMyStaleApproval(its, events, "renze") {
		t.Fatalf("expected stale=false (latest vote was on the latest iteration)")
	}
}

// TestComputeMyStaleApprovalNonApproveVotesIgnored: a "waiting for
// author" or "rejected" vote is never stale — re-approve doesn't
// fix those, they're deliberate non-approvals.
func TestComputeMyStaleApprovalNonApproveVotesIgnored(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	its := []ado.Iteration{
		{ID: 1, CreatedDate: t0},
		{ID: 2, CreatedDate: t0.Add(2 * time.Hour)},
	}
	for _, vote := range []int{-10, -5, 0} {
		events := []ado.VoteEvent{
			{ReviewerID: "renze", Vote: vote, PublishedAt: t0.Add(30 * time.Minute)},
		}
		if computeMyStaleApproval(its, events, "renze") {
			t.Fatalf("vote=%d should never be flagged stale", vote)
		}
	}
}

// TestComputeMyStaleApprovalOnlyMyVotes: votes from other reviewers
// don't trigger MY stale flag. Catches a regression where the loop
// would short-circuit on the first vote it saw, regardless of who
// cast it.
func TestComputeMyStaleApprovalOnlyMyVotes(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	its := []ado.Iteration{
		{ID: 1, CreatedDate: t0},
		{ID: 2, CreatedDate: t0.Add(2 * time.Hour)},
	}
	events := []ado.VoteEvent{
		// Someone else approved at it1 — irrelevant to me.
		{ReviewerID: "alice", Vote: 10, PublishedAt: t0.Add(30 * time.Minute)},
	}
	if computeMyStaleApproval(its, events, "renze") {
		t.Fatalf("expected stale=false; only alice voted, not me")
	}
}

// TestComputeMyStaleApprovalEmptyDataReturnsFalse: missing
// iterations or events must NOT produce a false positive. The
// staleness check is conservative — better silent than wrong.
func TestComputeMyStaleApprovalEmptyDataReturnsFalse(t *testing.T) {
	if computeMyStaleApproval(nil, nil, "renze") {
		t.Fatalf("expected stale=false on empty data")
	}
	if computeMyStaleApproval([]ado.Iteration{{ID: 1, CreatedDate: time.Now()}}, nil, "renze") {
		t.Fatalf("expected stale=false when no events")
	}
	if computeMyStaleApproval(nil, []ado.VoteEvent{{ReviewerID: "renze", Vote: 10}}, "renze") {
		t.Fatalf("expected stale=false when no iterations")
	}
	if computeMyStaleApproval(
		[]ado.Iteration{{ID: 1, CreatedDate: time.Now()}},
		[]ado.VoteEvent{{ReviewerID: "renze", Vote: 10, PublishedAt: time.Now()}},
		"",
	) {
		t.Fatalf("expected stale=false when myID is empty")
	}
}

// TestReviewerPanelRendersStaleAnnotation: when the staleApproval
// flag is set, the rendered panel must include the warning text on
// the My Vote line. Without this the user has no way to learn they
// need to re-approve.
func TestReviewerPanelRendersStaleAnnotation(t *testing.T) {
	s := ado.PRSummary{ID: 1, Title: "x", MyVote: 10}
	out := stripANSI(reviewerPanel(s, "", true))
	if !strings.Contains(out, "stale, re-approve needed") {
		t.Fatalf("expected stale annotation; got:\n%s", out)
	}
	out = stripANSI(reviewerPanel(s, "", false))
	if strings.Contains(out, "stale") {
		t.Fatalf("did NOT expect stale annotation when flag is false:\n%s", out)
	}
}

package ado

import (
	"testing"
	"time"
)

// helper: build a fake VoteUpdate thread the way ADO returns them.
func voteThread(reviewerID, voteStr string, publishedAt time.Time) rawThread {
	return rawThread{
		Properties: map[string]any{
			"CodeReviewThreadType": map[string]any{
				"$type": "System.String", "$value": "VoteUpdate",
			},
			"CodeReviewVoteResult": map[string]any{
				"$type": "System.String", "$value": voteStr,
			},
			"CodeReviewVotedByIdentity": map[string]any{
				"$type": "System.String", "$value": "1",
			},
		},
		Identities: map[string]struct {
			ID string `json:"id"`
		}{
			"1": {ID: reviewerID},
		},
		PublishedDate: publishedAt.Format(time.RFC3339),
	}
}

// TestExtractVoteEventsFiltersByThreadType: only VoteUpdate threads
// produce events. Other system thread types (PolicyStatusUpdate,
// RefUpdate, StatusUpdate) and regular human comments must be
// ignored — otherwise we'd report stale-vote on every thread.
func TestExtractVoteEventsFiltersByThreadType(t *testing.T) {
	now := time.Now()
	threads := []rawThread{
		voteThread("alice", "10", now),
		{Properties: map[string]any{
			"CodeReviewThreadType": map[string]any{"$type": "System.String", "$value": "PolicyStatusUpdate"},
		}},
		{Properties: map[string]any{
			"CodeReviewThreadType": map[string]any{"$type": "System.String", "$value": "RefUpdate"},
		}},
		// No properties at all — a normal human comment.
		{},
	}
	got := extractVoteEvents(threads)
	if len(got) != 1 {
		t.Fatalf("expected 1 vote event, got %d (%+v)", len(got), got)
	}
	if got[0].ReviewerID != "alice" || got[0].Vote != 10 {
		t.Fatalf("unexpected event: %+v", got[0])
	}
}

// TestLatestVoteByReviewerKeepsLatest: when a reviewer votes
// multiple times (approve → rescind → re-approve), the latest event
// wins. Without that, "your vote" would be ambiguous.
func TestLatestVoteByReviewerKeepsLatest(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)
	events := []VoteEvent{
		{ReviewerID: "renze", Vote: 10, PublishedAt: t0},
		{ReviewerID: "renze", Vote: 0, PublishedAt: t1},  // rescinded
		{ReviewerID: "renze", Vote: 10, PublishedAt: t2}, // re-approved
		{ReviewerID: "alice", Vote: 5, PublishedAt: t1},
	}
	got := LatestVoteByReviewer(events)
	if got["renze"].Vote != 10 || !got["renze"].PublishedAt.Equal(t2) {
		t.Fatalf("latest vote for renze=%+v, want vote=10 at %v", got["renze"], t2)
	}
	if got["alice"].Vote != 5 {
		t.Fatalf("latest vote for alice=%+v, want vote=5", got["alice"])
	}
}

// TestIterationOfBucketsByTimestamp: bucket a timestamp into the
// latest iteration whose CreatedDate <= ts. Edge cases: timestamp
// equal to an iteration's CreatedDate counts as that iteration
// (vote cast at the moment of the push); timestamp before all
// iterations returns 0 (impossible in practice).
func TestIterationOfBucketsByTimestamp(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)
	its := []Iteration{
		{ID: 1, CreatedDate: t0},
		{ID: 2, CreatedDate: t1},
		{ID: 3, CreatedDate: t2},
	}
	cases := []struct {
		name string
		ts   time.Time
		want int
	}{
		{"equal to it1", t0, 1},
		{"between it1 and it2", t0.Add(30 * time.Minute), 1},
		{"equal to it2", t1, 2},
		{"between it2 and it3", t1.Add(30 * time.Minute), 2},
		{"after it3", t2.Add(1 * time.Minute), 3},
		{"before all", t0.Add(-1 * time.Minute), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IterationOf(its, c.ts); got != c.want {
				t.Fatalf("IterationOf=%d, want %d", got, c.want)
			}
		})
	}
}

package ui

import (
	"github.com/superyyrrzz/adotop/internal/ado"
)

// computeMyStaleApproval returns true when the user's most recent
// vote was an approval (≥ 5) cast on an iteration earlier than the
// latest. This is the canonical "you need to re-approve" signal
// derived from VoteUpdate system threads + iteration timestamps —
// no local cache required.
//
// The check is conservative: any missing data (no iterations, no
// vote events for the user, vote is not an approval) returns false.
// We'd rather skip the badge than wrongly tell the user their
// approval is stale.
func computeMyStaleApproval(iterations []ado.Iteration, events []ado.VoteEvent, myID string) bool {
	if myID == "" || len(iterations) == 0 || len(events) == 0 {
		return false
	}
	// Find the latest vote event for me.
	var mine ado.VoteEvent
	have := false
	for _, e := range events {
		if e.ReviewerID != myID {
			continue
		}
		if !have || e.PublishedAt.After(mine.PublishedAt) {
			mine = e
			have = true
		}
	}
	if !have {
		return false
	}
	// Only approvals can be "stale" in the sense the user cares
	// about. Waiting/rejected votes wouldn't gate merge in a way
	// that "re-approve" fixes.
	if mine.Vote < 5 {
		return false
	}
	votedAt := ado.IterationOf(iterations, mine.PublishedAt)
	if votedAt == 0 {
		return false
	}
	latest := iterations[len(iterations)-1].ID
	return votedAt < latest
}
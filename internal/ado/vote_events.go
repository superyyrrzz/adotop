package ado

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Iteration is one push to the PR's source branch. ID is monotonic
// from 1; CreatedDate is when ADO ingested the push. The detail
// screen uses these to bucket vote-event timestamps into iterations
// (vote published at T → iteration N where iterations[N].createdDate
// is the latest with createdDate <= T).
type Iteration struct {
	ID          int
	CreatedDate time.Time
}

// VoteEvent is the parsed shape of a "VoteUpdate" system thread —
// ADO writes one of these every time a reviewer's vote changes.
// IteratorIndex isn't on the event itself; callers compute it by
// bucketing PublishedAt against the iteration list.
//
// The earlier probe confirmed these threads exist on this server
// even though the documented IdentityRefWithVote shape doesn't carry
// any iteration-aware vote info. Mining VoteUpdate threads is the
// only authoritative path to "your approval is stale."
type VoteEvent struct {
	ReviewerID   string    // identity ID of the voter
	Vote         int       // 10/5/0/-5/-10 (same scale as ReviewerVote.Vote)
	PublishedAt  time.Time // when ADO recorded the vote
}

type rawIterationFull struct {
	ID          int    `json:"id"`
	CreatedDate string `json:"createdDate"`
}

type rawIterationsRespFull struct {
	Value []rawIterationFull `json:"value"`
}

// GetPullRequestIterations returns iterations in chronological order
// (oldest first; ADO already returns them this way). The ID field
// matches what `iterations` returns elsewhere, but here we also pull
// CreatedDate so callers can answer "which iteration was this
// timestamp in." Errors returned verbatim — caller decides if a
// missing iteration list disables the staleness check.
func (c *Client) GetPullRequestIterations(ctx context.Context, repoID string, prID int) ([]Iteration, error) {
	if repoID == "" || prID == 0 {
		return nil, fmt.Errorf("GetPullRequestIterations: repoID and prID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/iterations",
		url.PathEscape(repoID), prID)
	var raw rawIterationsRespFull
	if err := c.GetJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]Iteration, 0, len(raw.Value))
	for _, ri := range raw.Value {
		t, _ := time.Parse(time.RFC3339, ri.CreatedDate)
		out = append(out, Iteration{ID: ri.ID, CreatedDate: t})
	}
	return out, nil
}

// GetPullRequestVoteEvents fetches the threads endpoint and extracts
// just the VoteUpdate system events. Returned events are flattened
// per reviewer change (one per vote, including vote-zero "rescinded"
// events) in publication order — caller decides whether to keep all
// or just the latest per reviewer.
//
// The thread-properties shape we mine:
//
//	properties.CodeReviewThreadType   == "VoteUpdate"
//	properties.CodeReviewVoteResult   == "10"           (vote as string)
//	properties.CodeReviewVotedByIdentity == "1"          (key into identities)
//	identities["1"].id                == "<reviewerID>"
//
// Plus the thread's PublishedDate (top-level) carries the timestamp.
func (c *Client) GetPullRequestVoteEvents(ctx context.Context, repoID string, prID int) ([]VoteEvent, error) {
	if repoID == "" || prID == 0 {
		return nil, fmt.Errorf("GetPullRequestVoteEvents: repoID and prID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/threads",
		url.PathEscape(repoID), prID)
	var resp rawThreadsResp
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	return extractVoteEvents(resp.Value), nil
}

func extractVoteEvents(threads []rawThread) []VoteEvent {
	out := make([]VoteEvent, 0, 4)
	for _, t := range threads {
		if t.IsDeleted {
			continue
		}
		// CodeReviewThreadType is wrapped in {"$type":"...","$value":"..."}
		// — the $value carries the actual string.
		if voteEventThreadType(t) != "VoteUpdate" {
			continue
		}
		voteStr := propString(t.Properties, "CodeReviewVoteResult")
		idKey := propString(t.Properties, "CodeReviewVotedByIdentity")
		if voteStr == "" || idKey == "" {
			continue
		}
		vote, err := strconv.Atoi(voteStr)
		if err != nil {
			continue
		}
		ident, ok := t.Identities[idKey]
		if !ok || ident.ID == "" {
			continue
		}
		published, _ := time.Parse(time.RFC3339, t.PublishedDate)
		out = append(out, VoteEvent{
			ReviewerID:  ident.ID,
			Vote:        vote,
			PublishedAt: published,
		})
	}
	return out
}

// voteEventThreadType pulls the $value out of a properties entry
// shaped like ADO's typed-property records. Returns "" when the
// key is missing or the value isn't a string.
func voteEventThreadType(t rawThread) string {
	return propString(t.Properties, "CodeReviewThreadType")
}

// propString reads a string $value from a typed-property entry.
// ADO wraps every property as {"$type": "System.String", "$value": "..."}
// (or System.Int32, etc.); we only need the string form for vote
// thread parsing, so this helper unwraps that single shape.
func propString(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	raw, ok := props[key]
	if !ok {
		return ""
	}
	wrapped, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	v, _ := wrapped["$value"].(string)
	return v
}

// LatestVoteByReviewer returns the most recent vote per reviewer ID.
// "Most recent" is by PublishedAt; ties broken by event order
// (later in slice wins, since ADO returns chronological).
//
// Vote-zero events (e.g., "vote rescinded") are kept — they mean
// the reviewer no longer has a vote, and treating them as the
// authoritative latest matches what ADO web shows.
func LatestVoteByReviewer(events []VoteEvent) map[string]VoteEvent {
	out := map[string]VoteEvent{}
	for _, e := range events {
		prev, ok := out[e.ReviewerID]
		if !ok || e.PublishedAt.After(prev.PublishedAt) {
			out[e.ReviewerID] = e
		}
	}
	return out
}

// IterationOf returns the iteration ID covering the given timestamp:
// the latest iteration whose CreatedDate <= ts. Returns 0 when ts
// predates all iterations (shouldn't happen in practice; vote can't
// land before iteration 1) so the caller can ignore the event.
func IterationOf(its []Iteration, ts time.Time) int {
	id := 0
	for _, it := range its {
		if it.CreatedDate.IsZero() {
			continue
		}
		if !it.CreatedDate.After(ts) {
			id = it.ID
		}
	}
	return id
}

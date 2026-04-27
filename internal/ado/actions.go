package ado

import (
	"context"
	"fmt"
	"net/url"
)

// SetReviewerVote sets the caller's vote on a PR.
//
// Vote values used by Azure DevOps:
//
//	 10 approved
//	  5 approved with suggestions
//	  0 reset / no vote
//	 -5 waiting for author
//	-10 rejected
//
// `myID` must be the descriptor returned by ConnectionData.AuthenticatedUser.ID.
// On success, returns no value — call GetPullRequest to refresh PR state.
func (c *Client) SetReviewerVote(ctx context.Context, repoID string, prID int, myID string, vote int) error {
	if repoID == "" || prID == 0 || myID == "" {
		return fmt.Errorf("SetReviewerVote: repoID, prID, myID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/reviewers/%s",
		url.PathEscape(repoID), prID, url.PathEscape(myID))
	body := map[string]any{"vote": vote, "id": myID}
	return c.PutJSON(ctx, path, body, nil)
}

// AbandonPullRequest sets the PR status to "abandoned". Idempotent — abandoning
// an already-abandoned PR returns 200 with the existing state.
func (c *Client) AbandonPullRequest(ctx context.Context, repoID string, prID int) error {
	if repoID == "" || prID == 0 {
		return fmt.Errorf("AbandonPullRequest: repoID, prID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d",
		url.PathEscape(repoID), prID)
	body := map[string]any{"status": "abandoned"}
	return c.PatchJSON(ctx, path, body, nil)
}

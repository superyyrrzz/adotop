package ado

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Commit summarizes one commit on a PR's source branch — enough for
// the picker UI to show short SHA, author, date, and the first line
// of the message. Full bodies aren't needed (review is per-file diff,
// not per-commit prose) so we don't ship the rest of the message.
type Commit struct {
	ID         string    // full SHA
	ParentID   string    // first parent SHA — diff base for this commit (empty for root)
	Author     string    // display name (committer falls back to author when missing)
	Subject    string    // first line of commit message
	CommitDate time.Time // when the commit was created on the source branch
}

// ShortID returns the conventional 7-char prefix used in UI lists.
// Defensive against unexpectedly short IDs (test fixtures, partial
// data) — never panics, just returns whatever we have.
func (c Commit) ShortID() string {
	if len(c.ID) <= 7 {
		return c.ID
	}
	return c.ID[:7]
}

type rawCommitAuthor struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

type rawCommit struct {
	CommitID  string          `json:"commitId"`
	Author    rawCommitAuthor `json:"author"`
	Committer rawCommitAuthor `json:"committer"`
	Comment   string          `json:"comment"`
	Parents   []string        `json:"parents"`
}

type rawCommitsResp struct {
	Value []rawCommit `json:"value"`
}

// GetPullRequestCommits returns the commits that make up the PR's
// source branch in chronological order (oldest first). The "Commit
// Picker" modal walks this list — newest commits at the bottom mirror
// how git log reads.
//
// The ADO endpoint orders commits newest-first; we reverse so the UI
// can render top-to-bottom-as-history without surprising the user.
//
// ADO's /pullRequests/{id}/commits returns simplified GitCommitRef
// records that often omit the `parents` field. When that's the case
// we infer each commit's parent from "the commit before it in the
// chronological list". That's correct for the common linear-PR case;
// for PRs with merge commits the inferred parent diverges from the
// actual git parent, but the diff still renders against a meaningful
// base (the previous PR commit) — degrading to "show everything new
// since the last PR commit" rather than failing.
func (c *Client) GetPullRequestCommits(ctx context.Context, repoID string, prID int) ([]Commit, error) {
	if repoID == "" || prID == 0 {
		return nil, fmt.Errorf("GetPullRequestCommits: repoID and prID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/commits",
		url.PathEscape(repoID), prID)
	var raw rawCommitsResp
	if err := c.GetJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw.Value))
	for i := len(raw.Value) - 1; i >= 0; i-- {
		rc := raw.Value[i]
		author := rc.Author.Name
		if author == "" {
			author = rc.Committer.Name
		}
		date, _ := time.Parse(time.RFC3339, rc.Author.Date)
		if date.IsZero() {
			date, _ = time.Parse(time.RFC3339, rc.Committer.Date)
		}
		var parent string
		if len(rc.Parents) > 0 {
			parent = rc.Parents[0]
		}
		out = append(out, Commit{
			ID:         rc.CommitID,
			ParentID:   parent,
			Author:     author,
			Subject:    firstLine(rc.Comment),
			CommitDate: date,
		})
	}
	// Backfill ParentID from chronological order when the API didn't
	// supply it. The first commit's parent stays empty — its diff base
	// is the PR's target branch tip, handled at the caller.
	for i := 1; i < len(out); i++ {
		if out[i].ParentID == "" {
			out[i].ParentID = out[i-1].ID
		}
	}
	return out, nil
}

// GetCommitChanges returns the files touched by a single commit. The
// per-commit view in the detail screen replaces the PR's full file
// list with this slice when the user picks a commit from the picker.
//
// The endpoint includes the commit's first parent for context lines,
// so the diff renderer can use commit^ as the "from" sha and commit
// as the "to" sha — same getFileAtCommit pair we use for PR diffs.
func (c *Client) GetCommitChanges(ctx context.Context, repoID, commitID string) ([]FileChange, error) {
	if repoID == "" || commitID == "" {
		return nil, fmt.Errorf("GetCommitChanges: repoID and commitID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/commits/%s/changes",
		url.PathEscape(repoID), url.PathEscape(commitID))
	var raw rawChangesResp
	if err := c.GetJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]FileChange, 0, len(raw.ChangeEntries))
	for _, e := range raw.ChangeEntries {
		path := e.Item.Path
		if path == "" {
			path = e.SourceServerItem
		}
		if path == "" {
			path = e.OriginalPath
		}
		out = append(out, FileChange{Path: path, ChangeType: e.ChangeType})
	}
	return out, nil
}

// firstLine returns the substring before the first newline, with
// surrounding whitespace stripped. Used to extract a commit subject
// from the full message body.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

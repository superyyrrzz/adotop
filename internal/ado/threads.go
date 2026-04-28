package ado

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Comment is a single message in an ADO PR thread.
type Comment struct {
	Author        string
	Content       string
	PublishedDate time.Time
	// Type is the ADO commentType: "text" (human comment),
	// "codeChange" (auto note about updates), "system" (automated).
	Type string
}

// Thread is a discussion attached to a PR — either to a specific file/line
// or to the PR as a whole when FilePath == "".
type Thread struct {
	ID        int
	Status    string // "active", "fixed", "wontFix", "closed", "byDesign", "pending", "unknown"
	FilePath  string
	RightLine int // 1-based right-side line; 0 when the thread isn't anchored to a line
	LeftLine  int
	Comments  []Comment
}

// IsResolved reports whether the thread is in a terminal "no further action"
// state. Active/pending/unknown threads are considered open.
func (t Thread) IsResolved() bool {
	switch strings.ToLower(t.Status) {
	case "fixed", "wontfix", "closed", "bydesign":
		return true
	}
	return false
}

type rawCommentPos struct {
	Line   int `json:"line"`
	Offset int `json:"offset"`
}

type rawThreadCtx struct {
	FilePath       string        `json:"filePath"`
	RightFileStart rawCommentPos `json:"rightFileStart"`
	LeftFileStart  rawCommentPos `json:"leftFileStart"`
}

type rawComment struct {
	Author struct {
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Content       string `json:"content"`
	PublishedDate string `json:"publishedDate"`
	CommentType   string `json:"commentType"`
}

type rawThread struct {
	ID            int           `json:"id"`
	Status        string        `json:"status"`
	IsDeleted     bool          `json:"isDeleted"`
	ThreadContext *rawThreadCtx `json:"threadContext"`
	Comments      []rawComment  `json:"comments"`
}

type rawThreadsResp struct {
	Value []rawThread `json:"value"`
}

// GetPullRequestThreads returns all comment threads on the PR, including
// resolved ones. Filtering / sorting is the caller's job. Deleted threads
// and threads with all comments deleted are dropped.
func (c *Client) GetPullRequestThreads(ctx context.Context, repoID string, prID int) ([]Thread, error) {
	if repoID == "" || prID == 0 {
		return nil, fmt.Errorf("GetPullRequestThreads: repoID and prID required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/threads",
		url.PathEscape(repoID), prID)
	var resp rawThreadsResp
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	out := make([]Thread, 0, len(resp.Value))
	for _, r := range resp.Value {
		if r.IsDeleted {
			continue
		}
		t := Thread{ID: r.ID, Status: r.Status}
		if r.ThreadContext != nil {
			t.FilePath = r.ThreadContext.FilePath
			t.RightLine = r.ThreadContext.RightFileStart.Line
			t.LeftLine = r.ThreadContext.LeftFileStart.Line
		}
		for _, rc := range r.Comments {
			// Drop only auto "codeChange" notes (e.g. "force-pushed an
			// update") — those are truly noise. Keep "system" comments:
			// they carry real signal from bots/pipelines like GitOps,
			// Ownership Enforcer, AI reviewers, etc.
			if rc.CommentType == "codeChange" {
				continue
			}
			pub, _ := time.Parse(time.RFC3339, rc.PublishedDate)
			t.Comments = append(t.Comments, Comment{
				Author:        rc.Author.DisplayName,
				Content:       rc.Content,
				PublishedDate: pub,
				Type:          rc.CommentType,
			})
		}
		if len(t.Comments) == 0 {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

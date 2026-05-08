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
	ID            int
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
	ID     int `json:"id"`
	Author struct {
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Content       string `json:"content"`
	PublishedDate string `json:"publishedDate"`
	CommentType   string `json:"commentType"`
}

type rawThread struct {
	ID            int            `json:"id"`
	Status        string         `json:"status"`
	IsDeleted     bool           `json:"isDeleted"`
	ThreadContext *rawThreadCtx  `json:"threadContext"`
	Comments      []rawComment   `json:"comments"`
	PublishedDate string         `json:"publishedDate"`
	Properties    map[string]any `json:"properties"`
	Identities    map[string]struct {
		ID string `json:"id"`
	} `json:"identities"`
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
		t := rawThreadToThread(r)
		if len(t.Comments) == 0 {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// rawThreadToThread converts the raw ADO response into the public Thread
// shape. Shared by GetPullRequestThreads and the write methods so the
// commentType filter and field mapping stay in sync.
func rawThreadToThread(r rawThread) Thread {
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
			ID:            rc.ID,
			Author:        rc.Author.DisplayName,
			Content:       rc.Content,
			PublishedDate: pub,
			Type:          rc.CommentType,
		})
	}
	return t
}

// validThreadStatuses is the closed set of statuses ADO accepts on PATCH.
// "unknown" is a read-side fallback (server may emit it) and not writable.
var validThreadStatuses = map[string]bool{
	"active": true, "fixed": true, "wontFix": true,
	"closed": true, "byDesign": true, "pending": true,
}

// PatchThreadStatus changes a thread's status. The TUI uses only "active"
// and "fixed" via the toggle key today; the full enum is exposed for
// future per-status menus.
func (c *Client) PatchThreadStatus(ctx context.Context, repoID string, prID, threadID int, status string) error {
	if repoID == "" || prID == 0 || threadID == 0 {
		return fmt.Errorf("PatchThreadStatus: repoID, prID, threadID required")
	}
	if !validThreadStatuses[status] {
		return fmt.Errorf("PatchThreadStatus: unknown status %q", status)
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/threads/%d",
		url.PathEscape(repoID), prID, threadID)
	body := map[string]any{"status": status}
	return c.PatchJSON(ctx, path, body, nil)
}

// PostThreadComment appends a comment to an existing thread. ADO threads
// model their messages as a chain rooted at the first comment (id=1
// within that thread). We always reply at root to keep the linear UX
// shape we render — sub-threading is supported by the API but isn't
// useful here.
func (c *Client) PostThreadComment(ctx context.Context, repoID string, prID, threadID int, body string) (Comment, error) {
	if repoID == "" || prID == 0 || threadID == 0 || strings.TrimSpace(body) == "" {
		return Comment{}, fmt.Errorf("PostThreadComment: repoID, prID, threadID, body required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/threads/%d/comments",
		url.PathEscape(repoID), prID, threadID)
	payload := map[string]any{
		"parentCommentId": 1,
		"content":         body,
		"commentType":     1,
	}
	var rc rawComment
	if err := c.PostJSON(ctx, path, payload, &rc); err != nil {
		return Comment{}, err
	}
	pub, _ := time.Parse(time.RFC3339, rc.PublishedDate)
	return Comment{
		ID:            rc.ID,
		Author:        rc.Author.DisplayName,
		Content:       rc.Content,
		PublishedDate: pub,
		Type:          rc.CommentType,
	}, nil
}
// empty, the thread is PR-level (no file anchor); otherwise it's anchored
// to the file's right side. Line-level anchoring isn't supported here —
// the TUI has no diff line cursor to drive it.
//
// The first comment on the new thread is `body`; ADO requires at least
// one comment when creating a thread.
func (c *Client) PostPRThread(ctx context.Context, repoID string, prID int, body, filePath string) (Thread, error) {
	if repoID == "" || prID == 0 || strings.TrimSpace(body) == "" {
		return Thread{}, fmt.Errorf("PostPRThread: repoID, prID, body required")
	}
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/threads",
		url.PathEscape(repoID), prID)
	payload := map[string]any{
		"comments": []map[string]any{{
			"parentCommentId": 0,
			"content":         body,
			"commentType":     1, // 1 = text
		}},
		"status": 1, // 1 = active
	}
	if filePath != "" {
		payload["threadContext"] = map[string]any{
			"filePath": filePath,
		}
	}
	var raw rawThread
	if err := c.PostJSON(ctx, path, payload, &raw); err != nil {
		return Thread{}, err
	}
	return rawThreadToThread(raw), nil
}

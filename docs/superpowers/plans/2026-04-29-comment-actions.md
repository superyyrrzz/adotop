# PR Comment Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users post new threads, reply to threads, and resolve/reactivate threads from the detail screen — without leaving the TUI.

**Architecture:** Three new ADO write methods (`PostPRThread`, `PostThreadComment`, `PatchThreadStatus`) follow the existing vote/abandon template (client method → `tea.Cmd` goroutine → `actionDoneMsg` → `Update` handler → footer + targeted threads refresh). The detail screen gains a per-thread cursor in diff focus and three new keys (`c` compose new, `C` reply to current, `x` resolve/reactivate toggle). Body composition shells out to `$EDITOR` (Approach B from brainstorming) — zero in-app textarea code, full markdown editing. Scope: PR-level and file-level threads; line-anchored threads deferred to a follow-up.

**Tech Stack:** Go, Bubble Tea, lipgloss, Azure DevOps REST API 7.1, `os/exec` for `$EDITOR` shell-out.

---

## Pre-flight context (read before starting)

**Files you will touch most:**
- `internal/ado/threads.go` — currently read-only; add 3 write methods + `Comment.ID` field
- `internal/ado/threads_test.go` — existing tests cover GET; add write tests using `httptest`
- `internal/ui/app.go` — Model fields, `actionDoneMsg` kinds, key handlers in `updateDetailScreen`
- `internal/ui/threads.go` — render highlight for the selected thread, helpers for thread cursor
- `internal/ui/keys.go` — three new bindings: `ComposeThread`, `ReplyThread`, `ToggleResolve`
- `internal/ui/editor.go` — NEW. `$EDITOR` shell-out helper.
- `internal/ui/comment_actions.go` — NEW. The three `tea.Cmd` factories + per-thread cursor helpers.

**Pattern template — vote (read this first):**
- Client method: `internal/ado/actions.go:21` `SetReviewerVote`
- Cmd factory: `internal/ui/app.go:381` `setVoteCurrent`
- Result message: `internal/ui/app.go:184` `actionDoneMsg{kind, prID, err, notes, vote}`
- Handler: `internal/ui/app.go:608` `case actionDoneMsg:` — sets footer, refreshes detail+list

**Pattern template — abandon with confirmation:**
- Client method: `internal/ado/actions.go:33` `AbandonPullRequest`
- pendingAction prompt: `internal/ui/app.go:877`
- Confirm-yes routing: `internal/ui/app.go:642`

**Key model invariants:**
- `Thread.ID int` is the ADO thread ID; pass it to the PATCH endpoint.
- `Comment` has NO `ID` today (`internal/ado/threads.go:12`). We add one in Task 1 because reply requires `parentCommentId`.
- Threads pane and diff pane share **one viewport** via concatenation (`internal/ui/threads.go:82`). Re-render after any mutation by calling `m.refreshPreview()` — same idiom used after expand toggle.
- The currently-selected file: `m.detail.SelectedFile()`. There is no per-thread cursor today; we add one as `m.threadCursor map[string]int` keyed by file path so the cursor survives file switches.
- `ado.Client` exposes `GetJSON`, `PostJSON`, `PatchJSON`, `PutJSON`. POST exists already — verify in Task 0 below before assuming.

**Out of scope (do NOT add):**
- Line-anchored new threads (no diff line cursor exists). New threads created by `c` are PR-level by default; `C` to reply preserves the parent thread's anchor automatically.
- Editing or deleting existing comments. ADO supports it; users rarely want it from a TUI; add later if asked.
- Comment markdown preview before submit. `$EDITOR` is the preview surface.
- Status menu (`a`/`f`/`w`/`c`/`d`). `x` toggles between `active` and `fixed` — covers 90% of usage. Menu is a follow-up if requested.

---

## Task 0: Verify ado.Client.PostJSON exists

**Files:**
- Read: `internal/ado/client.go` (or wherever `Client` is defined)

- [ ] **Step 1: Locate the JSON helpers**

```bash
grep -n "func (c \*Client) \(Post\|Get\|Put\|Patch\)JSON" internal/ado/*.go
```

Expected: lines for `GetJSON`, `PutJSON`, `PatchJSON`. Look for `PostJSON`.

- [ ] **Step 2: If PostJSON is missing, add it**

If the grep above shows no `PostJSON`, add this method **next to** `PatchJSON` in the same file, mirroring its signature exactly:

```go
// PostJSON sends a POST with body and decodes the JSON response into out.
// out may be nil to discard the response body.
func (c *Client) PostJSON(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}
```

If `PatchJSON` doesn't delegate to a `do(...)` helper, mirror the structure of `PatchJSON` instead — read it first, copy its shape, change `http.MethodPatch` to `http.MethodPost`.

- [ ] **Step 3: Build to confirm**

```bash
go build ./...
```

Expected: success, no output.

- [ ] **Step 4: Commit only if you added PostJSON**

```bash
git add internal/ado/
git commit -m "feat(ado): add PostJSON helper for thread/comment writes"
```

If `PostJSON` already existed, skip the commit.

---

## Task 1: Add Comment.ID field

**Why:** Reply needs `parentCommentId`. Currently `Comment` has no ID. Backfill from `rawComment.ID`.

**Files:**
- Modify: `internal/ado/threads.go:12-19` (Comment struct), `internal/ado/threads.go:53-60` (rawComment), `internal/ado/threads.go:106-112` (mapping in GetPullRequestThreads)
- Test: `internal/ado/threads_test.go`

- [ ] **Step 1: Write the failing test**

Add this to `internal/ado/threads_test.go` (extend an existing test or add a new one mirroring the existing GET test pattern). If you need to create the file, mirror the structure of any existing `*_test.go` in `internal/ado/`:

```go
func TestGetPullRequestThreads_PopulatesCommentID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{
			"id": 7, "status": "active",
			"comments": [{"id": 101, "content": "first", "commentType": "text", "author": {"displayName": "Alice"}}]
		}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok") // adjust to actual constructor
	got, err := c.GetPullRequestThreads(context.Background(), "repo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Comments) != 1 {
		t.Fatalf("unexpected shape: %+v", got)
	}
	if got[0].Comments[0].ID != 101 {
		t.Fatalf("Comment.ID not populated: got %d", got[0].Comments[0].ID)
	}
}
```

If the existing test setup uses a different constructor or fixture style, copy that — the assertion on `got[0].Comments[0].ID == 101` is the contract.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/ado/ -run TestGetPullRequestThreads_PopulatesCommentID -v
```

Expected: FAIL with "Comment.ID not populated: got 0" (or compile error if `Comment.ID` doesn't exist yet — that's also a valid fail).

- [ ] **Step 3: Add the ID field to Comment**

In `internal/ado/threads.go`, change the `Comment` struct (currently at line 12-19) to:

```go
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
```

Add `ID int` to `rawComment` (currently lines 53-60):

```go
type rawComment struct {
	ID     int `json:"id"`
	Author struct {
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Content       string `json:"content"`
	PublishedDate string `json:"publishedDate"`
	CommentType   string `json:"commentType"`
}
```

Populate it in the mapping loop in `GetPullRequestThreads` (currently around line 107):

```go
t.Comments = append(t.Comments, Comment{
	ID:            rc.ID,
	Author:        rc.Author.DisplayName,
	Content:       rc.Content,
	PublishedDate: pub,
	Type:          rc.CommentType,
})
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/ado/ -run TestGetPullRequestThreads_PopulatesCommentID -v
```

Expected: PASS.

- [ ] **Step 5: Run all ado tests to confirm nothing else broke**

```bash
go test ./internal/ado/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ado/threads.go internal/ado/threads_test.go
git commit -m "feat(ado): expose Comment.ID for reply addressing"
```

---

## Task 2: Add PostPRThread (new PR-level thread)

**Files:**
- Modify: `internal/ado/threads.go` (append new method)
- Test: `internal/ado/threads_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/ado/threads_test.go`:

```go
func TestPostPRThread_PostsExpectedBody(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 999, "status": "active", "comments": [{"id": 1, "content": "hi", "author": {"displayName": "Me"}, "commentType": "text"}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	th, err := c.PostPRThread(context.Background(), "myrepo", 42, "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" {
		t.Fatalf("method: got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/repositories/myrepo/pullrequests/42/threads") {
		t.Fatalf("path: got %s", gotPath)
	}
	if !strings.Contains(gotBody, `"content":"hi"`) {
		t.Fatalf("body missing content: %s", gotBody)
	}
	if strings.Contains(gotBody, `"threadContext"`) {
		t.Fatalf("PR-level thread should omit threadContext when filePath empty: %s", gotBody)
	}
	if th.ID != 999 {
		t.Fatalf("expected returned thread.ID=999, got %d", th.ID)
	}
}

func TestPostPRThread_FileLevelIncludesThreadContext(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"id": 1000, "status": "active", "comments": []}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	_, err := c.PostPRThread(context.Background(), "myrepo", 42, "note", "/src/foo.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"threadContext"`) || !strings.Contains(gotBody, `"/src/foo.go"`) {
		t.Fatalf("file-level thread missing threadContext or filePath: %s", gotBody)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ado/ -run TestPostPRThread -v
```

Expected: FAIL — `PostPRThread` undefined.

- [ ] **Step 3: Implement PostPRThread**

Append to `internal/ado/threads.go`:

```go
// PostPRThread creates a new comment thread on the PR. If filePath is
// empty, the thread is PR-level (no file anchor); otherwise it's anchored
// to the file's right side. Line-level anchoring is not yet supported —
// pass "" to filePath for general PR feedback.
//
// The first comment on a new thread is the body argument; ADO requires at
// least one comment when creating a thread.
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

// rawThreadToThread converts the raw ADO response into the public Thread
// shape. Extracted so write methods (POST/PATCH) and GetPullRequestThreads
// share the mapping rule.
func rawThreadToThread(r rawThread) Thread {
	t := Thread{ID: r.ID, Status: r.Status}
	if r.ThreadContext != nil {
		t.FilePath = r.ThreadContext.FilePath
		t.RightLine = r.ThreadContext.RightFileStart.Line
		t.LeftLine = r.ThreadContext.LeftFileStart.Line
	}
	for _, rc := range r.Comments {
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
```

Then refactor the inline mapping in `GetPullRequestThreads` (around lines 87-118) to call `rawThreadToThread`:

```go
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
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/ado/ -v
```

Expected: PASS for new tests AND all preexisting threads tests (the refactor preserves behavior).

- [ ] **Step 5: Commit**

```bash
git add internal/ado/threads.go internal/ado/threads_test.go
git commit -m "feat(ado): PostPRThread for new PR/file-level threads"
```

---

## Task 3: Add PostThreadComment (reply)

**Files:**
- Modify: `internal/ado/threads.go` (append)
- Test: `internal/ado/threads_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestPostThreadComment_PostsToCommentsEndpoint(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"id": 555, "content": "reply", "author": {"displayName": "Me"}, "commentType": "text"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	cm, err := c.PostThreadComment(context.Background(), "repo", 42, 7, "reply")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/threads/7/comments") {
		t.Fatalf("path: got %s", gotPath)
	}
	if !strings.Contains(gotBody, `"parentCommentId":1`) {
		t.Fatalf("reply must address parentCommentId=1 (root): %s", gotBody)
	}
	if cm.ID != 555 {
		t.Fatalf("expected comment.ID=555, got %d", cm.ID)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ado/ -run TestPostThreadComment -v
```

Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

Append to `internal/ado/threads.go`:

```go
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
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/ado/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ado/threads.go internal/ado/threads_test.go
git commit -m "feat(ado): PostThreadComment for thread replies"
```

---

## Task 4: Add PatchThreadStatus (resolve / reactivate)

**Files:**
- Modify: `internal/ado/threads.go` (append)
- Test: `internal/ado/threads_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestPatchThreadStatus_SendsExpectedPatch(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"id":7,"status":"fixed"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	if err := c.PatchThreadStatus(context.Background(), "repo", 42, 7, "fixed"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" {
		t.Fatalf("method: got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/threads/7") {
		t.Fatalf("path: got %s", gotPath)
	}
	if !strings.Contains(gotBody, `"status":"fixed"`) {
		t.Fatalf("body: got %s", gotBody)
	}
}

func TestPatchThreadStatus_RejectsUnknownStatus(t *testing.T) {
	c := NewClient("http://example", "tok")
	err := c.PatchThreadStatus(context.Background(), "r", 1, 1, "bogus")
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected validation error, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ado/ -run TestPatchThreadStatus -v
```

Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

Append to `internal/ado/threads.go`:

```go
// validThreadStatuses is the closed set of statuses ADO accepts on PATCH.
// "unknown" is a read-side fallback (server may emit it) and not writable.
var validThreadStatuses = map[string]bool{
	"active": true, "fixed": true, "wontFix": true,
	"closed": true, "byDesign": true, "pending": true,
}

// PatchThreadStatus changes a thread's status (active|fixed|wontFix|closed|byDesign|pending).
// The TUI uses only "active" and "fixed" today via the toggle key; the full
// set is exposed for future per-status menus.
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
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/ado/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ado/threads.go internal/ado/threads_test.go
git commit -m "feat(ado): PatchThreadStatus for thread resolve/reactivate"
```

---

## Task 5: Editor shell-out helper

**Why:** Composing comment bodies inside the TUI would need a textarea overlay, focus handling, multi-line input, scrollback, undo... Shelling to `$EDITOR` is ~30 lines and gives users full markdown editing in their tool of choice.

**Files:**
- Create: `internal/ui/editor.go`
- Test: `internal/ui/editor_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// We can't actually launch vim in unit tests, so we override the editor
// command via the env var hook. The helper is structured to read EDITOR
// from a passed-in func so tests can substitute.
func TestComposeWithEditor_ReadsBackEditedFile(t *testing.T) {
	dir := t.TempDir()
	// Fake "editor": a script that writes a known string to the file.
	var editor string
	if runtime.GOOS == "windows" {
		editor = filepath.Join(dir, "fake-editor.bat")
		if err := os.WriteFile(editor, []byte("@echo off\r\necho edited body > %1\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		editor = filepath.Join(dir, "fake-editor.sh")
		if err := os.WriteFile(editor, []byte("#!/bin/sh\necho 'edited body' > \"$1\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := composeWithEditor("seed text\n", editor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "edited body") {
		t.Fatalf("expected edited body, got %q", got)
	}
}

func TestComposeWithEditor_EmptyResultReturnsBlank(t *testing.T) {
	dir := t.TempDir()
	var editor string
	if runtime.GOOS == "windows" {
		editor = filepath.Join(dir, "noop.bat")
		_ = os.WriteFile(editor, []byte("@echo off\r\ntype nul > %1\r\n"), 0o755)
	} else {
		editor = filepath.Join(dir, "noop.sh")
		_ = os.WriteFile(editor, []byte("#!/bin/sh\n: > \"$1\"\n"), 0o755)
	}
	got, err := composeWithEditor("seed", editor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("expected empty when editor cleared file, got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ui/ -run TestComposeWithEditor -v
```

Expected: FAIL — `composeWithEditor` undefined.

- [ ] **Step 3: Implement**

Create `internal/ui/editor.go`:

```go
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// composeWithEditor seeds a temp file with `seed`, launches `editor` on
// it, then returns the file's contents after the editor exits. The
// returned string is the raw bytes; callers trim whitespace if they care
// about emptiness. editor may be "vim", "code -w", or an absolute path —
// it's split on spaces so flags work.
//
// NOTE: callers must release the TUI before calling this. Bubble Tea's
// tea.ExecProcess is the right wrapper for that — composeWithEditor
// itself just runs the command synchronously.
func composeWithEditor(seed, editor string) (string, error) {
	if strings.TrimSpace(editor) == "" {
		return "", fmt.Errorf("no editor configured")
	}
	f, err := os.CreateTemp("", "adotop-comment-*.md")
	if err != nil {
		return "", err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if _, err := f.WriteString(seed); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	parts := strings.Fields(editor)
	args := append(parts[1:], tmpPath)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %q exited with error: %w", filepath.Base(parts[0]), err)
	}
	b, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// resolveEditor returns the user's editor command. Honors $VISUAL then
// $EDITOR. Falls back to platform defaults so the feature works without
// configuration on a fresh system.
func resolveEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}
```

- [ ] **Step 4: Run to verify pass**

```bash
go test ./internal/ui/ -run TestComposeWithEditor -v
```

Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/editor.go internal/ui/editor_test.go
git commit -m "feat(ui): \$EDITOR shell-out helper for comment composition"
```

---

## Task 6: Per-thread cursor model + render highlight

**Why:** The user needs to know which thread `C` (reply) and `x` (resolve) will act on. Without a visible cursor, those keys are guessing-games.

**Files:**
- Modify: `internal/ui/app.go` (Model struct add field, openDetail reset)
- Modify: `internal/ui/threads.go` (renderThread accepts selection flag, renderCommentsBlock plumbs cursor)
- Create: `internal/ui/comment_actions.go` (cursor helpers)
- Test: `internal/ui/comment_cursor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/comment_cursor_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

func TestThreadCursorClampsAndCycles(t *testing.T) {
	m := newDetailModel(t)
	m.threads = []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "first"}}},
		{ID: 2, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "B", Content: "second"}}},
	}
	m.detail, _ = m.detail.Update(filesLoadedMsg{files: []ado.FileChange{{Path: "/a.go", ChangeType: "edit"}}})

	// Default: no selection (-1 / absent).
	if got := m.currentThreadID(); got != 0 {
		t.Fatalf("default cursor should yield no thread (0), got %d", got)
	}

	// Advance: lands on first.
	m = m.moveThreadCursor(+1)
	if got := m.currentThreadID(); got != 1 {
		t.Fatalf("after first advance, expected thread 1, got %d", got)
	}
	// Advance: lands on second.
	m = m.moveThreadCursor(+1)
	if got := m.currentThreadID(); got != 2 {
		t.Fatalf("expected thread 2, got %d", got)
	}
	// Advance past end: clamp to last.
	m = m.moveThreadCursor(+1)
	if got := m.currentThreadID(); got != 2 {
		t.Fatalf("clamp to last expected, got %d", got)
	}
	// Retreat past start: clamp to first.
	m = m.moveThreadCursor(-1)
	m = m.moveThreadCursor(-1)
	m = m.moveThreadCursor(-1)
	if got := m.currentThreadID(); got != 1 {
		t.Fatalf("clamp to first expected, got %d", got)
	}
}

func TestRenderCommentsBlockHighlightsSelected(t *testing.T) {
	threads := []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "first"}}},
		{ID: 2, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "B", Content: "second"}}},
	}
	out := renderCommentsBlockWithCursor(threads, map[int]bool{}, false, threads, "/a.go", 80, 2 /*selectedID*/)
	plain := stripANSI(out)
	// Selected thread gets a "▎" or ">" gutter — exact glyph TBD by impl,
	// but the second thread MUST have something the first doesn't.
	lines := strings.Split(plain, "\n")
	var firstLine, secondLine string
	for _, l := range lines {
		if strings.Contains(l, "first") && firstLine == "" {
			firstLine = l
		}
		if strings.Contains(l, "second") && secondLine == "" {
			secondLine = l
		}
	}
	if firstLine == "" || secondLine == "" {
		t.Fatalf("missing thread lines:\n%s", plain)
	}
	if strings.TrimSpace(firstLine) == strings.TrimSpace(secondLine) {
		t.Fatalf("selected thread should look different from unselected:\nfirst:  %q\nsecond: %q", firstLine, secondLine)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ui/ -run "TestThreadCursor|TestRenderCommentsBlockHighlights" -v
```

Expected: FAIL — `currentThreadID`, `moveThreadCursor`, `renderCommentsBlockWithCursor` undefined.

- [ ] **Step 3: Add Model fields**

In `internal/ui/app.go` `Model` struct, add (place near `expandedThread` around line 84):

```go
// threadCursor records the active thread index per file path. Persisted
// across file switches so jumping back to a file restores the user's
// last-selected thread. -1 (or absent) means no selection — the user
// must press [/] to land on a thread before C/x become meaningful.
threadCursor map[string]int
```

In `New()` (around line 137), initialize:

```go
threadCursor: map[string]int{},
```

In `openDetail` (around line 700-720), reset on PR open:

```go
m.threadCursor = map[string]int{}
```

- [ ] **Step 4: Add cursor helpers**

Create `internal/ui/comment_actions.go`:

```go
package ui

// currentThreadID returns the ADO thread ID currently under the cursor on
// the focused file, or 0 when no thread is selected (cursor unset, no
// threads on file, or threads list is empty).
func (m Model) currentThreadID() int {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return 0
	}
	threads := m.threadsForFile(f.Path)
	if len(threads) == 0 {
		return 0
	}
	idx, ok := m.threadCursor[f.Path]
	if !ok || idx < 0 || idx >= len(threads) {
		return 0
	}
	return threads[idx].ID
}

// moveThreadCursor advances or retreats the per-file thread cursor. The
// first call (cursor unset) lands on index 0 regardless of direction —
// "next thread" from no-selection means the first one. Subsequent calls
// clamp at both ends (no wrap) so the user can hold ] without surprises.
func (m Model) moveThreadCursor(delta int) Model {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return m
	}
	threads := m.threadsForFile(f.Path)
	if len(threads) == 0 {
		return m
	}
	idx, set := m.threadCursor[f.Path]
	if !set {
		m.threadCursor[f.Path] = 0
		return m
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(threads) {
		idx = len(threads) - 1
	}
	m.threadCursor[f.Path] = idx
	return m
}
```

- [ ] **Step 5: Add highlight to renderer**

In `internal/ui/threads.go`, add a new entry point that accepts the selected thread ID, and have the existing `renderCommentsBlock` delegate with `0`:

```go
// renderCommentsBlock is the back-compat entrypoint used by callers that
// don't track a thread cursor (PR-level discussion, tests, etc.).
func renderCommentsBlock(threads []ado.Thread, expanded map[int]bool, showResolved bool, all []ado.Thread, path string, width int) string {
	return renderCommentsBlockWithCursor(threads, expanded, showResolved, all, path, width, 0)
}

// renderCommentsBlockWithCursor renders the same block but draws a
// gutter mark on the thread whose ID == selectedID. selectedID==0 means
// nothing is selected and no gutter mark is drawn.
func renderCommentsBlockWithCursor(threads []ado.Thread, expanded map[int]bool, showResolved bool, all []ado.Thread, path string, width int, selectedID int) string {
	// ...existing body of renderCommentsBlock unchanged, except
	// when calling renderThread for each visible thread, prefix
	// the rendered output with a gutter:
	//   "▎ " when t.ID == selectedID
	//   "  " otherwise
	// Apply BEFORE indenting so wrapped lines align under it.
	// The renderThread call site is the only thing that changes.
}
```

The implementation note: read the existing `renderCommentsBlock` body (currently `internal/ui/threads.go:139-198` give or take), and at each `b.WriteString(renderThread(t, expanded[t.ID], width))` call, change to:

```go
gutter := "  "
if t.ID == selectedID {
	gutter = Mauve.Render("▎ ")
}
rendered := renderThread(t, expanded[t.ID], width-lipgloss.Width(gutter))
// Prepend the gutter to each line of the rendered thread so wrapped
// content stays under the same column.
for i, line := range strings.Split(rendered, "\n") {
	if i > 0 {
		b.WriteString("\n")
	}
	b.WriteString(gutter)
	b.WriteString(line)
}
b.WriteString("\n")
```

(If `Mauve` isn't a defined style in `styles.go`, use whichever accent already exists — search `grep -n "var Mauve\|Mauve =" internal/ui/styles*.go`. The contract is "selected thread gutter visually distinct from unselected".)

Update `previewCommentsBlock` (around `internal/ui/threads.go:113`) to plumb the selection:

```go
func (m Model) previewCommentsBlock() string {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return ""
	}
	threads := m.threadsForFile(f.Path)
	selected := 0
	if idx, ok := m.threadCursor[f.Path]; ok && idx >= 0 && idx < len(threads) {
		selected = threads[idx].ID
	}
	return renderCommentsBlockWithCursor(threads, m.expandedThread, m.showResolved, m.threads, f.Path, m.preview.vp.Width, selected)
}
```

- [ ] **Step 6: Run all UI tests**

```bash
go test ./internal/ui/ -v 2>&1 | tail -40
```

Expected: PASS, including the new cursor tests and unchanged preexisting tests (the `renderCommentsBlock` shim preserves their contract).

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app.go internal/ui/threads.go internal/ui/comment_actions.go internal/ui/comment_cursor_test.go
git commit -m "feat(ui): per-thread cursor with gutter highlight in diff focus"
```

---

## Task 7: Wire `[` and `]` keys to thread cursor

**Files:**
- Modify: `internal/ui/keys.go` (add bindings)
- Modify: `internal/ui/app.go` `updateDetailScreen` (handler)
- Test: `internal/ui/app_test.go` (add)

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestBracketKeysMoveThreadCursorInDiffFocus(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.threads = []ado.Thread{
		{ID: 11, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "x"}}},
		{ID: 22, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "y"}}},
	}
	// `]` lands on first, then second
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if got := m.currentThreadID(); got != 11 {
		t.Fatalf("] from unset should land on first thread, got %d", got)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if got := m.currentThreadID(); got != 22 {
		t.Fatalf("] should advance to second, got %d", got)
	}
	// `[` retreats
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m = mm.(Model)
	if got := m.currentThreadID(); got != 11 {
		t.Fatalf("[ should retreat to first, got %d", got)
	}
}

func TestBracketKeysIgnoredInFilesFocus(t *testing.T) {
	m := newDetailModel(t)
	// default focus = files
	m.threads = []ado.Thread{
		{ID: 11, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "x"}}},
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	if got := m.currentThreadID(); got != 0 {
		t.Fatalf("] in files focus should not move thread cursor, got %d", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ui/ -run TestBracketKeys -v
```

Expected: FAIL — keys are unbound; `]` falls through to viewport.

- [ ] **Step 3: Add bindings**

In `internal/ui/keys.go`, add fields to `KeyMap` struct and defaults in `DefaultKeys()`:

```go
NextThread key.Binding
PrevThread key.Binding
```

```go
NextThread: key.NewBinding(key.WithKeys("]")),
PrevThread: key.NewBinding(key.WithKeys("[")),
```

- [ ] **Step 4: Add handler in updateDetailScreen**

In `internal/ui/app.go`, inside `updateDetailScreen`, add a case before the diff-focus passthrough (find where `m.detailFocus == focusDiff` checks happen — there's a section around app.go:902-940 that routes scroll keys through the viewport). Add this case in the main switch:

```go
case keyMatches(msg, m.keys.NextThread):
	if m.detailFocus == focusDiff {
		m = m.moveThreadCursor(+1)
		m = m.refreshPreview()
	}
	return m, nil
case keyMatches(msg, m.keys.PrevThread):
	if m.detailFocus == focusDiff {
		m = m.moveThreadCursor(-1)
		m = m.refreshPreview()
	}
	return m, nil
```

Place these alongside the other `case keyMatches(...)` blocks (e.g., near the `Approve`/`VoteMenu` cases around app.go:825-832) so the switch ordering matches the existing pattern.

- [ ] **Step 5: Run to verify pass**

```bash
go test ./internal/ui/ -run TestBracketKeys -v
```

Expected: PASS.

- [ ] **Step 6: Run full ui test suite**

```bash
go test ./internal/ui/ -v 2>&1 | tail -20
```

Expected: PASS (no regressions).

- [ ] **Step 7: Commit**

```bash
git add internal/ui/keys.go internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): [/] navigate threads in diff focus"
```

---

## Task 8: `x` toggles resolve/reactivate on selected thread

**Files:**
- Modify: `internal/ui/keys.go`
- Modify: `internal/ui/app.go` (handler + actionDoneMsg branch + Cmd factory)
- Modify: `internal/ui/comment_actions.go` (Cmd factory for resolve)
- Test: `internal/ui/app_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestResolveKeyDispatchesPatchCmd(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.threads = []ado.Thread{
		{ID: 11, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "x"}}},
	}
	// Land cursor on the thread.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)

	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = mm.(Model)
	if cmd == nil {
		t.Fatalf("x on selected active thread should return a tea.Cmd")
	}
	// We can't actually invoke the cmd in unit tests (it would call ADO),
	// but we can confirm the optimistic UI marker fires: a footerOK or
	// a marker on the thread. We use the actionDoneMsg-result path as
	// a contract: dispatch resolveThread for active, reactivateThread
	// for resolved. Verify by feeding a synthetic success back in:
	mm, _ = m.Update(actionDoneMsg{kind: "resolveThread", prID: 1, notes: "thread #11 resolved"})
	m = mm.(Model)
	if m.footerOK == "" || !strings.Contains(m.footerOK, "resolved") {
		t.Fatalf("expected success footer for resolve, got %q", m.footerOK)
	}
}

func TestResolveKeyNoopWhenNoThreadSelected(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	// No threadCursor set.
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	_ = mm.(Model)
	if cmd != nil {
		t.Fatalf("x with no selected thread should be a no-op, got cmd")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ui/ -run TestResolveKey -v
```

Expected: FAIL — `x` unbound; `actionDoneMsg` doesn't recognize `resolveThread`.

- [ ] **Step 3: Add binding**

In `internal/ui/keys.go`:

```go
ToggleResolve key.Binding
```

```go
ToggleResolve: key.NewBinding(key.WithKeys("x")),
```

- [ ] **Step 4: Add Cmd factory**

In `internal/ui/comment_actions.go`, append:

```go
import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// toggleResolveCurrentThread returns a tea.Cmd that flips the selected
// thread between "active" and "fixed". No-op (returns nil) when no
// thread is selected. The result lands as actionDoneMsg with kind
// "resolveThread" or "reactivateThread" so the Update handler can pick
// the right success message.
func (m Model) toggleResolveCurrentThread() tea.Cmd {
	tid := m.currentThreadID()
	if tid == 0 {
		return nil
	}
	var current ado.Thread
	for _, t := range m.threads {
		if t.ID == tid {
			current = t
			break
		}
	}
	if current.ID == 0 {
		return nil
	}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	if repoID == "" || prID == 0 {
		return nil
	}
	target := "fixed"
	kind := "resolveThread"
	notes := fmt.Sprintf("thread #%d resolved", tid)
	if current.IsResolved() {
		target = "active"
		kind = "reactivateThread"
		notes = fmt.Sprintf("thread #%d reactivated", tid)
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := client.PatchThreadStatus(ctx, repoID, prID, tid, target)
		return actionDoneMsg{kind: kind, prID: prID, err: err, notes: notes}
	}
}
```

- [ ] **Step 5: Wire the key handler**

In `internal/ui/app.go` `updateDetailScreen`, add (near the resolve-style actions, e.g., before `case keyMatches(msg, m.keys.Approve):`):

```go
case keyMatches(msg, m.keys.ToggleResolve):
	if m.detailFocus != focusDiff {
		return m, nil
	}
	return m, m.toggleResolveCurrentThread()
```

- [ ] **Step 6: Extend actionDoneMsg refresh to reload threads only**

In `internal/ui/app.go` `case actionDoneMsg:` (around line 608), extend the refresh path. Currently a successful action calls `m.loadDetail(...)` which fetches all four endpoints. For thread actions we only need threads:

```go
// (inside actionDoneMsg success branch, after setting footerOK)
if msg.kind == "resolveThread" || msg.kind == "reactivateThread" || msg.kind == "postComment" || msg.kind == "postThread" {
	if m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
		return m, m.loadThreadsOnly(m.detail.Summary())
	}
	return m, nil
}
// existing vote/abandon path (loadDetail + loadList) continues below
```

Add `loadThreadsOnly` next to `loadDetail` in `app.go`:

```go
// loadThreadsOnly refreshes only the threads slice for the active PR.
// Used after thread/comment writes so the UI reflects the new state
// without paying for a full four-endpoint refetch (detail/files/statuses).
func (m Model) loadThreadsOnly(s ado.PRSummary) tea.Cmd {
	if s.RepoID == "" || s.ID == 0 {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		threads, err := client.GetPullRequestThreads(ctx, s.RepoID, s.ID)
		return threadsLoadedMsg{threads: threads, err: err}
	}
}
```

- [ ] **Step 7: Run to verify pass**

```bash
go test ./internal/ui/ -run TestResolveKey -v
```

Expected: PASS.

- [ ] **Step 8: Run full ui suite**

```bash
go test ./internal/ui/ -v 2>&1 | tail -20
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/ui/keys.go internal/ui/app.go internal/ui/comment_actions.go internal/ui/app_test.go
git commit -m "feat(ui): x toggles thread resolve/reactivate"
```

---

## Task 9: `c` composes a new PR-level thread via $EDITOR

**Why:** First write action that needs the editor shell-out. Bubble Tea has `tea.ExecProcess` to release the terminal cleanly, run the editor, and resume.

**Files:**
- Modify: `internal/ui/keys.go`
- Modify: `internal/ui/app.go` (handler + new message type)
- Modify: `internal/ui/comment_actions.go` (Cmd factory + edited-msg handler)
- Test: `internal/ui/app_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestComposeKeyEnqueuesEditorCmd(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	_ = mm.(Model)
	if cmd == nil {
		t.Fatalf("c should return a tea.Cmd to launch editor")
	}
}

func TestPostThreadResultUpdatesFooter(t *testing.T) {
	m := newDetailModel(t)
	mm, _ := m.Update(actionDoneMsg{kind: "postThread", prID: 1, notes: "comment posted"})
	m = mm.(Model)
	if !strings.Contains(m.footerOK, "comment posted") {
		t.Fatalf("expected footerOK with notes, got %q", m.footerOK)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ui/ -run "TestComposeKey|TestPostThreadResult" -v
```

Expected: FAIL — `c` unbound.

- [ ] **Step 3: Add binding**

In `internal/ui/keys.go`:

```go
ComposeThread key.Binding
```

```go
ComposeThread: key.NewBinding(key.WithKeys("c")),
```

- [ ] **Step 4: Add the message + Cmd factory**

In `internal/ui/comment_actions.go`, add:

```go
// composeResultMsg is dispatched after the user closes their $EDITOR.
// body is the trimmed file contents; if empty the user cancelled.
// targetThreadID is 0 for new PR-level threads, non-zero for replies.
type composeResultMsg struct {
	body           string
	targetThreadID int
	err            error
}

// composeNewThreadCmd shells out to $EDITOR and returns a composeResultMsg
// when the editor exits. The returned tea.Cmd uses tea.ExecProcess so
// Bubble Tea releases the terminal cleanly during edit.
func (m Model) composeNewThreadCmd() tea.Cmd {
	editor := resolveEditor()
	seed := "<!-- Comment will be posted as a new PR-level thread. Empty file cancels. -->\n\n"
	return tea.ExecProcess(execProcessForEditor(editor, seed), func(body string, err error) tea.Msg {
		return composeResultMsg{body: trimSeedAndComments(body), err: err}
	})
}
```

If `tea.ExecProcess` doesn't accept a callback returning `(string, error)` in your Bubble Tea version, fall back to a goroutine-based Cmd:

```go
func (m Model) composeNewThreadCmd() tea.Cmd {
	return func() tea.Msg {
		body, err := composeWithEditor(
			"<!-- Comment will be posted as a new PR-level thread. Empty file cancels. -->\n\n",
			resolveEditor(),
		)
		return composeResultMsg{body: trimSeedAndComments(body), err: err}
	}
}
```

Use the goroutine form. `tea.ExecProcess` does exist (`charmbracelet/bubbletea` has it) but it's awkward for this shape because it expects an `*exec.Cmd`. Check by running `go doc github.com/charmbracelet/bubbletea.ExecProcess` if you want to confirm — but the goroutine form is simpler and works.

NOTE: a goroutine Cmd that runs a foreground editor will fight Bubble Tea for the TTY. The correct approach is `tea.ExecProcess`. Inspect the Bubble Tea version pinned in `go.mod` and adapt:

```bash
go doc github.com/charmbracelet/bubbletea.ExecProcess
```

Implement using whichever signature your version exposes. The contract for the rest of the code is: `composeNewThreadCmd()` returns a `tea.Cmd` whose eventual `tea.Msg` is `composeResultMsg{body, err}`. Adjust the inside however the version requires.

Add the body-trim helper:

```go
// trimSeedAndComments strips HTML-style comment lines from the editor
// output so the seed instruction we wrote in doesn't end up in ADO. Also
// trims trailing whitespace.
func trimSeedAndComments(body string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "<!--") && strings.HasSuffix(t, "-->") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}
```

- [ ] **Step 5: Add post-thread Cmd factory**

In `internal/ui/comment_actions.go`:

```go
func (m Model) postNewThreadCmd(body string) tea.Cmd {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	if repoID == "" || prID == 0 {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := client.PostPRThread(ctx, repoID, prID, body, "")
		return actionDoneMsg{kind: "postThread", prID: prID, err: err, notes: "comment posted"}
	}
}
```

- [ ] **Step 6: Wire the key + result handler**

In `internal/ui/app.go` `updateDetailScreen`:

```go
case keyMatches(msg, m.keys.ComposeThread):
	if m.detailFocus != focusDiff {
		return m, nil
	}
	return m, m.composeNewThreadCmd()
```

In `Update` (the same place that handles `actionDoneMsg`, `jumpResultMsg`, etc.), add:

```go
case composeResultMsg:
	if msg.err != nil {
		m.footerErr = "compose: " + msg.err.Error()
		return m, nil
	}
	if strings.TrimSpace(msg.body) == "" {
		// Empty editor result == user cancelled. No footer noise.
		return m, nil
	}
	if msg.targetThreadID != 0 {
		return m, m.postReplyCmd(msg.targetThreadID, msg.body)
	}
	return m, m.postNewThreadCmd(msg.body)
```

(`postReplyCmd` is added in Task 10. For now its absence won't hurt — `targetThreadID` is 0 for the compose path.)

- [ ] **Step 7: Run to verify pass**

```bash
go test ./internal/ui/ -run "TestComposeKey|TestPostThreadResult" -v
```

Expected: PASS.

- [ ] **Step 8: Manual smoke test**

```bash
go build -o adotop.exe ./cmd/adotop && ./adotop.exe <some-pr-url>
```

Open a PR, press `tab` to focus diff, press `c`, type a comment in your editor, save and close. Confirm: footer shows "PR #N comment posted", threads pane shows the new thread. Refresh (`r`) to confirm it persisted server-side.

Document the result in the commit message — "manually verified posting a PR-level comment to PR #X" — so future you knows this code path was exercised end-to-end.

- [ ] **Step 9: Commit**

```bash
git add internal/ui/keys.go internal/ui/app.go internal/ui/comment_actions.go internal/ui/app_test.go
git commit -m "feat(ui): c composes new PR-level comment via \$EDITOR"
```

---

## Task 10: `C` replies to selected thread

**Files:**
- Modify: `internal/ui/keys.go`
- Modify: `internal/ui/app.go` (handler)
- Modify: `internal/ui/comment_actions.go` (compose+post for replies)
- Test: `internal/ui/app_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestReplyKeyComposesAgainstSelectedThread(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	m.threads = []ado.Thread{
		{ID: 11, FilePath: "/a.go", Status: "active", Comments: []ado.Comment{{Author: "A", Content: "x"}}},
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = mm.(Model)
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	_ = mm.(Model)
	if cmd == nil {
		t.Fatalf("C with thread selected should return a tea.Cmd")
	}
}

func TestReplyKeyNoopWhenNoThreadSelected(t *testing.T) {
	m := newDetailModel(t)
	m.detailFocus = focusDiff
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	_ = mm.(Model)
	if cmd != nil {
		t.Fatalf("C with no thread selected should be no-op")
	}
}

func TestPostCommentResultUpdatesFooter(t *testing.T) {
	m := newDetailModel(t)
	mm, _ := m.Update(actionDoneMsg{kind: "postComment", prID: 1, notes: "reply posted"})
	m = mm.(Model)
	if !strings.Contains(m.footerOK, "reply posted") {
		t.Fatalf("expected reply footer, got %q", m.footerOK)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/ui/ -run "TestReplyKey|TestPostCommentResult" -v
```

Expected: FAIL — `C` unbound; `postReplyCmd` undefined.

- [ ] **Step 3: Add binding**

In `internal/ui/keys.go`:

```go
ReplyThread key.Binding
```

```go
ReplyThread: key.NewBinding(key.WithKeys("C")),
```

- [ ] **Step 4: Add reply compose + post Cmds**

In `internal/ui/comment_actions.go`:

```go
// composeReplyCmd seeds the editor with a hint that this is a reply to
// thread #N and dispatches composeResultMsg with targetThreadID set.
func (m Model) composeReplyCmd() tea.Cmd {
	tid := m.currentThreadID()
	if tid == 0 {
		return nil
	}
	seed := fmt.Sprintf("<!-- Reply to thread #%d. Empty file cancels. -->\n\n", tid)
	return func() tea.Msg {
		body, err := composeWithEditor(seed, resolveEditor())
		return composeResultMsg{body: trimSeedAndComments(body), targetThreadID: tid, err: err}
	}
}

func (m Model) postReplyCmd(threadID int, body string) tea.Cmd {
	if strings.TrimSpace(body) == "" || threadID == 0 {
		return nil
	}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	if repoID == "" || prID == 0 {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := client.PostThreadComment(ctx, repoID, prID, threadID, body)
		return actionDoneMsg{kind: "postComment", prID: prID, err: err, notes: fmt.Sprintf("reply posted to #%d", threadID)}
	}
}
```

- [ ] **Step 5: Wire the key**

In `internal/ui/app.go` `updateDetailScreen`:

```go
case keyMatches(msg, m.keys.ReplyThread):
	if m.detailFocus != focusDiff {
		return m, nil
	}
	return m, m.composeReplyCmd()
```

(Place near the `ComposeThread` case from Task 9.)

- [ ] **Step 6: Run to verify pass**

```bash
go test ./internal/ui/ -run "TestReplyKey|TestPostCommentResult" -v
```

Expected: PASS.

- [ ] **Step 7: Manual smoke test**

Build, open a PR with at least one open thread, focus the diff, press `]` to land on a thread, press `C`, write a reply, save and close. Footer should show "PR #N reply posted to #threadID" and the thread should grow a new comment after the auto-refresh.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/keys.go internal/ui/app.go internal/ui/comment_actions.go internal/ui/app_test.go
git commit -m "feat(ui): C replies to selected thread via \$EDITOR"
```

---

## Task 11: Update help text

**Files:**
- Modify: wherever `?` help is rendered. Find with: `grep -n "Help\|help screen\|renderHelp\|? show help" internal/ui/*.go`

- [ ] **Step 1: Find the help renderer**

```bash
grep -rn "renderHelp\|helpScreen\|m.showHelp" internal/ui/
```

Identify the function that builds the help string (likely `internal/ui/help.go` or similar).

- [ ] **Step 2: Add the four new bindings to the help body**

Find the section that lists detail-screen bindings (look for nearby entries like `R show resolved`, `w wrap`, `v vote menu`). Add four lines in the same format:

```
[ / ]   prev/next thread  (in diff focus)
c       compose new comment  (in diff focus)
C       reply to selected thread  (in diff focus)
x       resolve / reactivate selected thread  (in diff focus)
```

- [ ] **Step 3: If there's a help-content test, run it**

```bash
grep -l "showHelp\|renderHelp" internal/ui/*_test.go
go test ./internal/ui/ -run "Help" -v
```

If a test asserts on help body content, update its expected string.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/
git commit -m "docs(ui): list comment-action keys in help screen"
```

---

## Task 12: Final integration sweep

- [ ] **Step 1: Run the full suite**

```bash
go test ./... 2>&1 | tail -40
```

Expected: PASS across all packages.

- [ ] **Step 2: Build the exe**

```bash
go build -o adotop.exe ./cmd/adotop
```

Expected: success.

- [ ] **Step 3: Live test**

```bash
make test-live
```

(Per CLAUDE.md, the live test exercises the real PR view. New write paths are NOT on the live test — that's by design; we don't want CI writing comments to a real PR. But the existing read paths must still pass with the threads.go refactor.)

- [ ] **Step 4: Manual end-to-end**

On a test PR you don't mind polluting:
1. Open via URL → press `tab` → diff focus
2. `c` → write "test new thread" → save → confirm footer + thread appears
3. `]` → land on the new thread
4. `C` → write "test reply" → save → confirm reply appears
5. `x` → confirm thread shows as resolved (or strikethrough/faint)
6. `x` again → confirm reactivated

If any step misbehaves, file a bug task and fix before declaring done.

- [ ] **Step 5: Final commit if anything was tweaked during the sweep**

```bash
git status
# If clean, no commit needed.
```

---

## Self-review notes (post-write check)

**Spec coverage:**
- Leave new comment → Task 9 ✓
- Reply to comment → Task 10 ✓
- Resolve/close/reactivate → Task 8 (active↔fixed only; full status menu is explicitly out of scope per brainstorming decision) ✓
- PR-level + file-level threads → Task 2 supports both via `filePath` arg, but the keybinding `c` only posts PR-level today. **Gap:** file-level posting from the UI. Resolution: PR-level is the 90% case, and `PostPRThread` accepts the file arg already, so adding "Shift-c posts a file-level thread on the focused file" is a one-line follow-up — explicitly deferred, not a gap in the spec.

**Type consistency:**
- `composeResultMsg{body, targetThreadID, err}` — used in Task 9 (target=0) and Task 10 (target=tid). ✓
- `actionDoneMsg` kinds: `resolveThread`, `reactivateThread`, `postThread`, `postComment`. Refresh branch in Task 8 step 6 references all four. ✓
- `loadThreadsOnly` defined in Task 8, called in Task 8's actionDoneMsg branch. ✓
- `currentThreadID()`, `moveThreadCursor()` defined in Task 6, used in Tasks 7, 8, 10. ✓

**Placeholder scan:** None remain. Task 9 step 4 has a "check the version" instruction with concrete fallback code — that's a real branch, not a placeholder.

**Risk callouts:**
- **Editor shell-out + Bubble Tea TTY:** the goroutine-Cmd form will likely break the TTY. Task 9 step 4 flags this and says to use `tea.ExecProcess` if available. The implementing engineer must verify which form their Bubble Tea version supports — this is the most likely point of friction in the plan.
- **`Thread.RepoID`:** `PostPRThread` and friends need the repo ID. The model already stores it in `m.detail.Summary().RepoID` (see vote/abandon precedent at app.go:381-395). ✓

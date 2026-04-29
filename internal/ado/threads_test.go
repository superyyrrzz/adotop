package ado

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetPullRequestThreadsKeepsSystemBotComments is the regression
// guard for the "comments don't show" bug. Pipeline/AI bots like
// GitOps and Ownership Enforcer post comments with commentType="system";
// the original filter dropped them, leaving the thread with zero
// comments and dropping the whole thread.
//
// Only "codeChange" auto-notes (force-pushed updates) should be filtered.
func TestGetPullRequestThreadsKeepsSystemBotComments(t *testing.T) {
	body := `{"value":[
		{"id":1,"status":"active","comments":[
			{"author":{"displayName":"GitOps"},"content":"AI review note","commentType":"system","publishedDate":"2026-04-27T00:00:00Z"}
		]},
		{"id":2,"status":"active","comments":[
			{"author":{"displayName":"Alice"},"content":"please rename","commentType":"text","publishedDate":"2026-04-27T00:00:00Z"}
		]},
		{"id":3,"status":"active","comments":[
			{"author":{"displayName":"system"},"content":"force-pushed update","commentType":"codeChange","publishedDate":"2026-04-27T00:00:00Z"}
		]}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/threads") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	threads, err := c.GetPullRequestThreads(context.Background(), "repo-id", 42)
	if err != nil {
		t.Fatalf("GetPullRequestThreads: %v", err)
	}

	// Threads 1 (system bot) and 2 (text) survive. Thread 3 (codeChange) is dropped.
	if len(threads) != 2 {
		t.Fatalf("want 2 threads (system bot + text), got %d: %+v", len(threads), threads)
	}
	gotIDs := []int{threads[0].ID, threads[1].ID}
	for _, want := range []int{1, 2} {
		found := false
		for _, id := range gotIDs {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing thread id %d in result %+v", want, gotIDs)
		}
	}
}

// Comment.ID is needed downstream so reply writes can populate
// parentCommentId. Without it we can't address an existing comment.
func TestGetPullRequestThreads_PopulatesCommentID(t *testing.T) {
	body := `{"value":[{
		"id": 7, "status": "active",
		"comments": [{"id": 101, "content": "first", "commentType": "text", "author": {"displayName": "Alice"}, "publishedDate": "2026-04-27T00:00:00Z"}]
	}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
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

func TestPostPRThread_PostsExpectedBody(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 999, "status": "active", "comments": [{"id": 1, "content": "hi", "author": {"displayName": "Me"}, "commentType": "text", "publishedDate": "2026-04-29T00:00:00Z"}]}`))
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	th, err := c.PostPRThread(context.Background(), "myrepo", 42, "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method: got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/repositories/myrepo/pullrequests/42/threads") {
		t.Fatalf("path: got %s", gotPath)
	}
	comments, _ := gotBody["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("body comments: %+v", gotBody)
	}
	first, _ := comments[0].(map[string]any)
	if first["content"] != "hi" {
		t.Fatalf("body content: %+v", first)
	}
	if _, has := gotBody["threadContext"]; has {
		t.Fatalf("PR-level thread should omit threadContext: %+v", gotBody)
	}
	if th.ID != 999 {
		t.Fatalf("expected returned thread.ID=999, got %d", th.ID)
	}
}

func TestPostPRThread_FileLevelIncludesThreadContext(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id": 1000, "status": "active", "comments": []}`))
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	_, err := c.PostPRThread(context.Background(), "myrepo", 42, "note", "/src/foo.go")
	if err != nil {
		t.Fatal(err)
	}
	tc, _ := gotBody["threadContext"].(map[string]any)
	if tc == nil || tc["filePath"] != "/src/foo.go" {
		t.Fatalf("file-level thread missing threadContext.filePath: %+v", gotBody)
	}
}

func TestPostThreadComment_PostsToCommentsEndpoint(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id": 555, "content": "reply", "author": {"displayName": "Me"}, "commentType": "text", "publishedDate": "2026-04-29T00:00:00Z"}`))
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	cm, err := c.PostThreadComment(context.Background(), "repo", 42, 7, "reply")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/threads/7/comments") {
		t.Fatalf("path: got %s", gotPath)
	}
	if pid, _ := gotBody["parentCommentId"].(float64); pid != 1 {
		t.Fatalf("reply must address parentCommentId=1 (root): %+v", gotBody)
	}
	if cm.ID != 555 {
		t.Fatalf("expected comment.ID=555, got %d", cm.ID)
	}
}

func TestPatchThreadStatus_SendsExpectedPatch(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id":7,"status":"fixed"}`))
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	if err := c.PatchThreadStatus(context.Background(), "repo", 42, 7, "fixed"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method: got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/threads/7") {
		t.Fatalf("path: got %s", gotPath)
	}
	if gotBody["status"] != "fixed" {
		t.Fatalf("body: %+v", gotBody)
	}
}

func TestPatchThreadStatus_RejectsUnknownStatus(t *testing.T) {
	c := NewClient("ignored", &fakeTokens{})
	err := c.PatchThreadStatus(context.Background(), "r", 1, 1, "bogus")
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

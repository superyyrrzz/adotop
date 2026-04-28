package ado

import (
	"context"
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

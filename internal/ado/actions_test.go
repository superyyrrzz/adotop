package ado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetReviewerVoteSendsPutWithBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	if err := c.SetReviewerVote(context.Background(), "repo-uuid", 1234, "me-uuid", 10); err != nil {
		t.Fatalf("SetReviewerVote: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if !strings.Contains(gotPath, "/repositories/repo-uuid/pullrequests/1234/reviewers/me-uuid") {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["vote"].(float64) != 10 || gotBody["id"].(string) != "me-uuid" {
		t.Fatalf("body = %+v", gotBody)
	}
}

func TestAbandonPullRequestSendsPatchWithStatus(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	if err := c.AbandonPullRequest(context.Background(), "repo-uuid", 1234); err != nil {
		t.Fatalf("AbandonPullRequest: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %q, want PATCH", gotMethod)
	}
	if gotBody["status"].(string) != "abandoned" {
		t.Fatalf("body = %+v", gotBody)
	}
}

func TestSetReviewerVoteValidatesArgs(t *testing.T) {
	c := NewClient("ignored", &fakeTokens{})
	if err := c.SetReviewerVote(context.Background(), "", 1, "me", 10); err == nil {
		t.Fatal("expected error for empty repoID")
	}
	if err := c.SetReviewerVote(context.Background(), "r", 0, "me", 10); err == nil {
		t.Fatal("expected error for zero prID")
	}
	if err := c.SetReviewerVote(context.Background(), "r", 1, "", 10); err == nil {
		t.Fatal("expected error for empty myID")
	}
}

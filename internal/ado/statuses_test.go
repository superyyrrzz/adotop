package ado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/_apis/git/repositories/repo-uuid/pullrequests/1234/statuses") {
			t.Fatalf("path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"context": map[string]any{"name": "ci", "genre": "build"}, "state": "succeeded"},
				{"context": map[string]any{"name": "sonar"}, "state": "pending"},
			},
		})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	got, err := c.GetStatuses(context.Background(), "repo-uuid", 1234)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Context != "build/ci" || got[0].State != "succeeded" {
		t.Fatalf("statuses: %+v", got)
	}
	if got[1].Context != "sonar" || got[1].State != "pending" {
		t.Fatalf("statuses[1]: %+v", got[1])
	}
}

// TestGetStatusesDedupesByContext is the regression for the "stale CI
// state in detail header" report. ADO's /statuses endpoint returns the
// FULL history of every status post — for a PR with N retries you get N
// rows, often with old "pending" entries lingering after a "succeeded"
// reply landed. We must keep only the latest (by updatedDate) per
// (genre, name) so the header reflects current state.
func TestGetStatusesDedupesByContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				// Three entries for the same context; the freshest is "succeeded".
				{"context": map[string]any{"name": "ci", "genre": "build"}, "state": "pending", "updatedDate": "2026-04-28T08:59:10Z"},
				{"context": map[string]any{"name": "ci", "genre": "build"}, "state": "pending", "updatedDate": "2026-04-28T09:00:00Z"},
				{"context": map[string]any{"name": "ci", "genre": "build"}, "state": "succeeded", "updatedDate": "2026-04-28T10:00:00Z"},
				// And one fresher pending for a different context that should pass through.
				{"context": map[string]any{"name": "policy"}, "state": "pending", "updatedDate": "2026-04-28T10:00:00Z"},
			},
		})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	got, err := c.GetStatuses(context.Background(), "r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped statuses, got %d: %+v", len(got), got)
	}
	// Sorted alphabetically: build/ci first, policy second.
	if got[0].Context != "build/ci" || got[0].State != "succeeded" {
		t.Fatalf("build/ci should be the freshest succeeded entry, got %+v", got[0])
	}
	if got[1].Context != "policy" || got[1].State != "pending" {
		t.Fatalf("policy should pass through, got %+v", got[1])
	}
}

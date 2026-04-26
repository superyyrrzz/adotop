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

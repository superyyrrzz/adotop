package ado

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListPullRequestsAssignedTab(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{
					"pullRequestId": 1234,
					"title":         "Fix login bug",
					"sourceRefName": "refs/heads/feat/login",
					"targetRefName": "refs/heads/main",
					"creationDate":  "2026-04-25T10:00:00Z",
					"isDraft":       false,
					"mergeStatus":   "succeeded",
					"createdBy":     map[string]any{"displayName": "alice"},
					"repository":    map[string]any{"id": "repo-uuid", "name": "MyRepo"},
					"reviewers": []map[string]any{
						{"displayName": "bob", "vote": 10, "isRequired": true},
					},
					"_links": map[string]any{"web": map[string]any{"href": "https://dev.azure.com/x/_git/MyRepo/pullrequest/1234"}},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	prs, err := c.ListPullRequests(context.Background(), ListPRFilter{Project: "Engineering", Tab: TabAssigned, MyID: "me-uuid"})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 || prs[0].ID != 1234 || prs[0].Title != "Fix login bug" {
		t.Fatalf("prs = %+v", prs)
	}
	if prs[0].SourceBranch != "feat/login" || prs[0].TargetBranch != "main" {
		t.Fatalf("branches: %+v", prs[0])
	}
	if prs[0].Repo != "MyRepo" || prs[0].RepoID != "repo-uuid" {
		t.Fatalf("repo: %+v", prs[0])
	}
	if len(prs[0].Reviewers) != 1 || prs[0].Reviewers[0].Vote != 10 || !prs[0].Reviewers[0].IsRequired {
		t.Fatalf("reviewers: %+v", prs[0].Reviewers)
	}

	if !strings.Contains(gotPath, "/Engineering/_apis/git/pullrequests") {
		t.Fatalf("path = %q", gotPath)
	}
	q, _ := url.ParseQuery(gotQuery)
	if q.Get("searchCriteria.reviewerId") != "me-uuid" {
		t.Fatalf("reviewerId not set: %s", gotQuery)
	}
	if q.Get("searchCriteria.status") != "active" {
		t.Fatalf("status not active: %s", gotQuery)
	}
}

func TestListPullRequestsCreatedTab(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]any{"value": []any{}})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	if _, err := c.ListPullRequests(context.Background(), ListPRFilter{Project: "Engineering", Tab: TabCreated, MyID: "me-uuid"}); err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(gotQuery)
	if q.Get("searchCriteria.creatorId") != "me-uuid" {
		t.Fatalf("creatorId not set: %s", gotQuery)
	}
	if q.Get("searchCriteria.reviewerId") != "" {
		t.Fatalf("reviewerId should not be set on Created tab")
	}
}

func TestListPullRequestsReviewRequestedFiltersClientSide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"pullRequestId": 1, "title": "voted", "createdBy": map[string]any{"displayName": "x"}, "repository": map[string]any{"id": "r", "name": "R"}, "creationDate": "2026-04-25T10:00:00Z",
					"reviewers": []map[string]any{{"id": "me-uuid", "vote": 10}},
				},
				{"pullRequestId": 2, "title": "no vote", "createdBy": map[string]any{"displayName": "x"}, "repository": map[string]any{"id": "r", "name": "R"}, "creationDate": "2026-04-25T10:00:00Z",
					"reviewers": []map[string]any{{"id": "me-uuid", "vote": 0}},
				},
			},
		})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	prs, err := c.ListPullRequests(context.Background(), ListPRFilter{Project: "Engineering", Tab: TabReviewRequested, MyID: "me-uuid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != 2 {
		t.Fatalf("expected only PR 2 (no vote yet), got %+v", prs)
	}
}

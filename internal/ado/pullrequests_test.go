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

func TestListPullRequestsAssignedFiltersUnvotedClientSide(t *testing.T) {
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
	prs, err := c.ListPullRequests(context.Background(), ListPRFilter{Project: "Engineering", Tab: TabAssigned, MyID: "me-uuid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].ID != 2 {
		t.Fatalf("Assigned should hide voted PRs, got %+v", prs)
	}
}

func TestListPullRequestsAllReviewingKeepsVoted(t *testing.T) {
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
	if len(prs) != 2 {
		t.Fatalf("All-reviewing should keep both PRs, got %+v", prs)
	}
}

func TestGetPullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/_apis/git/repositories/repo-uuid/pullrequests/1234") {
			t.Fatalf("path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"pullRequestId": 1234,
			"title":         "Fix login bug",
			"description":   "## Why\nFixes thing.",
			"sourceRefName": "refs/heads/feat/login",
			"targetRefName": "refs/heads/main",
			"creationDate":  "2026-04-25T10:00:00Z",
			"createdBy":     map[string]any{"displayName": "alice"},
			"repository":    map[string]any{"id": "repo-uuid", "name": "MyRepo"},
			"lastMergeSourceCommit": map[string]any{"commitId": "src-sha"},
			"lastMergeTargetCommit": map[string]any{"commitId": "tgt-sha"},
			"workItemRefs":  []map[string]any{{"id": "98765", "url": "https://x/_apis/wit/workItems/98765"}},
			"_links":        map[string]any{"web": map[string]any{"href": "https://x"}},
		})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	d, err := c.GetPullRequest(context.Background(), "repo-uuid", 1234, "me-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if d.DescriptionMD != "## Why\nFixes thing." {
		t.Fatalf("desc: %q", d.DescriptionMD)
	}
	if d.SourceSha != "src-sha" || d.TargetSha != "tgt-sha" {
		t.Fatalf("shas: %+v", d)
	}
	if len(d.WorkItemRefs) != 1 || d.WorkItemRefs[0].ID != "98765" {
		t.Fatalf("workitems: %+v", d.WorkItemRefs)
	}
}

func TestGetIterationChanges(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/iterations"):
			json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": 1}, {"id": 2}, {"id": 3},
				},
			})
		case strings.Contains(r.URL.Path, "/iterations/3/changes"):
			json.NewEncoder(w).Encode(map[string]any{
				"changeEntries": []map[string]any{
					{"changeType": "edit", "item": map[string]any{"path": "/src/login.go"}},
					{"changeType": "add", "item": map[string]any{"path": "/src/new.go"}},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	files, err := c.GetIterationChanges(context.Background(), "repo-uuid", 1234)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "/src/login.go" || files[0].ChangeType != "edit" {
		t.Fatalf("files: %+v", files)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls (iterations + changes), got %v", calls)
	}
}

func TestGetPullRequestByIDHitsOrgScopedPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{
			"pullRequestId": 999,
			"title":         "by id",
			"sourceRefName": "refs/heads/x",
			"targetRefName": "refs/heads/main",
			"creationDate":  "2026-04-25T10:00:00Z",
			"createdBy":     map[string]any{"displayName": "alice"},
			"repository": map[string]any{
				"id":      "repo-uuid",
				"name":    "MyRepo",
				"project": map[string]any{"name": "Engineering"},
			},
			"lastMergeSourceCommit": map[string]any{"commitId": "src"},
			"lastMergeTargetCommit": map[string]any{"commitId": "tgt"},
		})
	}))
	defer srv.Close()

	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	d, err := c.GetPullRequestByID(context.Background(), 999, "me-uuid")
	if err != nil {
		t.Fatalf("GetPullRequestByID: %v", err)
	}
	if !strings.Contains(gotPath, "/_apis/git/pullrequests/999") {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if d.ID != 999 || d.Repo != "MyRepo" || d.RepoID != "repo-uuid" {
		t.Fatalf("decoded wrong: %+v", d)
	}
	if d.URL == "" {
		t.Fatalf("expected synthesized URL")
	}
}

func TestListPullRequestsSynthesizesURLWhenLinksMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{
					"pullRequestId": 42,
					"title":         "no links",
					"sourceRefName": "refs/heads/x",
					"targetRefName": "refs/heads/main",
					"creationDate":  "2026-04-25T10:00:00Z",
					"createdBy":     map[string]any{"displayName": "alice"},
					"repository": map[string]any{
						"id":      "repo-uuid",
						"name":    "MyRepo",
						"project": map[string]any{"name": "Engineering"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("ceapex", &fakeTokens{})
	c.BaseURL = srv.URL
	prs, err := c.ListPullRequests(context.Background(), ListPRFilter{Project: "Engineering", Tab: TabCreated, MyID: "me"})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len = %d", len(prs))
	}
	want := "https://dev.azure.com/" // org segment depends on test BaseURL; just ensure non-empty + contains the canonical pullrequest path.
	if prs[0].URL == "" {
		t.Fatalf("expected synthesized URL when _links is absent, got empty")
	}
	if !strings.Contains(prs[0].URL, "/_git/MyRepo/pullrequest/42") {
		t.Fatalf("synthesized URL missing canonical suffix: %q", prs[0].URL)
	}
	_ = want
}

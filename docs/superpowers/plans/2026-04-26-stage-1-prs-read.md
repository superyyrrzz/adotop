# Stage 1 — PRs (read) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Stage 1 read-only PR dashboard for adotop: three filter tabs across the configured org+project, detail view with reviewers + status + files, and a diff viewer that prefers a local clone via `delta` and falls back to ADO REST.

**Architecture:** A single root Bubble Tea model coordinates three screens (`list`, `detail`, `diff`) via a `screen` enum. Per-screen update logic lives in its own file. ADO REST calls are typed cmds returning typed messages. A 60 s ticker fires list refreshes only when the list screen is active. Local-clone discovery walks user-configured `repo_roots`.

**Tech Stack:** Go (latest), Bubble Tea + Bubbles + Lipgloss, Glamour for markdown, BurntSushi/toml. Existing `internal/ado` client (auth + retries) is reused.

**Spec:** `docs/superpowers/specs/2026-04-26-stage-1-prs-read-design.md`

---

## File map

**Create**
- `internal/ado/pullrequests.go` — list + detail + iterations/changes API
- `internal/ado/pullrequests_test.go` — `httptest` coverage of the above
- `internal/ado/statuses.go` — PR status checks API
- `internal/ado/statuses_test.go`
- `internal/ado/diff.go` — REST diff fetcher
- `internal/ado/diff_test.go`
- `internal/gitlocal/finder.go` — discover local clones via `repo_roots`
- `internal/gitlocal/finder_test.go`
- `internal/gitlocal/diff.go` — shell out to `git diff` (+ `delta` if present)
- `internal/gitlocal/diff_test.go`
- `internal/ui/keys.go` — centralized key bindings
- `internal/ui/styles.go` — centralized lipgloss styles
- `internal/ui/list.go` — list screen sub-model
- `internal/ui/list_test.go`
- `internal/ui/detail.go` — detail screen sub-model
- `internal/ui/detail_test.go`
- `internal/ui/diff.go` — diff screen sub-model
- `internal/ui/diff_test.go`
- `internal/ui/browser.go` — `o` open-in-browser helper (cross-platform)

**Modify**
- `internal/config/config.go` — add `RepoRoots []string` field
- `internal/config/config_test.go` — cover the new field
- `internal/ui/app.go` — replace placeholder shell with the screen-coordinator root model
- `cmd/adotop/main.go` — pass `cfg.Project` and the connectionData user id to the UI

---

## Task 1: Config — add `repo_roots`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for repo_roots parsing**

Append to `internal/config/config_test.go`:

```go
func TestLoadRepoRoots(t *testing.T) {
	dir := t.TempDir()
	setHome(t, dir)
	p := filepath.Join(dir, ".adotop", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`org = "ceapex"
project = "Engineering"
repo_roots = ["~/git", "~/src"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.RepoRoots) != 2 || cfg.RepoRoots[0] != "~/git" || cfg.RepoRoots[1] != "~/src" {
		t.Fatalf("RepoRoots = %v", cfg.RepoRoots)
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/config/ -run TestLoadRepoRoots -v`
Expected: FAIL — `cfg.RepoRoots` undefined.

- [ ] **Step 3: Add the field**

In `internal/config/config.go`, modify the `Config` struct:

```go
type Config struct {
	Org             string            `toml:"org"`
	Project         string            `toml:"project"`
	RefreshInterval Duration          `toml:"refresh_interval"`
	RepoRoots       []string          `toml:"repo_roots"`
	Keybindings     map[string]string `toml:"keybindings"`
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS, all config tests green.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add repo_roots for local-clone diff discovery"
```

---

## Task 2: ADO — list PRs

**Files:**
- Create: `internal/ado/pullrequests.go`
- Create: `internal/ado/pullrequests_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ado/pullrequests_test.go`:

```go
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
```

- [ ] **Step 2: Run, verify it fails**

Run: `go test ./internal/ado/ -run TestListPullRequests -v`
Expected: FAIL — `ListPRFilter`, `TabAssigned`, etc. undefined.

- [ ] **Step 3: Implement**

Create `internal/ado/pullrequests.go`:

```go
package ado

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Tab int

const (
	TabAssigned Tab = iota
	TabCreated
	TabReviewRequested
)

func (t Tab) String() string {
	switch t {
	case TabAssigned:
		return "Assigned"
	case TabCreated:
		return "Created"
	case TabReviewRequested:
		return "Review requested"
	}
	return "?"
}

type ListPRFilter struct {
	Project string
	Tab     Tab
	MyID    string
	Top     int // default 100
}

type ReviewerVote struct {
	ID          string
	DisplayName string
	Vote        int
	IsRequired  bool
}

type PRSummary struct {
	ID           int
	Title        string
	Repo         string
	RepoID       string
	SourceBranch string
	TargetBranch string
	CreatedAt    time.Time
	Author       string
	URL          string
	Reviewers    []ReviewerVote
	MyVote       int
	Draft        bool
	MergeStatus  string
}

type rawPR struct {
	PullRequestID int    `json:"pullRequestId"`
	Title         string `json:"title"`
	SourceRefName string `json:"sourceRefName"`
	TargetRefName string `json:"targetRefName"`
	CreationDate  string `json:"creationDate"`
	IsDraft       bool   `json:"isDraft"`
	MergeStatus   string `json:"mergeStatus"`
	CreatedBy     struct {
		DisplayName string `json:"displayName"`
	} `json:"createdBy"`
	Repository struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"repository"`
	Reviewers []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Vote        int    `json:"vote"`
		IsRequired  bool   `json:"isRequired"`
	} `json:"reviewers"`
	Links struct {
		Web struct {
			Href string `json:"href"`
		} `json:"web"`
	} `json:"_links"`
}

type listResponse struct {
	Value []rawPR `json:"value"`
}

func (c *Client) ListPullRequests(ctx context.Context, f ListPRFilter) ([]PRSummary, error) {
	if f.Project == "" {
		return nil, fmt.Errorf("ListPullRequests: project required")
	}
	if f.MyID == "" {
		return nil, fmt.Errorf("ListPullRequests: MyID required")
	}
	top := f.Top
	if top <= 0 {
		top = 100
	}
	q := url.Values{}
	q.Set("searchCriteria.status", "active")
	q.Set("$top", fmt.Sprintf("%d", top))
	switch f.Tab {
	case TabCreated:
		q.Set("searchCriteria.creatorId", f.MyID)
	case TabAssigned, TabReviewRequested:
		q.Set("searchCriteria.reviewerId", f.MyID)
	}
	path := "/" + url.PathEscape(f.Project) + "/_apis/git/pullrequests?" + q.Encode()

	var resp listResponse
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	out := make([]PRSummary, 0, len(resp.Value))
	for _, r := range resp.Value {
		s := toSummary(r, f.MyID)
		if f.Tab == TabReviewRequested && s.MyVote != 0 {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func toSummary(r rawPR, myID string) PRSummary {
	t, _ := time.Parse(time.RFC3339, r.CreationDate)
	s := PRSummary{
		ID:           r.PullRequestID,
		Title:        r.Title,
		Repo:         r.Repository.Name,
		RepoID:       r.Repository.ID,
		SourceBranch: strings.TrimPrefix(r.SourceRefName, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(r.TargetRefName, "refs/heads/"),
		CreatedAt:    t,
		Author:       r.CreatedBy.DisplayName,
		URL:          r.Links.Web.Href,
		Draft:        r.IsDraft,
		MergeStatus:  r.MergeStatus,
	}
	for _, rv := range r.Reviewers {
		s.Reviewers = append(s.Reviewers, ReviewerVote{
			ID: rv.ID, DisplayName: rv.DisplayName, Vote: rv.Vote, IsRequired: rv.IsRequired,
		})
		if rv.ID == myID {
			s.MyVote = rv.Vote
		}
	}
	return s
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/ado/ -v`
Expected: all green (existing tests + 3 new).

- [ ] **Step 5: Commit**

```bash
git add internal/ado/pullrequests.go internal/ado/pullrequests_test.go
git commit -m "feat(ado): list pull requests with tab-aware filters"
```

---

## Task 3: ADO — PR detail + iterations + changes

**Files:**
- Modify: `internal/ado/pullrequests.go`
- Modify: `internal/ado/pullrequests_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/ado/pullrequests_test.go`:

```go
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
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/ado/ -run "TestGetPullRequest|TestGetIterationChanges" -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement**

Append to `internal/ado/pullrequests.go`:

```go
type WorkItemRef struct {
	ID  string
	URL string
}

type FileChange struct {
	Path       string
	ChangeType string // "edit" | "add" | "delete" | "rename"
}

type PRDetail struct {
	PRSummary
	DescriptionMD string
	WorkItemRefs  []WorkItemRef
	SourceSha     string
	TargetSha     string
}

type rawPRDetail struct {
	rawPR
	Description           string `json:"description"`
	LastMergeSourceCommit struct {
		CommitID string `json:"commitId"`
	} `json:"lastMergeSourceCommit"`
	LastMergeTargetCommit struct {
		CommitID string `json:"commitId"`
	} `json:"lastMergeTargetCommit"`
	WorkItemRefs []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"workItemRefs"`
}

func (c *Client) GetPullRequest(ctx context.Context, repoID string, prID int, myID string) (*PRDetail, error) {
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d?includeWorkItemRefs=true",
		url.PathEscape(repoID), prID)
	var r rawPRDetail
	if err := c.GetJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	d := &PRDetail{
		PRSummary:     toSummary(r.rawPR, myID),
		DescriptionMD: r.Description,
		SourceSha:     r.LastMergeSourceCommit.CommitID,
		TargetSha:     r.LastMergeTargetCommit.CommitID,
	}
	for _, w := range r.WorkItemRefs {
		d.WorkItemRefs = append(d.WorkItemRefs, WorkItemRef{ID: w.ID, URL: w.URL})
	}
	return d, nil
}

type rawIteration struct {
	ID int `json:"id"`
}
type rawIterationsResp struct {
	Value []rawIteration `json:"value"`
}
type rawChangeEntry struct {
	ChangeType string `json:"changeType"`
	Item       struct {
		Path string `json:"path"`
	} `json:"item"`
}
type rawChangesResp struct {
	ChangeEntries []rawChangeEntry `json:"changeEntries"`
}

func (c *Client) GetIterationChanges(ctx context.Context, repoID string, prID int) ([]FileChange, error) {
	itPath := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/iterations",
		url.PathEscape(repoID), prID)
	var its rawIterationsResp
	if err := c.GetJSON(ctx, itPath, &its); err != nil {
		return nil, err
	}
	if len(its.Value) == 0 {
		return nil, nil
	}
	latest := its.Value[len(its.Value)-1].ID
	chPath := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/iterations/%d/changes",
		url.PathEscape(repoID), prID, latest)
	var ch rawChangesResp
	if err := c.GetJSON(ctx, chPath, &ch); err != nil {
		return nil, err
	}
	out := make([]FileChange, 0, len(ch.ChangeEntries))
	for _, e := range ch.ChangeEntries {
		out = append(out, FileChange{Path: e.Item.Path, ChangeType: e.ChangeType})
	}
	return out, nil
}
```

- [ ] **Step 4: Run, verify pass**

Run: `go test ./internal/ado/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/ado/pullrequests.go internal/ado/pullrequests_test.go
git commit -m "feat(ado): get PR detail and iteration file changes"
```

---

## Task 4: ADO — PR statuses

**Files:**
- Create: `internal/ado/statuses.go`
- Create: `internal/ado/statuses_test.go`

- [ ] **Step 1: Failing test**

Create `internal/ado/statuses_test.go`:

```go
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
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/ado/ -run TestGetStatuses -v`
Expected: FAIL — `GetStatuses` undefined.

- [ ] **Step 3: Implement**

Create `internal/ado/statuses.go`:

```go
package ado

import (
	"context"
	"fmt"
	"net/url"
)

type StatusCheck struct {
	Context string
	State   string // "succeeded" | "pending" | "failed" | "error" | "notSet"
}

type rawStatus struct {
	State   string `json:"state"`
	Context struct {
		Name  string `json:"name"`
		Genre string `json:"genre"`
	} `json:"context"`
}

type rawStatusesResp struct {
	Value []rawStatus `json:"value"`
}

func (c *Client) GetStatuses(ctx context.Context, repoID string, prID int) ([]StatusCheck, error) {
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/statuses",
		url.PathEscape(repoID), prID)
	var r rawStatusesResp
	if err := c.GetJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	out := make([]StatusCheck, 0, len(r.Value))
	for _, s := range r.Value {
		ctxName := s.Context.Name
		if s.Context.Genre != "" {
			ctxName = s.Context.Genre + "/" + s.Context.Name
		}
		out = append(out, StatusCheck{Context: ctxName, State: s.State})
	}
	return out, nil
}
```

- [ ] **Step 4: Pass**

Run: `go test ./internal/ado/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/ado/statuses.go internal/ado/statuses_test.go
git commit -m "feat(ado): fetch PR status checks"
```

---

## Task 5: ADO — REST diff fetcher

**Files:**
- Create: `internal/ado/diff.go`
- Create: `internal/ado/diff_test.go`

- [ ] **Step 1: Failing test**

Create `internal/ado/diff_test.go`:

```go
package ado

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetFileDiffBothSides(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/_apis/git/repositories/repo-uuid/items") {
			t.Fatalf("path: %s", r.URL.Path)
		}
		v := r.URL.Query().Get("versionDescriptor.version")
		var body string
		switch v {
		case "src-sha":
			body = "new line\nshared\n"
		case "tgt-sha":
			body = "old line\nshared\n"
		default:
			t.Fatalf("unexpected version: %s", v)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content":         base64.StdEncoding.EncodeToString([]byte(body)),
			"contentMetadata": map[string]any{"encoding": "base64"},
		})
	}))
	defer srv.Close()
	c := NewClient("ignored", &fakeTokens{})
	c.BaseURL = srv.URL
	src, tgt, err := c.GetFileContents(context.Background(), "repo-uuid", "/src/login.go", "src-sha", "tgt-sha")
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != "new line\nshared\n" || string(tgt) != "old line\nshared\n" {
		t.Fatalf("src=%q tgt=%q", src, tgt)
	}
}
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/ado/ -run TestGetFileDiff -v`
Expected: FAIL — `GetFileContents` undefined.

- [ ] **Step 3: Implement**

Create `internal/ado/diff.go`:

```go
package ado

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
)

type rawItemContent struct {
	Content         string `json:"content"`
	ContentMetadata struct {
		Encoding string `json:"encoding"`
	} `json:"contentMetadata"`
}

func (c *Client) getFileAtCommit(ctx context.Context, repoID, path, sha string) ([]byte, error) {
	q := url.Values{}
	q.Set("path", path)
	q.Set("versionDescriptor.version", sha)
	q.Set("versionDescriptor.versionType", "commit")
	q.Set("includeContent", "true")
	q.Set("$format", "json")
	p := fmt.Sprintf("/_apis/git/repositories/%s/items?%s", url.PathEscape(repoID), q.Encode())
	var r rawItemContent
	if err := c.GetJSON(ctx, p, &r); err != nil {
		return nil, err
	}
	if r.ContentMetadata.Encoding == "base64" {
		return base64.StdEncoding.DecodeString(r.Content)
	}
	return []byte(r.Content), nil
}

// GetFileContents returns the source-side and target-side bytes for the file.
// A side may be empty bytes if the file did not exist there (added/deleted).
func (c *Client) GetFileContents(ctx context.Context, repoID, path, sourceSha, targetSha string) (src, tgt []byte, err error) {
	src, err = c.getFileAtCommit(ctx, repoID, path, sourceSha)
	if err != nil {
		// File missing on source = treat as empty (deleted).
		if isNotFound(err) {
			src = nil
		} else {
			return nil, nil, err
		}
	}
	tgt, err = c.getFileAtCommit(ctx, repoID, path, targetSha)
	if err != nil {
		if isNotFound(err) {
			tgt = nil
		} else {
			return nil, nil, err
		}
	}
	return src, tgt, nil
}

func isNotFound(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.Status == 404
	}
	return false
}
```

- [ ] **Step 4: Pass**

Run: `go test ./internal/ado/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/ado/diff.go internal/ado/diff_test.go
git commit -m "feat(ado): fetch file contents at commit for REST diff fallback"
```

---

## Task 6: gitlocal — find local clones

**Files:**
- Create: `internal/gitlocal/finder.go`
- Create: `internal/gitlocal/finder_test.go`

- [ ] **Step 1: Failing test**

Create `internal/gitlocal/finder_test.go`:

```go
package gitlocal

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func TestFindMatchesByRemote(t *testing.T) {
	gitOK(t)
	root := t.TempDir()
	repo := filepath.Join(root, "MyRepo")
	mustRun(t, "", "git", "init", "-b", "main", repo)
	mustRun(t, repo, "git", "remote", "add", "origin", "https://dev.azure.com/ceapex/Engineering/_git/MyRepo")

	f := New([]string{root})
	got, ok := f.Find("MyRepo", "ceapex")
	if !ok {
		t.Fatal("expected to find MyRepo")
	}
	if got != repo {
		t.Fatalf("got %q want %q", got, repo)
	}
}

func TestFindRejectsWrongRemote(t *testing.T) {
	gitOK(t)
	root := t.TempDir()
	repo := filepath.Join(root, "MyRepo")
	mustRun(t, "", "git", "init", "-b", "main", repo)
	mustRun(t, repo, "git", "remote", "add", "origin", "https://github.com/x/MyRepo")

	f := New([]string{root})
	if _, ok := f.Find("MyRepo", "ceapex"); ok {
		t.Fatal("expected no match — remote points elsewhere")
	}
}

func TestFindCachesLookup(t *testing.T) {
	gitOK(t)
	root := t.TempDir()
	repo := filepath.Join(root, "MyRepo")
	mustRun(t, "", "git", "init", "-b", "main", repo)
	mustRun(t, repo, "git", "remote", "add", "origin", "https://dev.azure.com/ceapex/_git/MyRepo")

	f := New([]string{root})
	if _, ok := f.Find("MyRepo", "ceapex"); !ok {
		t.Fatal("first lookup failed")
	}
	// Removing the remote should not change the cached answer.
	mustRun(t, repo, "git", "remote", "remove", "origin")
	got, ok := f.Find("MyRepo", "ceapex")
	if !ok || got != repo {
		t.Fatalf("cached lookup lost: ok=%v got=%q", ok, got)
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/gitlocal/ -v`
Expected: FAIL — package missing.

- [ ] **Step 3: Implement**

Create `internal/gitlocal/finder.go`:

```go
// Package gitlocal discovers local clones of ADO repositories and uses them
// to render diffs faster than the REST API can.
package gitlocal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Finder struct {
	roots []string

	mu    sync.Mutex
	cache map[string]string // repoName -> path (empty string = "no match")
}

func New(roots []string) *Finder {
	return &Finder{roots: expandRoots(roots), cache: map[string]string{}}
}

func expandRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	home, _ := os.UserHomeDir()
	for _, r := range roots {
		if strings.HasPrefix(r, "~") && home != "" {
			r = filepath.Join(home, strings.TrimPrefix(r, "~"))
		}
		out = append(out, r)
	}
	return out
}

// Find returns the local clone path for repoName whose remote URL contains
// the given org. The boolean is false when no clone matches.
func (f *Finder) Find(repoName, org string) (string, bool) {
	f.mu.Lock()
	if v, hit := f.cache[repoName]; hit {
		f.mu.Unlock()
		return v, v != ""
	}
	f.mu.Unlock()

	path := f.search(repoName, org)
	f.mu.Lock()
	f.cache[repoName] = path
	f.mu.Unlock()
	return path, path != ""
}

func (f *Finder) search(repoName, org string) string {
	for _, root := range f.roots {
		candidate := filepath.Join(root, repoName)
		if !isGitRepo(candidate) {
			continue
		}
		if remoteMatches(candidate, org, repoName) {
			return candidate
		}
	}
	return ""
}

func isGitRepo(path string) bool {
	st, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return st.IsDir() || st.Mode().IsRegular() // submodule .git can be a file
}

func remoteMatches(path, org, repoName string) bool {
	cmd := exec.Command("git", "-C", path, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	needleOrg := "dev.azure.com/" + strings.ToLower(org)
	needleRepo := "/" + strings.ToLower(repoName)
	for _, line := range strings.Split(string(out), "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, needleOrg) && strings.Contains(l, needleRepo) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Pass**

Run: `go test ./internal/gitlocal/ -v`
Expected: all green (skips on machines without git).

- [ ] **Step 5: Commit**

```bash
git add internal/gitlocal/finder.go internal/gitlocal/finder_test.go
git commit -m "feat(gitlocal): discover local ADO clones via repo_roots"
```

---

## Task 7: gitlocal — render diff via git (+ delta)

**Files:**
- Create: `internal/gitlocal/diff.go`
- Create: `internal/gitlocal/diff_test.go`

- [ ] **Step 1: Failing test**

Create `internal/gitlocal/diff_test.go`:

```go
package gitlocal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffProducesUnifiedDiff(t *testing.T) {
	gitOK(t)
	dir := t.TempDir()
	mustRun(t, "", "git", "init", "-b", "main", dir)
	mustRun(t, dir, "git", "config", "user.email", "t@t")
	mustRun(t, dir, "git", "config", "user.name", "t")

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", "f.txt")
	mustRun(t, dir, "git", "commit", "-m", "init")
	tgt := commitSha(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nB\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", "f.txt")
	mustRun(t, dir, "git", "commit", "-m", "change")
	src := commitSha(t, dir)

	out, err := Diff(context.Background(), dir, tgt, src, "f.txt", false)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "-b") || !strings.Contains(s, "+B") {
		t.Fatalf("diff missing changes: %s", s)
	}
}

func commitSha(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/gitlocal/ -run TestDiff -v`
Expected: FAIL — `Diff` undefined.

- [ ] **Step 3: Implement**

Create `internal/gitlocal/diff.go`:

```go
package gitlocal

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

// Diff runs `git -C clonePath diff target..source -- file`. If useDelta is true
// and `delta` is on PATH, the diff is piped through it for syntax highlighting.
func Diff(ctx context.Context, clonePath, targetSha, sourceSha, file string, useDelta bool) ([]byte, error) {
	args := []string{"-C", clonePath, "diff", "--no-color"}
	if useDelta {
		args = []string{"-C", clonePath, "diff"} // delta wants color
	}
	args = append(args, targetSha+".."+sourceSha, "--", file)

	gitCmd := exec.CommandContext(ctx, "git", args...)

	if !useDelta {
		out, err := gitCmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	if _, err := exec.LookPath("delta"); err != nil {
		// fall back to plain
		return Diff(ctx, clonePath, targetSha, sourceSha, file, false)
	}

	pr, pw := io.Pipe()
	gitCmd.Stdout = pw
	deltaCmd := exec.CommandContext(ctx, "delta", "--paging=never")
	deltaCmd.Stdin = pr
	var buf bytes.Buffer
	deltaCmd.Stdout = &buf

	if err := gitCmd.Start(); err != nil {
		return nil, err
	}
	if err := deltaCmd.Start(); err != nil {
		return nil, err
	}
	gitErr := gitCmd.Wait()
	pw.Close()
	deltaErr := deltaCmd.Wait()
	if gitErr != nil {
		return nil, gitErr
	}
	if deltaErr != nil {
		return nil, deltaErr
	}
	return buf.Bytes(), nil
}

// HasDelta reports whether `delta` is on PATH.
func HasDelta() bool {
	_, err := exec.LookPath("delta")
	return err == nil
}
```

- [ ] **Step 4: Pass**

Run: `go test ./internal/gitlocal/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/gitlocal/diff.go internal/gitlocal/diff_test.go
git commit -m "feat(gitlocal): render diffs via git, optionally piped through delta"
```

---

## Task 8: UI — centralized keys + styles + browser opener

**Files:**
- Create: `internal/ui/keys.go`
- Create: `internal/ui/styles.go`
- Create: `internal/ui/browser.go`

- [ ] **Step 1: Add keybindings module**

Create `internal/ui/keys.go`:

```go
package ui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up, Down       key.Binding
	NextTab, PrevTab key.Binding
	Open, Back      key.Binding
	Refresh         key.Binding
	Browser         key.Binding
	Filter          key.Binding
	Help, Quit      key.Binding
	PgUp, PgDn      key.Binding
	GotoTop, GotoEnd key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down")),
		NextTab:  key.NewBinding(key.WithKeys("tab", "l")),
		PrevTab:  key.NewBinding(key.WithKeys("shift+tab", "h")),
		Open:     key.NewBinding(key.WithKeys("enter")),
		Back:     key.NewBinding(key.WithKeys("esc")),
		Refresh:  key.NewBinding(key.WithKeys("r")),
		Browser:  key.NewBinding(key.WithKeys("o")),
		Filter:   key.NewBinding(key.WithKeys("/")),
		Help:     key.NewBinding(key.WithKeys("?")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c")),
		PgUp:     key.NewBinding(key.WithKeys("pgup")),
		PgDn:     key.NewBinding(key.WithKeys("pgdown")),
		GotoTop:  key.NewBinding(key.WithKeys("g")),
		GotoEnd:  key.NewBinding(key.WithKeys("G")),
	}
}
```

- [ ] **Step 2: Add styles module**

Create `internal/ui/styles.go`:

```go
package ui

import "github.com/charmbracelet/lipgloss"

var (
	Header   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	Footer   = lipgloss.NewStyle().Faint(true)
	ErrLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	TabOn    = lipgloss.NewStyle().Bold(true).Underline(true)
	TabOff   = lipgloss.NewStyle().Faint(true)
	Selected = lipgloss.NewStyle().Reverse(true)
	Faint    = lipgloss.NewStyle().Faint(true)
	HelpBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	Approve  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	Reject   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	Wait     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	None     = lipgloss.NewStyle().Faint(true)
)
```

- [ ] **Step 3: Add browser opener**

Create `internal/ui/browser.go`:

```go
package ui

import (
	"os/exec"
	"runtime"
)

func OpenInBrowser(url string) error {
	if url == "" {
		return nil
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/keys.go internal/ui/styles.go internal/ui/browser.go
git commit -m "feat(ui): centralize keys/styles and add cross-platform browser opener"
```

---

## Task 9: UI — list screen sub-model

**Files:**
- Create: `internal/ui/list.go`
- Create: `internal/ui/list_test.go`

- [ ] **Step 1: Failing test**

Create `internal/ui/list_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/superyyrrzz/adotop/internal/ado"
)

func samplePRs() []ado.PRSummary {
	t := time.Now().Add(-2 * time.Hour)
	return []ado.PRSummary{
		{ID: 1234, Title: "Fix login bug", Repo: "MyRepo", SourceBranch: "feat/login", TargetBranch: "main", CreatedAt: t, Author: "alice", Reviewers: []ado.ReviewerVote{{DisplayName: "bob", Vote: 10, IsRequired: true}}},
		{ID: 1235, Title: "Add dark mode", Repo: "MyRepo", SourceBranch: "feat/theme", TargetBranch: "main", CreatedAt: t, Author: "bob", Reviewers: []ado.ReviewerVote{{DisplayName: "carol", Vote: 0}}},
	}
}

func TestListRendersPRs(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: samplePRs()})
	out := m.View()
	if !strings.Contains(out, "#1234") || !strings.Contains(out, "Fix login bug") {
		t.Fatalf("missing PR row:\n%s", out)
	}
	if !strings.Contains(out, "Assigned") {
		t.Fatalf("missing tab label:\n%s", out)
	}
}

func TestListFilterNarrowsRows(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: samplePRs()})
	// Press '/', type "dark", Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "dark" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	out := m.View()
	if strings.Contains(out, "#1234") {
		t.Fatalf("filter should hide #1234:\n%s", out)
	}
	if !strings.Contains(out, "#1235") {
		t.Fatalf("filter should keep #1235:\n%s", out)
	}
}

func TestListNextTabEmitsLoad(t *testing.T) {
	m := NewList(DefaultKeys())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected a load cmd after tab switch")
	}
	if m.tab != ado.TabCreated {
		t.Fatalf("tab = %v, want Created", m.tab)
	}
}
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/ui/ -run TestList -v`
Expected: FAIL — `NewList`, `prsLoadedMsg`, etc. undefined.

- [ ] **Step 3: Implement**

Create `internal/ui/list.go`:

```go
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ado"
)

type prsLoadedMsg struct {
	tab ado.Tab
	prs []ado.PRSummary
	err error
}

type ListModel struct {
	keys     KeyMap
	tab      ado.Tab
	prs      map[ado.Tab][]ado.PRSummary
	cursor   int
	filter   string
	filtering bool
	width    int
	loadErr  string
}

func NewList(keys KeyMap) ListModel {
	return ListModel{
		keys: keys,
		prs:  map[ado.Tab][]ado.PRSummary{},
	}
}

// LoadCmd is the command the parent should issue to (re)load the current tab.
type LoadCmd func(tab ado.Tab) tea.Cmd

func (m ListModel) Tab() ado.Tab { return m.tab }
func (m ListModel) Selected() (ado.PRSummary, bool) {
	rows := m.visible()
	if len(rows) == 0 {
		return ado.PRSummary{}, false
	}
	if m.cursor >= len(rows) {
		return rows[len(rows)-1], true
	}
	return rows[m.cursor], true
}

func (m ListModel) visible() []ado.PRSummary {
	all := m.prs[m.tab]
	if m.filter == "" {
		return all
	}
	q := strings.ToLower(m.filter)
	out := all[:0:0]
	for _, p := range all {
		hay := strings.ToLower(p.Title + " " + p.Author + " " + p.SourceBranch + " " + p.TargetBranch)
		if strings.Contains(hay, q) {
			out = append(out, p)
		}
	}
	return out
}

func (m ListModel) Update(msg tea.Msg) (ListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case prsLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else {
			m.loadErr = ""
			m.prs[msg.tab] = msg.prs
			if msg.tab == m.tab && m.cursor >= len(msg.prs) {
				m.cursor = 0
			}
		}
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		switch {
		case keyMatches(msg, m.keys.NextTab):
			m.tab = (m.tab + 1) % 3
			m.cursor = 0
			return m, tabSwitchCmd(m.tab)
		case keyMatches(msg, m.keys.PrevTab):
			m.tab = (m.tab + 2) % 3
			m.cursor = 0
			return m, tabSwitchCmd(m.tab)
		case keyMatches(msg, m.keys.Down):
			rows := m.visible()
			if m.cursor < len(rows)-1 {
				m.cursor++
			}
		case keyMatches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case keyMatches(msg, m.keys.Filter):
			m.filtering = true
			m.filter = ""
		}
	}
	return m, nil
}

func (m ListModel) updateFiltering(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filter = ""
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
	}
	m.cursor = 0
	return m, nil
}

// tabSwitchCmd is a sentinel: parent intercepts and dispatches the actual fetch.
type tabSwitchedMsg struct{ Tab ado.Tab }

func tabSwitchCmd(t ado.Tab) tea.Cmd {
	return func() tea.Msg { return tabSwitchedMsg{Tab: t} }
}

func keyMatches(msg tea.KeyMsg, b interface{ Keys() []string }) bool {
	got := msg.String()
	for _, k := range b.Keys() {
		if k == got {
			return true
		}
	}
	return false
}

func (m ListModel) View() string {
	var b strings.Builder
	tabs := []string{"Assigned", "Created", "Review requested"}
	for i, name := range tabs {
		count := len(m.prs[ado.Tab(i)])
		label := fmt.Sprintf(" %s (%d) ", name, count)
		if ado.Tab(i) == m.tab {
			b.WriteString(TabOn.Render(label))
		} else {
			b.WriteString(TabOff.Render(label))
		}
	}
	b.WriteString("\n\n")

	rows := m.visible()
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render("error: " + m.loadErr))
		b.WriteString("\n")
	} else if len(rows) == 0 {
		b.WriteString(Faint.Render("No PRs in this tab.\n"))
	} else {
		for i, p := range rows {
			line := fmt.Sprintf("#%-5d %-40s %-12s %s → %s   %s",
				p.ID, truncate(p.Title, 40), truncate(p.Author, 12),
				p.SourceBranch, p.TargetBranch, age(p.CreatedAt))
			if p.Draft {
				line += "  [DRAFT]"
			}
			if i == m.cursor {
				line = Selected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
			b.WriteString("    ")
			b.WriteString(voteGlyphs(p.Reviewers))
			b.WriteString("\n")
		}
	}

	if m.filtering {
		b.WriteString("\n/" + m.filter + lipgloss.NewStyle().Faint(true).Render("█"))
	}
	return b.String()
}

func voteGlyphs(rs []ado.ReviewerVote) string {
	var b strings.Builder
	for _, r := range rs {
		var g string
		switch {
		case r.Vote >= 10:
			g = Approve.Render("✓")
		case r.Vote >= 5:
			g = Approve.Render("✓~")
		case r.Vote <= -10:
			g = Reject.Render("✗")
		case r.Vote <= -5:
			g = Wait.Render("⏳")
		default:
			g = None.Render("·")
		}
		if r.IsRequired {
			g = lipgloss.NewStyle().Bold(true).Render(g)
		}
		b.WriteString(g)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
```

- [ ] **Step 4: Run tests, pass**

Run: `go test ./internal/ui/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/list.go internal/ui/list_test.go
git commit -m "feat(ui): list screen with tabs, filter, vote glyphs"
```

---

## Task 10: UI — detail screen sub-model

**Files:**
- Create: `internal/ui/detail.go`
- Create: `internal/ui/detail_test.go`

- [ ] **Step 1: Failing test**

Create `internal/ui/detail_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

func TestDetailRendersDescriptionAndFiles(t *testing.T) {
	m := NewDetail(DefaultKeys())
	m = m.SetSummary(ado.PRSummary{ID: 1234, Title: "Fix login bug", Author: "alice", SourceBranch: "feat/login", TargetBranch: "main"})
	m, _ = m.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary:     ado.PRSummary{ID: 1234, Title: "Fix login bug"},
		DescriptionMD: "Fixes the issue where session tokens were not refreshed.",
	}})
	m, _ = m.Update(filesLoadedMsg{files: []ado.FileChange{{Path: "/src/login.go", ChangeType: "edit"}}})
	out := m.View()
	if !strings.Contains(out, "Fix login bug") || !strings.Contains(out, "session tokens") {
		t.Fatalf("missing description:\n%s", out)
	}
	if !strings.Contains(out, "/src/login.go") {
		t.Fatalf("missing file:\n%s", out)
	}
}

func TestDetailStatusesRendered(t *testing.T) {
	m := NewDetail(DefaultKeys())
	m = m.SetSummary(ado.PRSummary{ID: 1, Title: "x"})
	m, _ = m.Update(statusesLoadedMsg{statuses: []ado.StatusCheck{{Context: "build/ci", State: "succeeded"}, {Context: "policy", State: "pending"}}})
	out := m.View()
	if !strings.Contains(out, "build/ci") || !strings.Contains(out, "policy") {
		t.Fatalf("missing status contexts:\n%s", out)
	}
}
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/ui/ -run TestDetail -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/ui/detail.go`:

```go
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

type detailLoadedMsg struct {
	detail *ado.PRDetail
	err    error
}
type filesLoadedMsg struct {
	files []ado.FileChange
	err   error
}
type statusesLoadedMsg struct {
	statuses []ado.StatusCheck
	err      error
}

type DetailModel struct {
	keys      KeyMap
	summary   ado.PRSummary
	detail    *ado.PRDetail
	files     []ado.FileChange
	statuses  []ado.StatusCheck
	cursor    int
	loadErr   string
	width     int
}

func NewDetail(keys KeyMap) DetailModel { return DetailModel{keys: keys} }

func (m DetailModel) SetSummary(s ado.PRSummary) DetailModel {
	m.summary = s
	m.detail = nil
	m.files = nil
	m.statuses = nil
	m.cursor = 0
	m.loadErr = ""
	return m
}

func (m DetailModel) SelectedFile() (ado.FileChange, bool) {
	if len(m.files) == 0 {
		return ado.FileChange{}, false
	}
	if m.cursor >= len(m.files) {
		return m.files[len(m.files)-1], true
	}
	return m.files[m.cursor], true
}

func (m DetailModel) Summary() ado.PRSummary { return m.summary }
func (m DetailModel) Detail() *ado.PRDetail  { return m.detail }

func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case detailLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else {
			m.detail = msg.detail
		}
	case filesLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else {
			m.files = msg.files
		}
	case statusesLoadedMsg:
		if msg.err == nil {
			m.statuses = msg.statuses
		}
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.keys.Down):
			if m.cursor < len(m.files)-1 {
				m.cursor++
			}
		case keyMatches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m DetailModel) View() string {
	var b strings.Builder
	s := m.summary
	fmt.Fprintf(&b, "PR #%d  %s\n", s.ID, s.Title)
	fmt.Fprintf(&b, Faint.Render("%s  %s → %s")+"\n\n", s.Author, s.SourceBranch, s.TargetBranch)

	if m.detail != nil {
		b.WriteString(m.detail.DescriptionMD)
		b.WriteString("\n\n")
		if len(m.detail.WorkItemRefs) > 0 {
			b.WriteString("Work items: ")
			for i, w := range m.detail.WorkItemRefs {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("#" + w.ID)
			}
			b.WriteString("\n")
		}
	}
	if len(m.statuses) > 0 {
		b.WriteString("Status: ")
		for i, st := range m.statuses {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(st.Context + " " + statusGlyph(st.State))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n── Files ─────────────────────────────────\n")
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render(m.loadErr) + "\n")
	}
	for i, f := range m.files {
		marker := "  "
		if i == m.cursor {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%s  %s", marker, f.ChangeType, f.Path)
		if i == m.cursor {
			line = Selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func statusGlyph(state string) string {
	switch state {
	case "succeeded":
		return Approve.Render("✓")
	case "failed", "error":
		return Reject.Render("✗")
	case "pending":
		return Wait.Render("⏳")
	default:
		return None.Render("·")
	}
}
```

- [ ] **Step 4: Pass**

Run: `go test ./internal/ui/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/detail.go internal/ui/detail_test.go
git commit -m "feat(ui): detail screen with description, work items, statuses, files"
```

---

## Task 11: UI — diff screen sub-model

**Files:**
- Create: `internal/ui/diff.go`
- Create: `internal/ui/diff_test.go`

- [ ] **Step 1: Failing test**

Create `internal/ui/diff_test.go`:

```go
package ui

import (
	"strings"
	"testing"
)

func TestDiffViewShowsRendererBadge(t *testing.T) {
	m := NewDiff(DefaultKeys())
	m = m.SetHeader("/src/login.go", "local+delta")
	m, _ = m.Update(diffLoadedMsg{content: []byte("--- a/src/login.go\n+++ b/src/login.go\n-old\n+new\n")})
	out := m.View()
	if !strings.Contains(out, "/src/login.go") || !strings.Contains(out, "local+delta") {
		t.Fatalf("header missing:\n%s", out)
	}
	if !strings.Contains(out, "+new") {
		t.Fatalf("body missing:\n%s", out)
	}
}

func TestDiffViewShowsErrorOnFailure(t *testing.T) {
	m := NewDiff(DefaultKeys())
	m = m.SetHeader("/x", "rest")
	m, _ = m.Update(diffLoadedMsg{err: errString("boom")})
	out := m.View()
	if !strings.Contains(out, "boom") {
		t.Fatalf("error not shown:\n%s", out)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
```

- [ ] **Step 2: Run, fail**

Run: `go test ./internal/ui/ -run TestDiff -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/ui/diff.go`:

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type diffLoadedMsg struct {
	content []byte
	err     error
}

type DiffModel struct {
	keys     KeyMap
	file     string
	renderer string
	vp       viewport.Model
	loadErr  string
	loaded   bool
}

func NewDiff(keys KeyMap) DiffModel {
	vp := viewport.New(80, 20)
	return DiffModel{keys: keys, vp: vp}
}

func (m DiffModel) SetHeader(file, renderer string) DiffModel {
	m.file = file
	m.renderer = renderer
	m.loaded = false
	m.loadErr = ""
	m.vp.SetContent("loading…")
	return m
}

func (m DiffModel) Update(msg tea.Msg) (DiffModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 4 // header + footer
	case diffLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			m.vp.SetContent("error: " + m.loadErr)
		} else {
			m.loaded = true
			m.vp.SetContent(string(msg.content))
			m.vp.GotoTop()
		}
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.keys.GotoTop):
			m.vp.GotoTop()
		case keyMatches(msg, m.keys.GotoEnd):
			m.vp.GotoBottom()
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m DiffModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", Header.Render(m.file), Faint.Render("["+m.renderer+"]"))
	b.WriteString(m.vp.View())
	if m.loadErr != "" {
		b.WriteString("\n" + ErrLine.Render(m.loadErr))
	}
	return b.String()
}
```

- [ ] **Step 4: Pass**

Run: `go test ./internal/ui/ -v`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/diff.go internal/ui/diff_test.go
git commit -m "feat(ui): diff screen with viewport scrolling and renderer badge"
```

---

## Task 12: UI — root model wiring all three screens

**Files:**
- Modify: `internal/ui/app.go` (replace contents)

- [ ] **Step 1: Replace app.go**

Overwrite `internal/ui/app.go`:

```go
// Package ui hosts the Bubble Tea application shell.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
	"github.com/superyyrrzz/adotop/internal/gitlocal"
)

type screen int

const (
	screenList screen = iota
	screenDetail
	screenDiff
)

type Model struct {
	cfg    config.Config
	client *ado.Client
	git    *gitlocal.Finder
	keys   KeyMap

	user   string
	myID   string
	screen screen

	list   ListModel
	detail DetailModel
	diff   DiffModel

	width, height int
	footerErr     string
	showHelp      bool
	useDelta      bool
}

func New(cfg config.Config, client *ado.Client) Model {
	keys := DefaultKeys()
	return Model{
		cfg:      cfg,
		client:   client,
		git:      gitlocal.New(cfg.RepoRoots),
		keys:     keys,
		list:     NewList(keys),
		detail:   NewDetail(keys),
		diff:     NewDiff(keys),
		user:     "loading…",
		useDelta: gitlocal.HasDelta(),
	}
}

type connDataMsg struct {
	data *ado.ConnectionData
	err  error
}

type tickMsg time.Time

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchConnectionData(), tick(m.cfg.RefreshInterval.Duration))
}

func (m Model) fetchConnectionData() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := m.client.GetConnectionData(ctx)
		return connDataMsg{data: d, err: err}
	}
}

func (m Model) loadList(tab ado.Tab) tea.Cmd {
	if m.myID == "" || m.cfg.Project == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		prs, err := m.client.ListPullRequests(ctx, ado.ListPRFilter{
			Project: m.cfg.Project, Tab: tab, MyID: m.myID,
		})
		return prsLoadedMsg{tab: tab, prs: prs, err: err}
	}
}

func (m Model) loadDetail(s ado.PRSummary) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			d, err := m.client.GetPullRequest(ctx, s.RepoID, s.ID, m.myID)
			return detailLoadedMsg{detail: d, err: err}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			files, err := m.client.GetIterationChanges(ctx, s.RepoID, s.ID)
			return filesLoadedMsg{files: files, err: err}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			st, err := m.client.GetStatuses(ctx, s.RepoID, s.ID)
			return statusesLoadedMsg{statuses: st, err: err}
		},
	)
}

func (m Model) loadDiff(s ado.PRSummary, file ado.FileChange, sourceSha, targetSha string) (DiffModel, tea.Cmd) {
	renderer := "rest"
	var clonePath string
	if p, ok := m.git.Find(s.Repo, m.cfg.Org); ok {
		clonePath = p
		if m.useDelta {
			renderer = "local+delta"
		} else {
			renderer = "local"
		}
	}
	dm := m.diff.SetHeader(file.Path, renderer)
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), m.useDelta)
			return diffLoadedMsg{content: out, err: err}
		}
		src, tgt, err := m.client.GetFileContents(ctx, s.RepoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return diffLoadedMsg{err: err}
		}
		return diffLoadedMsg{content: simpleDiff(tgt, src, file.Path)}
	}
	return dm, cmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var c1, c2, c3 tea.Cmd
		m.list, c1 = m.list.Update(msg)
		m.detail, c2 = m.detail.Update(msg)
		m.diff, c3 = m.diff.Update(msg)
		return m, tea.Batch(c1, c2, c3)
	case connDataMsg:
		if msg.err != nil {
			m.footerErr = "auth: " + msg.err.Error()
			return m, nil
		}
		m.user = msg.data.DisplayName()
		m.myID = msg.data.AuthenticatedUser.ID
		return m, m.loadList(m.list.Tab())
	case tickMsg:
		var cmd tea.Cmd
		if m.screen == screenList {
			cmd = m.loadList(m.list.Tab())
		}
		return m, tea.Batch(cmd, tick(m.cfg.RefreshInterval.Duration))
	case tabSwitchedMsg:
		return m, m.loadList(msg.Tab)
	case tea.KeyMsg:
		if keyMatches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if keyMatches(msg, m.keys.Help) {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if m.showHelp {
			if msg.Type == tea.KeyEsc {
				m.showHelp = false
			}
			return m, nil
		}
		switch m.screen {
		case screenList:
			return m.updateListScreen(msg)
		case screenDetail:
			return m.updateDetailScreen(msg)
		case screenDiff:
			return m.updateDiffScreen(msg)
		}
	}
	// Forward unrecognized messages to the active screen.
	switch m.screen {
	case screenList:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case screenDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	case screenDiff:
		var cmd tea.Cmd
		m.diff, cmd = m.diff.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateListScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Open):
		if s, ok := m.list.Selected(); ok {
			m.detail = m.detail.SetSummary(s)
			m.screen = screenDetail
			return m, m.loadDetail(s)
		}
	case keyMatches(msg, m.keys.Refresh):
		return m, m.loadList(m.list.Tab())
	case keyMatches(msg, m.keys.Browser):
		if s, ok := m.list.Selected(); ok {
			OpenInBrowser(s.URL)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDetailScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Back):
		m.screen = screenList
		return m, nil
	case keyMatches(msg, m.keys.Open):
		f, ok := m.detail.SelectedFile()
		if !ok || m.detail.Detail() == nil {
			return m, nil
		}
		dm, cmd := m.loadDiff(m.detail.Summary(), f, m.detail.Detail().SourceSha, m.detail.Detail().TargetSha)
		m.diff = dm
		m.screen = screenDiff
		return m, cmd
	case keyMatches(msg, m.keys.Refresh):
		return m, m.loadDetail(m.detail.Summary())
	case keyMatches(msg, m.keys.Browser):
		OpenInBrowser(m.detail.Summary().URL)
		return m, nil
	}
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m Model) updateDiffScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Back):
		m.screen = screenDetail
		return m, nil
	case keyMatches(msg, m.keys.Browser):
		OpenInBrowser(m.detail.Summary().URL)
		return m, nil
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	header := Header.Render(fmt.Sprintf("adotop  %s/%s  user=%s", orPlaceholder(m.cfg.Org, "(no org)"), orPlaceholder(m.cfg.Project, "(no project)"), m.user))
	var body string
	switch m.screen {
	case screenList:
		body = m.list.View()
	case screenDetail:
		body = m.detail.View()
	case screenDiff:
		body = m.diff.View()
	}
	if m.showHelp {
		body = HelpBox.Render(strings.Join([]string{
			"Help",
			"",
			"  ?           toggle this help",
			"  q / ctrl+c  quit",
			"  r           refresh current screen",
			"  o           open in browser",
			"  /           filter (list)",
			"  tab / l     next tab",
			"  shift+tab/h prev tab",
			"  enter       open selected",
			"  esc         back",
			"  g / G       top / bottom (diff)",
		}, "\n"))
	}
	footer := Footer.Render(footerHints(m.screen))
	if m.footerErr != "" {
		footer = ErrLine.Render(m.footerErr)
	}
	return strings.Join([]string{header, "", body, "", footer}, "\n")
}

func footerHints(s screen) string {
	switch s {
	case screenList:
		return "/:filter  enter:open  o:browser  r:refresh  tab:next  ?:help  q:quit"
	case screenDetail:
		return "↑↓:files  enter:diff  o:browser  esc:back  r:refresh  ?:help  q:quit"
	case screenDiff:
		return "↑↓ pgup/pgdn g/G:scroll  esc:back  o:browser  ?:help  q:quit"
	}
	return ""
}

func orPlaceholder(s, p string) string {
	if s == "" {
		return p
	}
	return s
}

// simpleDiff produces a tiny unified diff for the REST fallback.
func simpleDiff(target, source []byte, path string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a%s\n+++ b%s\n", path, path)
	t := strings.Split(string(target), "\n")
	s := strings.Split(string(source), "\n")
	for _, line := range t {
		b.WriteString("- " + line + "\n")
	}
	for _, line := range s {
		b.WriteString("+ " + line + "\n")
	}
	return []byte(b.String())
}

// Run starts the Bubble Tea program with the alt screen.
func Run(cfg config.Config, client *ado.Client) error {
	p := tea.NewProgram(New(cfg, client), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat(ui): wire root model with list/detail/diff screen coordinator"
```

---

## Task 13: Verify build, smoke-test, tag

**Files:** none

- [ ] **Step 1: Build the binary**

Run: `go build -o adotop.exe ./cmd/adotop`
Expected: binary at repo root.

- [ ] **Step 2: Manual smoke test**

Run: `./adotop.exe`

Verify:
- Header shows `ceapex/Engineering  user=<your name>` after a moment
- List loads PRs in the Assigned tab
- `Tab` switches to Created, then Review requested — counts in tab labels match
- `/` opens filter; typing narrows the list; `Esc` clears
- `Enter` opens detail; description renders; file list appears; statuses appear
- `Enter` on a file opens diff; if you have a local clone of the repo under one of `repo_roots`, the badge says `local+delta` (or `local`); otherwise `rest`
- `o` opens the PR in your browser
- `Esc` returns from diff → detail → list
- `q` quits cleanly

If anything breaks, fix and re-test before tagging.

- [ ] **Step 3: Tag release**

```bash
git tag -a v0.1.0 -m "Stage 1: PR dashboard (read-only)"
```

(No push — local tag only, per user's standing instruction.)

---

## Self-review notes

- **Spec coverage:** all Stage 1 deliverables (3 tabs, detail, diff with local/REST fallback, refresh+auto-refresh, `/` filter, `o` browser) map to tasks 2-12.
- **Open question (per-row build status):** spec defers it to Stage 4; plan accordingly omits a build-status column. Statuses appear in detail view (Task 4 + Task 10).
- **"Review requested" semantics:** implemented as client-side `MyVote == 0` filter on the same `reviewerId` query — matches the spec.
- **Type consistency:** `PRSummary`, `PRDetail`, `FileChange`, `StatusCheck`, `ReviewerVote` defined in Task 2-5 and used unchanged in UI tasks 9-12. Message types (`prsLoadedMsg`, `detailLoadedMsg`, `filesLoadedMsg`, `statusesLoadedMsg`, `diffLoadedMsg`) are defined in their owning UI files and consumed by `app.go` in Task 12.
- **Caching:** explicitly none, per spec.
- **Errors:** auth failure surfaces in footer (Task 12); per-call errors render inline in each sub-model (Tasks 9-11).

package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
)

const (
	Org     = "fabrikam"
	Project = "Platform"
	repoID  = "repo-payments"
	repo    = "payments-api"
	myID    = "user-alice"
)

type tokenProvider struct{}

func (tokenProvider) Token(context.Context) (string, error) { return "demo-token", nil }
func (tokenProvider) Invalidate()                           {}

// NewClient returns a normal ADO client wired to deterministic fixture data.
// The TUI still exercises the same client methods as a live session; only the
// HTTP transport is swapped so no Azure DevOps tenant or token is touched.
func NewClient() *ado.Client {
	c := ado.NewClient(Org, tokenProvider{})
	c.HTTP = &http.Client{Transport: newTransport()}
	c.MaxRetries = 0
	return c
}

func Config() config.Config {
	cfg := config.Default()
	cfg.Org = Org
	cfg.Project = Project
	cfg.RefreshInterval = config.Duration{Duration: 10 * time.Minute}
	return cfg
}

func InitialPRs() []ado.PRSummary {
	return []ado.PRSummary{
		toSummary(prAprilRelease()),
		toSummary(prRetryUpload()),
		toSummary(prDocs()),
	}
}

type transport struct {
	mu       sync.Mutex
	approved bool
	threads  []threadFixture
}

func newTransport() *transport {
	return &transport{threads: demoThreads()}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	path := req.URL.EscapedPath()
	method := req.Method

	switch {
	case method == http.MethodGet && path == "/fabrikam/_apis/connectionData":
		return jsonResp(req, map[string]any{
			"authenticatedUser": map[string]any{
				"id":                  myID,
				"providerDisplayName": "Alice Anderson",
				"customDisplayName":   "Alice Anderson",
			},
			"authorizedUser": map[string]any{
				"id":                  myID,
				"providerDisplayName": "Alice Anderson",
			},
			"instanceId": "demo-instance",
		})
	case method == http.MethodGet && path == "/fabrikam/Platform/_apis/git/pullrequests":
		return t.list(req)
	case method == http.MethodGet && path == "/fabrikam/_apis/git/pullrequests/1145087":
		return jsonResp(req, prAprilRelease())
	case method == http.MethodGet && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087":
		return jsonResp(req, prAprilRelease())
	case method == http.MethodGet && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/statuses":
		return jsonResp(req, statuses())
	case method == http.MethodGet && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/threads":
		return t.threadList(req)
	case method == http.MethodGet && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/iterations":
		return jsonResp(req, iterations())
	case method == http.MethodGet && strings.HasPrefix(path, "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/iterations/") && strings.HasSuffix(path, "/changes"):
		return jsonResp(req, changes())
	case method == http.MethodGet && path == "/fabrikam/_apis/git/repositories/repo-payments/items":
		return t.item(req)
	case method == http.MethodPut && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/reviewers/user-alice":
		t.mu.Lock()
		t.approved = true
		t.mu.Unlock()
		return jsonResp(req, map[string]any{})
	case method == http.MethodPatch && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/threads/401":
		return t.patchThread(req)
	case method == http.MethodPost && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/threads/401/comments":
		return t.postComment(req)
	case method == http.MethodPost && path == "/fabrikam/_apis/git/repositories/repo-payments/pullrequests/1145087/threads":
		return t.postThread(req)
	}

	return textResp(req, http.StatusNotFound, fmt.Sprintf("demo fixture missing: %s %s", method, path)), nil
}

func (t *transport) list(req *http.Request) (*http.Response, error) {
	vals := []map[string]any{prAprilRelease(), prRetryUpload(), prDocs()}
	q := req.URL.Query()
	if q.Get("searchCriteria.creatorId") != "" {
		vals = []map[string]any{prDocs()}
	}
	if q.Get("searchCriteria.reviewerId") != "" {
		vals = []map[string]any{prAprilRelease(), prRetryUpload()}
	}
	return jsonResp(req, map[string]any{"value": vals})
}

func (t *transport) threadList(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	vals := make([]map[string]any, 0, len(t.threads)+1)
	for _, th := range t.threads {
		vals = append(vals, th.raw())
	}
	vals = append(vals, voteThread())
	return jsonResp(req, map[string]any{"value": vals})
}

func (t *transport) patchThread(req *http.Request) (*http.Response, error) {
	var body struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.threads {
		if t.threads[i].ID == 401 && body.Status != "" {
			t.threads[i].Status = body.Status
			return jsonResp(req, t.threads[i].raw())
		}
	}
	return textResp(req, http.StatusNotFound, "thread not found"), nil
}

func (t *transport) postComment(req *http.Request) (*http.Response, error) {
	var body struct {
		Content string `json:"content"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	t.mu.Lock()
	defer t.mu.Unlock()
	comment := commentFixture{ID: 3, Author: "Alice Anderson", Content: body.Content, PublishedDate: "2026-05-12T03:20:00Z", CommentType: "text"}
	for i := range t.threads {
		if t.threads[i].ID == 401 {
			t.threads[i].Comments = append(t.threads[i].Comments, comment)
			break
		}
	}
	return jsonResp(req, comment.raw())
}

func (t *transport) postThread(req *http.Request) (*http.Response, error) {
	var body struct {
		Comments []struct {
			Content string `json:"content"`
		} `json:"comments"`
		ThreadContext struct {
			FilePath string `json:"filePath"`
		} `json:"threadContext"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	content := "Looks good."
	if len(body.Comments) > 0 && strings.TrimSpace(body.Comments[0].Content) != "" {
		content = body.Comments[0].Content
	}
	file := body.ThreadContext.FilePath
	if file == "" {
		file = "/src/api/session.go"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	th := threadFixture{
		ID:        499,
		Status:    "active",
		FilePath:  file,
		RightLine: 47,
		Comments:  []commentFixture{{ID: 1, Author: "Alice Anderson", Content: content, PublishedDate: "2026-05-12T03:20:00Z", CommentType: "text"}},
	}
	t.threads = append(t.threads, th)
	return jsonResp(req, th.raw())
}

func (t *transport) item(req *http.Request) (*http.Response, error) {
	path := req.URL.Query().Get("path")
	sha := req.URL.Query().Get("versionDescriptor.version")
	content := fileContent(path, sha)
	if content == "" {
		return textResp(req, http.StatusNotFound, "not found"), nil
	}
	return jsonResp(req, map[string]any{
		"content":         content,
		"contentMetadata": map[string]any{"encoding": "utf-8"},
	})
}

func toSummary(raw map[string]any) ado.PRSummary {
	created, _ := time.Parse(time.RFC3339, raw["creationDate"].(string))
	repository := raw["repository"].(map[string]any)
	createdBy := raw["createdBy"].(map[string]any)
	s := ado.PRSummary{
		ID:           int(raw["pullRequestId"].(int)),
		Title:        raw["title"].(string),
		Repo:         repository["name"].(string),
		RepoID:       repository["id"].(string),
		SourceBranch: strings.TrimPrefix(raw["sourceRefName"].(string), "refs/heads/"),
		TargetBranch: strings.TrimPrefix(raw["targetRefName"].(string), "refs/heads/"),
		CreatedAt:    created,
		Author:       createdBy["displayName"].(string),
		URL:          fmt.Sprintf("https://dev.azure.com/fabrikam/Platform/_git/%s/pullrequest/%d", repository["name"], raw["pullRequestId"]),
		MergeStatus:  raw["mergeStatus"].(string),
		Status:       raw["status"].(string),
	}
	for _, r := range raw["reviewers"].([]map[string]any) {
		vote := r["vote"].(int)
		rv := ado.ReviewerVote{ID: r["id"].(string), DisplayName: r["displayName"].(string), Vote: vote, IsRequired: r["isRequired"].(bool)}
		s.Reviewers = append(s.Reviewers, rv)
		if rv.ID == myID {
			s.MyVote = vote
		}
	}
	return s
}

func prAprilRelease() map[string]any {
	return pr(1145087, "April release prep", "refs/heads/release/apr", "Bob Brown", []map[string]any{
		reviewer(myID, "Alice Anderson", 10, false),
		reviewer("user-carol", "Carol Chen", 0, false),
		reviewer("group-required", "Required Reviewers", 0, true),
	}, "Prepare the April service release. Includes retry hardening, clearer telemetry, and one small session-state fix.", "a1111111111111111111111111111111111111111", "b2222222222222222222222222222222222222222")
}

func prRetryUpload() map[string]any {
	return pr(1151413, "Add retry around artifact upload", "refs/heads/retry-upload", "Carol Chen", []map[string]any{
		reviewer(myID, "Alice Anderson", 0, false),
		reviewer("user-bob", "Bob Brown", 5, false),
	}, "Retry transient upload failures before marking the release job failed.", "c3333333333333333333333333333333333333333", "d4444444444444444444444444444444444444444")
}

func prDocs() map[string]any {
	return pr(1140193, "Document deployment toggles", "refs/heads/docs/deploy-toggles", "Alice Anderson", []map[string]any{
		reviewer(myID, "Alice Anderson", 0, false),
		reviewer("user-bob", "Bob Brown", 10, false),
	}, "Add operator notes for deployment toggles.", "e5555555555555555555555555555555555555555", "f6666666666666666666666666666666666666666")
}

func pr(id int, title, source, author string, reviewers []map[string]any, description, sourceSha, targetSha string) map[string]any {
	return map[string]any{
		"pullRequestId": id,
		"title":         title,
		"status":        "active",
		"sourceRefName": source,
		"targetRefName": "refs/heads/main",
		"creationDate":  "2026-05-09T14:10:00Z",
		"isDraft":       false,
		"mergeStatus":   "succeeded",
		"createdBy":     map[string]any{"displayName": author},
		"repository": map[string]any{
			"id":   repoID,
			"name": repo,
			"project": map[string]any{
				"name": Project,
			},
		},
		"reviewers": reviewers,
		"_links": map[string]any{"web": map[string]any{
			"href": fmt.Sprintf("https://dev.azure.com/fabrikam/Platform/_git/%s/pullrequest/%d", repo, id),
		}},
		"description":           description,
		"lastMergeSourceCommit": map[string]any{"commitId": sourceSha},
		"lastMergeTargetCommit": map[string]any{"commitId": targetSha},
		"workItemRefs": []map[string]any{
			{"id": "7421", "url": "https://dev.azure.com/fabrikam/_apis/wit/workItems/7421"},
		},
	}
}

func reviewer(id, name string, vote int, required bool) map[string]any {
	return map[string]any{"id": id, "displayName": name, "vote": vote, "isRequired": required}
}

type commentFixture struct {
	ID            int
	Author        string
	Content       string
	PublishedDate string
	CommentType   string
}

func (c commentFixture) raw() map[string]any {
	return map[string]any{
		"id": c.ID,
		"author": map[string]any{
			"displayName": c.Author,
		},
		"content":       c.Content,
		"publishedDate": c.PublishedDate,
		"commentType":   c.CommentType,
	}
}

type threadFixture struct {
	ID        int
	Status    string
	FilePath  string
	RightLine int
	Comments  []commentFixture
}

func (t threadFixture) raw() map[string]any {
	comments := make([]map[string]any, 0, len(t.Comments))
	for _, c := range t.Comments {
		comments = append(comments, c.raw())
	}
	r := map[string]any{
		"id":            t.ID,
		"status":        t.Status,
		"isDeleted":     false,
		"publishedDate": t.Comments[0].PublishedDate,
		"comments":      comments,
	}
	if t.FilePath != "" {
		r["threadContext"] = map[string]any{
			"filePath": t.FilePath,
			"rightFileStart": map[string]any{
				"line":   t.RightLine,
				"offset": 1,
			},
			"leftFileStart": map[string]any{
				"line":   t.RightLine - 1,
				"offset": 1,
			},
		}
	}
	return r
}

func demoThreads() []threadFixture {
	return []threadFixture{
		{
			ID:        401,
			Status:    "active",
			FilePath:  "/src/api/session.go",
			RightLine: 47,
			Comments: []commentFixture{
				{ID: 1, Author: "Carol Chen", Content: "Should we also refresh the expiry timestamp here?", PublishedDate: "2026-05-10T16:30:00Z", CommentType: "text"},
				{ID: 2, Author: "Bob Brown", Content: "Good catch. I added a guard so callers do not keep a stale session.", PublishedDate: "2026-05-10T17:05:00Z", CommentType: "text"},
			},
		},
		{
			ID:        402,
			Status:    "fixed",
			FilePath:  "/src/api/handlers.go",
			RightLine: 22,
			Comments:  []commentFixture{{ID: 1, Author: "Required Reviewers", Content: "Ownership check passed for src/api.", PublishedDate: "2026-05-10T18:10:00Z", CommentType: "system"}},
		},
	}
}

func voteThread() map[string]any {
	return map[string]any{
		"id":            490,
		"status":        "closed",
		"isDeleted":     false,
		"publishedDate": "2026-05-10T15:00:00Z",
		"comments":      []map[string]any{},
		"properties": map[string]any{
			"CodeReviewThreadType":      prop("VoteUpdate"),
			"CodeReviewVoteResult":      prop("10"),
			"CodeReviewVotedByIdentity": prop("1"),
		},
		"identities": map[string]any{
			"1": map[string]any{"id": myID},
		},
	}
}

func prop(v string) map[string]any {
	return map[string]any{"$type": "System.String", "$value": v}
}

func statuses() map[string]any {
	return map[string]any{"value": []map[string]any{
		{"state": "succeeded", "updatedDate": "2026-05-11T12:01:00Z", "context": map[string]any{"genre": "ci", "name": "unit-tests"}},
		{"state": "succeeded", "updatedDate": "2026-05-11T12:04:00Z", "context": map[string]any{"genre": "ci", "name": "lint"}},
		{"state": "succeeded", "updatedDate": "2026-05-11T12:07:00Z", "context": map[string]any{"genre": "policy", "name": "required-reviewers"}},
	}}
}

func iterations() map[string]any {
	return map[string]any{"value": []map[string]any{
		{"id": 1, "createdDate": "2026-05-10T14:30:00Z"},
		{"id": 2, "createdDate": "2026-05-11T13:45:00Z"},
	}}
}

func changes() map[string]any {
	return map[string]any{"changeEntries": []map[string]any{
		{"changeType": "edit", "item": map[string]any{"path": "/src/api/session.go"}},
		{"changeType": "edit", "item": map[string]any{"path": "/src/api/handlers.go"}},
		{"changeType": "add", "item": map[string]any{"path": "/src/api/token_store.go"}},
		{"changeType": "edit", "item": map[string]any{"path": "/README.md"}},
	}}
}

func fileContent(path, sha string) string {
	source := strings.HasPrefix(sha, "a111") || strings.HasPrefix(sha, "c333") || strings.HasPrefix(sha, "e555")
	switch path {
	case "/src/api/session.go":
		if source {
			return "package api\n\nimport \"time\"\n\ntype Session struct {\n\ttoken string\n\texpiresAt time.Time\n}\n\nfunc (s *Session) Refresh(newToken string) error {\n\tif s.expiresAt.Before(time.Now()) {\n\t\treturn ErrExpired\n\t}\n\ts.token = newToken\n\treturn nil\n}\n"
		}
		return "package api\n\nimport \"time\"\n\ntype Session struct {\n\ttoken string\n\texpiresAt time.Time\n\trefreshedAt time.Time\n}\n\nfunc (s *Session) Refresh(newToken string) error {\n\tif s.expiresAt.Before(time.Now()) {\n\t\treturn ErrExpired\n\t}\n\ts.token = newToken\n\ts.refreshedAt = time.Now()\n\treturn nil\n}\n"
	case "/src/api/handlers.go":
		if source {
			return "package api\n\nfunc HandleUpload() error {\n\treturn uploadOnce()\n}\n"
		}
		return "package api\n\nfunc HandleUpload() error {\n\tfor attempt := 0; attempt < 3; attempt++ {\n\t\tif err := uploadOnce(); err == nil {\n\t\t\treturn nil\n\t\t}\n\t}\n\treturn uploadOnce()\n}\n"
	case "/src/api/token_store.go":
		if source {
			return ""
		}
		return "package api\n\ntype TokenStore struct {\n\ttokens map[string]string\n}\n"
	case "/README.md":
		if source {
			return "# payments-api\n\nRelease service API.\n"
		}
		return "# payments-api\n\nRelease service API.\n\n## Operations\n\nUse deployment toggles for staged rollout.\n"
	}
	return ""
}

func jsonResp(req *http.Request, v any) (*http.Response, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	resp := response(req, http.StatusOK, b)
	resp.Header.Set("Content-Type", "application/json")
	return resp, nil
}

func textResp(req *http.Request, status int, body string) *http.Response {
	return response(req, status, []byte(body))
}

func response(req *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

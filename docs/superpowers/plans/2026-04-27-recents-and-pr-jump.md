# Recents Tab + PR-ID Jump Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a 4th list tab "Recents" that shows the user's locally-accessed PRs (most-recent first, capped at 50), plus a `#` shortcut on the list screen that prompts for a PR ID and jumps directly to its Detail screen — auto-recording the visit in Recents.

**Architecture:** Local-only feature. No new ADO endpoints beyond a thin `GetPullRequestByID(prID)` wrapper around `/_apis/git/pullrequests/{id}` (org-scoped, no repo/project required — confirmed live). Recents is a 4th value in the existing `ado.Tab` enum, but its rows come from `cache.Store` instead of the live REST list. Visits are recorded whenever the Detail screen is entered (via Open from list, Recents, or `#` jump).

**Tech Stack:** Go 1.26, existing Bubble Tea + cache infra. No new deps.

---

## File Structure

- `internal/ado/pullrequests.go` (modify) — add `GetPullRequestByID(ctx, prID)`.
- `internal/ado/pullrequests_test.go` (modify) — pin the by-ID call shape.
- `internal/ado/types.go` or `internal/ado/pullrequests.go` (modify) — extend `Tab` with `TabRecents`.
- `internal/cache/cache.go` (modify) — add `LoadRecents` / `SaveRecents` and a `RecordVisit(pr)` helper that prepends + dedupes + caps at 50.
- `internal/cache/cache_test.go` (modify) — pin recents persistence + cap + dedupe.
- `internal/ui/list.go` (modify) — render the new tab; route `#` to start ID-prompt mode.
- `internal/ui/list.go` (modify) — handle the ID prompt (digits-only input, enter, esc).
- `internal/ui/app.go` (modify) — bridge: when a PR is opened (from any source), call `cache.RecordVisit`; when `#` enter is pressed, kick off `GetPullRequestByID` → on success, switch to Detail; on Recents tab refresh, just reload from cache.
- `internal/ui/keys.go` (modify) — add `JumpToID` binding (`#`).
- Tests in `app_test.go` / `list_test.go` for tab presence, prompt flow, and visit recording.

---

## Task 1: ADO `GetPullRequestByID`

**Files:**
- Modify: `internal/ado/pullrequests.go`
- Modify: `internal/ado/pullrequests_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ado/pullrequests_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

`go test ./internal/ado/ -run TestGetPullRequestByID -v` — expect `undefined: GetPullRequestByID`.

- [ ] **Step 3: Implement**

Add to `internal/ado/pullrequests.go`:

```go
// GetPullRequestByID looks up a PR by its global PR ID. Unlike GetPullRequest,
// this does NOT require the repo ID — the org-scoped endpoint
// /_apis/git/pullrequests/{id} returns the repo+project on the response.
// Use this for the "jump to PR by number" UX when the user only knows the ID.
func (c *Client) GetPullRequestByID(ctx context.Context, prID int, myID string) (*PRDetail, error) {
	if prID == 0 {
		return nil, fmt.Errorf("GetPullRequestByID: prID required")
	}
	path := fmt.Sprintf("/_apis/git/pullrequests/%d", prID)
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
	if d.URL == "" {
		d.URL = c.webURLForPR(r.Repository.Project.Name, r.Repository.Name, r.PullRequestID)
	}
	return d, nil
}
```

- [ ] **Step 4: Run to verify it passes**

`go test ./internal/ado/ -run TestGetPullRequestByID -v` — PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ado/pullrequests.go internal/ado/pullrequests_test.go
git commit -m "feat(ado): GetPullRequestByID for org-scoped PR lookup"
```

---

## Task 2: Extend `Tab` enum with `TabRecents`

**Files:**
- Modify: `internal/ado/pullrequests.go`

- [ ] **Step 1: Add the value + String case**

In `internal/ado/pullrequests.go`, change:

```go
const (
	TabAssigned Tab = iota
	TabCreated
	TabReviewRequested
	TabRecents
)

func (t Tab) String() string {
	switch t {
	case TabAssigned:
		return "Assigned to me"
	case TabCreated:
		return "Created by me"
	case TabReviewRequested:
		return "All reviewing"
	case TabRecents:
		return "Recents"
	}
	return "?"
}
```

- [ ] **Step 2: Build to confirm no break**

`go build ./...` — expect no errors. Existing list-tab cycling logic uses `% 3` and will need updating in Task 4; that's fine for now because nothing yet pushes the cursor onto `TabRecents`.

- [ ] **Step 3: Commit**

```bash
git add internal/ado/pullrequests.go
git commit -m "feat(ado): add TabRecents to PR list tabs"
```

---

## Task 3: Cache layer for recents

**Files:**
- Modify: `internal/cache/cache.go`
- Modify: `internal/cache/cache_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cache/cache_test.go`:

```go
func TestRecentsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a := ado.PRSummary{ID: 1, Title: "a"}
	b := ado.PRSummary{ID: 2, Title: "b"}
	if err := s.RecordVisit(a); err != nil {
		t.Fatalf("RecordVisit a: %v", err)
	}
	if err := s.RecordVisit(b); err != nil {
		t.Fatalf("RecordVisit b: %v", err)
	}
	got, ok := s.LoadRecents()
	if !ok {
		t.Fatalf("LoadRecents: not ok")
	}
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Fatalf("expected [2,1], got %+v", got)
	}
}

func TestRecentsDedupesAndPromotes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s, _ := New()
	_ = s.RecordVisit(ado.PRSummary{ID: 1})
	_ = s.RecordVisit(ado.PRSummary{ID: 2})
	_ = s.RecordVisit(ado.PRSummary{ID: 1}) // re-visit 1
	got, _ := s.LoadRecents()
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("expected re-visit to promote 1 to front: %+v", got)
	}
}

func TestRecentsCap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	s, _ := New()
	for i := 1; i <= 60; i++ {
		_ = s.RecordVisit(ado.PRSummary{ID: i})
	}
	got, _ := s.LoadRecents()
	if len(got) != 50 {
		t.Fatalf("expected cap 50, got %d", len(got))
	}
	if got[0].ID != 60 || got[49].ID != 11 {
		t.Fatalf("oldest entries should be evicted: head=%d tail=%d", got[0].ID, got[49].ID)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

`go test ./internal/cache/ -run TestRecents -v` — expect `undefined: RecordVisit / LoadRecents`.

- [ ] **Step 3: Implement**

Add to `internal/cache/cache.go`:

```go
const recentsCap = 50

type RecentsSnapshot struct {
	Schema int               `json:"schema"`
	PRs    []ado.PRSummary   `json:"prs"`
}

func (s *Store) recentsPath() string { return filepath.Join(s.base, "recents.json") }

func (s *Store) LoadRecents() ([]ado.PRSummary, bool) {
	var snap RecentsSnapshot
	if !readJSON(s.recentsPath(), &snap) || snap.Schema != schemaVersion {
		return nil, false
	}
	return snap.PRs, true
}

// RecordVisit prepends pr to recents, removing any prior entry for the same
// PR ID, then writes the truncated list back. Capped at recentsCap.
func (s *Store) RecordVisit(pr ado.PRSummary) error {
	if pr.ID == 0 {
		return nil
	}
	cur, _ := s.LoadRecents()
	out := make([]ado.PRSummary, 0, len(cur)+1)
	out = append(out, pr)
	for _, p := range cur {
		if p.ID == pr.ID {
			continue
		}
		out = append(out, p)
		if len(out) >= recentsCap {
			break
		}
	}
	return writeJSON(s.recentsPath(), RecentsSnapshot{Schema: schemaVersion, PRs: out})
}
```

- [ ] **Step 4: Verify**

`go test ./internal/cache/ -run TestRecents -v` — all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cache/cache.go internal/cache/cache_test.go
git commit -m "feat(cache): persist recently-viewed PRs (capped at 50)"
```

---

## Task 4: List tab supports Recents (no live load)

**Files:**
- Modify: `internal/ui/list.go`
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Update tab cycling and rendering**

In `internal/ui/list.go`:

Change the View tab list:

```go
tabs := []string{
	ado.TabAssigned.String(),
	ado.TabCreated.String(),
	ado.TabReviewRequested.String(),
	ado.TabRecents.String(),
}
for i, name := range tabs { ... }
```

Change tab cycling from `% 3` to `% 4`:

```go
case keyMatches(msg, m.keys.NextTab):
	m.tab = (m.tab + 1) % 4
	m.cursor = 0
	return m, tabSwitchCmd(m.tab)
case keyMatches(msg, m.keys.PrevTab):
	m.tab = (m.tab + 3) % 4
	m.cursor = 0
	return m, tabSwitchCmd(m.tab)
```

- [ ] **Step 2: In `app.go`, wire Recents loading**

In `loadList(tab ado.Tab) tea.Cmd`, short-circuit Recents to read from cache:

```go
func (m Model) loadList(tab ado.Tab) tea.Cmd {
	if tab == ado.TabRecents {
		prs, _ := func() ([]ado.PRSummary, bool) {
			if m.cache == nil {
				return nil, false
			}
			return m.cache.LoadRecents()
		}()
		return func() tea.Msg {
			return prsLoadedMsg{tab: ado.TabRecents, prs: prs}
		}
	}
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
```

Also in `New(...)`: warm the Recents tab from cache on startup the same way the other 3 tabs are warmed:

```go
for _, tab := range []ado.Tab{ado.TabAssigned, ado.TabCreated, ado.TabReviewRequested} {
	if prs, ok := st.LoadList(cfg.Org, cfg.Project, tab); ok {
		m.list, _ = m.list.Update(prsLoadedMsg{tab: tab, prs: prs})
	}
}
if prs, ok := st.LoadRecents(); ok {
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})
}
```

- [ ] **Step 3: Skip persisting Recents under list cache key**

In the `prsLoadedMsg` handler in `Update`, the existing code calls `m.cache.SaveList(...)`. Skip it for Recents (recents has its own file):

```go
case prsLoadedMsg:
	if msg.err == nil && m.cache != nil && m.cfg.Org != "" && m.cfg.Project != "" && msg.tab != ado.TabRecents {
		if err := m.cache.SaveList(m.cfg.Org, m.cfg.Project, msg.tab, msg.prs); err != nil {
			slog.Warn("cache: save list", "tab", msg.tab, "err", err)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
```

- [ ] **Step 4: Build + test**

`go build ./... && go test ./internal/ui/ -run TestList -v` — existing list tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/list.go internal/ui/app.go
git commit -m "feat(ui): add Recents tab backed by local cache"
```

---

## Task 5: Record visits when Detail opens

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/app_test.go`

- [ ] **Step 1: Write failing test**

Append to `internal/ui/app_test.go`:

```go
func TestOpeningPRRecordsRecentVisit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	st, err := cache.New()
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	m := newTestModel()
	m.cache = st
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: []ado.PRSummary{
		{ID: 77, Title: "open me"},
	}})

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)

	got, ok := st.LoadRecents()
	if !ok || len(got) != 1 || got[0].ID != 77 {
		t.Fatalf("expected recents=[77], got ok=%v list=%+v", ok, got)
	}
}
```

You'll need `"github.com/renzeyu/adotop/internal/cache"` imported in `app_test.go`.

- [ ] **Step 2: Run to verify it fails**

`go test ./internal/ui/ -run TestOpeningPRRecordsRecentVisit -v` — expect FAIL.

- [ ] **Step 3: Wire `RecordVisit`**

In `app.go`, factor the "open this summary as detail" code into a helper, since it'll be called from both list-Open and ID-jump (Task 6). Add to `app.go`:

```go
// openDetail switches to the Detail screen for s, records the visit in the
// recents cache, and returns the load command. Idempotent: caller may call
// from list, recents tab, or ID-jump.
func (m Model) openDetail(s ado.PRSummary) (Model, tea.Cmd) {
	m.detail = m.detail.SetSummary(s)
	m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
	m.previewKey = ""
	m.detailFocus = focusFiles
	m.scrollMem = map[string]int{}
	m.previewBodies = map[string][]byte{}
	m.screen = screenDetail
	if m.cache != nil {
		if err := m.cache.RecordVisit(s); err != nil {
			slog.Warn("cache: record visit", "pr", s.ID, "err", err)
		}
		// Also refresh the in-memory recents tab so the row shows up if
		// the user pops back and switches to Recents immediately.
		if recents, ok := m.cache.LoadRecents(); ok {
			m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: recents})
		}
	}
	return m, m.loadDetail(s)
}
```

Replace the existing list-Open block with a call:

```go
case keyMatches(msg, m.keys.Open):
	if s, ok := m.list.Selected(); ok {
		mm, cmd := m.openDetail(s)
		return mm, cmd
	}
```

- [ ] **Step 4: Run to verify it passes**

`go test ./internal/ui/ -run TestOpeningPRRecordsRecentVisit -v` — PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): record visit in recents cache when Detail opens"
```

---

## Task 6: `#` jump-to-PR-ID prompt

**Files:**
- Modify: `internal/ui/keys.go`
- Modify: `internal/ui/list.go`
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/app_test.go`

- [ ] **Step 1: Add the keybinding**

In `internal/ui/keys.go`, add field and default:

```go
type KeyMap struct {
	... existing fields ...
	JumpToID key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		... existing ...
		JumpToID: key.NewBinding(key.WithKeys("#")),
	}
}
```

- [ ] **Step 2: Add prompt state to ListModel**

In `internal/ui/list.go` ListModel struct, add fields:

```go
type ListModel struct {
	... existing ...
	jumping    bool
	jumpInput  string
}
```

Add a public exit message + handler:

```go
type jumpRequestedMsg struct{ ID int }

func (m ListModel) updateJumping(msg tea.KeyMsg) (ListModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.jumping = false
		m.jumpInput = ""
	case tea.KeyEnter:
		id, _ := strconv.Atoi(m.jumpInput)
		m.jumping = false
		m.jumpInput = ""
		if id <= 0 {
			return m, nil
		}
		return m, func() tea.Msg { return jumpRequestedMsg{ID: id} }
	case tea.KeyBackspace:
		if len(m.jumpInput) > 0 {
			m.jumpInput = m.jumpInput[:len(m.jumpInput)-1]
		}
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if r >= '0' && r <= '9' {
				m.jumpInput += string(r)
			}
		}
	}
	return m, nil
}
```

Add `"strconv"` import.

In `Update`, before the existing `switch` on key matches, add:

```go
case tea.KeyMsg:
	if m.filtering {
		return m.updateFiltering(msg)
	}
	if m.jumping {
		return m.updateJumping(msg)
	}
	switch {
	... existing ...
	case keyMatches(msg, m.keys.JumpToID):
		m.jumping = true
		m.jumpInput = ""
	}
```

In `View`, before the existing filter-render block:

```go
if m.jumping {
	b.WriteString("\n#" + m.jumpInput + lipgloss.NewStyle().Faint(true).Render("█"))
}
```

- [ ] **Step 3: Handle `jumpRequestedMsg` in app.go**

Add the message handler — kicks off the by-ID lookup and opens detail on success:

```go
case jumpRequestedMsg:
	prID := msg.ID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := m.client.GetPullRequestByID(ctx, prID, m.myID)
		if err != nil {
			return jumpResultMsg{prID: prID, err: err}
		}
		return jumpResultMsg{prID: prID, summary: d.PRSummary}
	}
case jumpResultMsg:
	if msg.err != nil {
		m.footerErr = fmt.Sprintf("jump #%d: %v", msg.prID, msg.err)
		return m, nil
	}
	mm, cmd := m.openDetail(msg.summary)
	return mm, cmd
```

Add the message type near other msg defs:

```go
type jumpResultMsg struct {
	prID    int
	summary ado.PRSummary
	err     error
}
```

- [ ] **Step 4: Update help + footer**

In `View`, update help text:

```go
"  #           jump to PR by ID (list)",
```

In `footerHints`:

```go
case screenList:
	return "/:filter  #:goto  enter:open  o:browser  r:refresh  tab:next  ?:help  q:quit"
```

- [ ] **Step 5: Write a UI test for the jump prompt**

Append to `internal/ui/app_test.go`:

```go
func TestListJumpPromptCollectsDigitsAndEmitsRequest(t *testing.T) {
	m := newTestModel()
	m.screen = screenList

	// Press #
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'#'}})
	m = mm.(Model)
	if !m.list.jumping {
		t.Fatalf("expected jumping=true after #")
	}

	// Type "12"
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = mm.(Model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = mm.(Model)
	if m.list.jumpInput != "12" {
		t.Fatalf("expected jumpInput=12, got %q", m.list.jumpInput)
	}

	// Enter — should leave jumping mode and emit a jumpRequestedMsg cmd.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter to emit a jumpRequestedMsg cmd")
	}
	got := cmd()
	if jr, ok := got.(jumpRequestedMsg); !ok || jr.ID != 12 {
		t.Fatalf("expected jumpRequestedMsg{ID:12}, got %T %+v", got, got)
	}
}
```

- [ ] **Step 6: Build + test all UI**

`go build ./... && go test ./internal/ui/`. Existing tests must still pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/keys.go internal/ui/list.go internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): jump to PR by number with # prompt"
```

---

## Task 7: Rebuild + smoke test

- [ ] **Step 1: Build**

`go build -o adotop.exe ./cmd/adotop` — expect success.

- [ ] **Step 2: Manual smoke** (interactive — defer to user)

- Press `tab` repeatedly — Recents tab appears as the 4th.
- Open a PR with enter from Assigned. `esc` back. Switch to Recents — that PR appears.
- Press `#`, type `1137662`, enter. Detail loads for that PR. `esc` back. Recents now has it at the top.
- Reopen the same PR via list-enter — Recents promotes it to position 1 (no duplicate).

---

## Self-Review Notes

- **Spec coverage:** 4th tab → Tasks 2+4. Persisted recents capped 50 → Task 3. `#` jump → Task 6. Auto-record on Detail open → Task 5. Project-wide ID lookup → Task 1.
- **Type consistency:** `openDetail(s ado.PRSummary)` defined in Task 5 and called from Task 6's `jumpResultMsg` handler — both pass `ado.PRSummary`. ✓ `RecordVisit(pr ado.PRSummary)` defined in Task 3, called in Task 5. ✓ `GetPullRequestByID(ctx, prID, myID)` returns `*PRDetail` whose embedded `PRSummary` is what `openDetail` wants. ✓
- **Placeholders:** none.
- **Risk: tab cycling math.** Task 4 changes `% 3` to `% 4`; if any test relies on the old wrap (review-requested → assigned), it'll break. The existing tab-cycle test (if any) needs updating. Self-check during Task 4.
- **Risk: Recents shown but cache disabled.** If `m.cache` is nil (cache init failed), Recents tab will be empty and `RecordVisit` is a no-op. Current code already logs "cache disabled" at startup; recents inherits that gracefully without a separate failure path.
- **Risk: by-ID jump returns a PR from a different project.** The org-scoped endpoint succeeds for any PR in the org. We don't filter by configured project. Behavior: opens the Detail screen for that PR regardless. This is a feature (user explicitly asked for an ID); no guard needed.
- **Risk: `#` is shift+3 on US layouts but a different key elsewhere.** Bubble Tea's `key.NewBinding(key.WithKeys("#"))` matches the literal rune; should work on any layout that produces `#`. Document in help text.

---

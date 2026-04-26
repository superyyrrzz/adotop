# Stage 1 — PRs (read) — Design

**Status:** Approved (sections 1-4 walked through with user)
**Date:** 2026-04-26
**Owner:** renzeyu
**Roadmap:** `docs/superpowers/specs/2026-04-25-adotop-roadmap.md` (Stage 1)

The PR dashboard you want open in a tmux pane all day. Read-only — mutations come in Stage 2.

---

## Goals & non-goals

**Goals**

- Three filter tabs across the configured org+project: **Assigned to me**, **Created by me**, **Review requested from me**
- Snappy navigation through 100+ PRs
- Detail view: description (rendered markdown), reviewers + votes, work-item links, PR status checks, file list
- Diff view that handles large files without freezing the UI
- Refresh on demand (`r`) and auto-refresh on the list every 60 s (configurable)
- Local-substring filter via `/`
- Open in browser via `o`

**Non-goals (Stage 1)**

- Any mutation (comment, vote, complete, abandon)
- Multi-org / multi-project (deferred to Stage 6)
- Server-side search across all PRs (deferred; local filter only)
- Per-row build-status column (deferred to Stage 4 — see "Open question: build status" below)
- PR list paging beyond the first 100 results (deferred)
- Threads / inline comments rendering (Stage 2)

---

## Architecture

A single root Bubble Tea model coordinates three screens (`list`, `detail`, `diff`) via a `screen` enum. Per-screen state and update logic live in their own files; the root only handles cross-cutting concerns (header/footer, window size, screen transitions, the auto-refresh ticker).

```
internal/ui/
  app.go         root model + screen enum + Update/View dispatch
  list.go        list model: tabs, rows, /-filter, j/k navigation
  detail.go      detail model: description, reviewers, file list
  diff.go        diff model: file viewer with delta/REST fallback
  keys.go        key bindings (centralized)
  styles.go      lipgloss styles (centralized)

internal/ado/
  pullrequests.go  PR list + detail + iterations/changes
  statuses.go      PR status checks (build/policy/etc)
  diff.go          REST diff fetcher (used when no local clone)

internal/gitlocal/
  finder.go        scan repo_roots for a clone matching a repo
  diff.go          shell out to `git diff` / `delta` against the clone
```

**Data flow**

- List fetch is a `tea.Cmd` calling `ado.ListPullRequests(ctx, filter)`; returns `prsLoadedMsg{tab, prs, err}`.
- Detail fetch fans out two parallel cmds: `ado.GetPullRequest` and `ado.GetIterationChanges`; each returns its own message and the model merges as they arrive (description shows first, file list fills in).
- Status checks are fetched after detail+changes, as `statusesLoadedMsg`.
- Diff fetch is a single cmd that picks `gitlocal` (when `gitlocal.Find(repoName)` returns a path) or `ado.GetDiff` otherwise.

**Auto-refresh**

A single 60 s `tea.Tick` lives on the root. On every fire, it dispatches a list-refresh cmd **only if the current screen is `list`**. Manual `r` works on every screen. The ticker interval is `cfg.RefreshInterval`.

---

## Data model

```go
type PRSummary struct {
    ID           int
    Title        string
    Repo         string         // repository.name
    RepoID       string         // repository.id (used for detail/diff calls)
    SourceBranch string         // refs/heads/foo -> "foo"
    TargetBranch string
    CreatedAt    time.Time
    Author       string         // createdBy.displayName
    URL          string         // _links.web.href
    Reviewers    []ReviewerVote // for the vote glyph row
    MyVote       int            // -10..10; used for "Review requested" filter
    Draft        bool           // isDraft
    MergeStatus  string         // mergeStatus
}

type ReviewerVote struct {
    DisplayName string
    Vote        int  // 10 approve, 5 approve-w/-suggestions, -5 wait, -10 reject, 0 none
    IsRequired  bool
}

type PRDetail struct {
    PRSummary
    DescriptionMD string
    WorkItemRefs  []WorkItemRef
    Files         []FileChange  // populated by GetIterationChanges
    Statuses      []StatusCheck // populated by GetStatuses
    SourceSha     string
    TargetSha     string
}

type FileChange struct {
    Path       string
    ChangeType string // "edit" | "add" | "delete" | "rename"
    AddLines   int    // best-effort; may be unknown until diff is fetched
    DelLines   int
}

type StatusCheck struct {
    Context string  // e.g. "build/ci", "sonar", "policy"
    State   string  // "succeeded" | "pending" | "failed" | "error"
}
```

---

## ADO endpoints

`{org}` and `{project}` come from `~/.adotop/config.toml`. `{me}` comes from `connectionData.authenticatedUser.id` (already wired in Stage 0).

| Purpose | Method + path | Notes |
|---|---|---|
| Auth probe | `GET /_apis/connectionData` | Stage 0 already calls this |
| List PRs | `GET /{project}/_apis/git/pullrequests?searchCriteria.status=active&$top=100` | `searchCriteria.creatorId={me}` for "Created"; `searchCriteria.reviewerId={me}` for "Assigned" *and* "Review requested" (we filter the latter client-side to `MyVote == 0`) |
| PR detail | `GET /_apis/git/repositories/{repoId}/pullrequests/{id}?includeWorkItemRefs=true` | |
| Changed files | `GET /_apis/git/repositories/{repoId}/pullrequests/{id}/iterations/{latestIterationId}/changes` | Two-step: first `GET .../iterations` to find the latest id, then changes |
| Status checks | `GET /_apis/git/repositories/{repoId}/pullrequests/{id}/statuses` | Aggregates one icon per `context` |
| Diff (REST) | `GET /_apis/git/repositories/{repoId}/diffs/commits?baseVersion={target}&targetVersion={source}` then per-file content fetches | Used only when no local clone is found |

All requests use `api-version=7.1` (already added by `internal/ado/Client.GetJSON`).

---

## Screens & UX

### List

```
┌──────────────────────────────────────────────────────────────────┐
│ adotop  ceapex/Engineering  user=Renze Yu          5 PRs  60s ↻ │
├──────────────────────────────────────────────────────────────────┤
│  Assigned (5) │ Created (2) │ Review requested (3)               │
├──────────────────────────────────────────────────────────────────┤
│ #1234  Fix login bug              alice  feat/login → main   2h │
│        ✓✓·                                                      │
│ #1235  Add dark mode              bob    feat/theme → main   4h │
│        ⏳··                                                     │
│ #1236  Refactor auth   [DRAFT]    you    refactor → main     1d │
│        ···                                                      │
├──────────────────────────────────────────────────────────────────┤
│  /:filter  enter:open  o:browser  r:refresh  tab:next  ?:help  q │
└──────────────────────────────────────────────────────────────────┘
```

- Row format line 1: `#id  title  [DRAFT]?  author  source→target  age`
- Row format line 2: vote glyphs in `Reviewers` order (`✓` approve, `✓~` w/suggestions, `⏳` waiting, `✗` reject, `·` no vote). Required reviewers rendered bold.
- Tabs: `Tab` / `Shift-Tab` and `h` / `l`.
- `/` opens an inline filter at the bottom; substring match on `title + author + source + target`. `Esc` clears.
- Selection: `j` / `k` / arrows; `enter` opens detail.

### Detail

```
┌──────────────────────────────────────────────────────────────────┐
│ adotop  PR #1234  Fix login bug                                  │
├──────────────────────────────────────────────────────────────────┤
│ alice  feat/login → main  ·  opened 2h ago  ·  mergeable        │
│                                                                  │
│ ## Description                                                   │
│ Fixes the issue where session tokens were not refreshed when…    │
│                                                                  │
│ Reviewers:  alice ✓ (req)   bob ⏳   carol ·                    │
│ Work items: #98765 Bug — login session expires too early        │
│ Status:     build/ci ✓   sonar ✓   policy ✓                     │
│                                                                  │
│ ── Files (12) ─────────────────────────────────────────────────  │
│ ▸ src/login.go            +12 -4                                │
│   src/login_test.go       +30                                   │
│   ...                                                            │
├──────────────────────────────────────────────────────────────────┤
│  ↑↓:files  enter:diff  o:browser  esc:back  r:refresh  ?  q     │
└──────────────────────────────────────────────────────────────────┘
```

- Description rendered with `glamour` (markdown → ANSI).
- File list scrolls inside the detail viewport; `↑` / `↓` / `j` / `k` move the file cursor; `enter` opens the diff screen.

### Diff

A single file's diff in a `bubbles/viewport` (virtualized — handles large files without freezing).

- Title bar shows file path and renderer in use: `local+delta`, `local`, or `rest`.
- `↑` / `↓`, `pgup` / `pgdn`, `g` / `G` to scroll/jump.
- `esc` returns to detail.

### Keybinding table

| Context | Key | Action |
|---|---|---|
| any | `?` | toggle help |
| any | `q` / `ctrl+c` | quit |
| any | `r` | refresh current screen |
| any | `o` | open current PR (or file) in browser |
| list | `j`/`k` or `↑`/`↓` | move selection |
| list | `tab` / `shift-tab` or `l`/`h` | next / prev tab |
| list | `enter` | open detail |
| list | `/` | start filter; `esc` clears |
| detail | `↑`/`↓` or `j`/`k` | move file selection |
| detail | `enter` | open diff for selected file |
| detail | `esc` | back to list |
| diff | `↑`/`↓`, `pgup`/`pgdn`, `g`/`G` | scroll |
| diff | `esc` | back to detail |

---

## Local-clone discovery (for diffs)

New config keys:

```toml
repo_roots = ["~/git", "~/src"]
```

`gitlocal.Find(repoName) (path string, ok bool)`:

1. For each root, check if `<root>/<repoName>` exists and contains a `.git` dir.
2. Run `git -C <path> remote -v`; accept the path if any remote URL contains `dev.azure.com/{org}` and ends with `/{repoName}` (case-insensitive).
3. Cache results for the session in a `map[string]string`.

Diff path:

- Local: `git -C {path} diff {targetSha}..{sourceSha} -- {file}`. If `delta` is on PATH, pipe through it. Render the resulting ANSI in the diff viewport.
- REST: fetch base+target file contents via the diff API, run them through a small in-process unified-diff colorizer (red/green lines, no syntax highlighting).

If `repo_roots` is unset or no clone is found, fall through to REST silently.

---

## Caching

- PR list: in-memory only, replaced on refresh.
- Detail / changes / statuses / diff: **not cached** in Stage 1 — re-opening a PR re-fetches. Revisit if it feels slow in daily use.
- Local-clone path lookup: cached per session in `gitlocal`.

---

## Error UX

| Failure | Behavior |
|---|---|
| Auth failure on first call | Centered panel: `Couldn't get an ADO token. Try \`az login\`. Press q to quit, r to retry.` |
| Per-call timeout / 5xx after retries | Red footer line `error: {message}` for 8 s, then fade. Previous data stays. Logged at `error`. |
| Empty result set | Centered `No PRs in this tab.` (no error styling) |
| Diff fetch fails | Diff screen body shows the error and hints `o` to open in browser |
| `gitlocal` git command fails | Log a warning, fall through to REST diff |

All errors logged to `~/.adotop/logs/adotop.log` via the existing `applog` package.

---

## Testing

- **`internal/ado/*_test.go`** — `httptest.Server` verifies request shape (path, query params, `api-version`) and JSON decoding for list / detail / iterations / changes / statuses / diff.
- **`internal/gitlocal/*_test.go`** — `t.TempDir()` + `git init` + a fake remote URL; verifies `Find` matches by remote, not folder name; verifies cache.
- **`internal/ui/*_test.go`** — drive each sub-model with synthetic `tea.Msg`s and assert `View()` substrings: tab switching, `/`-filter typing, screen transitions, error rendering.
- **Manual smoke (recorded in PR description):** launch against `ceapex/Engineering`; verify all three tabs load; open a PR with a local clone (delta path) and one without (REST path); trigger a refresh; force a 401 by invalidating the cached token.

---

## Open questions

- **Build-status column.** ADO's PR list API doesn't return CI status — getting it per row is N+1 and would kill snappiness. Stage 1 shows status only in detail view; per-row column waits for Stage 4 (Pipelines) when we'll batch-fetch builds anyway. Roadmap text mentions a "build status icon" in the row but the trade-off is documented here and in commits.
- **"Review requested from me" semantics.** ADO doesn't separate "added as reviewer" from "vote requested". We approximate as "I am a reviewer AND my vote is 0". If users complain this matches "Assigned" too closely, revisit with a server-side `searchCriteria.reviewerId` + status filter once we learn the API better.

---

## Out of scope (handled in later stages)

- Stage 2: posting comments, voting, replying to threads, marking files viewed.
- Stage 4: full build/run views and per-row CI column.
- Stage 6: multi-org, themes, PAT auth fallback.

# adotop — Project Roadmap

**Status:** Draft roadmap, awaiting user review
**Date:** 2026-04-25
**Owner:** renzeyu

A terminal UI for Azure DevOps, built for daily heavy users. Ships as a single static binary on Windows, macOS, and Linux. Built in Go with Bubble Tea.

---

## Why this project

The ADO TUI space is wide open. ~10 hobby attempts exist on GitHub; none has more than 2 stars, none is feature-complete, several are abandoned. There is no `gh-dash` equivalent for ADO. Meanwhile, a meaningful population of engineers (especially in Microsoft and other ADO-using orgs) live in ADO 8 hours a day with no good terminal client. The goal is to become *the* ADO TUI by being built by someone who actually uses it daily.

## Guiding principles

1. **One screen, polished, before the next.** Every existing ADO TUI failed by sprawling. Each stage ships a single area to production quality before the next stage starts.
2. **Read before write.** Every area first ships read-only views. Mutations (comment, vote, transition state, queue build) come in a follow-up stage for that area.
3. **Reuse `az` CLI auth.** No PATs to manage; users who already `az login` get auth for free. PAT support added later if the community asks.
4. **Single binary, cross-platform, Windows-first.** Tested primarily on Windows Terminal; macOS and Linux are CI-validated.
5. **No premature abstraction across areas.** Don't build a generic "ADO entity browser." Build PRs well, then learn from that before building work items.

## Stack

- **Language:** Go (latest stable)
- **TUI framework:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) + Lipgloss + Bubbles
- **HTTP:** standard library `net/http`
- **ADO API:** REST (v7.1), GraphQL where it materially helps (mostly REST — ADO's GraphQL is limited)
- **Diff rendering:** shell out to `delta` if present on PATH, fallback to in-process colorized diff
- **Distribution:** GitHub Releases (cross-compiled binaries) → `winget`, `scoop`, `brew`, `go install`

---

## Roadmap overview

| Stage | Theme | Goal | Status |
|-------|-------|------|--------|
| 0 | Foundation | Auth, config, API client, app shell | Not started |
| 1 | PRs (read) | Browse and inspect PRs across repos | Not started |
| 2 | PRs (review) | Comment, vote, reply, approve from terminal | Not started |
| 3 | Work items | Browse, filter, transition my work items | Not started |
| 4 | Pipelines | Browse runs, view logs, retry failed builds | Not started |
| 5 | Repos | Browse code, view files, blame, commit history | Not started |
| 6 | Polish & ship | Multi-org, themes, packaging, docs | Not started |

Each stage ends with a tagged release. Stages 1–5 each get their own design spec written immediately before that stage starts (spec-driven, one stage at a time).

---

## Stage 0 — Foundation

**Goal:** Get to "I can run `adotop`, it authenticates, fetches my user info, and shows an empty shell." No business features.

**Deliverables:**
- Project scaffold: `cmd/adotop`, `internal/ado` (API client), `internal/ui` (Bubble Tea app), `internal/config`
- Auth: shell out to `az account get-access-token --resource 499b84ac-1321-427f-aa17-267ca6975798` to get an ADO bearer token; cache in memory; refresh on 401
- Config file at `~/.config/adotop/config.toml` (and `%APPDATA%\adotop\config.toml` on Windows): default org, default project, refresh interval, key bindings overrides
- API client with: typed request/response, retries with backoff, rate-limit handling, context cancellation
- App shell: header (current org/project/user), footer (keybinding hints), main area placeholder, `?` for help, `q` to quit
- Logging to a file (not stdout — would corrupt TUI)
- CI: build + test on Windows / macOS / Linux

**Done when:**
- `adotop` launches on Windows, macOS, Linux
- Shows "logged in as <user>" pulled from `/_apis/connectionData`
- Has a help screen and clean shutdown
- One internal API call works end-to-end

**Out of scope for stage 0:** any PR/work-item/pipeline UI.

---

## Stage 1 — PRs (read)

**Goal:** The PR dashboard you want open in a tmux pane all day.

**Deliverables:**
- Home screen: list of PRs in three filter tabs — **Assigned to me**, **Created by me**, **Review requested from me**
- Per-row: PR number, title, source→target branch, age, reviewer vote summary, build status icon
- Detail pane (toggle with `Enter`): title, description (markdown rendered), reviewers + each vote, linked work items, build/CI status, list of changed files
- Diff viewer: select a file, see its diff inline (delta if installed, fallback plain)
- Open in browser (`o`)
- Refresh on demand (`r`) + auto-refresh every N seconds (configurable, default 60s)
- Search/filter within current list (`/`)

**Done when:**
- All three filters work and feel snappy on a list of 100+ PRs
- Diff viewer handles large files without freezing the UI
- Tested daily by the author for 2 weeks

**Spec to write before this stage:** `2026-MM-DD-stage-1-prs-read-design.md`

---

## Stage 2 — PRs (review)

**Goal:** Do the actual code review without leaving the terminal. This is the differentiator no other ADO TUI has nailed.

**Deliverables:**
- View existing review threads inline with the diff (line-anchored where applicable)
- Reply to a thread
- Start a new thread on a specific line of a diff
- Submit your vote: Approve / Approve with suggestions / Wait for author / Reject
- Mark thread as Resolved / Active / Pending / Won't Fix
- Mark file as viewed (mirrors the ADO web "viewed" checkbox)

**Done when:**
- Author can do a full code review of a non-trivial PR (≥5 files, ≥3 review threads) entirely in the terminal
- Round-trip: comment posted in TUI is immediately visible in the ADO web UI

**Spec to write before this stage:** `2026-MM-DD-stage-2-prs-review-design.md`

---

## Stage 3 — Work items

**Goal:** Triage and update my work items without opening Boards.

**Deliverables:**
- Tabs: **Assigned to me**, **Following**, **Recently updated**, **Custom queries** (load saved ADO queries by ID)
- List view: ID, title, type (Bug/Task/User Story), state, sprint/iteration, tags
- Detail view: description (HTML→markdown), comments/discussion, links (to PRs, parent, children), attachments list
- State transitions via single keystroke (`s` → state picker showing valid next states)
- Add a comment
- Quick-create a child task off the current item

**Done when:**
- Author can run a daily standup off the TUI without opening the web UI

**Spec to write before this stage:** `2026-MM-DD-stage-3-work-items-design.md`

---

## Stage 4 — Pipelines

**Goal:** Babysit builds without browser tabs.

**Deliverables:**
- List recent runs across pipelines: pipeline name, branch, trigger, status, duration, who triggered
- Filter by pipeline / branch / status
- Detail view for a run: stages and jobs as a tree, status per node, durations
- Log viewer for a selected job: streaming if run is in progress, full log if finished, with search and filter to "errors only"
- Actions: re-run failed jobs, cancel running run, queue a new run against a chosen branch

**Done when:**
- Author can diagnose a CI failure (find the failing job, read the relevant log lines, retry) entirely in the TUI

**Spec to write before this stage:** `2026-MM-DD-stage-4-pipelines-design.md`

---

## Stage 5 — Repos

**Goal:** Browse code without `git clone` for a quick look.

**Deliverables:**
- File tree browser for a selected repo + branch
- File viewer with syntax highlighting (chroma)
- `git blame` view: per-line author + commit + age, jump to that commit
- Commit history for a file or directory
- Branch list with last-commit info; switch the active branch context
- Open file/line in browser

**Done when:**
- Author can answer "who wrote this line and why" entirely in the TUI

**Spec to write before this stage:** `2026-MM-DD-stage-5-repos-design.md`

**Explicit non-goal:** writing/committing code. This is a *reader*, not a replacement for git.

---

## Stage 6 — Polish & ship

**Goal:** Make it something other people install and stick with.

**Deliverables:**
- Multi-org / multi-project switcher with quick-switch UI
- Theme system + 2-3 built-in themes (default, light, high-contrast)
- PAT-based auth as a fallback for users without `az` CLI
- Packaging: `winget` manifest, `scoop` bucket entry, Homebrew tap, AUR package
- Telemetry opt-in (anonymous: which screens used, errors) — only if user opts in
- Documentation site (mkdocs or similar): install, configure, screenshots per stage
- Demo gif/video for the README
- Issue templates, contribution guide

**Done when:**
- Three external users have it installed and have used it for a week
- README looks good enough that someone lands on the repo and wants to try it

---

## What's deliberately not on the roadmap

- **Wiki, Test Plans, Artifacts, Service Connections, Permissions admin** — niche, not part of daily IC workflow
- **Mobile / web / desktop GUI** — we're a TUI, that's the point
- **AI features (suggested replies, summarization)** — could be stage 7+ but adds dependency risk; defer until core is solid
- **Plugin system** — premature; first prove the core areas

## Risks and mitigations

| Risk | Mitigation |
|------|-----------|
| ADO REST API surface is large and inconsistent across areas | Build one area fully before the next; learn the patterns as we go |
| `az` CLI not always installed | Stage 6 adds PAT fallback; auth interface kept narrow so this is a small change |
| Bubble Tea performance on 1000+ row lists | Use `bubbles/list` with virtualization; benchmark in stage 1 |
| Windows terminal rendering quirks | Test on Windows Terminal as primary; document known issues for ConHost/cmd |
| Project loses momentum after stage 2 | Each stage ships a tagged release that's useful on its own — abandoning at stage 3 still leaves a usable PR tool |

## How to use this roadmap

This is the *strategic* spec. Before starting each stage:

1. Re-read this roadmap; confirm the stage goal still matches reality
2. Write a stage-specific design spec (named above) covering screens, keybindings, API endpoints, data model, error cases
3. Write an implementation plan from that stage spec (via `superpowers:writing-plans`)
4. Execute the plan
5. Tag a release, update this roadmap's status table

Stages can be reordered or dropped based on what you learn. The roadmap is a guide, not a contract.

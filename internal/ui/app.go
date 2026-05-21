// Package ui hosts the Bubble Tea application shell.
package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/cache"
	"github.com/superyyrrzz/adotop/internal/config"
	"github.com/superyyrrzz/adotop/internal/gitlocal"
	"github.com/superyyrrzz/adotop/internal/ui/theme"
)

type screen int

const (
	screenList screen = iota
	screenDetail
)

type detailFocus int

const (
	focusFiles detailFocus = iota
	focusDiff
)

type Model struct {
	cfg    config.Config
	client *ado.Client
	git    *gitlocal.Finder
	cache  *cache.Store
	keys   KeyMap

	user   string
	myID   string
	screen screen

	list    ListModel
	detail  DetailModel
	preview DiffModel

	width, height int
	// footerErr is the sticky red banner for write-action failures
	// (vote/abandon errors, jump-to-PR misses). Cleared on next
	// keypress; "press any key to dismiss" is shown alongside.
	footerErr string
	// footerOK is the sticky neutral banner for write-action successes
	// ("PR #N abandoned"). Cleared on next keypress; no dismiss cue
	// because success is informational, not blocking.
	footerOK     string
	showHelp     bool
	// showSettings, when true, renders a read-only modal listing the
	// currently-loaded config values and the path they came from.
	// Toggled by the Settings key (`,`). Edits still go through
	// `adotop init` or the TOML file directly.
	showSettings bool
	useDelta     bool
	previewReqID int
	previewKey   string
	detailFocus  detailFocus
	scrollMem    map[string]int
	previewCache *diffBodyCache

	pendingAction pendingAction // empty action == no prompt

	// voteMenu is true when the `v` overlay is open and waiting for a
	// vote selection (a/s/w/r/c/esc). Modal: while open, all keypresses
	// route to the menu and global bindings (Refresh, ShowResolved, etc.)
	// are ignored.
	voteMenu bool

	// pendingG is set when the user pressed `g` and we're waiting for
	// the second key of the vim-style `gg` sequence. Any other key
	// cancels the pending state. Reset on screen change.
	pendingG bool

	threads        []ado.Thread
	showResolved   bool
	expandedThread map[int]bool
	// threadCursor records the active thread index per file path. Per-file
	// rather than global so jumping back to a file restores the cursor.
	// Absent or out-of-range == no thread selected; the user must press
	// [/] to land on a thread before C/x become meaningful.
	threadCursor map[string]int
	// prThreadCursor is the active index into the PR-level (unanchored)
	// thread list, used when the synthetic Discussion entry is selected
	// in the file list. Negative = unset (first [/] press lands on 0).
	// Single int (no map) since there's only one PR per detail screen.
	prThreadCursor int
	// inlineThreadLines maps a thread ID to the 0-based line index in
	// the spliced preview body where that thread starts. Populated by
	// refreshPreview after every inline splice; consumed by the cursor-
	// move handler to scroll the viewport to the selected thread.
	inlineThreadLines map[int]int
	// wrapDiff toggles soft-wrap on the diff preview viewport. Off by
	// default — most lines fit and visual line counts matter for
	// scanning large diffs. Press `w` to enable when reading a file
	// with long lines (minified JS, generated code, long URLs).
	wrapDiff bool

	// diffCtx is the unified-diff context size for the preview pane.
	// 0 = "use the default" (3 lines, matches `git diff` and ADO web).
	// Positive = explicit line count. -1 = "all" (full file, no folding).
	// `+`/`-` cycle through ctxLadder. The cache key includes this value
	// so revisiting a level is instant.
	diffCtx int

	// detailInflight counts how many of the four detail-screen background
	// fetches (detail, files, statuses, threads) are currently outstanding.
	// Bumped by loadDetail, decremented by each *LoadedMsg handler. When >0
	// the statusline shows a small ↻ glyph so the user knows the cached
	// snapshot they're looking at is being verified against the server.
	detailInflight int

	// initialPRID, when non-zero, makes the app jump to that PR's detail
	// screen as soon as auth resolves (myID is needed for the fetch).
	// Cleared after the jump fires so it doesn't repeat on later auth
	// refreshes. Set by Run() from the CLI argument.
	initialPRID int

	// loadingPRModal is non-zero while a "Loading PR #N…" modal is
	// overlaying the screen during URL-launch. Armed in Run() so the
	// very first frame already shows it (avoids list flash) and held
	// through the jump+detail fetches. Cleared by detailLoadedMsg
	// when the real PR data lands.
	loadingPRModal int
	// loadingFrame is the current spinner frame index, advanced by
	// loadingTickMsg every ~100ms while the modal is up.
	loadingFrame int

	// descModal holds the scrollable PR-description overlay state.
	// nil == closed. Populated by D in detail screen.
	descModal *descModalState

	// composeModal backs the in-TUI compose overlay used by c (new
	// thread) and C (reply). nil == closed. Replaced the previous
	// $EDITOR-suspends-TUI-immediately path so the diff stays visible
	// while the user types; ctrl+e from within the modal still drops
	// to $EDITOR for long prose.
	composeModal *composeModalState

	// commitsModal backs the M-key picker that lists the PR's commits
	// and lets the user view a single commit's diff. nil == closed.
	commitsModal *commitsModalState

	// viewingCommit, when non-nil, switches the detail screen from
	// "show the accumulated PR diff" to "show this commit's diff."
	// While set: m.detail.files is the commit's changed files,
	// effectiveSourceSha/effectiveTargetSha return the commit's
	// parent → commit SHA pair, and threads are hidden (PR iteration
	// line anchors don't align with arbitrary per-commit diffs).
	// Cleared by pressing M again or by exitCommitView.
	viewingCommit *ado.Commit
	// prFiles caches the full PR file list while viewingCommit is
	// set, so toggling back to "all commits" view restores the
	// original list without a re-fetch.
	prFiles []ado.FileChange

	// myVoteIsStale is true when the user has approved (vote ≥ 5) at
	// an earlier iteration than the latest push. Computed from the
	// VoteUpdate system threads + iteration timestamps in
	// staleVoteDataLoadedMsg. Surfaces as a "stale, re-approve
	// needed" annotation on the My Vote line.
	myVoteIsStale bool

	// recentsRefresh tracks the in-flight serial sweep that re-fetches
	// open PRs on the Recents tab so stale row data (votes, status,
	// merge state) gets refreshed without the user having to enter
	// each PR. queue holds the IDs left to refresh in order; lastAt
	// records when each PR was successfully refreshed in this session,
	// used to pick the oldest-refreshed PR first on the next sweep.
	// Both reset when the app exits — we don't persist refresh times.
	recentsRefresh recentsRefreshState
}

// recentsRefreshState is the in-flight queue + per-PR last-refreshed
// timestamps that drive the background recents sweep. Kept on Model so
// it survives between key handlers without globals.
type recentsRefreshState struct {
	queue  []int
	lastAt map[int]time.Time
	// inFlight is true while the sweep has work in progress. Used to
	// gate spinner ticks (no need to schedule frames when idle) and to
	// prevent re-entrant kickoffs (R while a sweep is already running
	// just no-ops; the user can wait for the in-flight sweep).
	inFlight bool
}

// pendingAction is a destructive operation awaiting y/n confirmation.
// `kind` is "" when no prompt is active.
type pendingAction struct {
	kind   string // "abandon" (extend later: "complete", etc.)
	prompt string // text shown to the user, e.g. "Abandon PR #123? (y/N)"
	run    func(m Model) tea.Cmd
}

func New(cfg config.Config, client *ado.Client) Model {
	// Resolve theme once at startup. ADOTOP_THEME can be "light",
	// "dark", "auto", or empty (auto-detect from terminal background).
	th := theme.New(os.Getenv("ADOTOP_THEME"))
	applyStyles(th)
	applyDiffTheme(th)
	keys := DefaultKeys()
	m := Model{
		cfg:            cfg,
		client:         client,
		git:            gitlocal.New(cfg.RepoRoots),
		keys:           keys,
		list:           NewList(keys),
		detail:         NewDetail(keys),
		preview:        NewDiff(keys),
		user:           "loading…",
		useDelta:       gitlocal.HasDelta(),
		scrollMem:      map[string]int{},
		previewCache:   newDiffBodyCache(5),
		expandedThread: map[int]bool{},
		threadCursor:   map[string]int{},
		prThreadCursor: -1,
	}
	st, err := cache.New()
	if err != nil {
		slog.Warn("cache disabled", "err", err)
		return m
	}
	m.cache = st
	if id, ok := st.LoadIdentity(); ok {
		m.user = id.DisplayName
		m.myID = id.UserID
		m.detail = m.detail.SetMyID(m.myID)
	}
	if cfg.Org != "" && cfg.Project != "" {
		for _, tab := range []ado.Tab{ado.TabAssigned, ado.TabCreated, ado.TabReviewRequested} {
			if prs, ok := st.LoadList(cfg.Org, cfg.Project, tab); ok {
				m.list, _ = m.list.Update(prsLoadedMsg{tab: tab, prs: prs})
			}
		}
	}
	if prs, ok := st.LoadRecents(); ok {
		prs = sortRecentsByStatus(prs)
		m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})
	}
	return m
}

type connDataMsg struct {
	data *ado.ConnectionData
	err  error
}

type tickMsg time.Time

// actionDoneMsg is the result of a write action (approve, abandon, etc.).
type actionDoneMsg struct {
	kind  string // "vote" | "abandon"
	prID  int
	err   error
	notes string // optional human note shown on success ("voted approve")

	// vote, when kind=="vote", is the vote value we just wrote (10/-10
	// etc.). The detail screen uses this for an optimistic update
	// because the round-trip GetPullRequest sometimes echoes the
	// reviewer back under a different ID descriptor (e.g., the AAD
	// group instead of the user) and our `rv.ID == myID` match misses.
	vote int
}

// threadsLoadedMsg is the response from GetPullRequestThreads.
type threadsLoadedMsg struct {
	threads   []ado.Thread
	err       error
	fromCache bool
}

// staleVoteDataLoadedMsg carries the iterations + vote events needed
// to compute "your approval is stale, re-approve required." Both
// fetches share one message because the staleness check needs both —
// either alone is useless. err is set when either underlying fetch
// failed; the staleness flag stays at false in that case (better to
// not flag stale than to wrongly flag it).
type staleVoteDataLoadedMsg struct {
	iterations []ado.Iteration
	events     []ado.VoteEvent
	err        error
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetchConnectionData(), tick(m.cfg.RefreshInterval.Duration)}
	if m.loadingPRModal != 0 {
		cmds = append(cmds, scheduleLoadingTick())
	}
	return tea.Batch(cmds...)
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
	if tab == ado.TabRecents {
		var prs []ado.PRSummary
		if m.cache != nil {
			prs, _ = m.cache.LoadRecents()
		} else {
			prs = append([]ado.PRSummary(nil), m.list.prs[ado.TabRecents]...)
		}
		prs = sortRecentsByStatus(prs)
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

// loadDetail fans out the four detail-screen fetches (PR, files,
// statuses, threads) and — when a cached snapshot exists for this PR —
// also dispatches synthetic *LoadedMsg events with the cached payload
// so the screen paints instantly. The fresh fetches still run in
// parallel and overwrite cached values via the existing Update path
// when they return.
//
// detailInflight tracks how many fetches are outstanding so the
// statusline can show a refresh indicator while the cached view is
// being verified against the server.
func (m Model) loadDetail(s ado.PRSummary) (Model, tea.Cmd) {
	m.detailInflight = 4

	cmds := make([]tea.Cmd, 0, 8)
	if m.cache != nil {
		if snap, ok := m.cache.LoadDetail(s.ID); ok {
			// Each cached field comes back as the same *LoadedMsg the
			// network path uses. Empty-field omission keeps the existing
			// "loading" placeholders if a previous session never managed
			// to fetch that endpoint.
			if snap.Detail != nil {
				d := snap.Detail
				cmds = append(cmds, func() tea.Msg { return detailLoadedMsg{detail: d, fromCache: true} })
			}
			if snap.Files != nil {
				files := snap.Files
				cmds = append(cmds, func() tea.Msg { return filesLoadedMsg{files: files, fromCache: true} })
			}
			if snap.Statuses != nil {
				st := snap.Statuses
				cmds = append(cmds, func() tea.Msg { return statusesLoadedMsg{statuses: st, fromCache: true} })
			}
			if snap.Threads != nil {
				th := snap.Threads
				cmds = append(cmds, func() tea.Msg { return threadsLoadedMsg{threads: th, fromCache: true} })
			}
		}
	}

	cmds = append(cmds,
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
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			threads, err := m.client.GetPullRequestThreads(ctx, s.RepoID, s.ID)
			return threadsLoadedMsg{threads: threads, err: err}
		},
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			its, iterErr := m.client.GetPullRequestIterations(ctx, s.RepoID, s.ID)
			events, voteErr := m.client.GetPullRequestVoteEvents(ctx, s.RepoID, s.ID)
			err := iterErr
			if err == nil {
				err = voteErr
			}
			return staleVoteDataLoadedMsg{iterations: its, events: events, err: err}
		},
	)
	return m, tea.Batch(cmds...)
}

// markDetailFetchDone decrements the in-flight counter that powers the
// statusline refresh indicator. Bottoms out at 0 so an extra arrival
// (cached + fresh both for the same field) doesn't underflow. We rely
// on counter==0 as the "all four fetches accounted for" signal.
func (m Model) markDetailFetchDone() Model {
	if m.detailInflight > 0 {
		m.detailInflight--
	}
	return m
}

// persistDetailField writes the current detail snapshot to the
// per-PR cache file, applying mutate to whatever is on disk first
// (read-modify-write so a parallel fetch's prior write isn't lost).
// Best-effort: cache failures are logged but don't surface to the user.
func (m Model) persistDetailField(mutate func(snap *cache.DetailSnapshot)) {
	if m.cache == nil {
		return
	}
	prID := m.detail.Summary().ID
	if prID == 0 {
		return
	}
	snap, _ := m.cache.LoadDetail(prID)
	if snap == nil {
		snap = &cache.DetailSnapshot{PRID: prID}
	}
	snap.PRID = prID
	mutate(snap)
	if err := m.cache.SaveDetail(*snap); err != nil {
		slog.Warn("cache: save detail", "pr", prID, "err", err)
	}
}

// loadThreadsOnly refreshes only the threads slice for the active PR.
// Used after thread/comment writes so the UI reflects the new state
// without paying for the four-endpoint fetch that loadDetail does.
func (m Model) loadThreadsOnly(s ado.PRSummary) tea.Cmd {
	if s.RepoID == "" || s.ID == 0 {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		threads, err := client.GetPullRequestThreads(ctx, s.RepoID, s.ID)
		return threadsLoadedMsg{threads: threads, err: err}
	}
}

// maybeDropTerminalCache removes the cached snapshot when the PR has
// transitioned to a terminal state (completed/abandoned). Those PRs
// don't change again, so the disk slot is better spent on active ones.
func (m Model) maybeDropTerminalCache(d *ado.PRDetail) {
	if m.cache == nil || d == nil {
		return
	}
	switch strings.ToLower(d.Status) {
	case "completed", "abandoned":
		if err := m.cache.DropDetail(d.ID); err != nil {
			slog.Warn("cache: drop terminal", "pr", d.ID, "err", err)
		}
	}
}

// approveCurrent issues a vote=10 against the current PR. Safe to call
// repeatedly — ADO treats it as setting the vote to 10.
func (m Model) approveCurrent() tea.Cmd {
	return m.setVoteCurrent(10, "voted approve")
}

// setVoteCurrent sets the caller's vote on the active PR. vote uses
// ADO's scale: 10=approved, 5=approved-with-suggestions, 0=no-vote,
// -5=waiting-for-author, -10=rejected. label is the success message
// shown in the footer.
//
// Logs the request to the file-only slog at Info level so we have a
// record of what was attempted; the result lands as actionDoneMsg and
// is logged by that handler at Error level if it fails.
func (m Model) setVoteCurrent(vote int, label string) tea.Cmd {
	s := m.detail.Summary()
	if s.ID == 0 || s.RepoID == "" || m.myID == "" {
		slog.Info("action skipped", "kind", "vote", "pr", s.ID, "reason", "missing PR/repo/identity")
		return func() tea.Msg {
			return actionDoneMsg{kind: "vote", prID: s.ID, err: fmt.Errorf("vote: missing PR/repo/identity")}
		}
	}
	repoID, prID := s.RepoID, s.ID
	slog.Info("action requested", "kind", "vote", "pr", prID, "vote", vote, "label", label)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.SetReviewerVote(ctx, repoID, prID, m.myID, vote)
		return actionDoneMsg{kind: "vote", prID: prID, err: err, notes: label, vote: vote}
	}
}

// abandonCurrent flips the PR status to abandoned.
func (m Model) abandonCurrent() tea.Cmd {
	s := m.detail.Summary()
	if s.ID == 0 || s.RepoID == "" {
		slog.Info("action skipped", "kind", "abandon", "pr", s.ID, "reason", "missing PR/repo")
		return func() tea.Msg {
			return actionDoneMsg{kind: "abandon", prID: s.ID, err: fmt.Errorf("abandon: missing PR/repo")}
		}
	}
	repoID, prID := s.RepoID, s.ID
	slog.Info("action requested", "kind", "abandon", "pr", prID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.client.AbandonPullRequest(ctx, repoID, prID)
		return actionDoneMsg{kind: "abandon", prID: prID, err: err, notes: "abandoned"}
	}
}

func (m Model) loadDiff(target diffTarget, current DiffModel, requestID int, s ado.PRSummary, file ado.FileChange, sourceSha, targetSha string) (DiffModel, tea.Cmd) {
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
	dm := m.sizeDiffModel(current.SetHeader(file.Path, renderer), target)
	ctxN := ctxLines(m.diffCtx)
	client := m.client
	repoID := s.RepoID
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), m.useDelta, ctxN)
			if err == nil {
				return diffLoadedMsg{content: out, target: target, requestID: requestID}
			}
			// Local diff failed (almost always: clone is behind the PR's
			// iteration SHAs). Fall back to REST so the user still sees
			// a diff. The renderer label flips so they know the local
			// fast-path didn't work — useful both for the "why is this
			// slow" question and as a hint to `git fetch`.
			slog.Warn("local diff failed; falling back to REST", "repo", s.Repo, "file", file.Path, "err", err)
			src, tgt, rerr := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
			if rerr != nil {
				// REST also failed — return the original git error,
				// which carries the actual reason (e.g. "fatal: Invalid
				// revision range …"). That's more actionable than the
				// REST error, which would just be the HTTP fallout.
				return diffLoadedMsg{err: err, target: target, requestID: requestID}
			}
			return diffLoadedMsg{
				content:   simpleDiff(tgt, src, file.Path, ctxN),
				target:    target,
				requestID: requestID,
				renderer:  "rest (local stale)",
			}
		}
		src, tgt, err := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return diffLoadedMsg{err: err, target: target, requestID: requestID}
		}
		return diffLoadedMsg{content: simpleDiff(tgt, src, file.Path, ctxN), target: target, requestID: requestID}
	}
	return dm, cmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		var c1, c2 tea.Cmd
		m.list, c1 = m.list.Update(msg)
		m.detail, c2 = m.detail.Update(msg)
		m.preview = m.sizeDiffModel(m.preview, diffTargetPreview)
		return m, tea.Batch(c1, c2)
	case connDataMsg:
		if msg.err != nil {
			m.footerErr = "auth: " + msg.err.Error()
			return m, nil
		}
		m.user = msg.data.DisplayName()
		m.myID = msg.data.AuthenticatedUser.ID
		m.detail = m.detail.SetMyID(m.myID)
		if m.cache != nil {
			if err := m.cache.SaveIdentity(m.myID, m.user); err != nil {
				slog.Warn("cache: save identity", "err", err)
			}
		}
		cmds := []tea.Cmd{m.loadList(m.list.Tab())}
		if m.list.Tab() == ado.TabRecents {
			cmds = append(cmds, startRecentsRefreshSweep())
		}
		if m.initialPRID != 0 {
			prID := m.initialPRID
			m.initialPRID = 0
			m.loadingPRModal = prID
			cmds = append(cmds, func() tea.Msg { return jumpRequestedMsg{ID: prID} })
		}
		return m, tea.Batch(cmds...)
	case loadingTickMsg:
		if m.loadingPRModal == 0 {
			return m, nil
		}
		m.loadingFrame++
		return m, scheduleLoadingTick()
	case tickMsg:
		// Refresh all three server-backed tabs regardless of screen so
		// when the user returns from detail view the list is current.
		// Recents is a local-only synthesized view (no server endpoint),
		// so it's skipped — UpdatePR keeps it fresh from detail loads
		// and PatchRecents persists it across restarts.
		var cmds []tea.Cmd
		for _, tab := range []ado.Tab{ado.TabAssigned, ado.TabCreated, ado.TabReviewRequested} {
			if c := m.loadList(tab); c != nil {
				cmds = append(cmds, c)
			}
		}
		cmds = append(cmds, tick(m.cfg.RefreshInterval.Duration))
		return m, tea.Batch(cmds...)
	case tabSwitchedMsg:
		var cmds []tea.Cmd
		cmds = append(cmds, m.loadList(msg.Tab))
		if msg.Tab == ado.TabRecents {
			// Switching to Recents triggers a background re-fetch of
			// open PRs so stale rows (votes / status / merge state)
			// catch up without the user opening each one.
			cmds = append(cmds, startRecentsRefreshSweep())
		}
		return m, tea.Batch(cmds...)
	case recentsRefreshKickoffMsg:
		mm, cmd := m.kickRecentsRefresh()
		return mm, cmd
	case prRefreshedMsg:
		mm, cmd := m.handlePRRefreshed(msg)
		return mm, cmd
	case commitsLoadedMsg:
		if m.commitsModal == nil {
			return m, nil
		}
		m.commitsModal.loading = false
		if msg.err != nil {
			m.commitsModal.err = msg.err.Error()
			return m, nil
		}
		m.commitsModal.commits = msg.commits
		return m, nil
	case commitChangesLoadedMsg:
		if msg.err != nil {
			m.footerErr = "view commit: " + msg.err.Error()
			return m, nil
		}
		// Stash the PR's full file list on first entry so the user
		// can toggle back without a re-fetch. Subsequent commit
		// switches reuse the same stash.
		if m.viewingCommit == nil {
			m.prFiles = m.detail.files
		}
		c := msg.commit
		m.viewingCommit = &c
		m.detail, _ = m.detail.Update(filesLoadedMsg{files: msg.files})
		// Reset preview key so the next selection re-fetches against
		// the commit's parent → commit SHA pair (driven by
		// effectiveSourceSha/effectiveTargetSha).
		m.previewKey = ""
		mm, cmd := m.queuePreviewForSelection()
		return mm, cmd
	case prsLoadedMsg:
		if msg.err == nil && m.cache != nil && m.cfg.Org != "" && m.cfg.Project != "" && msg.tab != ado.TabRecents {
			if err := m.cache.SaveList(m.cfg.Org, m.cfg.Project, msg.tab, msg.prs); err != nil {
				slog.Warn("cache: save list", "tab", msg.tab, "err", err)
			}
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case detailLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
			if msg.err == nil {
				m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Detail = msg.detail })
				m.maybeDropTerminalCache(msg.detail)
				// Propagate fresh PR state to the list and recents cache so
				// the row the user came from updates without a list refetch.
				if msg.detail != nil {
					m.list = m.list.UpdatePR(msg.detail.PRSummary)
					if m.cache != nil {
						if err := m.cache.PatchRecents(msg.detail.PRSummary); err != nil {
							slog.Warn("cache: patch recents", "pr", msg.detail.ID, "err", err)
						}
					}
				}
			}
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	case filesLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
			if msg.err == nil {
				m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Files = msg.files })
			}
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	case statusesLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
			if msg.err == nil {
				m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Statuses = msg.statuses })
			}
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	case threadsLoadedMsg:
		if !msg.fromCache {
			m = m.markDetailFetchDone()
		}
		if msg.err != nil {
			slog.Warn("threads: fetch failed", "err", msg.err)
			return m, nil
		}
		m.threads = msg.threads
		m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
		m = m.refreshPreview()
		if !msg.fromCache {
			m.persistDetailField(func(snap *cache.DetailSnapshot) { snap.Threads = msg.threads })
		}
		return m, nil
	case staleVoteDataLoadedMsg:
		// Compute "your approval is stale" from iterations +
		// VoteUpdate events. Errors are non-fatal — if either fetch
		// fails we just leave myVoteIsStale=false (don't pretend to
		// know without the data).
		if msg.err != nil {
			slog.Warn("stale-vote: fetch failed", "err", msg.err)
			return m, nil
		}
		m.myVoteIsStale = computeMyStaleApproval(msg.iterations, msg.events, m.myID)
		m.detail = m.detail.SetMyVoteIsStale(m.myVoteIsStale)
		return m, nil
	case diffLoadedMsg:
		if msg.target != diffTargetPreview {
			return m, nil
		}
		if msg.requestID != m.previewReqID {
			return m, nil
		}
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		if msg.err == nil && m.previewKey != "" {
			m.previewCache.Set(m.detail.Summary().ID, m.previewKey, msg.content)
		}
		if off, ok := m.scrollMem[m.previewKey]; ok {
			m.preview.vp.SetYOffset(off)
		}
		m = m.refreshPreview()
		return m, cmd
	case prefetchLoadedMsg:
		if msg.err == nil {
			m.previewCache.Set(m.detail.Summary().ID, msg.key, msg.content)
		} else {
			m.previewCache.Drop(msg.key)
		}
		return m, nil
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
		m.loadingPRModal = 0
		if msg.err != nil {
			m.footerErr = fmt.Sprintf("jump #%d: %v", msg.prID, msg.err)
			return m, nil
		}
		return m.openDetail(msg.summary)
	case composeResultMsg:
		if msg.tmpPath != "" {
			os.Remove(msg.tmpPath)
		}
		if msg.err != nil {
			m.footerErr = "compose: " + msg.err.Error()
			return m, nil
		}
		if strings.TrimSpace(msg.body) == "" {
			// Empty == cancelled; quiet no-op.
			return m, nil
		}
		if msg.targetThreadID != 0 {
			return m, m.postReplyCmd(msg.targetThreadID, msg.body)
		}
		return m, m.postNewThreadCmd(msg.body, msg.targetFilePath)
	case actionDoneMsg:
		if msg.err != nil {
			// File-only log: the footer banner is transient so an
			// after-the-fact debug needs the structured record. Prefer
			// the raw err.Error() over friendlyErr — the friendly form
			// strips the ADO-side reason which is exactly what we
			// want when reading the log.
			slog.Error("action failed", "kind", msg.kind, "pr", msg.prID, "err", msg.err.Error())
			m.footerErr = fmt.Sprintf("%s PR #%d: %s", msg.kind, msg.prID, friendlyErr(msg.err))
			return m, nil
		}
		slog.Info("action succeeded", "kind", msg.kind, "pr", msg.prID, "notes", msg.notes)
		m.footerOK = fmt.Sprintf("PR #%d %s", msg.prID, msg.notes)
		// Thread-scoped writes only need the threads list refreshed; pay
		// for one fetch instead of the four-endpoint full refresh that
		// vote/abandon trigger. List rows don't reflect thread state, so
		// we skip loadList too.
		switch msg.kind {
		case "resolveThread", "reactivateThread", "postThread", "postComment":
			if m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
				return m, m.loadThreadsOnly(m.detail.Summary())
			}
			return m, nil
		}
		// Optimistic local update for votes: ADO sometimes echoes the
		// reviewer back under a different identity descriptor (group vs
		// user) so the GET-side `rv.ID == myID` match misses and the
		// reviewer line still shows "No vote" after refresh. We KNOW the
		// vote we just wrote succeeded, so write it locally too.
		if msg.kind == "vote" && m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
			m.detail = m.detail.SetMyVote(msg.vote, m.myID, m.user)
		}
		// Refresh the detail in place so vote/status update; also bust the
		// list cache for the active tab so the row reflects the change.
		var cmds []tea.Cmd
		if m.screen == screenDetail && m.detail.Summary().ID == msg.prID {
			var dCmd tea.Cmd
			m, dCmd = m.loadDetail(m.detail.Summary())
			cmds = append(cmds, dCmd)
		}
		cmds = append(cmds, m.loadList(m.list.Tab()))
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		// Modal overlays trap keys before any screen handler sees
		// them, so j/k inside the description scroll the modal
		// instead of the file list, and esc closes the modal
		// instead of leaving the PR.
		if m.descModalOpen() {
			mm, cmd := m.updateDescModal(msg)
			return mm, cmd
		}
		// Compose overlay is similarly modal — while it's up, every
		// key (including the textarea's own enter/arrow handling)
		// belongs to it. ctrl+s/esc/ctrl+e are intercepted inside;
		// everything else delegates to the textarea.
		if m.composeModalOpen() {
			mm, cmd := m.updateComposeModal(msg)
			return mm, cmd
		}
		// Commits picker is modal too — j/k navigates the list,
		// enter selects, esc cancels. M toggles it open/closed
		// from the global path below.
		if m.commitsModalOpen() {
			mm, cmd := m.updateCommitsModal(msg)
			return mm, cmd
		}
		// Confirmation prompt swallows all keys until resolved.
		if m.pendingAction.kind != "" {
			if keyMatches(msg, m.keys.ConfirmYes) {
				run := m.pendingAction.run
				m.pendingAction = pendingAction{}
				return m, run(m)
			}
			if keyMatches(msg, m.keys.ConfirmNo) {
				m.pendingAction = pendingAction{}
				return m, nil
			}
			return m, nil
		}
		// Any key dismisses a sticky error or success banner. The
		// "press any key to dismiss" cue in the statusline tells the
		// user this is what'll happen for errors. Successes also
		// clear so the banner doesn't follow you across screens.
		// We do NOT swallow the key — the dismiss is a side effect of
		// whatever the user typed next, so nav stays fluid.
		if m.footerErr != "" {
			m.footerErr = ""
		}
		if m.footerOK != "" {
			m.footerOK = ""
		}
		if keyMatches(msg, m.keys.QuitForce) {
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
		if keyMatches(msg, m.keys.Settings) {
			m.showSettings = !m.showSettings
			return m, nil
		}
		if m.showSettings {
			if msg.Type == tea.KeyEsc {
				m.showSettings = false
			}
			return m, nil
		}
		switch m.screen {
		case screenList:
			return m.updateListScreen(msg)
		case screenDetail:
			return m.updateDetailScreen(msg)
		}
	}
	switch m.screen {
	case screenList:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case screenDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	return m, nil
}

// openDetail switches to the Detail screen for s, records the visit in the
// recents cache, and returns the load command. Idempotent: caller may call
// from list, recents tab, or ID-jump.
func (m Model) openDetail(s ado.PRSummary) (Model, tea.Cmd) {
	m.detail = m.detail.SetSummary(s)
	m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
	m.previewKey = ""
	m.detailFocus = focusFiles
	m.scrollMem = map[string]int{}
	m.threads = nil
	m.expandedThread = map[int]bool{}
	m.threadCursor = map[string]int{}
	m.prThreadCursor = -1
	m.myVoteIsStale = false
	m.screen = screenDetail
	// NOTE: previewCache survives PR re-open so bouncing list↔detail
	// stays snappy. Refresh (R) explicitly clears the current PR below.
	if m.cache != nil {
		if err := m.cache.RecordVisit(s); err != nil {
			slog.Warn("cache: record visit", "pr", s.ID, "err", err)
		}
		if recents, ok := m.cache.LoadRecents(); ok {
			recents = sortRecentsByStatus(recents)
			m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: recents})
		}
	}
	return m.loadDetail(s)
}

func (m Model) updateListScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a prompt (filter or jump-to-ID) is active, route all keys to the
	// list so its prompt handlers see Enter/Esc/Backspace before the global
	// Open/Quit/Browser shortcuts intercept them.
	if m.list.filtering || m.list.jumping {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	switch {
	case keyMatches(msg, m.keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.Open):
		if s, ok := m.list.Selected(); ok {
			mm, cmd := m.openDetail(s)
			return mm, cmd
		}
	case keyMatches(msg, m.keys.Refresh):
		cmds := []tea.Cmd{m.loadList(m.list.Tab())}
		if m.list.Tab() == ado.TabRecents {
			// R on the Recents tab also re-runs the background sweep
			// so the user can force a re-fetch when something looks
			// stale. Idempotent — kickRecentsRefresh no-ops when a
			// sweep is already running.
			cmds = append(cmds, startRecentsRefreshSweep())
		}
		return m, tea.Batch(cmds...)
	case keyMatches(msg, m.keys.Browser):
		if s, ok := m.list.Selected(); ok {
			if s.URL == "" {
				slog.Warn("open in browser: PR has no URL", "pr", s.ID)
			} else if err := OpenInBrowser(s.URL); err != nil {
				slog.Warn("open in browser failed", "url", s.URL, "err", err)
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDetailScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Vote menu is modal: while open, only its own letters and esc are
	// recognized. This lets us reuse single chars like `r` for reject
	// without conflicting with the global Refresh binding.
	if m.voteMenu {
		return m.handleVoteMenuKey(msg)
	}
	// Vim-style gg/G: `g` is a lead-in that arms pendingG, the next
	// keystroke completes (or cancels) the sequence. `G` is a one-shot
	// jump-to-end. Behavior depends on which pane is focused:
	//   - Files focus: jump file cursor to first/last in display order.
	//   - Diff focus:  jump preview viewport to top/bottom.
	if m.pendingG {
		m.pendingG = false
		if keyMatches(msg, m.keys.GotoTop) {
			return m.gotoTop()
		}
		// Any other key cancels and falls through to normal handling.
	}
	if keyMatches(msg, m.keys.GotoTop) {
		// First `g`: arm pending and wait for the second.
		m.pendingG = true
		return m, nil
	}
	if keyMatches(msg, m.keys.GotoEnd) {
		return m.gotoEnd()
	}

	switch {
	case msg.Type == tea.KeyTab:
		if m.detailFocus == focusFiles {
			m.detailFocus = focusDiff
		} else {
			m.detailFocus = focusFiles
		}
		return m, nil
	case msg.Type == tea.KeyShiftTab:
		if m.detailFocus == focusDiff {
			m.detailFocus = focusFiles
		} else {
			m.detailFocus = focusDiff
		}
		return m, nil
	case keyMatches(msg, m.keys.Back), keyMatches(msg, m.keys.Quit):
		// Cascaded back-out: from diff focus, step back to the file list
		// (one screen feels like two). From file focus, leave the PR.
		// ctrl+c still hard-quits unconditionally.
		if m.detailFocus == focusDiff {
			m.detailFocus = focusFiles
			return m, nil
		}
		m.screen = screenList
		m.detailFocus = focusFiles
		return m, nil
	case keyMatches(msg, m.keys.Refresh):
		m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
		m.previewKey = ""
		m.scrollMem = map[string]int{}
		m.previewCache.ClearPR(m.detail.Summary().ID)
		return m.loadDetail(m.detail.Summary())
	case keyMatches(msg, m.keys.Browser):
		u := m.detail.Summary().URL
		if u == "" {
			slog.Warn("open in browser: PR has no URL", "pr", m.detail.Summary().ID)
		} else if err := OpenInBrowser(u); err != nil {
			slog.Warn("open in browser failed", "url", u, "err", err)
		}
		return m, nil
	case keyMatches(msg, m.keys.Approve):
		return m, m.approveCurrent()
	case keyMatches(msg, m.keys.VoteMenu):
		// Open the vote overlay; next key picks the vote (a/s/w/r/c/esc).
		m.voteMenu = true
		return m, nil
	case keyMatches(msg, m.keys.DescModal):
		// D opens the full description in a scrollable modal so the
		// user can read past the inline cap without leaving the TUI.
		// No-op when the description is empty (openDescModal returns
		// the model unchanged).
		m = m.openDescModal()
		return m, nil
	case keyMatches(msg, m.keys.CommitsPicker):
		// M toggles between the per-commit view and the full PR
		// view. When viewingCommit is set, M exits commit view
		// (restores the PR file list and SHAs); otherwise it opens
		// the picker so the user can choose a commit to inspect.
		if m.viewingCommit != nil {
			m = m.exitCommitView()
			return m, nil
		}
		mm, cmd := m.openCommitsModal()
		return mm, cmd
	case keyMatches(msg, m.keys.JumpToComments):
		// J jumps the diff viewport to the comments block on the
		// selected file. Two side effects, both in service of "land on
		// something readable":
		//
		//  1. If the file's only comments are resolved AND the show-
		//     resolved filter is off, flip it on first — otherwise J
		//     would land on "(no open comments on this file)" which
		//     defeats the point.
		//  2. Expand all visible threads, so the viewport opens at the
		//     full content rather than headlines the user must Enter
		//     to expand.
		if f, ok := m.detail.SelectedFile(); ok {
			if !m.showResolved && !hasAnyOpenForFile(m.threads, f.Path) && hasAnyResolvedForFile(m.threads, f.Path) {
				m.showResolved = true
				m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
				m = m.refreshPreview()
			}
			m, _ = m.expandThreadsForFile(f.Path)
			m = m.refreshPreview()
		}
		m = m.scrollPreviewToComments()
		return m, nil
	case keyMatches(msg, m.keys.ShowResolved):
		// R is a pure filter toggle — flip showResolved and rebuild
		// the preview so resolved threads appear/disappear in place.
		// Earlier this key also expanded threads and scrolled to the
		// comments block on the SHOW direction, but that conflated
		// "filter" with "navigate" — J now owns the navigate role, so
		// R can be the single-purpose toggle its label promises.
		m.showResolved = !m.showResolved
		m.detail = m.detail.SetPRThreads(m.threads, m.showResolved)
		m = m.refreshPreview()
		return m, nil
	case keyMatches(msg, m.keys.NextThread):
		if !m.threadKeysActive() {
			return m, nil
		}
		m = m.moveThreadCursor(+1)
		m = m.refreshPreview()
		m = m.scrollPreviewToSelectedThread()
		return m, nil
	case keyMatches(msg, m.keys.PrevThread):
		if !m.threadKeysActive() {
			return m, nil
		}
		m = m.moveThreadCursor(-1)
		m = m.refreshPreview()
		m = m.scrollPreviewToSelectedThread()
		return m, nil
	case keyMatches(msg, m.keys.ToggleResolve):
		if !m.threadKeysActive() {
			return m, nil
		}
		return m, m.toggleResolveCurrentThread()
	case keyMatches(msg, m.keys.ComposeThread):
		if !m.threadKeysActive() {
			return m, nil
		}
		return m.openComposeNewModal(), nil
	case keyMatches(msg, m.keys.ReplyThread):
		if !m.threadKeysActive() {
			return m, nil
		}
		return m.openComposeReplyModal(), nil
	case keyMatches(msg, m.keys.WrapDiff):
		// Soft-wrap toggle for the diff preview. Off by default so
		// large diffs stay scannable; on for files with long lines.
		// refreshPreview re-feeds the viewport from the cached
		// unwrapped render, applying wrapDiffLines when wrapDiff=on.
		m.wrapDiff = !m.wrapDiff
		// Reset scroll so the user lands at the top of the wrapped
		// view — the visual line numbering changed under them.
		m.preview.vp.GotoTop()
		m = m.refreshPreview()
		return m, nil
	case keyMatches(msg, m.keys.CtxMore), keyMatches(msg, m.keys.CtxLess):
		return m.cycleDiffCtx(keyMatches(msg, m.keys.CtxMore))
	case keyMatches(msg, m.keys.Open):
		// enter has exactly one meaning on the detail screen: drill
		// from Files focus into Diff focus. No-op in Diff focus
		// (use esc/tab to leave). Thread expansion is handled by
		// ExpandThread (space) — separating the two prevents the
		// "enter does different things based on selection state"
		// overload that caused enter-on-Discussion to silently no-op.
		if m.detailFocus == focusFiles {
			m.detailFocus = focusDiff
		}
		return m, nil
	case keyMatches(msg, m.keys.ExpandThread):
		// space toggles expansion of the thing under the cursor:
		//   * Discussion selected → toggle the selected PR thread.
		//   * Diff focus + file selected → toggle ALL visible threads
		//     on the file (matches the historical batch behavior).
		// Works in either focus when Discussion is selected so the
		// user doesn't have to tab back to Files just to expand a
		// PR-level comment they're reading in the preview pane.
		if m.detail.IsDiscussionSelected() {
			tid := m.currentThreadID()
			if tid != 0 {
				m.expandedThread[tid] = !m.expandedThread[tid]
				m = m.refreshPreview()
				m = m.scrollPreviewToSelectedThread()
			}
			return m, nil
		}
		if m.detailFocus == focusDiff {
			f, ok := m.detail.SelectedFile()
			if ok {
				var expanded bool
				m, expanded = m.toggleThreadsForFile(f.Path)
				m = m.refreshPreview()
				if expanded {
					m = m.scrollPreviewToComments()
				}
			}
			return m, nil
		}
		// Files focus + a real file selected: nothing to expand —
		// the user would need to drop into Diff focus first to see
		// the threads. Silent no-op rather than an error, since
		// space on a file row reads as "nothing happened" naturally.
		return m, nil
	case keyMatches(msg, m.keys.Abandon):
		s := m.detail.Summary()
		if s.ID == 0 {
			return m, nil
		}
		m.pendingAction = pendingAction{
			kind:   "abandon",
			prompt: fmt.Sprintf("Abandon PR #%d? (y/esc)", s.ID),
			run:    func(mm Model) tea.Cmd { return mm.abandonCurrent() },
		}
		return m, nil
	case keyMatches(msg, m.keys.NextFile):
		// Walk in display (tree) order so n/N matches j/k. Without
		// neighborFile this would step through API order, which can
		// jump across the file tree in non-obvious directions.
		next := m.detail.neighborFile(+1)
		if next == m.detail.cursor {
			return m, nil
		}
		m.detail.cursor = next
		mm, previewCmd := m.syncPreviewForSelection()
		return mm, previewCmd
	case keyMatches(msg, m.keys.PrevFile):
		prev := m.detail.neighborFile(-1)
		if prev == m.detail.cursor {
			return m, nil
		}
		m.detail.cursor = prev
		mm, previewCmd := m.syncPreviewForSelection()
		return mm, previewCmd
	}
	if m.detailFocus == focusDiff {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}
	beforeDisc := m.detail.IsDiscussionSelected()
	before, beforeOK := m.detail.SelectedFile()
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	after, afterOK := m.detail.SelectedFile()
	afterDisc := m.detail.IsDiscussionSelected()
	cursorChanged := afterDisc != beforeDisc ||
		(afterOK && (!beforeOK || before.Path != after.Path))
	if cursorChanged {
		m, previewCmd := m.syncPreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	}
	return m, cmd
}

// gotoTop implements the gg sequence: in Files focus, jump cursor to
// the first file in display order and reload its preview; in Diff
// focus, scroll the preview viewport to the top.
func (m Model) gotoTop() (tea.Model, tea.Cmd) {
	if m.detailFocus == focusDiff {
		m.preview.vp.GotoTop()
		return m, nil
	}
	idx := m.detail.FirstDisplayFile()
	if idx < 0 || idx == m.detail.cursor {
		return m, nil
	}
	m.detail.cursor = idx
	mm, cmd := m.queuePreviewForSelection()
	return mm, cmd
}

// gotoEnd implements G: in Files focus, jump cursor to the last file
// in display order; in Diff focus, scroll viewport to the bottom.
func (m Model) gotoEnd() (tea.Model, tea.Cmd) {
	if m.detailFocus == focusDiff {
		m.preview.vp.GotoBottom()
		return m, nil
	}
	idx := m.detail.LastDisplayFile()
	if idx < 0 || idx == m.detail.cursor {
		return m, nil
	}
	m.detail.cursor = idx
	mm, cmd := m.queuePreviewForSelection()
	return mm, cmd
}

// handleVoteMenuKey resolves the open vote overlay. It always closes
// the menu (success or cancel) so the user can never get stuck in it.
// Recognized keys:
//
//	a -> approve (+10)
//	s -> approve with suggestions (+5)
//	w -> wait for author (-5)
//	r -> reject (-10)
//	c -> clear vote (0)
//	esc / any other key -> cancel, no API call
func (m Model) handleVoteMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.voteMenu = false
	if msg.Type == tea.KeyEsc {
		return m, nil
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return m, nil
	}
	switch msg.Runes[0] {
	case 'a':
		return m, m.setVoteCurrent(10, "voted approve")
	case 's':
		return m, m.setVoteCurrent(5, "voted approve w/ suggestions")
	case 'w':
		return m, m.setVoteCurrent(-5, "voted wait for author")
	case 'r':
		return m, m.setVoteCurrent(-10, "voted reject")
	case 'c':
		return m, m.setVoteCurrent(0, "cleared vote")
	}
	return m, nil
}

func (m Model) View() string {
	header := renderTopbar(m)
	footer := renderStatusline(m)
	var body string
	switch m.screen {
	case screenList:
		body = m.list.View()
	case screenDetail:
		body = m.detailPreviewView()
	}
	if m.loadingPRModal != 0 {
		bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
		if bodyH < 3 {
			bodyH = 24
		}
		body = overlayLoadingModal(body, m.loadingPRModal, m.loadingFrame, m.width, bodyH)
	}
	if m.descModalOpen() {
		bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
		if bodyH < 3 {
			bodyH = 24
		}
		body = overlayBox(body, m.renderDescModal(), m.width, bodyH)
	}
	if m.composeModalOpen() {
		bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
		if bodyH < 3 {
			bodyH = 24
		}
		body = overlayBox(body, m.renderComposeModal(), m.width, bodyH)
	}
	if m.commitsModalOpen() {
		bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
		if bodyH < 3 {
			bodyH = 24
		}
		body = overlayBox(body, m.renderCommitsModal(), m.width, bodyH)
	}
	if m.showHelp {
		bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
		if bodyH < 3 {
			bodyH = 24
		}
		body = overlayBox(body, renderHelpModal(m.width), m.width, bodyH)
	}
	if m.showSettings {
		bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
		if bodyH < 3 {
			bodyH = 24
		}
		path, _ := config.Path()
		body = overlayBox(body, renderSettingsModal(m.cfg, path, m.width), m.width, bodyH)
	}
	// Pad the body so the footer sticks to the bottom of the terminal,
	// regardless of how much content the current screen produced.
	if m.height > 0 {
		headerH := lipgloss.Height(header)
		footerH := lipgloss.Height(footer)
		bodyH := lipgloss.Height(body)
		// 2 blank-line spacers between header/body and body/footer.
		used := headerH + footerH + bodyH + 2
		if pad := m.height - used; pad > 0 {
			body = body + strings.Repeat("\n", pad)
		}
	}
	return strings.Join([]string{header, "", body, "", footer}, "\n")
}

// cycleDiffCtx advances (or rewinds) the diff context level and reloads
// the preview at the new level. Each level has its own cache slot, so
// flipping back to a previously-visited level is instant.
//
// We clear previewKey so queuePreviewForSelection treats this as a
// selection change — that's the existing path that handles cache lookup
// or fresh fetch + neighbor prefetch.
func (m Model) cycleDiffCtx(forward bool) (Model, tea.Cmd) {
	if m.screen != screenDetail || m.detail.Detail() == nil {
		return m, nil
	}
	if forward {
		m.diffCtx = nextCtx(m.diffCtx)
	} else {
		m.diffCtx = prevCtx(m.diffCtx)
	}
	m.previewKey = ""
	return m.queuePreviewForSelection()
}

// syncPreviewForSelection refreshes the preview pane after the file-
// list cursor moves. Three cases:
//  1. Discussion entry selected → render PR threads into the viewport
//     (no diff fetch needed).
//  2. Real file selected → delegate to queuePreviewForSelection (cache
//     hit or async fetch as before).
//  3. No selection (empty file list) → no-op.
//
// Also clears previewKey when leaving a file → Discussion so the next
// file selection re-queues a fetch instead of treating the cached key
// as still-current.
func (m Model) syncPreviewForSelection() (Model, tea.Cmd) {
	if m.detail.IsDiscussionSelected() {
		if m.previewKey != "" {
			m.scrollMem[m.previewKey] = m.preview.vp.YOffset
			m.previewKey = ""
		}
		m = m.refreshPreview()
		m.preview.vp.GotoTop()
		return m, nil
	}
	return m.queuePreviewForSelection()
}

func (m Model) queuePreviewForSelection() (Model, tea.Cmd) {
	if m.screen != screenDetail || m.detail.Detail() == nil {
		return m, nil
	}
	f, ok := m.detail.SelectedFile()
	if !ok {
		return m, nil
	}
	key := diffSelectionKey(m.effectiveSourceSha(), m.effectiveTargetSha(), f.Path, m.diffCtx)
	if key == m.previewKey {
		return m, nil
	}
	if m.previewKey != "" {
		m.scrollMem[m.previewKey] = m.preview.vp.YOffset
	}
	m.previewKey = key

	// Cache hit: render immediately (no async msg) and prefetch neighbors.
	if body, ok := m.previewCache.Get(key); ok && body != nil {
		renderer := "rest"
		if p, ok := m.git.Find(m.detail.Summary().Repo, m.cfg.Org); ok && p != "" {
			if m.useDelta {
				renderer = "local+delta"
			} else {
				renderer = "local"
			}
		}
		m.preview = m.sizeDiffModel(m.preview.SetHeader(f.Path, renderer), diffTargetPreview)
		m.previewReqID++
		// Pull the precolorized render so we skip Colorize on every j/k.
		rendered, _ := m.previewCache.Rendered(key)
		m.preview, _ = m.preview.Update(diffLoadedMsg{
			content: body, rendered: rendered, target: diffTargetPreview, requestID: m.previewReqID,
		})
		if off, ok := m.scrollMem[key]; ok {
			m.preview.vp.SetYOffset(off)
		}
		m = m.refreshPreview()
		mm, prefetchCmd := m.prefetchNeighbors()
		return mm, prefetchCmd
	}

	m.previewReqID++
	pm, cmd := m.loadDiff(diffTargetPreview, m.preview, m.previewReqID, m.detail.Summary(), f, m.effectiveSourceSha(), m.effectiveTargetSha())
	m.preview = pm
	mm, prefetchCmd := m.prefetchNeighbors()
	m = mm
	return m, tea.Batch(cmd, prefetchCmd)
}

type prefetchLoadedMsg struct {
	key     string
	content []byte
	err     error
}

type jumpResultMsg struct {
	prID    int
	summary ado.PRSummary
	err     error
}

func (m Model) prefetchNeighbors() (Model, tea.Cmd) {
	if m.detail.Detail() == nil {
		return m, nil
	}
	files := m.detail.files
	prID := m.detail.Summary().ID
	// Prefetch the ±3 nearest files in DISPLAY order so the cache warms
	// the rows the user will actually navigate to with j/k, not the
	// API-order neighbors that rarely match the sorted tree.
	const radius = 3
	idxs := m.detail.DisplayNeighbors(radius)
	var cmds []tea.Cmd
	for _, idx := range idxs {
		if idx < 0 || idx >= len(files) {
			continue
		}
		f := files[idx]
		key := diffSelectionKey(m.effectiveSourceSha(), m.effectiveTargetSha(), f.Path, m.diffCtx)
		if _, ok := m.previewCache.Get(key); ok {
			continue
		}
		// Reserve cache slot (nil) so we don't re-issue while in flight.
		m.previewCache.Reserve(prID, key)
		cmds = append(cmds, m.prefetchOne(f, key, m.effectiveSourceSha(), m.effectiveTargetSha()))
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m Model) prefetchOne(file ado.FileChange, key, sourceSha, targetSha string) tea.Cmd {
	s := m.detail.Summary()
	var clonePath string
	if p, ok := m.git.Find(s.Repo, m.cfg.Org); ok {
		clonePath = p
	}
	useDelta := m.useDelta
	repoID := s.RepoID
	client := m.client
	ctxN := ctxLines(m.diffCtx)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), useDelta, ctxN)
			if err == nil {
				return prefetchLoadedMsg{key: key, content: out}
			}
			// Mirror loadDiff: local-diff failure falls back to REST so
			// the prefetch still warms the cache. If we returned the
			// error here, the user's next selection would re-pay the
			// full fetch latency on top of seeing the broken state.
			if client == nil {
				return prefetchLoadedMsg{key: key, err: err}
			}
			src, tgt, rerr := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
			if rerr != nil {
				return prefetchLoadedMsg{key: key, err: err}
			}
			return prefetchLoadedMsg{key: key, content: simpleDiff(tgt, src, file.Path, ctxN)}
		}
		if client == nil {
			return prefetchLoadedMsg{key: key, err: nil, content: nil}
		}
		src, tgt, err := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return prefetchLoadedMsg{key: key, err: err}
		}
		return prefetchLoadedMsg{key: key, content: simpleDiff(tgt, src, file.Path, ctxN)}
	}
}

func (m Model) detailPreviewView() string {
	layout := m.detailLayout()
	detail := m.detail.SetPaneSize(layout.leftWidth, layout.bodyHeight)
	left := detail.ViewWithFocus(m.detailFocus == focusFiles)
	if !layout.split {
		// Single-column layout: stack the two panes vertically. The
		// preview still gets the bordered chrome so the user sees it
		// as a discrete unit, just full-width.
		previewBody := m.previewPaneBody()
		previewTitle := m.previewPaneTitle()
		right := borderedPane(previewTitle, previewBody, layout.rightWidth, maxInt(6, layout.bodyHeight/2), m.detailFocus == focusDiff)
		return strings.Join([]string{left, "", right}, "\n")
	}
	leftPane := lipgloss.NewStyle().
		Width(layout.leftWidth - 2).
		MaxWidth(layout.leftWidth - 2).
		Height(layout.bodyHeight).
		Render(left)
	leftPane = leftPaneFocusStripe(leftPane, m.detailFocus == focusFiles)
	previewBody := m.previewPaneBody()
	previewTitle := m.previewPaneTitle()
	rightPane := borderedPane(previewTitle, previewBody, layout.rightWidth, layout.bodyHeight, m.detailFocus == focusDiff)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

// leftPaneFocusStripe prefixes each line of the left pane with a 1-col
// mauve accent stripe ("▌") when focused, or 2 spaces when not. Same
// vocabulary as the list cursor stripe — focus is a peer of "you are
// here" so it gets the same Cursor color. The right pane already
// carries its own focus cue (border tint), so we only style the left.
//
// Caller must reduce the leftPane render width by 2 cols beforehand so
// the total horizontal budget stays unchanged.
func leftPaneFocusStripe(pane string, focused bool) string {
	var prefix string
	if focused {
		prefix = Cursor.Render("▌") + " "
	} else {
		prefix = "  "
	}
	lines := strings.Split(pane, "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}

// previewPaneTitle returns the label spliced into the right pane's
// border. The selected file path is what's actually useful — the
// previous "Diff Preview" label was redundant chrome since the user
// already knows they're looking at a diff.
func (m Model) previewPaneTitle() string {
	if m.preview.file != "" {
		return m.preview.file
	}
	if f, ok := m.detail.SelectedFile(); ok {
		return f.Path
	}
	return "Diff"
}

// previewPaneBody is what goes inside the bordered preview pane. No
// title prefix — the border carries that — just the diff viewport, or
// a placeholder when no file is loaded.
//
// When the selected file has a sticky resolved-comments hint to show
// (resolved comments hidden, OR the show-resolved toggle is on), the
// hint is spliced over the row that will become the pane's bottom row
// after borderedPane's fit pass. That index — `innerH-1`, where
// innerH = bodyHeight - paneChromeHeight — is the last visible body
// row the user actually sees, so the band stays sticky regardless of
// scroll position.
func (m Model) previewPaneBody() string {
	if m.preview.file == "" {
		if _, ok := m.detail.SelectedFile(); !ok {
			return Faint.Render("No changed files available.")
		}
		return Faint.Render("Loading selected file diff…")
	}
	body := m.preview.View()
	band := m.stickyResolvedBand()
	if band == "" {
		return body
	}
	layout := m.detailLayout()
	bodyH := layout.bodyHeight
	if !layout.split {
		bodyH = maxInt(6, layout.bodyHeight/2)
	}
	innerH := bodyH - paneChromeHeight
	if innerH <= 0 {
		return body
	}
	return spliceAtIndex(body, innerH-1, band)
}

// stickyResolvedBand returns a one-line affordance to splice at the
// bottom of the diff pane. Empty when there's nothing worth showing —
// no selected file, or no resolved comments on it.
//
// The band is intentionally NOT suppressed when the viewport is at the
// bottom: short diffs hit AtBottom on first render (maxYOffset==0) and
// would silently lose the affordance, which is the whole point of the
// sticky band. A small duplicate with the in-flow comments header is
// the lesser evil.
func (m Model) stickyResolvedBand() string {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return ""
	}
	hidden := 0
	for _, t := range m.threads {
		if t.FilePath == f.Path && t.IsResolved() {
			hidden++
		}
	}
	width := m.preview.vp.Width
	if width <= 0 {
		return ""
	}
	switch {
	case !m.showResolved && hidden > 0:
		label := fmt.Sprintf(" ▾ %d resolved comment", hidden)
		if hidden != 1 {
			label += "s"
		}
		label += " on this file — R to show "
		return padBandToWidth(Wait.Bold(true).Render(label), width)
	case m.showResolved && hidden > 0:
		return padBandToWidth(Approve.Render(" ▾ showing resolved — R to hide "), width)
	}
	return ""
}

// hasAnyResolved is the PR-wide variant of hasAnyResolvedForFile —
// returns true when ANY thread on the PR is resolved. Used by the
// statusline to decide whether to advertise the R toggle: hiding it
// when no resolved threads exist means the user never wonders why R
// "does nothing" on a fresh PR.
func hasAnyResolved(all []ado.Thread) bool {
	for _, t := range all {
		if t.IsResolved() {
			return true
		}
	}
	return false
}

// hasAnyResolvedForFile is the predicate behind the show-resolved
// mirror band — the band only makes sense when the file actually has
// resolved threads to look at.
func hasAnyResolvedForFile(all []ado.Thread, path string) bool {
	for _, t := range all {
		if t.FilePath == path && t.IsResolved() {
			return true
		}
	}
	return false
}

// hasAnyOpenForFile is the inverse predicate, used by the J handler to
// decide whether jumping to the comments block would land the user on
// "(no open comments on this file)" — in which case J should flip the
// showResolved filter on so there's something to read.
func hasAnyOpenForFile(all []ado.Thread, path string) bool {
	for _, t := range all {
		if t.FilePath == path && !t.IsResolved() {
			return true
		}
	}
	return false
}

// padBandToWidth right-pads a styled band string with spaces so the
// chip sits flush left and the rest of the row is blank — keeps the
// band feeling like the bottom edge of the pane.
func padBandToWidth(band string, width int) string {
	used := lipgloss.Width(band)
	if used >= width {
		return band
	}
	return band + strings.Repeat(" ", width-used)
}

// spliceAtIndex replaces the line at idx in body with replacement.
// If body has fewer than idx+1 lines it is padded with empty rows so
// the replacement still lands at idx — needed because borderedPane's
// fit pass would otherwise pad AFTER the band and push it up.
func spliceAtIndex(body string, idx int, replacement string) string {
	lines := strings.Split(body, "\n")
	for len(lines) <= idx {
		lines = append(lines, "")
	}
	lines[idx] = replacement
	return strings.Join(lines, "\n")
}

type previewLayout struct {
	split      bool
	bodyHeight int
	leftWidth  int
	rightWidth int
}

func (m Model) detailLayout() previewLayout {
	// Chrome: topbar bar + topbar rule + blank + blank + footer = 5 lines.
	// Underestimating this pushes the topbar's first row off the top of
	// the alt-screen, which is how "the bar disappears" manifests on the
	// detail screen.
	layout := previewLayout{
		bodyHeight: maxInt(10, m.height-5),
		leftWidth:  80,
		rightWidth: 80,
	}
	if m.width <= 0 {
		return layout
	}
	if m.width < 100 {
		layout.leftWidth = m.width
		layout.rightWidth = m.width
		return layout
	}
	left := m.width * 2 / 5
	left = maxInt(36, minInt(left, m.width-40))
	right := maxInt(30, m.width-left-1)
	if right < 30 {
		layout.leftWidth = m.width
		layout.rightWidth = m.width
		return layout
	}
	layout.split = true
	layout.leftWidth = left
	layout.rightWidth = right
	return layout
}

func (m Model) sizeDiffModel(dm DiffModel, target diffTarget) DiffModel {
	width, height := m.diffViewportSize(target)
	return dm.SetSize(width, height)
}

func (m Model) diffViewportSize(target diffTarget) (int, int) {
	switch target {
	case diffTargetPreview:
		layout := m.detailLayout()
		// borderedPane wraps the preview in a rounded border + 1col
		// inner padding, eating paneChromeWidth horizontally and
		// paneChromeHeight vertically. The viewport sees the inside.
		if layout.split {
			return maxInt(20, layout.rightWidth-paneChromeWidth), maxInt(3, layout.bodyHeight-paneChromeHeight)
		}
		return maxInt(20, layout.rightWidth-paneChromeWidth), maxInt(6, layout.bodyHeight/2-paneChromeHeight)
	default:
		return maxInt(20, m.width), maxInt(3, m.height-5)
	}
}

func diffSelectionKey(sourceSha, targetSha, path string, ctxLevel int) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00c%d", sourceSha, targetSha, path, ctxLevel)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func orPlaceholder(s, p string) string {
	if s == "" {
		return p
	}
	return s
}

// friendlyErr unwraps an ADO APIError to its server-supplied "message" field
// so the footer shows e.g. "Pull request is completed" instead of a raw 400
// dump. Falls back to err.Error() for non-API errors.
func friendlyErr(err error) string {
	var apiErr *ado.APIError
	if errors.As(err, &apiErr) {
		return apiErr.FriendlyMessage()
	}
	return err.Error()
}

// simpleDiff produces a unified diff (target → source) for the REST
// fallback when no local clone is available. Uses an LCS-based line
// comparison so the output is line-accurate (not a wholesale "everything
// removed, everything added" dump). Applies the same whitespace
// normalization as the local-git path so it matches the ADO web UI:
//   - CRLF → LF
//   - trailing whitespace stripped
//
// ctxLines controls how many surrounding lines each hunk emits; pass 3
// for the standard view, larger for the user's "expand context" mode.
// Set ADOTOP_DIFF_STRICT=1 to skip normalization.
func simpleDiff(target, source []byte, path string, ctxLines int) []byte {
	// Binary files: line-based LCS would happily split the bytes on
	// LF and emit them as `+`/`-` lines, which renders as a wall of
	// terminal garbage (the symptom we hit when a PR added an image
	// or compressed asset). Mirror what `git diff` does — emit a
	// single "Binary files ... differ" stanza so the user can tell
	// what changed without us trying to text-diff bytes that aren't
	// text.
	if isBinaryDiffInput(target) || isBinaryDiffInput(source) {
		return binaryDiffSummary(target, source, path)
	}
	t := normalizeForDiff(string(target))
	s := normalizeForDiff(string(source))
	tl := strings.Split(t, "\n")
	sl := strings.Split(s, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "--- a%s\n+++ b%s\n", path, path)
	hunks := lcsUnifiedHunks(tl, sl, ctxLines)
	for _, h := range hunks {
		b.WriteString(h)
	}
	return []byte(b.String())
}

// isBinaryDiffInput uses the same heuristic as `git diff`: a NUL byte
// in the first 8 KB marks the buffer as binary. Cheap, well-known,
// and matches user expectations from git. nil/empty buffers count as
// not-binary (one side of an add/delete is always empty bytes).
func isBinaryDiffInput(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	const probe = 8 * 1024
	end := len(b)
	if end > probe {
		end = probe
	}
	for i := 0; i < end; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

// binaryDiffSummary emits a unified-diff-shaped placeholder so the
// downstream renderer (Colorize, line-number map, splice walker)
// keeps working. The sole hunk is one `+` line carrying a human-
// readable byte-size summary — no actual content, so nothing to
// mangle. Sizes use binary KB (1024) like the rest of the app and
// `git diff --stat`.
func binaryDiffSummary(target, source []byte, path string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a%s\n+++ b%s\n", path, path)
	fmt.Fprintf(&b, "Binary files differ (%s → %s)\n",
		formatByteSize(len(target)), formatByteSize(len(source)))
	return []byte(b.String())
}

// formatByteSize returns a short human-readable size like "12.3 KB"
// for diff summaries. Matches the units `git diff --stat` uses so
// the output reads as a peer of git's own placeholders.
func formatByteSize(n int) string {
	const (
		kb = 1024
		mb = 1024 * 1024
	)
	switch {
	case n == 0:
		return "0 bytes"
	case n < kb:
		return fmt.Sprintf("%d bytes", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	}
}

func normalizeForDiff(s string) string {
	if v := strings.TrimSpace(os.Getenv("ADOTOP_DIFF_STRICT")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// lcsUnifiedHunks computes a Myers-style LCS between two line slices and
// emits unified-diff hunks with the given context size. Good enough to
// match what `git diff` would produce for typical PR-sized files.
func lcsUnifiedHunks(a, b []string, ctx int) []string {
	n, m := len(a), len(b)
	// Build LCS length table.
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	// Walk the table to emit ops: " " (equal), "-" (a-only), "+" (b-only).
	type op struct {
		kind byte
		text string
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for i < n {
		ops = append(ops, op{'-', a[i]})
		i++
	}
	for j < m {
		ops = append(ops, op{'+', b[j]})
		j++
	}

	// Group ops into hunks with `ctx` lines of surrounding context.
	var hunks []string
	k := 0
	for k < len(ops) {
		// Find the next change.
		for k < len(ops) && ops[k].kind == ' ' {
			k++
		}
		if k >= len(ops) {
			break
		}
		// Hunk start = max(0, k-ctx).
		start := k - ctx
		if start < 0 {
			start = 0
		}
		// Find hunk end: walk forward including changes; allow up to 2*ctx
		// equal lines between change blocks before splitting.
		end := k
		gap := 0
		for end < len(ops) {
			if ops[end].kind == ' ' {
				gap++
				if gap > 2*ctx {
					end -= gap - ctx
					break
				}
			} else {
				gap = 0
			}
			end++
		}
		if end > len(ops) {
			end = len(ops)
		}
		// Compute line numbers for header.
		var aStart, bStart, aLen, bLen int
		for x := 0; x < start; x++ {
			if ops[x].kind != '+' {
				aStart++
			}
			if ops[x].kind != '-' {
				bStart++
			}
		}
		aStart++
		bStart++
		var body strings.Builder
		for x := start; x < end; x++ {
			body.WriteByte(ops[x].kind)
			body.WriteString(ops[x].text)
			body.WriteByte('\n')
			switch ops[x].kind {
			case ' ':
				aLen++
				bLen++
			case '-':
				aLen++
			case '+':
				bLen++
			}
		}
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
		hunks = append(hunks, header+body.String())
		k = end
	}
	return hunks
}

// Run starts the Bubble Tea program with the alt screen. If initialPRID
// is non-zero, the app jumps directly to that PR's detail screen on
// startup (the list still loads in the background so Esc returns to a
// populated list).
func Run(cfg config.Config, client *ado.Client, initialPRID int) error {
	m := New(cfg, client)
	m.initialPRID = initialPRID
	// Arm the modal up front so it's visible on the very first frame
	// — otherwise the list flashes before connDataMsg arrives and sets
	// the flag.
	if initialPRID != 0 {
		m.loadingPRModal = initialPRID
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// RunDemo starts the normal TUI against preloaded fixture rows. It is used by
// `adotop demo` so recordings exercise the real render/update path without
// requiring Azure DevOps credentials or leaking tenant data.
func RunDemo(cfg config.Config, client *ado.Client, prs []ado.PRSummary) error {
	m := New(cfg, client)
	m.cache = nil
	m.user = "Alice Anderson"
	m.myID = "user-alice"
	m.detail = m.detail.SetMyID(m.myID)
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: prs})
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

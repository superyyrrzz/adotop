package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// recentsRefreshTickMsg drives the spinner animation while a sweep is
// in flight. We tick at ~100ms — fast enough to read as motion, slow
// enough that we don't re-render constantly when no actual refresh
// completes between frames.
type recentsRefreshTickMsg time.Time

const recentsRefreshTickInterval = 100 * time.Millisecond

// prRefreshedMsg is the single-PR result of a sweep step. The handler
// patches the list row, persists the fresh PR to recents, records the
// refresh time, and dispatches the next fetch (or finishes the sweep
// when the queue drains).
type prRefreshedMsg struct {
	pr  ado.PRSummary
	err error
}

// recentsRefreshKickoffMsg is enqueued whenever we want to start (or
// restart) the sweep. The handler builds the queue from the current
// Recents list, sorts it oldest-refreshed first, and dispatches the
// first fetch. Kept as a message (not a direct call) so it can be
// batched with prsLoadedMsg without ordering hazards.
type recentsRefreshKickoffMsg struct{}

// startRecentsRefreshSweep returns a tea.Cmd that emits a kickoff
// message. Cheap to call from anywhere; the handler decides whether
// there's anything to refresh.
func startRecentsRefreshSweep() tea.Cmd {
	return func() tea.Msg { return recentsRefreshKickoffMsg{} }
}

// recentsRefreshTick schedules one spinner-animation frame.
func recentsRefreshTick() tea.Cmd {
	return tea.Tick(recentsRefreshTickInterval, func(t time.Time) tea.Msg {
		return recentsRefreshTickMsg(t)
	})
}

// isOpenForRefresh reports whether a PR's lifecycle state is one that
// can still change. Merged/Abandoned are terminal and never need
// refresh; everything else (active, draft, conflict, blocked, queued,
// failed, checking, empty status) might.
func isOpenForRefresh(p ado.PRSummary) bool {
	switch strings.ToLower(p.Status) {
	case "completed", "abandoned":
		return false
	}
	return true
}

// buildRefreshQueue returns the open-PR IDs from the Recents tab
// ordered by least-recently-refreshed first. PRs we've never refreshed
// in this session sort to the front (zero time < any real time), so a
// fresh app launch sweeps the entire open list before re-touching any
// PR. Stable ordering by ID for deterministic test behavior when two
// PRs share a refresh timestamp.
func buildRefreshQueue(recents []ado.PRSummary, lastAt map[int]time.Time) []int {
	type item struct {
		id int
		ts time.Time
	}
	items := make([]item, 0, len(recents))
	for _, p := range recents {
		if !isOpenForRefresh(p) {
			continue
		}
		items = append(items, item{id: p.ID, ts: lastAt[p.ID]})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ts.Equal(items[j].ts) {
			return items[i].id < items[j].id
		}
		return items[i].ts.Before(items[j].ts)
	})
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

// fetchOnePRForRefreshCmd issues a single GetPullRequestByID and
// returns a prRefreshedMsg. Errors don't abort the sweep — we want to
// keep going even if one PR is permission-blocked or rate-limited.
func (m Model) fetchOnePRForRefreshCmd(prID int) tea.Cmd {
	client := m.client
	myID := m.myID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := client.GetPullRequestByID(ctx, prID, myID)
		if err != nil || d == nil {
			return prRefreshedMsg{pr: ado.PRSummary{ID: prID}, err: err}
		}
		return prRefreshedMsg{pr: d.PRSummary}
	}
}

// kickRecentsRefresh starts (or restarts) the sweep. Returns the
// updated model and the cmd that fetches the first PR (plus a spinner
// tick if work is pending). Idempotent: a no-op when the Recents tab
// has no open PRs, or when a sweep is already in flight.
func (m Model) kickRecentsRefresh() (Model, tea.Cmd) {
	if m.recentsRefresh.inFlight {
		return m, nil
	}
	if m.cache == nil {
		return m, nil
	}
	recents, ok := m.cache.LoadRecents()
	if !ok || len(recents) == 0 {
		return m, nil
	}
	if m.recentsRefresh.lastAt == nil {
		m.recentsRefresh.lastAt = map[int]time.Time{}
	}
	queue := buildRefreshQueue(recents, m.recentsRefresh.lastAt)
	if len(queue) == 0 {
		return m, nil
	}
	m.recentsRefresh.queue = queue
	m.recentsRefresh.inFlight = true
	first := queue[0]
	m.recentsRefresh.queue = queue[1:]
	m.list = m.list.SetRefreshing(first)
	return m, tea.Batch(
		m.fetchOnePRForRefreshCmd(first),
		recentsRefreshTick(),
	)
}

// handlePRRefreshed processes a single sweep result and dispatches the
// next fetch (or finishes the sweep). The list row update happens via
// UpdatePR — same path detailLoadedMsg uses, so visible state stays
// consistent with the rest of the app.
func (m Model) handlePRRefreshed(msg prRefreshedMsg) (Model, tea.Cmd) {
	if msg.err == nil && msg.pr.ID != 0 {
		m.list = m.list.UpdatePR(msg.pr)
		if m.cache != nil {
			_ = m.cache.PatchRecents(msg.pr)
		}
	}
	if m.recentsRefresh.lastAt == nil {
		m.recentsRefresh.lastAt = map[int]time.Time{}
	}
	if msg.pr.ID != 0 {
		m.recentsRefresh.lastAt[msg.pr.ID] = time.Now()
	}
	if len(m.recentsRefresh.queue) == 0 {
		// Sweep finished — clear spinner state, leave inFlight=false so
		// future kickoffs can run.
		m.recentsRefresh.inFlight = false
		m.list = m.list.SetRefreshing(0)
		return m, nil
	}
	next := m.recentsRefresh.queue[0]
	m.recentsRefresh.queue = m.recentsRefresh.queue[1:]
	m.list = m.list.SetRefreshing(next)
	return m, m.fetchOnePRForRefreshCmd(next)
}

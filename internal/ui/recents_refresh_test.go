package ui

import (
	"testing"
	"time"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestIsOpenForRefreshTerminalStates verifies the small status filter
// that drives the sweep — completed/abandoned PRs are terminal and
// must never be refreshed; everything else (active, draft, conflict,
// blocked, empty) might still change and stays in the queue.
func TestIsOpenForRefreshTerminalStates(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{"Active", true},
		{"", true},
		{"completed", false},
		{"Completed", false},
		{"abandoned", false},
		{"draft", true}, // draft is a flag, not a Status; still in-flight
	}
	for _, c := range cases {
		got := isOpenForRefresh(ado.PRSummary{ID: 1, Status: c.status})
		if got != c.want {
			t.Fatalf("status=%q: got %v want %v", c.status, got, c.want)
		}
	}
}

// TestBuildRefreshQueueOldestFirst: PRs we've never refreshed sort to
// the head; ties broken by ID for deterministic ordering. Terminal PRs
// drop out entirely.
func TestBuildRefreshQueueOldestFirst(t *testing.T) {
	now := time.Now()
	recents := []ado.PRSummary{
		{ID: 10, Status: "active"},
		{ID: 20, Status: "completed"}, // dropped
		{ID: 30, Status: "active"},
		{ID: 40, Status: "active"},
		{ID: 50, Status: "abandoned"}, // dropped
	}
	lastAt := map[int]time.Time{
		10: now.Add(-1 * time.Minute),       // refreshed recently
		30: now.Add(-1 * time.Hour),         // older
		// 40 has no entry — never refreshed, sorts first
	}
	got := buildRefreshQueue(recents, lastAt)
	want := []int{40, 30, 10}
	if len(got) != len(want) {
		t.Fatalf("len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

// TestBuildRefreshQueueStableByID: equal timestamps (e.g., the
// initial all-zero state) must order by ID so test runs and screen
// renders are deterministic.
func TestBuildRefreshQueueStableByID(t *testing.T) {
	recents := []ado.PRSummary{
		{ID: 30, Status: "active"},
		{ID: 10, Status: "active"},
		{ID: 20, Status: "active"},
	}
	got := buildRefreshQueue(recents, map[int]time.Time{})
	want := []int{10, 20, 30}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

// TestHandlePRRefreshedAdvancesQueue: each prRefreshedMsg removes the
// completed PR from the queue, marks the next one as in-flight, and
// returns a fetch cmd. When the queue drains, in-flight clears and
// the spinner stops.
func TestHandlePRRefreshedAdvancesQueue(t *testing.T) {
	m := newTestModel()
	m.recentsRefresh.queue = []int{20, 30}
	m.recentsRefresh.inFlight = true
	m.list = m.list.SetRefreshing(10)

	mm, cmd := m.handlePRRefreshed(prRefreshedMsg{pr: ado.PRSummary{ID: 10, Status: "active"}})
	if cmd == nil {
		t.Fatalf("expected fetch cmd for next PR, got nil")
	}
	if mm.list.refreshingPRID != 20 {
		t.Fatalf("after PR 10 refreshed: refreshingPRID=%d, want 20", mm.list.refreshingPRID)
	}
	if got, ok := mm.recentsRefresh.lastAt[10]; !ok || got.IsZero() {
		t.Fatalf("expected lastAt[10] to be recorded, got %v ok=%v", got, ok)
	}
	if len(mm.recentsRefresh.queue) != 1 || mm.recentsRefresh.queue[0] != 30 {
		t.Fatalf("queue not advanced: %v", mm.recentsRefresh.queue)
	}

	// Process the second PR — queue still has one (30) → fetch cmd, refreshingPRID=30.
	mm, cmd = mm.handlePRRefreshed(prRefreshedMsg{pr: ado.PRSummary{ID: 20, Status: "active"}})
	if cmd == nil {
		t.Fatalf("expected fetch cmd for PR 30")
	}
	if mm.list.refreshingPRID != 30 {
		t.Fatalf("refreshingPRID=%d, want 30", mm.list.refreshingPRID)
	}

	// Process the last PR — queue empty → no cmd, in-flight cleared, spinner cleared.
	mm, cmd = mm.handlePRRefreshed(prRefreshedMsg{pr: ado.PRSummary{ID: 30, Status: "active"}})
	if cmd != nil {
		t.Fatalf("expected nil cmd at end of sweep, got %v", cmd)
	}
	if mm.recentsRefresh.inFlight {
		t.Fatalf("inFlight should clear when queue drains")
	}
	if mm.list.refreshingPRID != 0 {
		t.Fatalf("spinner should clear, got refreshingPRID=%d", mm.list.refreshingPRID)
	}
}

// TestHandlePRRefreshedSkipsErrorRowUpdate: when the per-PR fetch
// errors, we still advance the queue — but we don't overwrite the row
// with empty data (which would wipe the existing badge/votes).
func TestHandlePRRefreshedSkipsErrorRowUpdate(t *testing.T) {
	m := newTestModel()
	m.recentsRefresh.queue = []int{}
	m.recentsRefresh.inFlight = true
	m.list = m.list.SetRefreshing(10)
	// Pre-seed the list with a row so we can detect a wipe.
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: []ado.PRSummary{
		{ID: 10, Title: "should survive", Status: "active"},
	}})

	mm, _ := m.handlePRRefreshed(prRefreshedMsg{
		pr:  ado.PRSummary{ID: 10},
		err: errFakeFetch,
	})
	rows := mm.list.prs[ado.TabRecents]
	if len(rows) != 1 || rows[0].Title != "should survive" {
		t.Fatalf("error path wiped row: %+v", rows)
	}
}

var errFakeFetch = &fetchErr{}

type fetchErr struct{}

func (*fetchErr) Error() string { return "fake fetch err" }

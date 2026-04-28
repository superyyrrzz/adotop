package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/cache"
)

// TestLoadDetailDispatchesCachedMessagesFirst is the regression guard
// for the "PR opens slowly" UX work: when a cached snapshot exists for
// the requested PR, loadDetail must emit synthetic *LoadedMsgs (with
// fromCache=true) so the screen paints before the network responds.
//
// We can't actually run network fetches here — the test client has no
// real server — but we can drain the returned tea.Cmd one step at a
// time and assert that the cached messages are present and tagged.
func TestLoadDetailDispatchesCachedMessagesFirst(t *testing.T) {
	st := newTestCache(t)
	prID := 42
	snap := cache.DetailSnapshot{
		PRID:   prID,
		Detail: &ado.PRDetail{PRSummary: ado.PRSummary{ID: prID, Title: "cached title"}},
		Files:  []ado.FileChange{{Path: "/cached.go", ChangeType: "edit"}},
		Statuses: []ado.StatusCheck{{Context: "ci", State: "succeeded"}},
		Threads: []ado.Thread{{ID: 1, Status: "active",
			Comments: []ado.Comment{{Author: "x", Content: "hi"}}}},
	}
	if err := st.SaveDetail(snap); err != nil {
		t.Fatalf("SaveDetail: %v", err)
	}

	m := newTestModel()
	m.cache = st
	m, cmd := m.loadDetail(ado.PRSummary{ID: prID, Title: "open-time title"})

	if cmd == nil {
		t.Fatalf("loadDetail returned nil cmd")
	}
	if m.detailInflight != 4 {
		t.Fatalf("detailInflight after loadDetail: got %d want 4", m.detailInflight)
	}

	// Drain the batch and bucket cached vs network messages. Network
	// commands will block on real I/O so we run each command in its own
	// goroutine with a short timeout — for the test we only need to
	// observe the immediately-returning cached commands.
	got := drainCachedMsgs(cmd)
	wantTypes := map[string]bool{
		"detailLoadedMsg":   false,
		"filesLoadedMsg":    false,
		"statusesLoadedMsg": false,
		"threadsLoadedMsg":  false,
	}
	for _, msg := range got {
		switch v := msg.(type) {
		case detailLoadedMsg:
			if !v.fromCache {
				continue
			}
			wantTypes["detailLoadedMsg"] = true
			if v.detail == nil || v.detail.Title != "cached title" {
				t.Fatalf("cached detail wrong: %+v", v.detail)
			}
		case filesLoadedMsg:
			if !v.fromCache {
				continue
			}
			wantTypes["filesLoadedMsg"] = true
			if len(v.files) != 1 || v.files[0].Path != "/cached.go" {
				t.Fatalf("cached files wrong: %+v", v.files)
			}
		case statusesLoadedMsg:
			if !v.fromCache {
				continue
			}
			wantTypes["statusesLoadedMsg"] = true
		case threadsLoadedMsg:
			if !v.fromCache {
				continue
			}
			wantTypes["threadsLoadedMsg"] = true
		}
	}
	for k, ok := range wantTypes {
		if !ok {
			t.Fatalf("missing cached %s in batch", k)
		}
	}
}

// TestLoadDetailNoCacheStillReturnsNetworkCmds — sanity check that the
// new code path doesn't break the original network-only case. With no
// cache plumbed in (or no entry for this PR), loadDetail must still
// schedule the four background fetches.
func TestLoadDetailNoCacheStillReturnsNetworkCmds(t *testing.T) {
	m := newTestModel() // m.cache == nil
	m, cmd := m.loadDetail(ado.PRSummary{ID: 1})
	if cmd == nil {
		t.Fatalf("loadDetail without cache returned nil cmd")
	}
	if m.detailInflight != 4 {
		t.Fatalf("detailInflight: got %d want 4", m.detailInflight)
	}
}

// TestStatuslineShowsRefreshIndicator is the visible-feedback half of
// the cache work — when fetches are in flight the user must see a hint
// that the cached view is being verified. Without this the user can't
// tell stale-from-cache from authoritative-from-server.
func TestStatuslineShowsRefreshIndicator(t *testing.T) {
	m := newTestModel()
	m.screen = screenDetail
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: 99, Title: "x"})
	m.width = 200

	// Quiet state: no indicator.
	m.detailInflight = 0
	if got := renderStatusline(m); strings.Contains(got, "refreshing") {
		t.Fatalf("statusline showed refreshing chip with no fetches in flight: %q", got)
	}

	// In flight: the chip must appear.
	m.detailInflight = 2
	if got := renderStatusline(m); !strings.Contains(got, "refreshing") {
		t.Fatalf("statusline missing refreshing chip while in-flight: %q", got)
	}
}

// TestPersistDetailFieldWritesBackToCache verifies the save-back path:
// when a fresh (non-cached) *LoadedMsg arrives, the snapshot on disk
// gains the new payload while preserving the other fields. This is
// what makes the cache survive a process exit.
func TestPersistDetailFieldWritesBackToCache(t *testing.T) {
	st := newTestCache(t)
	m := newTestModel()
	m.cache = st
	prID := 7
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: prID, Title: "x"})

	// Simulate a fresh detailLoadedMsg arriving for the first time.
	m.persistDetailField(func(snap *cache.DetailSnapshot) {
		snap.Detail = &ado.PRDetail{PRSummary: ado.PRSummary{ID: prID, Title: "fresh"}}
	})

	loaded, ok := st.LoadDetail(prID)
	if !ok || loaded.Detail == nil || loaded.Detail.Title != "fresh" {
		t.Fatalf("first persist failed; loaded=%+v", loaded)
	}

	// Now a fresh threadsLoadedMsg arrives for the same PR.
	m.persistDetailField(func(snap *cache.DetailSnapshot) {
		snap.Threads = []ado.Thread{{ID: 1, Status: "active"}}
	})
	loaded, ok = st.LoadDetail(prID)
	if !ok || loaded.Detail == nil || loaded.Detail.Title != "fresh" {
		t.Fatalf("threads persist clobbered detail; loaded=%+v", loaded)
	}
	if len(loaded.Threads) != 1 {
		t.Fatalf("threads not persisted; loaded=%+v", loaded)
	}
}

// drainCachedMsgs runs cmd one-deep and collects all messages produced
// synchronously. tea.Batch returns a tea.BatchMsg containing child
// commands; we run each child once. Network commands block on real
// I/O — to keep the test hermetic we run them in goroutines with a
// short timeout and collect only those that return quickly. Cached
// commands return immediately so they always make the cut.
func drainCachedMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	first := cmd()
	batch, ok := first.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{first}
	}
	out := make([]tea.Msg, 0, len(batch))
	for _, child := range batch {
		ch := make(chan tea.Msg, 1)
		go func(c tea.Cmd) {
			defer func() { recover() }()
			ch <- c()
		}(child)
		select {
		case msg := <-ch:
			out = append(out, msg)
		case <-time.After(100 * time.Millisecond):
			// Network command — skip.
		}
	}
	return out
}

func newTestCache(t *testing.T) *cache.Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows
	st, err := cache.New()
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	return st
}

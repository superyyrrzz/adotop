package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/superyyrrzz/adotop/internal/ado"
)

func TestDetailRoundTrip(t *testing.T) {
	s := newTestStore(t)
	snap := DetailSnapshot{
		PRID: 42,
		Detail: &ado.PRDetail{
			PRSummary: ado.PRSummary{ID: 42, Title: "x"},
		},
		Files:    []ado.FileChange{{Path: "/a.go", ChangeType: "edit"}},
		Statuses: []ado.StatusCheck{{Context: "ci", State: "succeeded"}},
		Threads:  []ado.Thread{{ID: 1, Status: "active"}},
	}
	if err := s.SaveDetail(snap); err != nil {
		t.Fatalf("SaveDetail: %v", err)
	}
	got, ok := s.LoadDetail(42)
	if !ok {
		t.Fatalf("LoadDetail miss")
	}
	if got.PRID != 42 || got.Detail.Title != "x" || len(got.Files) != 1 || len(got.Statuses) != 1 || len(got.Threads) != 1 {
		t.Fatalf("LoadDetail returned wrong payload: %+v", got)
	}
}

func TestDetailMissReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	if _, ok := s.LoadDetail(99); ok {
		t.Fatalf("expected miss for unknown PR")
	}
}

func TestDetailEvictsLRUByMtime(t *testing.T) {
	s := newTestStore(t)
	// Drop the cap to keep the test small. Using a closure-captured
	// override would need a setter; instead, drive it via real cap.
	for i := 1; i <= detailCacheCap+5; i++ {
		snap := DetailSnapshot{PRID: i, Detail: &ado.PRDetail{PRSummary: ado.PRSummary{ID: i}}}
		if err := s.SaveDetail(snap); err != nil {
			t.Fatalf("SaveDetail %d: %v", i, err)
		}
		// Stagger mtimes so LRU has a deterministic order.
		path := s.detailPath(i)
		mt := time.Unix(int64(1_700_000_000+i), 0)
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatalf("chtimes %d: %v", i, err)
		}
	}
	// Trigger eviction with one final save (the loop already exceeded
	// cap, so subsequent saves will evict). Save id=999 with the newest
	// mtime so it survives.
	final := DetailSnapshot{PRID: 999, Detail: &ado.PRDetail{PRSummary: ado.PRSummary{ID: 999}}}
	if err := s.SaveDetail(final); err != nil {
		t.Fatalf("SaveDetail 999: %v", err)
	}
	mt := time.Unix(int64(1_900_000_000), 0)
	_ = os.Chtimes(s.detailPath(999), mt, mt)
	if err := s.evictDetailLRU(); err != nil {
		t.Fatalf("evictDetailLRU: %v", err)
	}
	// Count surviving pr-*.json files.
	entries, err := os.ReadDir(s.base)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var n int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" && len(e.Name()) > 3 && e.Name()[:3] == "pr-" {
			n++
		}
	}
	if n != detailCacheCap {
		t.Fatalf("after eviction want %d files, got %d", detailCacheCap, n)
	}
	// PR id=1 had the oldest mtime — must be gone.
	if _, ok := s.LoadDetail(1); ok {
		t.Fatalf("oldest PR (id=1) should have been evicted")
	}
	// PR id=999 had the newest mtime — must remain.
	if _, ok := s.LoadDetail(999); !ok {
		t.Fatalf("newest PR (id=999) should have survived")
	}
}

func TestDropDetailRemovesFile(t *testing.T) {
	s := newTestStore(t)
	snap := DetailSnapshot{PRID: 5, Detail: &ado.PRDetail{PRSummary: ado.PRSummary{ID: 5}}}
	if err := s.SaveDetail(snap); err != nil {
		t.Fatalf("SaveDetail: %v", err)
	}
	if err := s.DropDetail(5); err != nil {
		t.Fatalf("DropDetail: %v", err)
	}
	if _, ok := s.LoadDetail(5); ok {
		t.Fatalf("DropDetail did not remove the cached file")
	}
	// Idempotent — a second drop is fine.
	if err := s.DropDetail(5); err != nil {
		t.Fatalf("second DropDetail: %v", err)
	}
}

func TestLoadDetailTouchesMtime(t *testing.T) {
	s := newTestStore(t)
	snap := DetailSnapshot{PRID: 7, Detail: &ado.PRDetail{PRSummary: ado.PRSummary{ID: 7}}}
	if err := s.SaveDetail(snap); err != nil {
		t.Fatalf("SaveDetail: %v", err)
	}
	old := time.Unix(1_500_000_000, 0)
	if err := os.Chtimes(s.detailPath(7), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	// Override nowFn so the touch is observable and deterministic.
	want := time.Unix(1_700_000_000, 0)
	prev := nowFn
	nowFn = func() time.Time { return want }
	defer func() { nowFn = prev }()

	if _, ok := s.LoadDetail(7); !ok {
		t.Fatalf("LoadDetail miss")
	}
	info, err := os.Stat(s.detailPath(7))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.ModTime().Unix() != want.Unix() {
		t.Fatalf("LoadDetail did not touch mtime: got %s want %s", info.ModTime(), want)
	}
}

// guard: the eviction routine must skip files that don't match
// pr-<int>.json so we don't accidentally clobber list-* / identity / recents.
func TestEvictDetailIgnoresNonDetailFiles(t *testing.T) {
	s := newTestStore(t)
	// Plant a file the eviction must NOT touch.
	other := filepath.Join(s.base, "list-foo-bar-0.json")
	if err := os.WriteFile(other, []byte("{}"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for i := 1; i <= detailCacheCap+3; i++ {
		_ = s.SaveDetail(DetailSnapshot{PRID: i, Detail: &ado.PRDetail{PRSummary: ado.PRSummary{ID: i}}})
		mt := time.Unix(int64(1_700_000_000+i), 0)
		_ = os.Chtimes(s.detailPath(i), mt, mt)
	}
	_ = s.evictDetailLRU()
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("eviction removed an unrelated file: %v", err)
	}
}

// Compile-time assertion that the helper newTestStore exists; it lives
// in cache_test.go. We reference it here to make any rename loud.
var _ = func(t *testing.T) *Store { return newTestStore(t) }

// Sanity: schema mismatch is treated as a miss (mirrors list/recents).
func TestLoadDetailRejectsSchemaMismatch(t *testing.T) {
	s := newTestStore(t)
	// Hand-write a file with the wrong schema.
	bad := fmt.Sprintf(`{"schema":99,"pr_id":11}`)
	if err := os.WriteFile(s.detailPath(11), []byte(bad), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := s.LoadDetail(11); ok {
		t.Fatalf("schema mismatch should miss")
	}
}

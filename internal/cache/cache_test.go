package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	d := t.TempDir()
	t.Setenv("USERPROFILE", d) // Windows
	t.Setenv("HOME", d)        // *nix
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !filepath.IsAbs(s.base) || filepath.Dir(s.base) != filepath.Join(d, ".adotop") {
		t.Fatalf("base path %q not under %q", s.base, d)
	}
	return s
}

func TestIdentityRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if _, ok := s.LoadIdentity(); ok {
		t.Fatal("expected miss on empty cache")
	}
	if err := s.SaveIdentity("uid-1", "Alice Anderson"); err != nil {
		t.Fatal(err)
	}
	got, ok := s.LoadIdentity()
	if !ok || got.UserID != "uid-1" || got.DisplayName != "Alice Anderson" {
		t.Fatalf("LoadIdentity = %+v ok=%v", got, ok)
	}
}

func TestListRoundTripPerTab(t *testing.T) {
	s := newTestStore(t)
	prs := []ado.PRSummary{{ID: 1, Title: "x"}, {ID: 2, Title: "y"}}
	if err := s.SaveList("acme", "Platform", ado.TabAssigned, prs); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.LoadList("acme", "Platform", ado.TabCreated); ok {
		t.Fatal("Created tab should miss when only Assigned was written")
	}
	got, ok := s.LoadList("acme", "Platform", ado.TabAssigned)
	if !ok || len(got) != 2 || got[0].ID != 1 {
		t.Fatalf("LoadList = %+v ok=%v", got, ok)
	}
}

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
	for i := 1; i <= 25; i++ {
		_ = s.RecordVisit(ado.PRSummary{ID: i})
	}
	got, _ := s.LoadRecents()
	if len(got) != 15 {
		t.Fatalf("expected cap 15, got %d", len(got))
	}
	if got[0].ID != 25 || got[14].ID != 11 {
		t.Fatalf("oldest entries should be evicted: head=%d tail=%d", got[0].ID, got[14].ID)
	}
}

func TestSchemaMismatchTreatedAsMiss(t *testing.T) {
	s := newTestStore(t)
	// Hand-write a file with the wrong schema.
	bad := `{"schema": 999, "user_id": "x", "display_name": "y"}`
	if err := os.WriteFile(s.identityPath(), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.LoadIdentity(); ok {
		t.Fatal("expected schema mismatch to be treated as miss")
	}
}

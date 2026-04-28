package cache

import (
	"testing"

	"github.com/renzeyu/adotop/internal/ado"
)

// TestPatchRecentsRewritesMatchingEntry is the persistence half of the
// detail→list sync fix. After the in-memory list is patched, the
// recents file on disk must also reflect fresh votes — otherwise a
// restart sends the user back to the stale snapshot.
func TestPatchRecentsRewritesMatchingEntry(t *testing.T) {
	s := newTestStore(t)
	stale := ado.PRSummary{
		ID: 7, Title: "x",
		Reviewers: []ado.ReviewerVote{{ID: "u", DisplayName: "Alice", Vote: 0}},
	}
	other := ado.PRSummary{ID: 8, Title: "other"}
	if err := s.RecordVisit(stale); err != nil {
		t.Fatalf("RecordVisit stale: %v", err)
	}
	if err := s.RecordVisit(other); err != nil {
		t.Fatalf("RecordVisit other: %v", err)
	}

	fresh := ado.PRSummary{
		ID: 7, Title: "x",
		Reviewers: []ado.ReviewerVote{{ID: "u", DisplayName: "Alice", Vote: 10}},
	}
	if err := s.PatchRecents(fresh); err != nil {
		t.Fatalf("PatchRecents: %v", err)
	}

	loaded, ok := s.LoadRecents()
	if !ok {
		t.Fatalf("LoadRecents miss after patch")
	}
	if len(loaded) != 2 {
		t.Fatalf("PatchRecents should preserve all entries, got %d", len(loaded))
	}
	for _, p := range loaded {
		if p.ID == 7 && p.Reviewers[0].Vote != 10 {
			t.Fatalf("vote not patched: %+v", p)
		}
	}
}

// TestPatchRecentsPreservesOrder is the explicit guarantee that
// PatchRecents *patches* — it does NOT promote the entry to the front
// the way RecordVisit does. Promoting would corrupt the visit order
// each time a background fetch refreshes the data.
func TestPatchRecentsPreservesOrder(t *testing.T) {
	s := newTestStore(t)
	// Record A then B so B is at index 0, A at index 1.
	_ = s.RecordVisit(ado.PRSummary{ID: 1, Title: "A"})
	_ = s.RecordVisit(ado.PRSummary{ID: 2, Title: "B"})

	if err := s.PatchRecents(ado.PRSummary{ID: 1, Title: "A patched"}); err != nil {
		t.Fatalf("PatchRecents: %v", err)
	}
	got, _ := s.LoadRecents()
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Fatalf("PatchRecents reordered entries: %+v", got)
	}
	if got[1].Title != "A patched" {
		t.Fatalf("patch payload not applied: %+v", got[1])
	}
}

// TestPatchRecentsMissIsNoop — patching a PR that's not in recents
// (user hasn't visited it yet) must not write a new entry. RecordVisit
// is the way to add; PatchRecents is the way to update.
func TestPatchRecentsMissIsNoop(t *testing.T) {
	s := newTestStore(t)
	_ = s.RecordVisit(ado.PRSummary{ID: 1, Title: "A"})
	if err := s.PatchRecents(ado.PRSummary{ID: 999, Title: "never visited"}); err != nil {
		t.Fatalf("PatchRecents: %v", err)
	}
	got, _ := s.LoadRecents()
	if len(got) != 1 {
		t.Fatalf("PatchRecents miss should not add: %+v", got)
	}
}

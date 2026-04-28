package ui

import (
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestUpdatePRPatchesAllTabs is the regression guard for the "list view
// shows stale votes after detail view picked up a fresh approval" bug.
// A PR can appear in multiple tabs at once (Created + Reviewing if you
// requested review on your own PR, or Recents + any other tab). Patching
// must hit every copy.
func TestUpdatePRPatchesAllTabs(t *testing.T) {
	m := NewList(DefaultKeys())
	prID := 7
	stale := ado.PRSummary{
		ID: prID, Title: "x",
		Reviewers: []ado.ReviewerVote{{ID: "u1", DisplayName: "Alice", Vote: 0}},
	}
	other := ado.PRSummary{ID: 99, Title: "other"}
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: []ado.PRSummary{stale, other}})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabReviewRequested, prs: []ado.PRSummary{stale}})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabCreated, prs: []ado.PRSummary{other}})

	fresh := ado.PRSummary{
		ID: prID, Title: "x",
		Reviewers: []ado.ReviewerVote{{ID: "u1", DisplayName: "Alice", Vote: 10}},
	}
	m = m.UpdatePR(fresh)

	for _, tab := range []ado.Tab{ado.TabRecents, ado.TabReviewRequested} {
		rows := m.prs[tab]
		var found bool
		for _, p := range rows {
			if p.ID == prID {
				found = true
				if p.Reviewers[0].Vote != 10 {
					t.Fatalf("tab %s: vote not patched: got %d", tab, p.Reviewers[0].Vote)
				}
			}
		}
		if !found {
			t.Fatalf("tab %s: PR %d missing after UpdatePR", tab, prID)
		}
	}

	// Unrelated rows are untouched.
	if m.prs[ado.TabCreated][0].ID != 99 {
		t.Fatalf("UpdatePR clobbered an unrelated tab")
	}
}

// TestUpdatePRZeroIDIsNoOp guards the defensive shortcut so a zero-PR
// patch can't accidentally wipe matching rows (PR id=0 isn't valid in
// ADO, but defending against it costs nothing).
func TestUpdatePRZeroIDIsNoOp(t *testing.T) {
	m := NewList(DefaultKeys())
	stale := ado.PRSummary{ID: 5, Title: "keep"}
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: []ado.PRSummary{stale}})
	m = m.UpdatePR(ado.PRSummary{ID: 0, Title: "should be ignored"})
	if m.prs[ado.TabRecents][0].Title != "keep" {
		t.Fatalf("zero-ID UpdatePR mutated the row: %+v", m.prs[ado.TabRecents][0])
	}
}

package ui

import (
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// TestDetailLoadedSyncsListAndRecents is the end-to-end regression
// guard for the issue: opening a PR, seeing fresh data in the detail
// view, then going back to the list — the list row should reflect the
// new vote count without a manual refresh.
//
// We feed the app the cached-stale list, then a fresh detailLoadedMsg,
// and assert that the list-tab row was patched. This wires the full
// chain: detailLoadedMsg → app handler → ListModel.UpdatePR → row.
func TestDetailLoadedSyncsListAndRecents(t *testing.T) {
	prID := 42
	stale := ado.PRSummary{
		ID: prID, Title: "x", Status: "active",
		Reviewers: []ado.ReviewerVote{{ID: "u", DisplayName: "Alice", Vote: 0}},
	}

	m := newTestModel()
	m.cache = newTestCache(t)
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabReviewRequested, prs: []ado.PRSummary{stale}})
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabRecents, prs: []ado.PRSummary{stale}})
	m.detail = m.detail.SetSummary(stale)
	m.screen = screenDetail
	m.detailInflight = 4 // pretend loadDetail just fired

	fresh := ado.PRSummary{
		ID: prID, Title: "x", Status: "active",
		Reviewers: []ado.ReviewerVote{{ID: "u", DisplayName: "Alice", Vote: 10}},
	}
	freshDetail := &ado.PRDetail{PRSummary: fresh}

	mm, _ := m.Update(detailLoadedMsg{detail: freshDetail})
	m = mm.(Model)

	// The list-side rows must reflect Alice's approval without a refetch.
	for _, tab := range []ado.Tab{ado.TabReviewRequested, ado.TabRecents} {
		rows := m.list.prs[tab]
		if len(rows) != 1 {
			t.Fatalf("tab %s: row count changed: %+v", tab, rows)
		}
		if rows[0].Reviewers[0].Vote != 10 {
			t.Fatalf("tab %s: vote not patched after detail arrived: %+v", tab, rows[0])
		}
	}
}

// TestCachedDetailDoesNotPatchList is the inverse guarantee: a
// fromCache=true detail message must NOT clobber the list, because the
// cached payload is by definition stale (it's what the user saw last
// time they opened the PR; the live state may have advanced since).
func TestCachedDetailDoesNotPatchList(t *testing.T) {
	prID := 42
	live := ado.PRSummary{
		ID: prID, Title: "x", Status: "active",
		Reviewers: []ado.ReviewerVote{{ID: "u", DisplayName: "Alice", Vote: 10}},
	}
	m := newTestModel()
	m.cache = newTestCache(t)
	m.list, _ = m.list.Update(prsLoadedMsg{tab: ado.TabReviewRequested, prs: []ado.PRSummary{live}})
	m.detail = m.detail.SetSummary(live)
	m.screen = screenDetail

	stale := ado.PRSummary{
		ID: prID, Title: "x", Status: "active",
		Reviewers: []ado.ReviewerVote{{ID: "u", DisplayName: "Alice", Vote: 0}},
	}
	mm, _ := m.Update(detailLoadedMsg{detail: &ado.PRDetail{PRSummary: stale}, fromCache: true})
	m = mm.(Model)

	if got := m.list.prs[ado.TabReviewRequested][0].Reviewers[0].Vote; got != 10 {
		t.Fatalf("cached detail wrongly clobbered live list row: vote=%d", got)
	}
}

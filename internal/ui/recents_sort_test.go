package ui

import (
	"testing"

	"github.com/superyyrrzz/adotop/internal/ado"
)

func TestSortRecentsByStatus(t *testing.T) {
	in := []ado.PRSummary{
		{ID: 1, Status: "completed"},
		{ID: 2, Status: "active", Draft: true},
		{ID: 3, Status: "abandoned"},
		{ID: 4, Status: "active"},
		{ID: 5, Status: "active"}, // second open, should follow #4
		{ID: 6, Status: "completed"},
	}
	got := sortRecentsByStatus(in)
	want := []int{4, 5, 2, 1, 6, 3}
	for i, w := range want {
		if got[i].ID != w {
			t.Fatalf("position %d: want PR %d, got %d (full order: %v)", i, w, got[i].ID, ids(got))
		}
	}
}

func ids(prs []ado.PRSummary) []int {
	out := make([]int, len(prs))
	for i, p := range prs {
		out[i] = p.ID
	}
	return out
}

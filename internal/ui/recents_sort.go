package ui

import (
	"sort"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// sortRecentsByStatus reorders the recents list so open PRs surface first,
// then drafts, then merged, then abandoned/other. Stable so within each
// bucket the cache's existing visit-recency order is preserved.
func sortRecentsByStatus(prs []ado.PRSummary) []ado.PRSummary {
	if len(prs) < 2 {
		return prs
	}
	out := make([]ado.PRSummary, len(prs))
	copy(out, prs)
	sort.SliceStable(out, func(i, j int) bool {
		return statusRank(out[i]) < statusRank(out[j])
	})
	return out
}

func statusRank(p ado.PRSummary) int {
	switch p.Status {
	case "active":
		if p.Draft {
			return 1
		}
		return 0
	case "completed":
		return 2
	case "abandoned":
		return 3
	default:
		return 4
	}
}

//go:build live

package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/renzeyu/adotop/internal/ado"
)

// TestLivePR1145087HeaderVisible fetches a real PR and verifies that the
// always-visible header (repo line, title, reviewer panel) survives at
// every common pane geometry.
//
// Run with:
//
//	go test -tags=live -run TestLivePR1145087HeaderVisible ./internal/ui
func TestLivePR1145087HeaderVisible(t *testing.T) {
	tokens := ado.NewAzCLITokenProvider()
	c := ado.NewClient("ceapex", tokens)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	d, err := c.GetPullRequestByID(ctx, 1145087, "")
	if err != nil {
		t.Fatalf("GetPullRequestByID: %v", err)
	}
	t.Logf("PR #%d  repo=%q  title=%q  desc=%d chars (%d lines)",
		d.ID, d.Repo, d.Title, len(d.DescriptionMD),
		strings.Count(d.DescriptionMD, "\n")+1)

	files, err := c.GetIterationChanges(ctx, d.RepoID, 1145087)
	if err != nil {
		t.Logf("GetIterationChanges err (non-fatal): %v", err)
	}

	cases := []struct{ w, h int }{
		{40, 26}, {60, 30}, {80, 40}, {40, 50}, {30, 24}, {50, 35},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%dx%d", tc.w, tc.h), func(t *testing.T) {
			m := NewDetail(DefaultKeys())
			m = m.SetSummary(d.PRSummary)
			m = m.SetPaneSize(tc.w, tc.h)
			m, _ = m.Update(detailLoadedMsg{detail: d})
			m, _ = m.Update(filesLoadedMsg{files: files})

			out := m.ViewWithFocus(true)
			fmt.Printf("\n===== pane %dx%d =====\n%s\n", tc.w, tc.h, out)
			assertHeaderVisible(t, out, d.PRSummary, tc.w, tc.h)
		})
	}
}

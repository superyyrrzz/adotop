//go:build live

package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
)

// TestLivePRHeaderVisible fetches a real PR and verifies that the
// always-visible header (repo line, title, reviewer panel) survives
// at every common pane geometry.
//
// The PR is read from your local ~/.adotop/config.toml so each
// contributor can point the suite at something their account can
// access. Set:
//
//	org              = "your-org"
//	project          = "your-project"
//	pr_id_for_live_test = 12345  # any PR you can read
//
// Run with:
//
//	go test -tags=live -run TestLivePRHeaderVisible ./internal/ui
func TestLivePRHeaderVisible(t *testing.T) {
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Org == "" || cfg.PRIDForLiveTest == 0 {
		t.Skip("set org and pr_id_for_live_test in ~/.adotop/config.toml to run live tests")
	}

	tokens := ado.NewAzCLITokenProvider()
	c := ado.NewClient(cfg.Org, tokens)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prID := cfg.PRIDForLiveTest
	d, err := c.GetPullRequestByID(ctx, prID, "")
	if err != nil {
		t.Fatalf("GetPullRequestByID: %v", err)
	}
	t.Logf("PR #%d  repo=%q  title=%q  desc=%d chars (%d lines)",
		d.ID, d.Repo, d.Title, len(d.DescriptionMD),
		strings.Count(d.DescriptionMD, "\n")+1)

	files, err := c.GetIterationChanges(ctx, d.RepoID, prID)
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

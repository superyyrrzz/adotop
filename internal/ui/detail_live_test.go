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

			if d.Repo != "" && !strings.Contains(out, d.Repo) {
				t.Errorf("repo %q missing at %dx%d", d.Repo, tc.w, tc.h)
			}
			if !strings.Contains(out, fmt.Sprintf("PR #%d", d.ID)) {
				t.Errorf("PR title missing at %dx%d", tc.w, tc.h)
			}
			if !strings.Contains(out, "● Files") && !strings.Contains(out, "○ Files") {
				t.Errorf("Files sub-header missing at %dx%d", tc.w, tc.h)
			}
			if lines := strings.Count(out, "\n"); lines > tc.h {
				t.Errorf("rendered %d lines, exceeds pane height %d", lines, tc.h)
			}
		})
	}
}

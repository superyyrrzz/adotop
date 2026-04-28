package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
)

// TestAppViewKeepsTopbarVisibleForTallList is the regression guard for
// the bug where a long PR list (more rows than fit on screen) caused
// the topbar and tab strip to scroll off the top of the alt-screen.
//
// Root cause: ListModel.window underestimated per-row line count
// (counted 2 lines per row instead of 3 — data + meta + separator) and
// underestimated the chrome budget (4 instead of ~9 once topbar +
// statusline + tab strip + column header + pager are all summed).
// Result: View() emitted more lines than the terminal had, the alt
// screen scrolled, and the topbar disappeared above the viewport.
//
// The contract: for any reasonable terminal height with more PRs than
// fit, app.View() must produce a render where:
//  1. The first line is the topbar (contains a breadcrumb crumb), and
//  2. The total rendered height does not exceed terminal height.
func TestAppViewKeepsTopbarVisibleForTallList(t *testing.T) {
	for _, h := range []int{20, 30, 50, 80} {
		t.Run("h"+itoa(h), func(t *testing.T) {
			m := newTestModel()
			m.cfg = config.Config{Org: "ceapex", Project: "Engineering"}
			m.user = "renzeyu"
			mm, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: h})
			m = mm.(Model)
			mm, _ = m.Update(prsLoadedMsg{tab: ado.TabRecents, prs: manyPRs(50)})
			m = mm.(Model)

			out := m.View()
			lines := strings.Split(out, "\n")
			if len(lines) == 0 {
				t.Fatalf("empty View")
			}
			if !strings.Contains(lines[0], "ceapex") {
				t.Fatalf("h=%d: topbar must be the first line; got first line=%q\nfull:\n%s", h, lines[0], out)
			}
			if got := lipgloss.Height(out); got > h {
				t.Fatalf("h=%d: View height %d exceeds terminal height — alt screen will scroll topbar off", h, got)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

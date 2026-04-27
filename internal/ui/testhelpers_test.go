package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ado"
)

// renderDetailInLayout renders a DetailModel through the same path
// that production uses in Model.detailPreviewView: it computes the
// detail layout for a (termW, termH) terminal and threads the resulting
// pane size into the model before calling ViewWithFocus.
//
// Tests that assert on rendered output MUST use this helper instead of
// m.View() directly — otherwise the model thinks it has the full
// terminal width and wraps differently than what the user sees.
func renderDetailInLayout(t *testing.T, m DetailModel, termW, termH int) string {
	t.Helper()
	parent := Model{width: termW, height: termH}
	layout := parent.detailLayout()
	w, h := layout.leftWidth, layout.bodyHeight
	if w <= 0 {
		w = termW
	}
	if h <= 0 {
		h = termH
	}
	m = m.SetPaneSize(w, h)
	return m.ViewWithFocus(true)
}

// paneSizeCase is one entry in the geometry matrix. termW/termH are the
// outer terminal dimensions; the helper computes the actual left-pane
// size from detailLayout so tests exercise the same math production
// uses.
type paneSizeCase struct {
	termW, termH int
}

// paneSizeMatrix is the canonical set of geometries every TUI test
// should run against. Hits both narrow split (40-col left pane) and
// wide layouts, plus the small-terminal stacked fallback (<100 cols).
var paneSizeMatrix = []paneSizeCase{
	{80, 24},   // small split
	{100, 30},  // just-barely-split
	{120, 40},  // typical
	{160, 50},  // wide
	{80, 50},   // tall narrow
	{90, 30},   // stacked (no split: width<100)
}

// forEachPaneSize runs fn against every entry in paneSizeMatrix as a
// subtest named WxH. fn receives the resolved pane width/height (post
// detailLayout), not the raw terminal size.
func forEachPaneSize(t *testing.T, fn func(t *testing.T, paneW, paneH int)) {
	t.Helper()
	for _, c := range paneSizeMatrix {
		c := c
		parent := Model{width: c.termW, height: c.termH}
		layout := parent.detailLayout()
		t.Run(fmt.Sprintf("term%dx%d_pane%dx%d", c.termW, c.termH, layout.leftWidth, layout.bodyHeight), func(t *testing.T) {
			fn(t, layout.leftWidth, layout.bodyHeight)
		})
	}
}

// assertHeaderVisible enforces the "always-visible PR chrome" invariant
// against a rendered detail view: repo line, "PR #N" title, and the
// Files sub-header must all be present, and the rendered VISUAL height
// (after the parent pane wraps long lines at paneW) must not exceed
// paneH. paneW=0 skips the wrap simulation and falls back to source-line
// counting (use only when wrap behavior isn't relevant).
//
// New chrome invariants belong here, not scattered across tests.
func assertHeaderVisible(t *testing.T, out string, s ado.PRSummary, paneW, paneH int) {
	t.Helper()
	if s.Repo != "" && !strings.Contains(out, s.Repo) {
		t.Errorf("repo %q missing from view:\n%s", s.Repo, out)
	}
	if !strings.Contains(out, fmt.Sprintf("PR #%d", s.ID)) {
		t.Errorf("PR title (PR #%d) missing from view:\n%s", s.ID, out)
	}
	if !strings.Contains(out, "● Files") && !strings.Contains(out, "○ Files") {
		t.Errorf("Files sub-header missing from view:\n%s", out)
	}
	measured := out
	if paneW > 0 {
		// Simulate what lipgloss.JoinHorizontal does in production:
		// render the pane string at the pane width so long lines wrap,
		// THEN count visual rows. Without this we count source \n only
		// and miss the bug class where a 277-char description line
		// reports as 1 row but renders as 7.
		measured = lipgloss.NewStyle().Width(paneW).Render(out)
	}
	if h := lipgloss.Height(measured); h > paneH {
		t.Errorf("rendered %d visual rows at paneW=%d, exceeds pane height %d:\n%s", h, paneW, paneH, out)
	}
}

package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/config"
)

// TestTopbarShowsCrumbsForListScreen is the contract test for the
// breadcrumb on the PR list. The deepest crumb is the active tab name
// — that's the dynamic "where am I" signal — and the org/project sit
// to the left as static context.
func TestTopbarShowsCrumbsForListScreen(t *testing.T) {
	m := newTestModel()
	m.cfg = config.Config{Org: "ceapex", Project: "Engineering"}
	m.width = 120

	out := renderTopbar(m)

	for _, want := range []string{"ceapex", "Engineering", "Recents"} {
		if !strings.Contains(out, want) {
			t.Fatalf("topbar missing crumb %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "›") {
		t.Fatalf("topbar should join crumbs with chevrons:\n%s", out)
	}
}

// TestTopbarShowsPRIDOnDetailScreen is the same contract for the detail
// screen: the deepest crumb becomes the PR ID. We deliberately do NOT
// include the tab name in the trail to keep the bar short.
func TestTopbarShowsPRIDOnDetailScreen(t *testing.T) {
	m := newDetailModel(t)
	m.cfg = config.Config{Org: "ceapex", Project: "Engineering"}
	m.width = 120

	out := renderTopbar(m)

	want := "#" + strconv.Itoa(m.detail.Summary().ID)
	if !strings.Contains(out, want) {
		t.Fatalf("detail topbar should end with PR ID %s:\n%s", want, out)
	}
}

// TestTopbarFitsWithinWidth is the regression guard for the same
// "header eats the screen" class of bug we just fixed in the tab
// strip. Each rendered line must be <= terminal width at every size.
func TestTopbarFitsWithinWidth(t *testing.T) {
	m := newTestModel()
	m.cfg = config.Config{Org: "ceapex", Project: "Engineering"}

	for _, w := range []int{20, 30, 40, 60, 80, 120, 200} {
		m.width = w
		out := renderTopbar(m)
		for _, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Fatalf("width=%d: topbar line %d cells exceeds terminal\nline=%q", w, got, line)
			}
		}
	}
}

// TestTopbarCrumbsTruncateLeadingFirst guards the truncation policy:
// when the budget is tight we sacrifice org/project to keep the active
// view visible. Losing the active crumb would defeat the whole point.
func TestTopbarCrumbsTruncateLeadingFirst(t *testing.T) {
	m := newTestModel()
	m.cfg = config.Config{Org: "supercalifragilistic", Project: "expialidocious-engineering"}
	m.width = 30

	out := renderTopbar(m)
	if !strings.Contains(out, "Recents") {
		t.Fatalf("active crumb 'Recents' must survive truncation:\n%s", out)
	}
}

// TestTopbarRightZoneCarriesIdentity verifies the right side carries
// the user. Clock is best-effort (env-suppressed in CI sometimes), so
// only identity is asserted.
func TestTopbarRightZoneCarriesIdentity(t *testing.T) {
	m := newTestModel()
	m.user = "renzeyu"
	m.width = 120

	out := renderTopbar(m)
	if !strings.Contains(out, "renzeyu") {
		t.Fatalf("topbar should show identity on the right:\n%s", out)
	}
}

// TestTopbarHandlesMissingOrgProject confirms the placeholders make it
// through so a misconfigured user sees what's wrong rather than a
// phantom-empty header.
func TestTopbarHandlesMissingOrgProject(t *testing.T) {
	m := newTestModel()
	m.cfg = config.Config{}
	m.width = 80

	out := renderTopbar(m)
	if !strings.Contains(out, "(no org)") || !strings.Contains(out, "(no project)") {
		t.Fatalf("missing-config placeholders should show:\n%s", out)
	}
}

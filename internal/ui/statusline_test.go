package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
)

// TestStatuslineModeReflectsState: each modal flag on the model should
// pick the matching mode label so the user can see what input we're
// waiting for.
func TestStatuslineModeReflectsState(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Model)
		want string
	}{
		{"normal", func(m *Model) {}, "NORMAL"},
		{"voteMenu", func(m *Model) { m.voteMenu = true }, "MENU"},
		{"pendingAction", func(m *Model) { m.pendingAction = pendingAction{kind: "abandon", prompt: "?"} }, "CONFIRM"},
		{"footerErr", func(m *Model) { m.footerErr = "boom" }, "ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newDetailModel(t)
			m.width = 200 // wide so nothing overflows
			c.set(&m)
			out := renderStatusline(m)
			if !strings.Contains(out, c.want) {
				t.Fatalf("mode=%s: want substring %q in %q", c.name, c.want, out)
			}
		})
	}
}

// TestStatuslineDropsHintsOnNarrowWidth: when terminal width is too
// small to fit all hints, hints should be dropped from the tail. The
// mode + context segments must always remain.
func TestStatuslineDropsHintsOnNarrowWidth(t *testing.T) {
	m := newDetailModel(t)
	m.width = 200
	wide := renderStatusline(m)
	m.width = 50
	narrow := renderStatusline(m)
	if !strings.Contains(narrow, "NORMAL") {
		t.Fatalf("narrow statusline lost mode segment: %q", narrow)
	}
	if !strings.Contains(narrow, "PR #") {
		t.Fatalf("narrow statusline lost context segment: %q", narrow)
	}
	if len(narrow) >= len(wide) {
		t.Fatalf("expected narrow statusline shorter than wide; narrow=%d wide=%d", len(narrow), len(wide))
	}
}

// Comment-action hints are diff-focus-only. In files focus the bar
// shouldn't waste width advertising c/C/x/[/] — those keys are no-ops
// there. In diff focus they should appear. Hints render two-tone
// today ("c new" not "c:new"), so the assertion uses the rendered form.
func TestStatuslineCommentHintsOnlyInDiffFocus(t *testing.T) {
	m := newDetailModel(t)
	m.width = 300 // wide enough that no hints get dropped
	// The thread cluster is gated on "threads exist on the PR" too —
	// seed one so the focus-gate is what actually drives the assertion.
	m.threads = []ado.Thread{
		{ID: 1, FilePath: "/a.go", Status: "active",
			Comments: []ado.Comment{{Author: "A", Content: "x"}}},
	}

	// Files focus: comment hints absent (focus gate).
	filesBar := renderStatusline(m)
	for _, h := range []string{"c new", "C reply", "x resolve", "[/] thread"} {
		if strings.Contains(filesBar, h) {
			t.Fatalf("files focus should not surface %q in hints, got: %s", h, filesBar)
		}
	}

	// Diff focus: comment hints present.
	m.detailFocus = focusDiff
	diffBar := renderStatusline(m)
	for _, h := range []string{"c new", "C reply", "x resolve", "[/] thread"} {
		if !strings.Contains(diffBar, h) {
			t.Fatalf("diff focus should surface %q in hints, got: %s", h, diffBar)
		}
	}
}

// TestStatuslineHidesThreadHintsWhenNoThreads: the thread cluster is
// gated on "PR has threads" too — a brand-new PR with zero comments
// should not advertise [/]/c/C/x/space because pressing any of them
// would silently no-op. Hiding cuts the wall-of-keys problem for new
// users.
func TestStatuslineHidesThreadHintsWhenNoThreads(t *testing.T) {
	m := newDetailModel(t)
	m.width = 300
	m.detailFocus = focusDiff
	// no threads seeded
	out := renderStatusline(m)
	for _, h := range []string{"[/] thread", "c new", "C reply", "x resolve", "space expand"} {
		if strings.Contains(out, h) {
			t.Fatalf("expected %q to be hidden when PR has no threads, got: %s", h, out)
		}
	}
	// Sanity: J:jump and R:show-resolved are also gated on threads
	// existing. They should be absent too.
	for _, h := range []string{"J jump", "R show-resolved"} {
		if strings.Contains(out, h) {
			t.Fatalf("expected %q to be hidden when PR has no threads, got: %s", h, out)
		}
	}
}

// TestStatuslineHidesViewTogglesInFilesFocus: w (wrap) and +/- (ctx)
// only act on the diff viewport. In Files focus they're no-ops, so
// hiding them keeps the statusline honest about what's actually
// reachable from the current state.
func TestStatuslineHidesViewTogglesInFilesFocus(t *testing.T) {
	m := newDetailModel(t)
	m.width = 300
	// default focus = files
	out := renderStatusline(m)
	for _, h := range []string{"w wrap", "+/- context"} {
		if strings.Contains(out, h) {
			t.Fatalf("expected %q hidden in Files focus, got: %s", h, out)
		}
	}
}

// TestStatuslineSurvivalKeysAlwaysVisible: the keys a confused user
// most needs to find — help and back/quit — must survive every
// gating decision. Catches a regression where a future cleanup
// accidentally cuts ?:help out of one branch.
func TestStatuslineSurvivalKeysAlwaysVisible(t *testing.T) {
	m := newDetailModel(t)
	m.width = 300
	for _, focus := range []detailFocus{focusFiles, focusDiff} {
		m.detailFocus = focus
		out := renderStatusline(m)
		if !strings.Contains(out, "? help") {
			t.Fatalf("?:help missing in focus=%v: %s", focus, out)
		}
		if !strings.Contains(out, "esc back") {
			t.Fatalf("esc:back missing in focus=%v: %s", focus, out)
		}
	}
}

// TestStatuslineMenuModeReplacesContext: when the vote menu is open,
// the statusline shows the menu prompt rather than the regular
// context, so the user always sees the active key options.
func TestStatuslineMenuModeReplacesContext(t *testing.T) {
	m := newDetailModel(t)
	m.width = 200
	m.voteMenu = true
	out := renderStatusline(m)
	for _, want := range []string{"a:approve", "esc:cancel"} {
		if !strings.Contains(out, want) {
			t.Fatalf("vote menu statusline missing %q: %q", want, out)
		}
	}
}

// TestStatuslineUsesShortTabLabel guards the regression where the
// list-screen context segment showed "All reviewing" — long enough to
// crowd hints off the right side at typical widths. The statusline,
// like the topbar breadcrumb, must use Tab.Short().
func TestStatuslineUsesShortTabLabel(t *testing.T) {
	m := newTestModel()
	m.cfg = config.Config{Org: "ceapex", Project: "Engineering"}
	m.width = 200

	for i := 0; i < 3; i++ {
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = mm.(Model)
	}

	out := renderStatusline(m)
	if strings.Contains(out, "All reviewing") {
		t.Fatalf("statusline should NOT use long tab label:\n%s", out)
	}
	if !strings.Contains(out, "Reviewing") {
		t.Fatalf("statusline should use short tab label:\n%s", out)
	}
}

// TestStatuslineSurfacesErrorMessage is the regression guard for the
// "see only ERROR with no clue what failed" complaint. When
// m.footerErr is set, the rendered statusline must contain the actual
// message (not just the ERROR mode label) AND the dismiss cue, so the
// user has the context they need to react.
func TestStatuslineSurfacesErrorMessage(t *testing.T) {
	m := newDetailModel(t)
	m.width = 200
	m.footerErr = "abandon PR #1145756: pull request is completed"

	out := renderStatusline(m)
	if !strings.Contains(out, "ERROR") {
		t.Fatalf("statusline should show ERROR mode pill:\n%s", out)
	}
	if !strings.Contains(out, "pull request is completed") {
		t.Fatalf("statusline should surface the error message text:\n%s", out)
	}
	if !strings.Contains(out, "press any key to dismiss") {
		t.Fatalf("statusline should show dismiss cue in error mode:\n%s", out)
	}
	// And the routine binding hints should NOT appear — they would
	// compete with the error message for attention.
	if strings.Contains(out, "tab:focus") {
		t.Fatalf("error mode should drop routine hints, found tab:focus:\n%s", out)
	}
}

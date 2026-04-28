package ui

import (
	"strings"
	"testing"
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

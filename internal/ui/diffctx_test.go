package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestCtxLadderCycles is the contract test for the +/- step function.
// The ladder is small but the wrap behavior is the part that's easy to
// get subtly wrong (off-by-one at either end).
func TestCtxLadderCycles(t *testing.T) {
	want := []int{10, 25, -1, 0, 10}
	got := []int{}
	cur := 0
	for range want {
		cur = nextCtx(cur)
		got = append(got, cur)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("nextCtx step %d: got %d want %d (full path %v)", i, got[i], w, got)
		}
	}

	// Walk back the same way and make sure we land on each rung in
	// reverse — proves prevCtx is the true inverse, not a shifted copy.
	cur = 10
	rev := []int{0, -1, 25, 10, 0}
	for i, w := range rev {
		cur = prevCtx(cur)
		if cur != w {
			t.Fatalf("prevCtx step %d: got %d want %d", i, cur, w)
		}
	}
}

// TestSimpleDiffRespectsCtxLines guards the ctx-line plumbing into the
// REST-fallback diff. With a 30-line file and one change in the middle,
// ctx=3 should leave most of the file folded; ctx=20 should show the
// whole thing as a single hunk.
func TestSimpleDiffRespectsCtxLines(t *testing.T) {
	var src, tgt strings.Builder
	for i := 0; i < 30; i++ {
		if i == 15 {
			tgt.WriteString("OLD\n")
			src.WriteString("NEW\n")
			continue
		}
		line := "line\n"
		tgt.WriteString(line)
		src.WriteString(line)
	}

	small := string(simpleDiff([]byte(tgt.String()), []byte(src.String()), "/x.txt", 3))
	big := string(simpleDiff([]byte(tgt.String()), []byte(src.String()), "/x.txt", 50))

	smallCtxLines := countCtxLines(small)
	bigCtxLines := countCtxLines(big)
	if bigCtxLines <= smallCtxLines {
		t.Fatalf("expanded ctx should produce more context lines: small=%d big=%d\n--- small ---\n%s\n--- big ---\n%s",
			smallCtxLines, bigCtxLines, small, big)
	}
	if !strings.Contains(big, "line\nline\nline\nline\nline\nline\nline\nline\nline\nline\n line\n") &&
		!strings.Contains(big, " line\n line\n line\n line\n line\n line\n line\n line\n line\n line\n") {
		// loose check: many leading-space context lines should appear
		if bigCtxLines < 20 {
			t.Fatalf("ctx=50 should show ~all surrounding lines, got %d:\n%s", bigCtxLines, big)
		}
	}
}

func countCtxLines(diff string) int {
	n := 0
	for _, ln := range strings.Split(diff, "\n") {
		if strings.HasPrefix(ln, " ") {
			n++
		}
	}
	return n
}

// TestDiffSelectionKeyIncludesCtx is the cache-isolation guarantee: two
// requests for the same file at different ctx levels must not collide,
// otherwise toggling +/- would serve the previous level's bytes.
func TestDiffSelectionKeyIncludesCtx(t *testing.T) {
	a := diffSelectionKey("src", "tgt", "/x.go", 0)
	b := diffSelectionKey("src", "tgt", "/x.go", 10)
	c := diffSelectionKey("src", "tgt", "/x.go", -1)
	if a == b || a == c || b == c {
		t.Fatalf("ctx levels must produce distinct keys: a=%q b=%q c=%q", a, b, c)
	}
}

// TestCtxLabel covers the user-visible rendering. The "default" rung is
// labeled "ctx:3" rather than "ctx:0" because the on-disk meaning of 0
// is "use the git default" — we don't want to leak the sentinel.
func TestCtxLabel(t *testing.T) {
	cases := map[int]string{
		0:  "ctx:3",
		10: "ctx:10",
		25: "ctx:25",
		-1: "ctx:all",
	}
	for in, want := range cases {
		if got := ctxLabel(in); got != want {
			t.Fatalf("ctxLabel(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestStatuslineShowsCtxSegment proves the statusline composition picks
// up the ctx label when the user is on the detail screen. Without this
// the user has no way to know which ladder rung they're on.
func TestStatuslineShowsCtxSegment(t *testing.T) {
	m := newDetailModel(t)
	m.width = 200
	out := renderStatusline(m)
	if !strings.Contains(out, "ctx:3") {
		t.Fatalf("statusline missing default ctx segment:\n%s", out)
	}
	m.diffCtx = -1
	out = renderStatusline(m)
	if !strings.Contains(out, "ctx:all") {
		t.Fatalf("statusline missing ctx:all after toggle:\n%s", out)
	}
}

// TestCtxKeyAdvancesAndWraps presses + repeatedly and confirms the
// model's diffCtx walks the ladder. Pressing - returns to the start.
// This is the integration-level guarantee that the binding is wired.
func TestCtxKeyAdvancesAndWraps(t *testing.T) {
	m := newDetailModel(t)
	steps := []int{10, 25, -1, 0}
	for i, want := range steps {
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
		m = mm.(Model)
		if m.diffCtx != want {
			t.Fatalf("after %d presses of +, diffCtx=%d want %d", i+1, m.diffCtx, want)
		}
	}

	// Now -, which should walk backward from 0 → -1.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	m = mm.(Model)
	if m.diffCtx != -1 {
		t.Fatalf("after - from 0, diffCtx=%d want -1", m.diffCtx)
	}
}

package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/renzeyu/adotop/internal/ado"
)

func samplePRs() []ado.PRSummary {
	t := time.Now().Add(-2 * time.Hour)
	return []ado.PRSummary{
		{ID: 1234, Title: "Fix login bug", Repo: "MyRepo", SourceBranch: "feat/login", TargetBranch: "main", CreatedAt: t, Author: "alice", Reviewers: []ado.ReviewerVote{{DisplayName: "bob", Vote: 10, IsRequired: true}}},
		{ID: 1235, Title: "Add dark mode", Repo: "MyRepo", SourceBranch: "feat/theme", TargetBranch: "main", CreatedAt: t, Author: "bob", Reviewers: []ado.ReviewerVote{{DisplayName: "carol", Vote: 0}}},
	}
}

func TestListRendersPRs(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: samplePRs()})
	out := m.View()
	if !strings.Contains(out, "#1234") || !strings.Contains(out, "Fix login bug") {
		t.Fatalf("missing PR row:\n%s", out)
	}
	if !strings.Contains(out, "Assigned") {
		t.Fatalf("missing tab label:\n%s", out)
	}
}

func TestListFilterNarrowsRows(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: samplePRs()})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "dark" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	out := m.View()
	if strings.Contains(out, "#1234") {
		t.Fatalf("filter should hide #1234:\n%s", out)
	}
	if !strings.Contains(out, "#1235") {
		t.Fatalf("filter should keep #1235:\n%s", out)
	}
}

func TestListColumnsAlignWithEllipsizedTitle(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: []ado.PRSummary{
		{ID: 1, Title: strings.Repeat("a", 60), Author: "alice", SourceBranch: "f", TargetBranch: "main"},
		{ID: 2, Title: "short", Author: "bob", SourceBranch: "f", TargetBranch: "main"},
	}})
	out := m.View()
	// Find the visual column (rune count) of the author marker on each row.
	runeIndex := func(line, sub string) int {
		i := strings.Index(line, sub)
		if i < 0 {
			return -1
		}
		return len([]rune(line[:i]))
	}
	var col1, col2 int = -1, -1
	for _, line := range strings.Split(out, "\n") {
		if c := runeIndex(line, "alice"); c >= 0 {
			col1 = c
		}
		if c := runeIndex(line, "bob"); c >= 0 {
			col2 = c
		}
	}
	if col1 < 0 || col2 < 0 {
		t.Fatalf("could not locate author columns:\n%s", out)
	}
	if col1 != col2 {
		t.Fatalf("author columns not aligned: alice@col%d bob@col%d\n%s", col1, col2, out)
	}
}

func TestListAgeColumnAlignsAcrossVaryingBranchLengths(t *testing.T) {
	now := time.Now()
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: []ado.PRSummary{
		{ID: 1, Title: "t1", Author: "a", SourceBranch: "f", TargetBranch: "main", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: 2, Title: "t2", Author: "b", SourceBranch: "users/someone/very-long-feature-branch-name", TargetBranch: "release/2026.05", CreatedAt: now.Add(-3 * time.Hour)},
	}})
	out := m.View()
	runeIndex := func(line, sub string) int {
		i := strings.Index(line, sub)
		if i < 0 {
			return -1
		}
		return len([]rune(line[:i]))
	}
	var col1, col2 int = -1, -1
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "#1 ") || strings.Contains(line, "#1\t") || strings.HasPrefix(strings.TrimLeft(line, " "), "#1 ") {
			if c := runeIndex(line, "2h"); c >= 0 {
				col1 = c
			}
		}
		if strings.Contains(line, "#2 ") || strings.Contains(line, "#2\t") || strings.HasPrefix(strings.TrimLeft(line, " "), "#2 ") {
			if c := runeIndex(line, "3h"); c >= 0 {
				col2 = c
			}
		}
	}
	if col1 < 0 || col2 < 0 {
		t.Fatalf("could not locate age columns: col1=%d col2=%d\n%s", col1, col2, out)
	}
	if col1 != col2 {
		t.Fatalf("age column not aligned across varying branch lengths: 2h@col%d 3h@col%d\n%s", col1, col2, out)
	}
}

func TestListAllColumnsAlignAcrossVaryingInputs(t *testing.T) {
	now := time.Now()
	prs := []ado.PRSummary{
		{ID: 7, Title: "tiny", Author: "a", SourceBranch: "f", TargetBranch: "main", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: 1234567, Title: strings.Repeat("x", 60), Author: "Some Long Author Name", SourceBranch: "users/x/very-long-feature-branch", TargetBranch: "release/2026.05", CreatedAt: now.Add(-3 * 24 * time.Hour)},
		{ID: 999, Title: "中文标题混合 ascii", Author: "李雷", SourceBranch: "feat/中文", TargetBranch: "main", CreatedAt: now.Add(-30 * time.Second)},
	}
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 250, Height: 40})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: prs})

	out := m.View()
	rows := []string{}
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trim, "#") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 PR rows, got %d:\n%s", len(rows), out)
	}

	runeCol := func(line, sub string) int {
		i := strings.Index(line, sub)
		if i < 0 {
			return -1
		}
		return runewidth.StringWidth(line[:i])
	}
	type colCheck struct {
		name    string
		needles []string // one per row
	}
	checks := []colCheck{
		{"title", []string{"tiny", "xxxx", "中文标题"}},
		{"author", []string{"a   ", "Some Long", "李雷"}},
		{"source", []string{"f   ", "users/x", "feat/中文"}},
		{"target", []string{"main ", "release", "main "}},
		{"age", []string{"2h", "3d", "now"}},
	}
	for _, ch := range checks {
		var first int = -1
		for r, needle := range ch.needles {
			c := runeCol(rows[r], needle)
			if c < 0 {
				t.Fatalf("col %s: needle %q not found in row %d:\n%s", ch.name, needle, r, rows[r])
			}
			if first < 0 {
				first = c
			} else if c != first {
				t.Fatalf("col %s misaligned: row 0 col=%d, row %d col=%d (needle %q)\nrows:\n%s\n%s\n%s",
					ch.name, first, r, c, needle, rows[0], rows[1], rows[2])
			}
		}
	}
}

func TestListDraftBadgeAlignsAcrossAges(t *testing.T) {
	now := time.Now()
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 250, Height: 40})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: []ado.PRSummary{
		{ID: 1, Title: "fresh", Author: "a", SourceBranch: "f", TargetBranch: "main", CreatedAt: now.Add(-30 * time.Second), Draft: true},
		{ID: 2, Title: "old", Author: "b", SourceBranch: "f", TargetBranch: "main", CreatedAt: now.Add(-9999 * 24 * time.Hour), Draft: true},
	}})
	out := m.View()
	col := func(line, sub string) int {
		i := strings.Index(line, sub)
		if i < 0 {
			return -1
		}
		return runewidth.StringWidth(line[:i])
	}
	var c1, c2 int = -1, -1
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "fresh") {
			c1 = col(line, "[DRAFT]")
		}
		if strings.Contains(line, "old") {
			c2 = col(line, "[DRAFT]")
		}
	}
	if c1 < 0 || c2 < 0 {
		t.Fatalf("could not locate DRAFT badge: c1=%d c2=%d\n%s", c1, c2, out)
	}
	if c1 != c2 {
		t.Fatalf("[DRAFT] not aligned across age widths: fresh@col%d old@col%d\n%s", c1, c2, out)
	}
}

func TestListShowsColumnHeadings(t *testing.T) {
	m := NewList(DefaultKeys())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: samplePRs()})
	out := m.View()
	for _, h := range []string{"ID", "Title", "Author", "Source", "Target", "Age"} {
		if !strings.Contains(out, h) {
			t.Fatalf("missing header %q in:\n%s", h, out)
		}
	}
	// Header should appear above the PR rows.
	hi := strings.Index(out, "Title")
	ri := strings.Index(out, "#1234")
	if hi < 0 || ri < 0 || hi > ri {
		t.Fatalf("header should precede rows: hi=%d ri=%d\n%s", hi, ri, out)
	}
}

func TestListNextTabEmitsLoad(t *testing.T) {
	m := NewList(DefaultKeys())
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected a load cmd after tab switch")
	}
	if m.tab != ado.TabCreated {
		t.Fatalf("tab = %v, want Created", m.tab)
	}
}

func manyPRs(n int) []ado.PRSummary {
	out := make([]ado.PRSummary, n)
	for i := range out {
		out[i] = ado.PRSummary{ID: 1000 + i, Title: fmt.Sprintf("PR-%d", i), Author: "a", SourceBranch: "f", TargetBranch: "main"}
	}
	return out
}

func TestListWindowsLongListAroundCursor(t *testing.T) {
	m := NewList(DefaultKeys())
	// Tabs(1) + blank(1) + 2 lines per row. Height 14 ⇒ ~6 rows fit.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 14})
	m, _ = m.Update(prsLoadedMsg{tab: ado.TabAssigned, prs: manyPRs(40)})

	out := m.View()
	if !strings.Contains(out, "#1000") {
		t.Fatalf("first row should be visible at top:\n%s", out)
	}
	if strings.Contains(out, "#1039") {
		t.Fatalf("last row must NOT be visible when cursor is at top:\n%s", out)
	}

	// Move cursor far down; the window must follow.
	for i := 0; i < 30; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	out = m.View()
	if !strings.Contains(out, "#1030") {
		t.Fatalf("cursor row #1030 should be visible after scrolling:\n%s", out)
	}
	if strings.Contains(out, "#1000") {
		t.Fatalf("top row should have scrolled out of view:\n%s", out)
	}

	// Scroll back up; top row should reappear.
	for i := 0; i < 30; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	out = m.View()
	if !strings.Contains(out, "#1000") {
		t.Fatalf("top row should be back in view after scrolling up:\n%s", out)
	}
}

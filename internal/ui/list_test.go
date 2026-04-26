package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

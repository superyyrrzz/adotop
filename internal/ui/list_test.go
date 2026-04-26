package ui

import (
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

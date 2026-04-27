package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/renzeyu/adotop/internal/ado"
)

func TestDetailRendersDescriptionAndFiles(t *testing.T) {
	m := NewDetail(DefaultKeys())
	m = m.SetSummary(ado.PRSummary{ID: 1234, Title: "Fix login bug", Author: "alice", SourceBranch: "feat/login", TargetBranch: "main"})
	m, _ = m.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary:     ado.PRSummary{ID: 1234, Title: "Fix login bug"},
		DescriptionMD: "Fixes the issue where session tokens were not refreshed.",
	}})
	m, _ = m.Update(filesLoadedMsg{files: []ado.FileChange{{Path: "/src/login.go", ChangeType: "edit"}}})
	out := m.View()
	if !strings.Contains(out, "Fix login bug") || !strings.Contains(out, "session tokens") {
		t.Fatalf("missing description:\n%s", out)
	}
	if !strings.Contains(out, "src/") || !strings.Contains(out, "login.go") {
		t.Fatalf("missing file:\n%s", out)
	}
}

func TestDetailStatusesRendered(t *testing.T) {
	m := NewDetail(DefaultKeys())
	m = m.SetSummary(ado.PRSummary{ID: 1, Title: "x"})
	m, _ = m.Update(statusesLoadedMsg{statuses: []ado.StatusCheck{{Context: "build/ci", State: "succeeded"}, {Context: "policy", State: "pending"}}})
	out := m.View()
	if !strings.Contains(out, "build/ci") || !strings.Contains(out, "policy") {
		t.Fatalf("missing status contexts:\n%s", out)
	}
}

func TestDetailHeaderVisibleAcrossPaneSizes(t *testing.T) {
	summary := ado.PRSummary{
		ID: 7, Title: "Big change", Repo: "acme/web",
		Author: "alice", SourceBranch: "feat/x", TargetBranch: "main",
	}
	// Long description with WIDE lines that will wrap heavily in narrow panes.
	desc := ""
	for i := 0; i < 50; i++ {
		desc += fmt.Sprintf("description line %d with extra padding text to force the renderer to wrap because the pane may be narrow and lines are long\n", i)
	}
	files := make([]ado.FileChange, 40)
	for i := range files {
		files[i] = ado.FileChange{Path: fmt.Sprintf("/src/file_%03d.go", i), ChangeType: "edit"}
	}

	forEachPaneSize(t, func(t *testing.T, paneW, paneH int) {
		m := NewDetail(DefaultKeys())
		m = m.SetSummary(summary)
		m, _ = m.Update(detailLoadedMsg{detail: &ado.PRDetail{PRSummary: summary, DescriptionMD: desc}})
		m, _ = m.Update(filesLoadedMsg{files: files})
		// Render through the same path production uses.
		m = m.SetPaneSize(paneW, paneH)
		out := m.ViewWithFocus(true)
		assertHeaderVisible(t, out, summary, paneH)
	})
}

func TestDetailWindowsLongFileListAroundCursor(t *testing.T) {
	m := NewDetail(DefaultKeys())
	m = m.SetSummary(ado.PRSummary{ID: 99, Title: "Many files"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	files := make([]ado.FileChange, 60)
	for i := range files {
		files[i] = ado.FileChange{Path: fmt.Sprintf("/src/file_%03d.go", i), ChangeType: "edit"}
	}
	m, _ = m.Update(filesLoadedMsg{files: files})

	out := m.View()
	if !strings.Contains(out, "PR #99") || !strings.Contains(out, "Many files") {
		t.Fatalf("PR title bar missing:\n%s", out)
	}
	if strings.Contains(out, "file_059") {
		t.Fatalf("last file should NOT be visible at top:\n%s", out)
	}

	for i := 0; i < 50; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	out = m.View()
	if !strings.Contains(out, "file_050") {
		t.Fatalf("cursor file should be visible after scrolling:\n%s", out)
	}
	if strings.Contains(out, "file_000") {
		t.Fatalf("top file should have scrolled out:\n%s", out)
	}
}

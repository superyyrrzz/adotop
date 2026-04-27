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
		assertHeaderVisible(t, out, summary, paneW, paneH)
	})
}

func TestDetailHeaderFitsWithLongRepoLineAndManyReviewers(t *testing.T) {
	// Regression for PR #1145102: long repo+branch line and 5 reviewer
	// rows wrap inside a ~40-col left pane, so the rendered header is
	// taller than its source-line count. If we measure pre-wrap, the
	// file list over-budgets and the total view exceeds bodyHeight,
	// causing the terminal to scroll and clip the top.
	summary := ado.PRSummary{
		ID: 1145102, Title: "release 4.27", Repo: "Docs.RelationalContentService",
		Author: "Some Long Author Name", SourceBranch: "develop", TargetBranch: "master",
		Reviewers: []ado.ReviewerVote{
			{DisplayName: "Reviewer One", Vote: 10, IsRequired: true},
			{DisplayName: "Reviewer Two With Long Name", Vote: 0},
			{DisplayName: "Reviewer Three", Vote: 5},
			{DisplayName: "Reviewer Four Long-ish", Vote: 0, IsRequired: true},
			{DisplayName: "Reviewer Five", Vote: 0},
		},
	}
	desc := "Dependency upgrade and code refactoring: Migrates object mapping from " +
		"AutoMapper to ForgeMap library across the entire service surface, including " +
		"all the request handlers, response serializers, and integration test fixtures."
	files := make([]ado.FileChange, 25)
	for i := range files {
		files[i] = ado.FileChange{Path: fmt.Sprintf("/src/file_%03d.go", i), ChangeType: "edit"}
	}
	forEachPaneSize(t, func(t *testing.T, paneW, paneH int) {
		m := NewDetail(DefaultKeys())
		m = m.SetSummary(summary)
		m, _ = m.Update(detailLoadedMsg{detail: &ado.PRDetail{PRSummary: summary, DescriptionMD: desc}})
		m, _ = m.Update(filesLoadedMsg{files: files})
		m = m.SetPaneSize(paneW, paneH)
		out := m.ViewWithFocus(true)
		assertHeaderVisible(t, out, summary, paneW, paneH)
	})
}

func TestDetailDisplayNeighborsWalksSortedTree(t *testing.T) {
	// API order is reversed; sorted tree puts /a before /b before /c.
	files := []ado.FileChange{
		{Path: "/c.go"}, {Path: "/b.go"}, {Path: "/a.go"},
	}
	m := NewDetail(DefaultKeys())
	m, _ = m.Update(filesLoadedMsg{files: files})
	// API index 1 == /b.go (the middle file in display order too).
	m.cursor = 1
	got := m.DisplayNeighbors(1)
	if len(got) != 2 {
		t.Fatalf("want 2 neighbors, got %v", got)
	}
	// Neighbors are returned d=1: [pos-1, pos+1].
	// Display order: a(idx2), b(idx1), c(idx0). pos for cursor=1 is 1.
	// pos-1 -> idx2 (a.go), pos+1 -> idx0 (c.go).
	if got[0] != 2 || got[1] != 0 {
		t.Fatalf("want [2,0] (display-order neighbors of /b.go), got %v", got)
	}
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

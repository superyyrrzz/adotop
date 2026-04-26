package ui

import (
	"strings"
	"testing"

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
	if !strings.Contains(out, "/src/login.go") {
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

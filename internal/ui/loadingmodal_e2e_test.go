package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/config"
)

// TestURLLaunchModalAppearsBeforeJumpResolves simulates the actual
// runtime sequence Run() drives:
//   1. New() → m.initialPRID = 1145743 (set by Run before Init)
//   2. Init() runs → fires fetchConnectionData
//   3. connDataMsg arrives → handler should set loadingPRModal and
//      dispatch jumpRequestedMsg
//   4. View() at this point must contain the "Loading PR #N…" text
//
// If any link in the chain breaks, the user sees the bare list flash
// just like before. This test catches that without needing a terminal.
func TestURLLaunchModalAppearsBeforeJumpResolves(t *testing.T) {
	cfg := config.Config{Org: "ceapex", Project: "Engineering"}
	m := New(cfg, nil)
	m.initialPRID = 1145743
	// Window-size pass first so list/detail compute their inner sizes
	// — Run() also sends one of these before any business messages.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	// Simulate connDataMsg arriving with valid auth. The connData
	// handler is what sets loadingPRModal and fires the jump.
	cd := &ado.ConnectionData{}
	cd.AuthenticatedUser.ID = "me"
	cd.AuthenticatedUser.CustomDisplayName = "Me"
	updated, _ = m.Update(connDataMsg{data: cd})
	m = updated.(Model)

	if m.loadingPRModal != 1145743 {
		t.Fatalf("loadingPRModal not set after connDataMsg: got %d", m.loadingPRModal)
	}

	out := m.View()
	if !strings.Contains(out, "PR #1145743") {
		// Print the full view so we can see what's actually rendering.
		t.Fatalf("View should contain loading modal text.\n--- View output ---\n%s\n--- end ---", out)
	}
	// Always dump for inspection.
	t.Logf("--- VIEW OUTPUT (width=120, height=40) ---\n%s\n--- END ---", stripANSI(out))
}

// TestURLLaunchModalBeforeWindowSize: in real usage on a fast machine,
// connDataMsg can land before the first WindowSizeMsg. m.height is
// then 0, so the original height computation collapses to 0 and the
// modal lands inside a 0-row body — invisible.
func TestURLLaunchModalBeforeWindowSize(t *testing.T) {
	cfg := config.Config{Org: "ceapex", Project: "Engineering"}
	m := New(cfg, nil)
	m.initialPRID = 999
	cd := &ado.ConnectionData{}
	cd.AuthenticatedUser.ID = "me"
	cd.AuthenticatedUser.CustomDisplayName = "Me"
	updated, _ := m.Update(connDataMsg{data: cd})
	m = updated.(Model)
	out := m.View()
	t.Logf("--- VIEW (no WindowSize yet) ---\n%s\n--- END ---", stripANSI(out))
	if !strings.Contains(out, "PR #999") {
		t.Fatalf("modal text missing when WindowSizeMsg hasn't arrived yet")
	}
}

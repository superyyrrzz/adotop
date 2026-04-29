package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// currentThreadID returns the ADO thread ID currently under the cursor on
// the focused file, or 0 when no thread is selected (cursor unset, no
// threads on file, or threads list is empty). Callers use 0 as the
// "nothing selected → no-op" sentinel; the C/x action keys check this
// before dispatching.
func (m Model) currentThreadID() int {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return 0
	}
	threads := m.threadsForFile(f.Path)
	if len(threads) == 0 {
		return 0
	}
	idx, ok := m.threadCursor[f.Path]
	if !ok || idx < 0 || idx >= len(threads) {
		return 0
	}
	return threads[idx].ID
}

// moveThreadCursor advances or retreats the per-file thread cursor. The
// first call (cursor unset) lands on index 0 regardless of direction —
// "next thread" from no-selection means the first thread. Subsequent
// calls clamp at both ends; no wraparound, so holding ] doesn't surprise.
func (m Model) moveThreadCursor(delta int) Model {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return m
	}
	threads := m.threadsForFile(f.Path)
	if len(threads) == 0 {
		return m
	}
	idx, set := m.threadCursor[f.Path]
	if !set {
		m.threadCursor[f.Path] = 0
		return m
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(threads) {
		idx = len(threads) - 1
	}
	m.threadCursor[f.Path] = idx
	return m
}

// toggleResolveCurrentThread returns a tea.Cmd that flips the selected
// thread between "active" and "fixed". No-op (returns nil) when no
// thread is selected. The result lands as actionDoneMsg with kind
// "resolveThread" or "reactivateThread" so the Update handler picks the
// right success message and refresh path.
func (m Model) toggleResolveCurrentThread() tea.Cmd {
	tid := m.currentThreadID()
	if tid == 0 {
		return nil
	}
	var current ado.Thread
	for _, t := range m.threads {
		if t.ID == tid {
			current = t
			break
		}
	}
	if current.ID == 0 {
		return nil
	}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	if repoID == "" || prID == 0 {
		return nil
	}
	target := "fixed"
	kind := "resolveThread"
	notes := fmt.Sprintf("thread #%d resolved", tid)
	if current.IsResolved() {
		target = "active"
		kind = "reactivateThread"
		notes = fmt.Sprintf("thread #%d reactivated", tid)
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := client.PatchThreadStatus(ctx, repoID, prID, tid, target)
		return actionDoneMsg{kind: kind, prID: prID, err: err, notes: notes}
	}
}


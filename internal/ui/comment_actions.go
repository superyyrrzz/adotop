package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// threadKeysActive reports whether thread-action keys ([/], c, C, x)
// should fire in the current focus + selection. Active in Diff focus
// (where the cursor walks file-anchored threads) and in Files focus
// when the synthetic Discussion entry is selected (where the cursor
// walks PR-level threads). Both routes converge on currentThreadID,
// so the action handlers don't need to branch.
func (m Model) threadKeysActive() bool {
	if m.detailFocus == focusDiff {
		return true
	}
	return m.detail.IsDiscussionSelected()
}

// currentThreadID returns the ADO thread ID currently under the cursor on
// the focused file, or 0 when no thread is selected (cursor unset, no
// threads on file, or threads list is empty). Callers use 0 as the
// "nothing selected → no-op" sentinel; the C/x action keys check this
// before dispatching.
func (m Model) currentThreadID() int {
	if m.detail.IsDiscussionSelected() {
		threads := m.prLevelThreads()
		if len(threads) == 0 {
			return 0
		}
		idx := m.prThreadCursor
		if idx < 0 || idx >= len(threads) {
			return 0
		}
		return threads[idx].ID
	}
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
	if m.detail.IsDiscussionSelected() {
		threads := m.prLevelThreads()
		if len(threads) == 0 {
			return m
		}
		if m.prThreadCursor < 0 {
			m.prThreadCursor = 0
			return m
		}
		idx := m.prThreadCursor + delta
		if idx < 0 {
			idx = 0
		}
		if idx >= len(threads) {
			idx = len(threads) - 1
		}
		m.prThreadCursor = idx
		return m
	}
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

// composeResultMsg is dispatched after the user closes their $EDITOR.
// body is the trimmed editor output — empty means the user cancelled.
// targetThreadID is non-zero for replies; targetFilePath is non-empty
// for file-level new threads (PR-level when both are zero/empty).
// tmpPath is the temp file the editor wrote to; the result handler is
// responsible for cleaning it up.
type composeResultMsg struct {
	body           string
	targetThreadID int
	targetFilePath string
	tmpPath        string
	err            error
}

// postNewThreadCmd POSTs a new thread. filePath == "" creates a
// PR-level thread; non-empty anchors it to the file. Returns nil for
// empty bodies so cancelled compose sessions don't pester ADO. The
// success notes string carries the routing target so the user's
// banner reads "comment posted on foo.go" rather than a generic
// "comment posted" — useful when several modals were dispatched
// quickly and the user wants to confirm where each one landed.
func (m Model) postNewThreadCmd(body, filePath string) tea.Cmd {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	if repoID == "" || prID == 0 {
		return nil
	}
	client := m.client
	notes := "comment posted"
	if filePath != "" {
		notes = "comment posted on " + filePath
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := client.PostPRThread(ctx, repoID, prID, body, filePath)
		return actionDoneMsg{kind: "postThread", prID: prID, err: err, notes: notes}
	}
}

// postReplyCmd POSTs a reply to the given thread. Mirrors
// postNewThreadCmd in shape; the kind tag in actionDoneMsg lets the
// handler distinguish them in the success footer.
func (m Model) postReplyCmd(threadID int, body string) tea.Cmd {
	if strings.TrimSpace(body) == "" || threadID == 0 {
		return nil
	}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	if repoID == "" || prID == 0 {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := client.PostThreadComment(ctx, repoID, prID, threadID, body)
		return actionDoneMsg{kind: "postComment", prID: prID, err: err, notes: fmt.Sprintf("reply posted to #%d", threadID)}
	}
}


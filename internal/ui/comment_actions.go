package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// composeResultMsg is dispatched after the user closes their $EDITOR.
// body is the trimmed editor output — empty means the user cancelled.
// targetThreadID is 0 for new PR-level threads, non-zero for replies.
// tmpPath is the temp file the editor wrote to; the result handler is
// responsible for cleaning it up.
type composeResultMsg struct {
	body           string
	targetThreadID int
	tmpPath        string
	err            error
}

// composeNewThreadCmd seeds a temp file, suspends the TUI via
// tea.ExecProcess, and returns a composeResultMsg with the edited
// body when the editor exits. targetThreadID==0 marks this as a new
// thread (vs. reply) so the result handler routes to PostPRThread.
func (m Model) composeNewThreadCmd() tea.Cmd {
	seed := "<!-- Comment will be posted as a new PR-level thread. Save empty to cancel. -->\n\n"
	tmpPath, err := writeSeedFile(seed)
	if err != nil {
		return func() tea.Msg { return composeResultMsg{err: err} }
	}
	cmd, err := buildEditorCmd(resolveEditor(), tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return func() tea.Msg { return composeResultMsg{err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpPath)
			return composeResultMsg{err: err}
		}
		b, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			os.Remove(tmpPath)
			return composeResultMsg{err: readErr}
		}
		return composeResultMsg{body: trimSeedAndComments(string(b)), tmpPath: tmpPath}
	})
}

// postNewThreadCmd POSTs a new PR-level thread. Returns nil for empty
// bodies so cancelled compose sessions don't pester ADO.
func (m Model) postNewThreadCmd(body string) tea.Cmd {
	if strings.TrimSpace(body) == "" {
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
		_, err := client.PostPRThread(ctx, repoID, prID, body, "")
		return actionDoneMsg{kind: "postThread", prID: prID, err: err, notes: "comment posted"}
	}
}

// composeReplyCmd seeds the editor with a reply hint and returns a
// composeResultMsg with targetThreadID set so the result handler routes
// to PostThreadComment instead of PostPRThread.
func (m Model) composeReplyCmd() tea.Cmd {
	tid := m.currentThreadID()
	if tid == 0 {
		return nil
	}
	seed := fmt.Sprintf("<!-- Reply to thread #%d. Save empty to cancel. -->\n\n", tid)
	tmpPath, err := writeSeedFile(seed)
	if err != nil {
		return func() tea.Msg { return composeResultMsg{targetThreadID: tid, err: err} }
	}
	cmd, err := buildEditorCmd(resolveEditor(), tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return func() tea.Msg { return composeResultMsg{targetThreadID: tid, err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpPath)
			return composeResultMsg{targetThreadID: tid, err: err}
		}
		b, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			os.Remove(tmpPath)
			return composeResultMsg{targetThreadID: tid, err: readErr}
		}
		return composeResultMsg{body: trimSeedAndComments(string(b)), targetThreadID: tid, tmpPath: tmpPath}
	})
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


package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ado"
)

// commitsModalState backs the M-key picker that lets the user view a
// single commit's diff instead of the accumulated PR diff. Lives on
// Model; nil means closed. Cursor walks the list with j/k; enter
// selects; esc cancels without changing the active commit view.
type commitsModalState struct {
	commits []ado.Commit
	cursor  int
	loading bool
	err     string
}

// commitsLoadedMsg is the response from GetPullRequestCommits, fired
// asynchronously when the modal opens. We render a "loading…" state
// in the meantime so the user gets immediate feedback that the modal
// opened (rather than seeing a blank box for a network round-trip).
type commitsLoadedMsg struct {
	commits []ado.Commit
	err     error
}

// commitsModalOpen reports whether the picker is currently up.
func (m Model) commitsModalOpen() bool { return m.commitsModal != nil }

// openCommitsModal kicks off the commit fetch and shows the loading
// box. The handler for commitsLoadedMsg fills in the list.
func (m Model) openCommitsModal() (Model, tea.Cmd) {
	if m.detail.Detail() == nil {
		return m, nil
	}
	m.commitsModal = &commitsModalState{loading: true}
	repoID := m.detail.Summary().RepoID
	prID := m.detail.Summary().ID
	client := m.client
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		commits, err := client.GetPullRequestCommits(ctx, repoID, prID)
		return commitsLoadedMsg{commits: commits, err: err}
	}
	return m, cmd
}

// closeCommitsModal tears down the modal state without changing the
// active commit-view. esc inside the modal calls this to cancel
// without affecting whatever the user is currently looking at.
func (m Model) closeCommitsModal() Model {
	m.commitsModal = nil
	return m
}

// updateCommitsModal handles the keys recognized while the picker is
// open. j/k move the cursor; enter selects; esc cancels. Everything
// else is swallowed so a stray keystroke can't act on the underlying
// screen.
func (m Model) updateCommitsModal(msg tea.KeyMsg) (Model, tea.Cmd) {
	st := m.commitsModal
	if st == nil {
		return m, nil
	}
	if st.loading {
		// Only esc is meaningful while the fetch is in flight.
		if msg.Type == tea.KeyEsc {
			return m.closeCommitsModal(), nil
		}
		return m, nil
	}
	switch {
	case msg.Type == tea.KeyEsc:
		return m.closeCommitsModal(), nil
	case keyMatches(msg, m.keys.Down):
		if len(st.commits) > 0 {
			st.cursor = (st.cursor + 1) % len(st.commits)
		}
	case keyMatches(msg, m.keys.Up):
		if len(st.commits) > 0 {
			st.cursor = (st.cursor - 1 + len(st.commits)) % len(st.commits)
		}
	case keyMatches(msg, m.keys.Open):
		if len(st.commits) == 0 {
			return m.closeCommitsModal(), nil
		}
		picked := st.commits[st.cursor]
		mm, cmd := m.enterCommitView(picked)
		mm = mm.closeCommitsModal()
		return mm, cmd
	}
	return m, nil
}

// renderCommitsModal returns the bordered overlay. Loading state
// renders a centered "loading…" so the user sees the modal opened.
// Error state surfaces the message inline (modal stays interactive
// — esc closes).
func (m Model) renderCommitsModal() string {
	st := m.commitsModal
	if st == nil {
		return ""
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(Cursor.GetForeground()).Render("Commits in this PR")
	hint := Faint.Render("j/k navigate · enter view · esc cancel · M toggles")
	if st.loading {
		body := lipgloss.JoinVertical(lipgloss.Left, title, "", Faint.Render("loading…"), "", hint)
		return ModalBox.Render(body)
	}
	if st.err != "" {
		body := lipgloss.JoinVertical(lipgloss.Left, title, "", ErrLine.Render(st.err), "", hint)
		return ModalBox.Render(body)
	}
	if len(st.commits) == 0 {
		body := lipgloss.JoinVertical(lipgloss.Left, title, "", Faint.Render("(no commits)"), "", hint)
		return ModalBox.Render(body)
	}
	rows := make([]string, 0, len(st.commits))
	for i, c := range st.commits {
		date := ""
		if !c.CommitDate.IsZero() {
			date = c.CommitDate.Format("01-02 15:04")
		}
		line := fmt.Sprintf("  %s  %s  %s  %s",
			Faint.Render(c.ShortID()),
			Faint.Render(date),
			padCols(truncCols(c.Author, 18), 18),
			truncCols(c.Subject, 60))
		if i == st.cursor {
			line = "▸" + line[1:] // swap leading space for the cursor mark
			line = Selected.Render(line)
		}
		rows = append(rows, line)
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		"",
		hint,
	)
	return ModalBox.Render(body)
}

// enterCommitView swaps the detail-screen file list and diff base
// over to the picked commit. Threads are intentionally hidden in
// this view (line anchors come from the PR's iteration and won't
// align with arbitrary per-commit diffs).
//
// Asynchronous: the commit's file list is fetched, then a
// commitChangesLoadedMsg flips the model into the per-commit view
// and refreshes the preview. Until that arrives, the screen still
// shows the previous content — preserves the user's place if the
// fetch fails.
func (m Model) enterCommitView(c ado.Commit) (Model, tea.Cmd) {
	repoID := m.detail.Summary().RepoID
	client := m.client
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		files, err := client.GetCommitChanges(ctx, repoID, c.ID)
		return commitChangesLoadedMsg{commit: c, files: files, err: err}
	}
	return m, cmd
}

// commitChangesLoadedMsg lands when the per-commit file list arrives.
// The handler swaps it into the detail screen and rebuilds the
// preview against the commit's parent SHA → commit SHA pair.
type commitChangesLoadedMsg struct {
	commit ado.Commit
	files  []ado.FileChange
	err    error
}

// exitCommitView restores the PR-wide file list and SHAs cached at
// entry. Called by the second M-press, returning the user to the
// "all commits" view they came from.
func (m Model) exitCommitView() Model {
	if m.viewingCommit == nil {
		return m
	}
	files := m.prFiles
	m.viewingCommit = nil
	m.prFiles = nil
	m.detail, _ = m.detail.Update(filesLoadedMsg{files: files})
	m.previewKey = ""
	mm, _ := m.queuePreviewForSelection()
	return mm
}

// effectiveSourceSha returns the SHA the diff "from" side should use
// for fetches and cache keys. PR view uses the PR's source/target
// pair as before; commit view uses the commit's parent (the diff
// base) → commit (the diff tip), which is the same shape but scoped
// to a single commit's changes.
func (m Model) effectiveSourceSha() string {
	if m.viewingCommit != nil {
		// "Source" in the diff renderer means the side we're moving
		// TOWARD (the right-hand column); for a commit view that's
		// the commit itself.
		return m.viewingCommit.ID
	}
	if m.detail.Detail() == nil {
		return ""
	}
	return m.detail.Detail().SourceSha
}

// effectiveTargetSha is the symmetric "from" side.
func (m Model) effectiveTargetSha() string {
	if m.viewingCommit != nil {
		// Empty parent (root commit, or first commit when ADO
		// omitted parents) falls back to the PR's target SHA so the
		// diff still renders against something meaningful instead of
		// failing — degrades to "everything new in this commit
		// relative to the PR base."
		if m.viewingCommit.ParentID != "" {
			return m.viewingCommit.ParentID
		}
		if m.detail.Detail() != nil {
			return m.detail.Detail().TargetSha
		}
		return ""
	}
	if m.detail.Detail() == nil {
		return ""
	}
	return m.detail.Detail().TargetSha
}

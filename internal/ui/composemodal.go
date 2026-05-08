package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// composeModalState backs the in-TUI compose overlay. It wraps a
// bubbles textarea and remembers the routing context: targetThreadID
// is 0 for a new PR-level thread, non-zero for a reply to that thread.
//
// Lives on Model; nil means the modal is closed. The textarea owns its
// own key bindings (enter inserts a newline, etc.); the parent Update
// only intercepts the modal-level chrome keys (esc, ctrl+s, ctrl+e).
type composeModalState struct {
	ta             textarea.Model
	targetThreadID int
	// kind is "new" or "reply" — used for the modal title and the
	// editor-seed comment when the user escapes out to $EDITOR via
	// ctrl+e. The success message (notes on actionDoneMsg) is built
	// elsewhere from the same inputs.
	kind string
}

// composeModalOpen reports whether the compose overlay is up.
func (m Model) composeModalOpen() bool { return m.composeModal != nil }

// openComposeNewModal opens the modal in "new PR-level thread" mode.
// Cursor focus shifts to the textarea immediately so the user can
// start typing without an extra keypress.
func (m Model) openComposeNewModal() Model {
	ta := newComposeTextarea(composeModalInnerSize(m.width, m.height))
	ta.Placeholder = "Type your comment. ctrl+s to send · ctrl+e to open $EDITOR · esc to cancel"
	ta.Focus()
	m.composeModal = &composeModalState{ta: ta, kind: "new"}
	return m
}

// openComposeReplyModal opens the modal in "reply to thread N" mode.
// No-op when there's no thread under the cursor — there's nothing to
// reply to, and silently dropping the open is friendlier than
// flashing an empty modal.
func (m Model) openComposeReplyModal() Model {
	tid := m.currentThreadID()
	if tid == 0 {
		return m
	}
	ta := newComposeTextarea(composeModalInnerSize(m.width, m.height))
	ta.Placeholder = fmt.Sprintf("Reply to thread #%d. ctrl+s to send · ctrl+e to open $EDITOR · esc to cancel", tid)
	ta.Focus()
	m.composeModal = &composeModalState{ta: ta, targetThreadID: tid, kind: "reply"}
	return m
}

// closeComposeModal tears down the modal state. The textarea is
// dropped along with the parent struct so any partial buffer is gone
// — by design: cancelling means cancelling, not "park this draft."
func (m Model) closeComposeModal() Model {
	m.composeModal = nil
	return m
}

// updateComposeModal handles the chrome keys recognized while the
// modal is open. Everything else is delegated to the textarea so
// arrow keys, enter (newline), home/end, etc. behave naturally.
//
//	ctrl+s → submit (route to postNewThreadCmd or postReplyCmd)
//	ctrl+e → suspend and open $EDITOR pre-seeded with current buffer
//	esc    → cancel without submitting
func (m Model) updateComposeModal(msg tea.KeyMsg) (Model, tea.Cmd) {
	st := m.composeModal
	switch msg.Type {
	case tea.KeyEsc:
		return m.closeComposeModal(), nil
	case tea.KeyCtrlS:
		body := strings.TrimSpace(st.ta.Value())
		target := st.targetThreadID
		m = m.closeComposeModal()
		if body == "" {
			return m, nil
		}
		if target != 0 {
			return m, m.postReplyCmd(target, body)
		}
		return m, m.postNewThreadCmd(body)
	case tea.KeyCtrlE:
		// Escape hatch for long prose — drop the current buffer into
		// $EDITOR. We reuse the existing tea.ExecProcess path so the
		// result lands on composeResultMsg like before. Close the
		// modal first so we don't try to render it underneath the
		// suspended TUI.
		seed := buildEditorSeed(st.kind, st.targetThreadID, st.ta.Value())
		target := st.targetThreadID
		m = m.closeComposeModal()
		return m, openEditorWithSeed(seed, target)
	}
	var cmd tea.Cmd
	st.ta, cmd = st.ta.Update(msg)
	return m, cmd
}

// renderComposeModal returns the bordered overlay box ready for
// overlayBox to splice over the underlying screen. Title carries the
// thread context (new vs. reply); footer reminds the user of the
// submit/cancel/escape-hatch keys.
func (m Model) renderComposeModal() string {
	if m.composeModal == nil {
		return ""
	}
	st := m.composeModal
	titleText := "New PR Comment"
	if st.kind == "reply" {
		titleText = fmt.Sprintf("Reply to Thread #%d", st.targetThreadID)
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(Cursor.GetForeground()).Render(titleText)
	hint := Faint.Render("ctrl+s send · ctrl+e $EDITOR · esc cancel · enter newline")
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		st.ta.View(),
		"",
		hint,
	)
	return ModalBox.Render(content)
}

// newComposeTextarea constructs a textarea sized to the inner (w, h)
// of the modal box — caller already accounted for borders, title,
// and hint rows.
func newComposeTextarea(innerW, innerH int) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""        // strip the default "│ " gutter — the modal border carries the visual
	ta.ShowLineNumbers = false
	ta.CharLimit = 0      // no hard cap — long technical reviews are common
	ta.SetWidth(innerW)
	ta.SetHeight(innerH)
	return ta
}

// composeModalSize picks a comfortable box size — wide enough to
// wrap a paragraph without horizontal eye-strain, short enough to
// leave context visible above. Mirrors descModalSize so the two
// overlays feel like siblings, not separate visual languages.
func composeModalSize(termW, termH int) (int, int) {
	if termW <= 0 || termH <= 0 {
		return 80, 16
	}
	w := termW * 4 / 5
	h := termH * 3 / 5
	if w > 100 {
		w = 100
	}
	if w < 40 {
		w = 40
	}
	if h < 12 {
		h = 12
	}
	if h > 24 {
		h = 24
	}
	return w, h
}

// composeModalInnerSize returns the textarea's content size after
// reserving space for the modal border (2 cols, 2 rows), title row,
// blank spacer rows, and hint row.
func composeModalInnerSize(termW, termH int) (int, int) {
	w, h := composeModalSize(termW, termH)
	innerW := w - 4
	// 4 chrome rows: title + blank + blank + hint. Plus 2 for borders.
	innerH := h - 6
	if innerW < 20 {
		innerW = 20
	}
	if innerH < 4 {
		innerH = 4
	}
	return innerW, innerH
}

// buildEditorSeed produces the initial file contents handed to
// $EDITOR when the user uses ctrl+e to escape out of the in-TUI
// modal. We preserve the textarea buffer below the seed comment so
// nothing is lost in the handoff.
func buildEditorSeed(kind string, targetThreadID int, current string) string {
	header := "<!-- Comment will be posted as a new PR-level thread. Save empty to cancel. -->\n\n"
	if kind == "reply" && targetThreadID != 0 {
		header = fmt.Sprintf("<!-- Reply to thread #%d. Save empty to cancel. -->\n\n", targetThreadID)
	}
	return header + current
}

// openEditorWithSeed writes the given seed to a temp file, suspends
// the TUI via tea.ExecProcess, and returns a composeResultMsg with
// the edited body — same shape the legacy compose*Cmd helpers used,
// so the result lands on the existing composeResultMsg handler.
func openEditorWithSeed(seed string, targetThreadID int) tea.Cmd {
	tmpPath, err := writeSeedFile(seed)
	if err != nil {
		return func() tea.Msg { return composeResultMsg{targetThreadID: targetThreadID, err: err} }
	}
	cmd, err := buildEditorCmd(resolveEditor(), tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return func() tea.Msg { return composeResultMsg{targetThreadID: targetThreadID, err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpPath)
			return composeResultMsg{targetThreadID: targetThreadID, err: err}
		}
		b, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			os.Remove(tmpPath)
			return composeResultMsg{targetThreadID: targetThreadID, err: readErr}
		}
		return composeResultMsg{body: trimSeedAndComments(string(b)), targetThreadID: targetThreadID, tmpPath: tmpPath}
	})
}

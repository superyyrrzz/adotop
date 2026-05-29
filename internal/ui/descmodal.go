package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// descModalState holds the scrollable viewport for the full PR
// description overlay. Lives on Model; nil means the modal is closed.
// We keep the rendered glamour body cached on the struct so re-renders
// (j/k scroll) don't re-run the markdown pipeline on every keypress.
type descModalState struct {
	vp      viewport.Model
	body    string // rendered body the viewport was filled with
	width   int    // width the body was rendered at — invalidate on resize
}

// descModalOpen returns true when the description modal is up.
func (m Model) descModalOpen() bool { return m.descModal != nil }

// openDescModal renders the current PR description through the same
// glamour pipeline as the inline header, then sizes a viewport to fit
// most of the terminal so the user can scroll a long description
// without it competing with the file list. Returns the model unchanged
// when there's no description to show.
func (m Model) openDescModal() Model {
	if m.detail.Detail() == nil {
		return m
	}
	desc := strings.TrimSpace(m.detail.Detail().DescriptionMD)
	if desc == "" {
		return m
	}
	w, h := descModalSize(m.width, m.height)
	// Reserve 2 cols of inner padding on each side and 2 rows for the
	// title + footer hint inside the box. The glamour body is rendered
	// to the inner content width.
	innerW := w - 4
	innerH := h - 4
	if innerW < 20 {
		innerW = 20
	}
	if innerH < 5 {
		innerH = 5
	}
	body := strings.TrimRight(renderCommentBody(desc, innerW, ""), "\n")
	vp := viewport.New(innerW, innerH)
	vp.SetContent(body)
	m.descModal = &descModalState{vp: vp, body: body, width: innerW}
	return m
}

// closeDescModal tears down the modal state so the next render skips
// the overlay path. Cheap — the cached body is dropped along with the
// state struct.
func (m Model) closeDescModal() Model {
	m.descModal = nil
	return m
}

// updateDescModal handles the keys recognized while the modal is open.
// j/k/PgUp/PgDn/g/G scroll; esc/q/D close. Any other key is swallowed
// so the underlying screen doesn't act on it (avoids the user
// accidentally voting on a PR while reading its description).
func (m Model) updateDescModal(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc, keyMatches(msg, m.keys.Quit), keyMatches(msg, m.keys.DescModal):
		return m.closeDescModal(), nil
	}
	// Delegate scroll keys to the viewport. We update its key bindings
	// on the fly to reuse the standard j/k/g/G/PgUp/PgDn idiom that
	// every other pane in the app supports.
	var cmd tea.Cmd
	m.descModal.vp, cmd = m.descModal.vp.Update(msg)
	return m, cmd
}

// renderDescModal returns the box ready for overlayBox to splice over
// the underlying screen. Title carries the scroll percent so the user
// knows there's more below; footer reminds them of the close key.
func (m Model) renderDescModal() string {
	if m.descModal == nil {
		return ""
	}
	st := m.descModal
	pct := int(st.vp.ScrollPercent() * 100)
	// Title uses the Cursor accent so the modal's header reads as a
	// peer of the loading modal's "LOADING" header — both are
	// "this is the active surface" labels.
	title := lipgloss.NewStyle().Bold(true).Foreground(Cursor.GetForeground()).Render("PR Description")
	pos := Faint.Render(formatPercent(pct))
	titleBar := lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", pos)
	hint := Faint.Render("j/k scroll · g/G top/end · esc/D/q close")
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		"",
		st.vp.View(),
		"",
		hint,
	)
	return ModalBox.Render(content)
}

// descModalSize picks a comfortable box size — wide enough to read
// prose without horizontal eye-strain, but always with margin around
// it so the underlying screen stays visible as context. Height is
// capped at termH-12 so the rendered modal (descModalSize.h plus the
// 4-row ModalBox chrome) still fits inside overlayBox's body area
// (termH minus the ~8-row header+footer+spacer cost). Without the
// cap, the bottom of the modal clipped at every common terminal
// height — bug existed since the modal was introduced; surfaced
// during the layout-primitives refactor.
func descModalSize(termW, termH int) (int, int) {
	if termW <= 0 || termH <= 0 {
		return 80, 24
	}
	w := clamp(termW*4/5, 40, 100)
	h := clamp(termH*4/5, 10, termH-12)
	return w, h
}

func formatPercent(p int) string {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return strconv.Itoa(p) + "%"
}

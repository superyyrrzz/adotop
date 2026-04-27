package ui

import (
	"fmt"
	"strings"

	"github.com/renzeyu/adotop/internal/ado"
)

// threadsForFile returns threads attached to the given file path, applying
// the showResolved filter. Threads not anchored to a file (PR-level) are
// returned only when path == "".
func (m Model) threadsForFile(path string) []ado.Thread {
	out := make([]ado.Thread, 0, len(m.threads))
	for _, t := range m.threads {
		if t.FilePath != path {
			continue
		}
		if !m.showResolved && t.IsResolved() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// toggleThreadsForFile flips the expanded state of every visible thread on
// the given file. If any thread is currently collapsed, this expands all of
// them; otherwise collapses all.
func (m Model) toggleThreadsForFile(path string) Model {
	threads := m.threadsForFile(path)
	if len(threads) == 0 {
		return m
	}
	anyCollapsed := false
	for _, t := range threads {
		if !m.expandedThread[t.ID] {
			anyCollapsed = true
			break
		}
	}
	for _, t := range threads {
		m.expandedThread[t.ID] = anyCollapsed
	}
	return m
}

// refreshPreview re-renders the diff preview viewport's content so that
// changes to thread filters / expansion show up immediately. No-op when
// no preview is loaded yet, or when there's no comments block AND the
// viewport already has content (the diffLoadedMsg path or a previous
// refresh already wrote it).
//
// We skip the SetContent when the comments block is empty because the
// viewport already shows the colorized diff that was set in the
// diffLoadedMsg handler — recomputing it just trashes the scroll
// position and wastes a Colorize.
func (m Model) refreshPreview() Model {
	if m.previewKey == "" {
		return m
	}
	rendered, ok := m.previewCache.Rendered(m.previewKey)
	if !ok {
		return m
	}
	commentsBlock := m.previewCommentsBlock()
	composed := composeDiffWithComments(nil, commentsBlock)
	if composed == "" && m.preview.loaded {
		// Diff already in viewport; nothing to fold in. Skip SetContent
		// so we don't pay for a viewport rebuild on every j/k.
		return m
	}
	m.preview.vp.SetContent(rendered + composed)
	return m
}

// previewCommentsBlock returns the rendered "Comments" footer for the
// currently-selected file in the diff preview pane.
func (m Model) previewCommentsBlock() string {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return ""
	}
	threads := m.threadsForFile(f.Path)
	return renderCommentsBlock(threads, m.expandedThread, m.showResolved, m.threads, f.Path)
}

// composeDiffWithComments stitches the rendered diff and the comments block
// into a single viewport string. Currently the commentsBlock is appended
// directly because Colorize is applied separately by the caller.
func composeDiffWithComments(diffBody []byte, commentsBlock string) string {
	if commentsBlock == "" {
		return ""
	}
	return "\n" + commentsBlock
}

// renderCommentsBlock formats the threads attached to a file. Each thread
// shows the first comment plus a `[N replies]` suffix; expanded threads
// show every comment. Header summarizes total open / hidden-resolved count.
func renderCommentsBlock(threads []ado.Thread, expanded map[int]bool, showResolved bool, all []ado.Thread, path string) string {
	if len(threads) == 0 && !hasAnyForFile(all, path) {
		return ""
	}
	var b strings.Builder
	hidden := 0
	if !showResolved {
		for _, t := range all {
			if t.FilePath == path && t.IsResolved() {
				hidden++
			}
		}
	}
	header := fmt.Sprintf("─ Comments on %s  (%d open", path, len(threads))
	if hidden > 0 {
		header += fmt.Sprintf(", %d resolved hidden — press R", hidden)
	}
	header += ") "
	b.WriteString(Faint.Render(header))
	b.WriteString("\n")
	if len(threads) == 0 {
		b.WriteString(Faint.Render("  (no open comments on this file)"))
		return b.String()
	}
	for _, t := range threads {
		b.WriteString(renderThread(t, expanded[t.ID]))
	}
	return b.String()
}

func hasAnyForFile(all []ado.Thread, path string) bool {
	for _, t := range all {
		if t.FilePath == path {
			return true
		}
	}
	return false
}

func renderThread(t ado.Thread, expand bool) string {
	if len(t.Comments) == 0 {
		return ""
	}
	var b strings.Builder
	first := t.Comments[0]
	loc := "-"
	if t.RightLine > 0 {
		loc = fmt.Sprintf("Ln %d", t.RightLine)
	} else if t.LeftLine > 0 {
		loc = fmt.Sprintf("Ln %d (left)", t.LeftLine)
	}
	bullet := "💬"
	if t.IsResolved() {
		bullet = "✓"
	}
	head := fmt.Sprintf("  %s %s  %s: %s",
		bullet, Faint.Render(loc), Header.Render(first.Author), squeezeOneLine(first.Content, 200))
	if t.IsResolved() {
		head = Faint.Render(head)
	}
	b.WriteString(head)
	if !expand && len(t.Comments) > 1 {
		b.WriteString(Faint.Render(fmt.Sprintf("  [%d more — enter to expand]", len(t.Comments)-1)))
	}
	b.WriteString("\n")
	if expand {
		for _, c := range t.Comments[1:] {
			b.WriteString(fmt.Sprintf("      %s: %s\n",
				Header.Render(c.Author),
				squeezeOneLine(c.Content, 1000)))
		}
	}
	return b.String()
}

// squeezeOneLine collapses internal whitespace and truncates so a comment
// preview stays on one row. Newlines become " ¶ " so structure is hinted.
func squeezeOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ¶ ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}

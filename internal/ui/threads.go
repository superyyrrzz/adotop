package ui

import (
	"fmt"
	"strings"

	"github.com/superyyrrzz/adotop/internal/ado"
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
	body := rendered
	if m.wrapDiff {
		body = wrapDiffLines(body, m.preview.vp.Width)
	}
	// Always rewrite the viewport. Earlier code skipped SetContent when
	// there were no comments to fold in and the diff was already loaded —
	// but that branch made the wrap toggle one-way: turning wrap OFF
	// would early-return with the still-wrapped content in place.
	// refreshPreview is only ever called from explicit user actions
	// (toggle, expand, refresh) or message arrivals (diffLoadedMsg,
	// threadsLoadedMsg) — never from j/k — so the rebuild cost is fine.
	m.preview.vp.SetContent(body + composed)
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
	return renderCommentsBlock(threads, m.expandedThread, m.showResolved, m.threads, f.Path, m.preview.vp.Width)
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
//
// width is the diff viewport width; it bounds how wide the wrapped body
// lines of expanded threads may go. Pass 0 to skip wrapping (long lines
// will extend off the right edge).
func renderCommentsBlock(threads []ado.Thread, expanded map[int]bool, showResolved bool, all []ado.Thread, path string, width int) string {
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
	header += ") "
	b.WriteString(Faint.Render(header))
	if hidden > 0 {
		// Pull the resolved-hidden affordance forward so the user
		// doesn't miss it in the surrounding faint header. Wait/yellow
		// reads as "attention" without escalating to error red.
		hint := fmt.Sprintf(" %d resolved hidden — press R to show ", hidden)
		b.WriteString(Wait.Bold(true).Render(hint))
	} else if showResolved {
		// Mirror affordance: when resolved are visible, remind the user
		// the toggle is on so they can flip back.
		b.WriteString(Approve.Render(" showing resolved — press R to hide "))
	}
	b.WriteString("\n")
	if len(threads) == 0 {
		b.WriteString(Faint.Render("  (no open comments on this file)"))
		return b.String()
	}
	for _, t := range threads {
		b.WriteString(renderThread(t, expanded[t.ID], width))
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

// renderThread emits one thread.
//
// Collapsed: a single line with the first comment squeezed to fit, plus
// a "[N more — enter to expand]" cue when there are replies. This is
// the default density-optimized form.
//
// Expanded: a header line with location + author, then the full first
// comment body wrapped to width, followed by each reply with author
// label + wrapped body. Newlines in the body survive as real newlines;
// long unwrapped lines are hard-wrapped at width.
//
// width is the viewport width. 0 disables wrapping (long lines extend
// off the right edge — only used by callers that don't have a width
// available, like tests).
func renderThread(t ado.Thread, expand bool, width int) string {
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

	if !expand {
		head := fmt.Sprintf("  %s %s  %s: %s",
			bullet, Faint.Render(loc), Header.Render(first.Author), squeezeCommentOneLine(first.Content, 200))
		if t.IsResolved() {
			head = Faint.Render(head)
		}
		b.WriteString(head)
		if len(t.Comments) > 1 {
			b.WriteString(Faint.Render(fmt.Sprintf("  [%d more — enter to expand]", len(t.Comments)-1)))
		}
		b.WriteString("\n")
		return b.String()
	}

	// Expanded form. Header carries location + author of the OP; the
	// body is rendered on its own indented lines so newlines and code
	// blocks stay readable. renderCommentBody handles HTML and
	// markdown bodies — ADO returns either, depending on author.
	const bodyIndent = "      "
	head := fmt.Sprintf("  %s %s  %s:", bullet, Faint.Render(loc), Header.Render(first.Author))
	if t.IsResolved() {
		head = Faint.Render(head)
	}
	b.WriteString(head)
	b.WriteString("\n")
	b.WriteString(renderCommentBody(first.Content, width, bodyIndent))
	for _, c := range t.Comments[1:] {
		b.WriteString(fmt.Sprintf("  ↳ %s:\n", Header.Render(c.Author)))
		b.WriteString(renderCommentBody(c.Content, width, bodyIndent))
	}
	return b.String()
}

// wrapBodyLines renders a comment body across multiple lines.
//
// Each source line (split on \n) is hard-wrapped at width-len(indent) so
// the visible right edge of the comment respects the viewport. width=0
// disables wrapping.
//
// Trailing newline is always emitted so the next thread starts cleanly.
func wrapBodyLines(body, indent string, width int) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return ""
	}
	max := width - len(indent)
	if width <= 0 || max < 10 {
		// No useful budget — emit one line per source line, no wrap.
		var b strings.Builder
		for _, ln := range strings.Split(body, "\n") {
			b.WriteString(indent)
			b.WriteString(ln)
			b.WriteString("\n")
		}
		return b.String()
	}
	var b strings.Builder
	for _, src := range strings.Split(body, "\n") {
		if src == "" {
			b.WriteString(indent)
			b.WriteString("\n")
			continue
		}
		for _, chunk := range hardWrap(src, max) {
			b.WriteString(indent)
			b.WriteString(chunk)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// hardWrap splits s into chunks of at most max runes. Word-breaks at
// spaces when one's available within max; otherwise falls back to a
// strict rune split so a long URL/identifier still wraps instead of
// blowing past the right edge.
func hardWrap(s string, max int) []string {
	if max <= 0 {
		return []string{s}
	}
	runes := []rune(s)
	if len(runes) <= max {
		return []string{s}
	}
	var out []string
	for len(runes) > 0 {
		if len(runes) <= max {
			out = append(out, string(runes))
			break
		}
		// Look for the last space in [0, max] to break on.
		breakAt := -1
		for i := max; i >= max/2; i-- {
			if runes[i] == ' ' {
				breakAt = i
				break
			}
		}
		if breakAt < 0 {
			breakAt = max
		}
		out = append(out, strings.TrimRight(string(runes[:breakAt]), " "))
		// Skip the space we broke on.
		next := breakAt
		for next < len(runes) && runes[next] == ' ' {
			next++
		}
		runes = runes[next:]
	}
	return out
}

// renderPRDiscussion formats PR-level threads (those not anchored to a
// specific file) for the detail header. Returns "" when there are no
// PR-level threads so the caller can skip the section entirely.
//
// Each thread is rendered as one line: status glyph + first author +
// squeezed first comment, mirroring renderThread but without the file
// location since these threads have none.
//
// We deliberately don't expose expand-all here — the header is fixed
// height; if you want to read the full thread, the future "thread
// view" pane (Task 3 in the plan) will host it.
func renderPRDiscussion(threads []ado.Thread) string {
	if len(threads) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(Faint.Render(fmt.Sprintf("─ Discussion  (%d) ", len(threads))))
	b.WriteString("\n")
	const maxRender = 6
	for i, t := range threads {
		if i >= maxRender {
			b.WriteString(Faint.Render(fmt.Sprintf("  … (%d more)", len(threads)-maxRender)))
			b.WriteString("\n")
			break
		}
		if len(t.Comments) == 0 {
			continue
		}
		first := t.Comments[0]
		bullet := "💬"
		if t.IsResolved() {
			bullet = "✓"
		}
		// Compact one-line form: " 💬 Alice: comment text…  [N more]"
		line := fmt.Sprintf("  %s %s: %s",
			bullet,
			Header.Render(first.Author),
			squeezeCommentOneLine(first.Content, 140))
		if len(t.Comments) > 1 {
			line += Faint.Render(fmt.Sprintf("  [%d more]", len(t.Comments)-1))
		}
		if t.IsResolved() {
			line = Faint.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}


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

package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// anchoredThreadsForFile returns threads attached to the file path with
// a right-side line anchor (RightLine > 0), applying showResolved.
// These render INLINE under their target diff line via spliceInlineComments.
func (m Model) anchoredThreadsForFile(path string) []ado.Thread {
	out := make([]ado.Thread, 0, len(m.threads))
	for _, t := range m.threads {
		if t.FilePath != path || t.RightLine <= 0 {
			continue
		}
		if !m.showResolved && t.IsResolved() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// unanchoredThreadsForFile returns threads attached to the file but
// without a right-side line anchor (RightLine == 0). These render in
// the file-level footer block since there's no specific line to
// attach them to. Includes left-side-only deletions and pre-line
// "general file feedback" threads.
func (m Model) unanchoredThreadsForFile(path string) []ado.Thread {
	out := make([]ado.Thread, 0, len(m.threads))
	for _, t := range m.threads {
		if t.FilePath != path || t.RightLine > 0 {
			continue
		}
		if !m.showResolved && t.IsResolved() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// selectedThreadIDForFile returns the ID of the thread currently under
// the per-file cursor, or 0 when there's no cursor (no threads, or the
// cursor index is out of range). Used by both inline and footer
// renderers to highlight "the one I'm on".
func (m Model) selectedThreadIDForFile(path string) int {
	threads := m.threadsForFile(path)
	if idx, ok := m.threadCursor[path]; ok && idx >= 0 && idx < len(threads) {
		return threads[idx].ID
	}
	return 0
}

// expandThreadsForFile unconditionally expands every visible thread on
// the given file. Returns true when at least one thread was visible
// (so the caller knows there's something worth scrolling to). Used by
// R-show: when the user reveals resolved comments, they want to see
// the details, not have to press Enter again to expand.
func (m Model) expandThreadsForFile(path string) (Model, bool) {
	threads := m.threadsForFile(path)
	if len(threads) == 0 {
		return m, false
	}
	for _, t := range threads {
		m.expandedThread[t.ID] = true
	}
	return m, true
}

// toggleThreadsForFile flips the expand state of every visible thread on
// the given file. If any thread is currently collapsed, this expands all of
// them; otherwise collapses all. Returns the new model and a bool that's
// true when the toggle EXPANDED (so the caller can auto-scroll into view).
func (m Model) toggleThreadsForFile(path string) (Model, bool) {
	threads := m.threadsForFile(path)
	if len(threads) == 0 {
		return m, false
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
	return m, anyCollapsed
}

// refreshPreview re-renders the diff preview viewport's content so that
// changes to thread filters / expansion show up immediately. No-op when
// no preview is loaded yet.
//
// The viewport is always rewritten — earlier code skipped SetContent
// when there were no comments to fold in, but that branch made the wrap
// toggle one-way: turning wrap OFF would early-return with the still-
// wrapped content in place. refreshPreview is only ever called from
// explicit user actions (toggle, expand, refresh) or message arrivals
// (diffLoadedMsg, threadsLoadedMsg) — never from j/k — so the rebuild
// cost is fine.
//
// Pipeline:
//  1. Get the colorized diff body from cache.
//  2. Splice anchored thread comment blocks INLINE under their target
//     diff line (reuses the raw uncolorized bytes to map line numbers).
//  3. Optionally wrap long diff lines if the user toggled wrap.
//  4. Append the file-level footer block for unanchored threads.
func (m Model) refreshPreview() Model {
	if m.previewKey == "" {
		return m
	}
	rendered, ok := m.previewCache.Rendered(m.previewKey)
	if !ok {
		return m
	}
	body := rendered
	// Inline-splice anchored threads. Pull raw bytes (Get) for the line-
	// number map; both cache entries are bytes for the same diff so they
	// stay in lockstep.
	if raw, rok := m.previewCache.Get(m.previewKey); rok && raw != nil {
		if f, fok := m.detail.SelectedFile(); fok {
			anchored := m.anchoredThreadsForFile(f.Path)
			if len(anchored) > 0 {
				selected := m.selectedThreadIDForFile(f.Path)
				lineNums := rightLineNumbers(raw)
				body = spliceInlineComments(body, lineNums, inlineThreadsByLine(anchored), m.expandedThread, m.preview.vp.Width, selected)
			}
		}
	}
	if m.wrapDiff {
		body = wrapDiffLines(body, m.preview.vp.Width)
	}
	commentsBlock := m.previewCommentsBlock()
	composed := composeDiffWithComments(nil, commentsBlock)
	m.preview.vp.SetContent(body + composed)
	return m
}

// scrollPreviewToComments puts the preview viewport at the start of the
// comments block — used after Enter expands a thread, and after R reveals
// resolved threads, so the user doesn't have to scroll past the diff to
// find what they just opened.
//
// The diff body and the comments block are concatenated into a single
// viewport string; the comments start at line `lipgloss.Height(body)`.
// We clamp to the viewport's max scroll position so a comments block
// that's shorter than one screen still scrolls to a sensible spot.
func (m Model) scrollPreviewToComments() Model {
	if m.previewKey == "" {
		return m
	}
	rendered, ok := m.previewCache.Rendered(m.previewKey)
	if !ok {
		return m
	}
	body := rendered
	if m.wrapDiff {
		body = wrapDiffLines(body, m.preview.vp.Width)
	}
	off := lipgloss.Height(body)
	m.preview.vp.SetYOffset(off)
	return m
}

// previewCommentsBlock returns the rendered "Comments" footer for the
// currently-selected file in the diff preview pane. Anchored threads
// render inline under their diff line (see refreshPreview), so the
// footer carries only unanchored threads — file-level feedback that
// has no specific line to attach to.
//
// Returns empty when there's nothing unanchored to show; the sticky
// resolved-comments band at the bottom of the pane already carries the
// "N hidden resolved" affordance, so we don't need a redundant footer
// header just to repeat it.
func (m Model) previewCommentsBlock() string {
	f, ok := m.detail.SelectedFile()
	if !ok {
		return ""
	}
	threads := m.unanchoredThreadsForFile(f.Path)
	if len(threads) == 0 {
		return ""
	}
	selected := m.selectedThreadIDForFile(f.Path)
	return renderCommentsBlockWithCursor(threads, m.expandedThread, m.showResolved, m.threads, f.Path, m.preview.vp.Width, selected)
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
	return renderCommentsBlockWithCursor(threads, expanded, showResolved, all, path, width, 0)
}

// renderCommentsBlockWithCursor renders the same block but draws a gutter
// mark on the thread whose ID == selectedID. selectedID==0 means nothing
// is selected and no gutter mark is drawn — preserves the prior look for
// callers that don't track a cursor (PR-level discussion, tests).
func renderCommentsBlockWithCursor(threads []ado.Thread, expanded map[int]bool, showResolved bool, all []ado.Thread, path string, width int, selectedID int) string {
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
		// Draw a 2-col gutter: ▎ + space when this thread is selected,
		// 2 spaces otherwise. Gutter eats into the available width so
		// the thread renderer doesn't overflow the pane.
		gutter := "  "
		if selectedID != 0 && t.ID == selectedID {
			gutter = Cursor.Render("▎") + " "
		}
		gutterW := 2
		threadW := width - gutterW
		if width == 0 {
			threadW = 0 // pass-through "no wrap"
		}
		rendered := renderThread(t, expanded[t.ID], threadW)
		// Prepend gutter to every line of the rendered thread so wrapped
		// content stays under the same column. renderThread always
		// emits a trailing \n, so the last split element is empty —
		// don't write the gutter for that one.
		lines := strings.Split(rendered, "\n")
		for i, line := range lines {
			if i == len(lines)-1 && line == "" {
				continue
			}
			b.WriteString(gutter)
			b.WriteString(line)
			b.WriteString("\n")
		}
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
			bullet, Faint.Render(loc), Header.Render(first.Author), squeezeCommentOneLine(sanitizeComment(first.Author, first.Content), 200))
		if t.IsResolved() {
			head = Faint.Render(head)
		}
		more := ""
		if len(t.Comments) > 1 {
			more = Faint.Render(fmt.Sprintf("  [%d more — enter to expand]", len(t.Comments)-1))
		}
		// Hard-fit to the pane width: if head + more would overflow,
		// shrink the more-hint to a compact form first (so the affordance
		// stays visible even in narrow panes), then truncate head with
		// ellipsis. Without this the viewport silently clips at its right
		// edge and the user sees no indication that there's hidden text
		// or that the thread has more comments.
		if width > 0 && lipgloss.Width(head)+lipgloss.Width(more) > width {
			if len(t.Comments) > 1 {
				more = Faint.Render(fmt.Sprintf(" +%d", len(t.Comments)-1))
			}
			budget := width - lipgloss.Width(more)
			if budget < 8 {
				budget = 8
			}
			if lipgloss.Width(head) > budget {
				head = ansi.Truncate(head, budget-1, "…")
			}
		}
		b.WriteString(head)
		b.WriteString(more)
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
	b.WriteString(renderCommentBody(sanitizeComment(first.Author, first.Content), width, bodyIndent))
	for _, c := range t.Comments[1:] {
		b.WriteString(fmt.Sprintf("  ↳ %s:\n", Header.Render(c.Author)))
		b.WriteString(renderCommentBody(sanitizeComment(c.Author, c.Content), width, bodyIndent))
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
			squeezeCommentOneLine(sanitizeComment(first.Author, first.Content), 140))
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

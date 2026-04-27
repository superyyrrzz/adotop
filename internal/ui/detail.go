package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ado"
)

type detailLoadedMsg struct {
	detail *ado.PRDetail
	err    error
}
type filesLoadedMsg struct {
	files []ado.FileChange
	err   error
}
type statusesLoadedMsg struct {
	statuses []ado.StatusCheck
	err      error
}

type DetailModel struct {
	keys      KeyMap
	summary   ado.PRSummary
	detail    *ado.PRDetail
	files     []ado.FileChange
	statuses  []ado.StatusCheck
	cursor    int
	loadErr   string
	width     int
	height    int
	paneWidth int // width of the left pane (set by parent); 0 = unknown
	paneHeight int
	myID      string
	// treeMemo caches the result of buildFileTree(m.files). The same
	// keypress fans out to neighborFile, DisplayNeighbors, and the
	// renderFilesBlock — without memoization we sort the file slice 3-4
	// times per j/k. Pointer so the cache survives value copies of
	// DetailModel; treeFor checks identity (len + first/last path) before
	// returning the memo.
	treeMemo *fileTreeMemo
}

// fileTreeMemo is the cached result of buildFileTree plus a cheap
// fingerprint we can compare to detect when m.files has changed.
type fileTreeMemo struct {
	rows      []fileTreeRow
	filesLen  int
	firstPath string
	lastPath  string
}

func NewDetail(keys KeyMap) DetailModel { return DetailModel{keys: keys} }

// SetMyID stores the current user descriptor so the reviewer panel can mark
// the caller's row with "(you)".
func (m DetailModel) SetMyID(id string) DetailModel {
	m.myID = id
	return m
}

// SetPaneSize tells the detail model the actual rendered pane size so
// it can wrap long description lines and budget the file list against
// the visible area (not the full terminal height).
func (m DetailModel) SetPaneSize(w, h int) DetailModel {
	m.paneWidth = w
	m.paneHeight = h
	return m
}

func (m DetailModel) SetSummary(s ado.PRSummary) DetailModel {
	m.summary = s
	m.detail = nil
	m.files = nil
	m.statuses = nil
	m.cursor = 0
	m.loadErr = ""
	m.treeMemo = nil
	return m
}

func (m DetailModel) SelectedFile() (ado.FileChange, bool) {
	if len(m.files) == 0 {
		return ado.FileChange{}, false
	}
	if m.cursor >= len(m.files) {
		return m.files[len(m.files)-1], true
	}
	return m.files[m.cursor], true
}

func (m DetailModel) Summary() ado.PRSummary { return m.summary }
func (m DetailModel) Detail() *ado.PRDetail  { return m.detail }

func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case detailLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else {
			m.detail = msg.detail
		}
	case filesLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		} else {
			m.files = msg.files
			// Pre-allocate the memo holder so subsequent value copies of
			// DetailModel share the cache via the pointer. fileTree
			// fills/refreshes it in place.
			m.treeMemo = &fileTreeMemo{filesLen: -1}
		}
	case statusesLoadedMsg:
		if msg.err == nil {
			m.statuses = msg.statuses
		}
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.keys.Down):
			m.cursor = m.neighborFile(+1)
		case keyMatches(msg, m.keys.Up):
			m.cursor = m.neighborFile(-1)
		}
	}
	return m, nil
}

// fileTree returns the cached buildFileTree result for m.files, building
// it on first call. The memo holder is pre-allocated whenever m.files
// changes (see filesLoadedMsg handler) so the *fileTreeMemo pointer is
// shared across value copies of DetailModel — fileTree can then mutate
// the holder in place and the new rows are visible to every copy that
// holds the same pointer.
//
// Cheap fingerprint (len + first/last path) guards against the
// (unlikely) case where the slice was mutated underneath us without
// going through a message.
func (m DetailModel) fileTree() []fileTreeRow {
	if m.treeMemo != nil &&
		m.treeMemo.filesLen == len(m.files) &&
		m.treeMemo.firstPath == firstPath(m.files) &&
		m.treeMemo.lastPath == lastPath(m.files) {
		return m.treeMemo.rows
	}
	rows := buildFileTree(m.files)
	if m.treeMemo != nil {
		// Mutate in place so the cached pointer (shared across value
		// copies) sees the freshly-built rows.
		m.treeMemo.rows = rows
		m.treeMemo.filesLen = len(m.files)
		m.treeMemo.firstPath = firstPath(m.files)
		m.treeMemo.lastPath = lastPath(m.files)
	}
	return rows
}

func firstPath(files []ado.FileChange) string {
	if len(files) == 0 {
		return ""
	}
	return files[0].Path
}

func lastPath(files []ado.FileChange) string {
	if len(files) == 0 {
		return ""
	}
	return files[len(files)-1].Path
}

// neighborFile returns the file index that comes after (delta=+1) or
// before (delta=-1) the current cursor when files are walked in
// **display order** (the sorted/grouped tree). This keeps j/k in sync
// with what the user sees instead of the original API order.
func (m DetailModel) neighborFile(delta int) int {
	if len(m.files) == 0 {
		return m.cursor
	}
	rows := m.fileTree()
	// Collect file rows in display order.
	order := make([]int, 0, len(m.files))
	for _, r := range rows {
		if !r.isDir {
			order = append(order, r.fileIdx)
		}
	}
	// Find current cursor position in display order.
	pos := 0
	for i, idx := range order {
		if idx == m.cursor {
			pos = i
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(order) {
		pos = len(order) - 1
	}
	return order[pos]
}

// DisplayNeighbors returns up to `radius` file indices on each side of
// the cursor, walked in **display order** (the sorted/grouped tree).
// Used by the preview prefetcher so it warms the bodies the user is
// most likely to navigate to next, not the API-order neighbors which
// rarely match what's on screen.
func (m DetailModel) DisplayNeighbors(radius int) []int {
	if radius <= 0 || len(m.files) == 0 {
		return nil
	}
	rows := m.fileTree()
	order := make([]int, 0, len(m.files))
	for _, r := range rows {
		if !r.isDir {
			order = append(order, r.fileIdx)
		}
	}
	pos := 0
	for i, idx := range order {
		if idx == m.cursor {
			pos = i
			break
		}
	}
	out := make([]int, 0, 2*radius)
	for d := 1; d <= radius; d++ {
		if pos-d >= 0 {
			out = append(out, order[pos-d])
		}
		if pos+d < len(order) {
			out = append(out, order[pos+d])
		}
	}
	return out
}

func (m DetailModel) View() string { return m.ViewWithFocus(true) }

func (m DetailModel) FilesHeader(focused bool) string {
	dot := "○ "
	if focused {
		dot = "● "
	}
	return Header.Render(dot + "Files")
}

// effectiveHeight returns the pane height if set, else the terminal height.
func (m DetailModel) effectiveHeight() int {
	if m.paneHeight > 0 {
		return m.paneHeight
	}
	return m.height
}

// effectiveWidth returns the pane width if set, else the terminal width.
func (m DetailModel) effectiveWidth() int {
	if m.paneWidth > 0 {
		return m.paneWidth
	}
	return m.width
}

func (m DetailModel) ViewWithFocus(focused bool) string {
	header := m.renderHeader(focused)
	// Pre-wrap the header at the actual pane width BEFORE measuring.
	// Long lines (long repo+branch line, dense reviewer list, single
	// long description line) report as 1 source line but render as
	// several visual rows once the parent pane wraps them. If we don't
	// wrap-then-measure, the file-list budget is wrong and the total
	// view exceeds bodyHeight, causing the terminal to scroll and clip
	// the top of the pane.
	if w := m.effectiveWidth(); w > 0 {
		header = lipgloss.NewStyle().Width(w).Render(header)
	}
	header = m.clampHeader(header)
	filesBlock := m.renderFilesBlock(focused, lipgloss.Height(header))
	return header + filesBlock
}

// clampHeader truncates the rendered header to the budget computed from
// pane height. Description lines are the first thing trimmed (they're
// the bulkiest); the title / repo / reviewer / files-sub-header chrome
// is preserved. An ellipsis line marks the truncation.
func (m DetailModel) clampHeader(header string) string {
	h := m.effectiveHeight()
	if h <= 0 {
		return header
	}
	// Reserve at least 6 rows for the file list + 4 for footer/padding.
	maxHeader := h - 10
	if maxHeader < 6 {
		maxHeader = 6
	}
	lines := strings.Split(header, "\n")
	if len(lines) <= maxHeader {
		return header
	}
	keep := maxHeader - 3
	if keep < 1 {
		keep = 1
	}
	tail := lines[len(lines)-2:]
	out := append([]string{}, lines[:keep]...)
	out = append(out, Faint.Render("  … (description truncated; press o to open in browser)"))
	out = append(out, tail...)
	return strings.Join(out, "\n")
}

// renderHeader returns the always-visible top section: title+badge,
// repo/author/branch line, reviewers, description, work items, statuses,
// and the "● Files" sub-header. This is rendered first and never
// truncated, so the user always knows which PR they're looking at.
func (m DetailModel) renderHeader(focused bool) string {
	var b strings.Builder
	s := m.summary
	b.WriteString(Header.Render(fmt.Sprintf("PR #%d  %s", s.ID, s.Title)))
	if badge := prStateBadge(s); badge != "" {
		b.WriteString("  " + badge)
	}
	b.WriteString("\n")
	repo := s.Repo
	if repo == "" {
		repo = "(unknown repo)"
	}
	b.WriteString(Faint.Render(fmt.Sprintf("%s  ·  %s  ·  %s → %s", repo, s.Author, s.SourceBranch, s.TargetBranch)))
	b.WriteString("\n")
	b.WriteString(reviewerPanel(s, m.myID))
	b.WriteString("\n\n")

	if m.detail != nil {
		desc := strings.TrimSpace(m.detail.DescriptionMD)
		if desc != "" {
			lines := wrapLines(strings.Split(desc, "\n"), m.effectiveWidth())
			descCap := m.descCap()
			if len(lines) > descCap {
				lines = append(lines[:descCap], Faint.Render(fmt.Sprintf("… (%d more lines)", len(lines)-descCap)))
			}
			b.WriteString(strings.Join(lines, "\n"))
			b.WriteString("\n\n")
		}
		if len(m.detail.WorkItemRefs) > 0 {
			b.WriteString("Work items: ")
			for i, w := range m.detail.WorkItemRefs {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("#" + w.ID)
			}
			b.WriteString("\n")
		}
	}
	if block := renderStatusBlock(m.statuses); block != "" {
		b.WriteString(block)
	}
	b.WriteString("\n" + m.FilesHeader(focused) + "\n")
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render(m.loadErr) + "\n")
	}
	return b.String()
}

// renderFilesBlock renders the (possibly windowed) file tree, sized to
// whatever vertical space remains after the header. headerLines is the
// measured height of renderHeader; we subtract it from m.height (with a
// small safety margin) so the file list never pushes the header off the
// top of the pane.
func (m DetailModel) renderFilesBlock(_ bool, headerLines int) string {
	var b strings.Builder
	rows := m.fileTree()
	cursorRow := rowIndexForFile(rows, m.cursor)
	start, end := m.rowWindowFitting(rows, cursorRow, headerLines)
	for i := start; i < end; i++ {
		r := rows[i]
		if r.isDir {
			b.WriteString(Faint.Render(strings.Repeat("  ", r.depth)+r.label+"/") + "\n")
			continue
		}
		marker := "  "
		if r.fileIdx == m.cursor {
			marker = "▸ "
		}
		f := m.files[r.fileIdx]
		line := fmt.Sprintf("%s%s%s  %s", strings.Repeat("  ", r.depth), marker, f.ChangeType, r.label)
		if r.fileIdx == m.cursor {
			line = Selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if start > 0 || end < len(rows) {
		b.WriteString(Faint.Render(fmt.Sprintf("  [%d-%d of %d]\n", start+1, end, len(rows))))
	}
	return b.String()
}

func (m DetailModel) descCap() int {
	h := m.effectiveHeight()
	if h <= 0 {
		return 8
	}
	c := h / 4
	if c < 4 {
		c = 4
	}
	if c > 20 {
		c = 20
	}
	return c
}

func (m DetailModel) fileWindow() (int, int) {
	// Deprecated: kept for tests; see rowWindow for the live tree pane.
	total := len(m.files)
	if total == 0 {
		return 0, 0
	}
	return 0, total
}

// wrapLines breaks each input line at width columns (rune-aware-ish; we
// approximate with byte length since most ADO descriptions are ASCII).
// This is what the lipgloss left-pane renderer will do anyway, so we
// need to count the wrapped lines accurately when budgeting the file
// list.
func wrapLines(in []string, width int) []string {
	if width <= 0 {
		return in
	}
	var out []string
	for _, line := range in {
		// Strip ANSI for measurement? Description is plain markdown, so
		// just count runes.
		runes := []rune(line)
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		out = append(out, string(runes))
	}
	return out
}

// fileTreeRow is one rendered line in the file pane: either a directory
// header (isDir=true, fileIdx=-1) or a file row (isDir=false, fileIdx is
// the index into m.files). depth is the indent level.
type fileTreeRow struct {
	isDir   bool
	depth   int
	label   string
	fileIdx int
}

// buildFileTree groups m.files by their parent directory and returns a flat
// slice of tree rows. Common prefix folders that contain only one child are
// collapsed into the child's label so the tree doesn't waste rows on
// single-folder chains (e.g. "internal/ui/" stays one header instead of
// "internal" → "ui").
func buildFileTree(files []ado.FileChange) []fileTreeRow {
	if len(files) == 0 {
		return nil
	}
	type entry struct {
		path string
		idx  int
	}
	entries := make([]entry, len(files))
	for i, f := range files {
		p := strings.TrimPrefix(f.Path, "/")
		if p == "" {
			// Defensive: never render a blank file row. If the API
			// returned an empty path (we've seen this for some delete
			// entries when sourceServerItem is also missing), surface
			// it as <unknown> so the user can at least see the
			// changeType column.
			p = "<unknown>"
		}
		entries[i] = entry{path: p, idx: i}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })

	var rows []fileTreeRow
	prevDirs := []string{}
	for _, e := range entries {
		parts := strings.Split(e.path, "/")
		dirs := parts[:len(parts)-1]
		base := parts[len(parts)-1]
		// Find the common prefix length with the previously emitted dirs.
		common := 0
		for common < len(dirs) && common < len(prevDirs) && dirs[common] == prevDirs[common] {
			common++
		}
		// Emit any new directory segments as collapsed headers.
		for i := common; i < len(dirs); i++ {
			rows = append(rows, fileTreeRow{
				isDir:   true,
				depth:   i,
				label:   dirs[i],
				fileIdx: -1,
			})
		}
		rows = append(rows, fileTreeRow{
			isDir:   false,
			depth:   len(dirs),
			label:   base,
			fileIdx: e.idx,
		})
		prevDirs = dirs
	}
	return rows
}

// rowIndexForFile returns the row index whose fileIdx matches the given
// file index, or -1 if not found.
func rowIndexForFile(rows []fileTreeRow, fileIdx int) int {
	for i, r := range rows {
		if !r.isDir && r.fileIdx == fileIdx {
			return i
		}
	}
	return -1
}

// rowWindow scrolls the rendered tree so the cursor row stays visible,
// keeping the parent directory header above the cursor in view when
// possible.
func (m DetailModel) rowWindow(rows []fileTreeRow, cursorRow int) (int, int) {
	return m.rowWindowFitting(rows, cursorRow, 0)
}

// rowWindowFitting is like rowWindow but reserves headerLines for the
// renderHeader block above. This guarantees the header stays on screen
// even when the file list would otherwise overflow.
//
// The file list is also capped at roughly half the pane's height so the
// PR description above doesn't get squeezed when there are many files.
func (m DetailModel) rowWindowFitting(rows []fileTreeRow, cursorRow, headerLines int) (int, int) {
	total := len(rows)
	if total == 0 {
		return 0, 0
	}
	h := m.effectiveHeight()
	if h <= 0 {
		return 0, total
	}
	// Reserve headerLines for the always-visible top section, plus a
	// small footer/scrollbar/padding margin (4 rows).
	cap := h - headerLines - 4
	if headerLines == 0 {
		// Legacy path (rowWindow caller): use the old descCap estimate.
		cap = h - m.descCap() - 12
	} else {
		// Cap the file list at ~half the pane height so the description
		// (rendered inside the header block above) stays readable. We
		// never go below 6 rows so a tiny pane still shows a few files.
		half := h / 2
		if half < 6 {
			half = 6
		}
		if cap > half {
			cap = half
		}
	}
	if cap < 3 {
		cap = 3
	}
	if cap >= total {
		return 0, total
	}
	if cursorRow < 0 {
		cursorRow = 0
	}
	start := cursorRow - cap/2
	if start < 0 {
		start = 0
	}
	if start+cap > total {
		start = total - cap
	}
	if cursorRow < start {
		start = cursorRow
	}
	if cursorRow >= start+cap {
		start = cursorRow - cap + 1
	}
	return start, start + cap
}

// renderStatusBlock summarizes CI statuses as a one-line counts row
// followed by a short list of the checks the user actually needs to act
// on (failing, errored, in-progress/pending, unknown). Succeeded checks
// are never named individually — an all-green PR collapses to a single
// "Status: 12 ✓" row. Capped at maxDetail rows; overflow is summarized.
//
// Returns "" if there are no statuses, so the header omits the section.
func renderStatusBlock(sts []ado.StatusCheck) string {
	if len(sts) == 0 {
		return ""
	}
	const maxDetail = 8
	var ok, fail, pend, other int
	type interesting struct {
		state, ctx string
	}
	var picks []interesting
	for _, s := range sts {
		switch strings.ToLower(s.State) {
		case "succeeded":
			ok++
		case "failed", "error":
			fail++
			picks = append(picks, interesting{s.State, s.Context})
		case "pending":
			pend++
			picks = append(picks, interesting{s.State, s.Context})
		default:
			other++
			picks = append(picks, interesting{s.State, s.Context})
		}
	}
	var b strings.Builder
	b.WriteString("Status:")
	parts := []struct {
		n     int
		state string
	}{
		{ok, "succeeded"},
		{fail, "failed"},
		{pend, "pending"},
		{other, ""},
	}
	for _, p := range parts {
		if p.n == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf(" %d %s ", p.n, statusGlyph(p.state)))
	}
	b.WriteString("\n")
	for i, p := range picks {
		if i >= maxDetail {
			b.WriteString(Faint.Render(fmt.Sprintf("  … (%d more)", len(picks)-maxDetail)))
			b.WriteString("\n")
			break
		}
		b.WriteString("  " + statusGlyph(strings.ToLower(p.state)) + " " + p.ctx + "\n")
	}
	return b.String()
}

func statusGlyph(state string) string {
	switch state {
	case "succeeded":
		return Approve.Render("✓")
	case "failed", "error":
		return Reject.Render("✗")
	case "pending":
		return Wait.Render("⏳")
	default:
		return None.Render("·")
	}
}

// prStateBadgeCompact returns a short label and the lipgloss style to
// color it with — separated so the caller can pad the plain text to a
// fixed column width before applying ANSI escapes (runewidth doesn't
// strip ANSI). Suitable for tabular list views.
func prStateBadgeCompact(s ado.PRSummary) (string, lipgloss.Style) {
	switch strings.ToLower(s.Status) {
	case "completed":
		return "MERGED", Approve
	case "abandoned":
		return "ABANDON", Reject
	}
	if s.Draft {
		return "DRAFT", Wait
	}
	switch strings.ToLower(s.MergeStatus) {
	case "conflicts":
		return "CONFLICT", Reject
	case "rejectedbypolicy":
		return "BLOCKED", Reject
	case "queued":
		return "MERGING", Wait
	case "failure":
		return "FAILED", Reject
	case "notset":
		return "CHECKING", Wait
	case "succeeded":
		return "OPEN", Approve
	}
	if strings.ToLower(s.Status) == "active" || s.Status == "" {
		return "OPEN", Faint
	}
	return strings.ToUpper(s.Status), Faint
}

// prStateBadge returns a short, colorized label summarizing the PR's
// lifecycle + draft + mergeability. Always returns a non-empty badge so
// the user can see the state at a glance.
//
// Precedence: lifecycle (completed/abandoned) > draft > merge issue > active.
func prStateBadge(s ado.PRSummary) string {
	switch strings.ToLower(s.Status) {
	case "completed":
		return Approve.Render("[MERGED]")
	case "abandoned":
		return Reject.Render("[ABANDONED]")
	}
	if s.Draft {
		return Wait.Render("[DRAFT]")
	}
	switch strings.ToLower(s.MergeStatus) {
	case "conflicts":
		return Reject.Render("[CONFLICTS]")
	case "rejectedbypolicy":
		return Reject.Render("[POLICY-BLOCKED]")
	case "queued":
		return Wait.Render("[MERGING]")
	case "failure":
		return Reject.Render("[MERGE-FAILED]")
	case "notset":
		return Wait.Render("[CHECKING]")
	case "succeeded":
		return Approve.Render("[READY]")
	}
	// Fall-through: PR is active but server hasn't reported a merge status
	// yet (rare). Show "ACTIVE" rather than nothing.
	if strings.ToLower(s.Status) == "active" || s.Status == "" {
		return Faint.Render("[ACTIVE]")
	}
	return Faint.Render("[" + strings.ToUpper(s.Status) + "]")
}

// voteLabel maps an ADO reviewer vote integer to a (glyph, text) pair.
// ADO vote scale: 10 approved, 5 approved-with-suggestions, 0 no vote,
// -5 waiting for author, -10 rejected.
func voteLabel(vote int) (string, string) {
	switch {
	case vote >= 10:
		return Approve.Render("✓"), Approve.Render("Approved")
	case vote >= 5:
		return Approve.Render("✓~"), Approve.Render("Approved w/ suggestions")
	case vote <= -10:
		return Reject.Render("✗"), Reject.Render("Rejected")
	case vote <= -5:
		return Wait.Render("⏳"), Wait.Render("Waiting for author")
	default:
		return None.Render("·"), Faint.Render("No vote")
	}
}

// reviewerPanel renders a one-line "My vote" badge plus a compact list of
// reviewers and their votes. This makes the post-approve state visible
// without forcing the user to read the footer flash. `myID` is the caller's
// descriptor, used to tag the caller's own reviewer row with "(you)".
func reviewerPanel(s ado.PRSummary, myID string) string {
	var b strings.Builder
	myGlyph, myText := voteLabel(s.MyVote)
	b.WriteString("My vote: " + myGlyph + " " + myText)
	if len(s.Reviewers) == 0 {
		return b.String()
	}
	b.WriteString("   ")
	b.WriteString(Faint.Render("Reviewers: "))
	for i, r := range s.Reviewers {
		if i > 0 {
			b.WriteString(Faint.Render(", "))
		}
		g, _ := voteLabel(r.Vote)
		name := r.DisplayName
		if r.IsRequired {
			name = "*" + name
		}
		if myID != "" && r.ID == myID {
			name += " (you)"
			name = Header.Render(name)
		}
		b.WriteString(g + " " + name)
	}
	return b.String()
}

package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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
	keys     KeyMap
	summary  ado.PRSummary
	detail   *ado.PRDetail
	files    []ado.FileChange
	statuses []ado.StatusCheck
	cursor   int
	loadErr  string
	width    int
	height   int
	myID     string
}

func NewDetail(keys KeyMap) DetailModel { return DetailModel{keys: keys} }

// SetMyID stores the current user descriptor so the reviewer panel can mark
// the caller's row with "(you)".
func (m DetailModel) SetMyID(id string) DetailModel {
	m.myID = id
	return m
}

func (m DetailModel) SetSummary(s ado.PRSummary) DetailModel {
	m.summary = s
	m.detail = nil
	m.files = nil
	m.statuses = nil
	m.cursor = 0
	m.loadErr = ""
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
		}
	case statusesLoadedMsg:
		if msg.err == nil {
			m.statuses = msg.statuses
		}
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.keys.Down):
			if m.cursor < len(m.files)-1 {
				m.cursor++
			}
		case keyMatches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m DetailModel) View() string { return m.ViewWithFocus(true) }

func (m DetailModel) FilesHeader(focused bool) string {
	dot := "○ "
	if focused {
		dot = "● "
	}
	return Header.Render(dot + "Files")
}

func (m DetailModel) ViewWithFocus(focused bool) string {
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
			lines := strings.Split(desc, "\n")
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
	if len(m.statuses) > 0 {
		b.WriteString("Status: ")
		for i, st := range m.statuses {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(st.Context + " " + statusGlyph(st.State))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + m.FilesHeader(focused) + "\n")
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render(m.loadErr) + "\n")
	}
	rows := buildFileTree(m.files)
	cursorRow := rowIndexForFile(rows, m.cursor)
	start, end := m.rowWindow(rows, cursorRow)
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
	if m.height <= 0 {
		return 8
	}
	// Grow with terminal height: roughly a third of the screen, with a
	// generous ceiling so tall windows don't waste vertical space.
	c := m.height / 3
	if c < 4 {
		c = 4
	}
	if c > 40 {
		c = 40
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
		entries[i] = entry{path: strings.TrimPrefix(f.Path, "/"), idx: i}
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
	total := len(rows)
	if total == 0 {
		return 0, 0
	}
	if m.height <= 0 {
		return 0, total
	}
	cap := m.height - m.descCap() - 12
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

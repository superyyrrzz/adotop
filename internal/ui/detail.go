package ui

import (
	"fmt"
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
	start, end := m.fileWindow()
	for i := start; i < end; i++ {
		f := m.files[i]
		marker := "  "
		if i == m.cursor {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%s  %s", marker, f.ChangeType, f.Path)
		if i == m.cursor {
			line = Selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if start > 0 || end < len(m.files) {
		b.WriteString(Faint.Render(fmt.Sprintf("  [%d-%d of %d]\n", start+1, end, len(m.files))))
	}
	return b.String()
}

func (m DetailModel) descCap() int {
	if m.height <= 0 {
		return 8
	}
	c := m.height / 4
	if c < 4 {
		c = 4
	}
	if c > 12 {
		c = 12
	}
	return c
}

func (m DetailModel) fileWindow() (int, int) {
	total := len(m.files)
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
	start := m.cursor - cap/2
	if start < 0 {
		start = 0
	}
	if start+cap > total {
		start = total - cap
	}
	if m.cursor < start {
		start = m.cursor
	}
	if m.cursor >= start+cap {
		start = m.cursor - cap + 1
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

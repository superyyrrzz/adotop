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
}

func NewDetail(keys KeyMap) DetailModel { return DetailModel{keys: keys} }

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

func (m DetailModel) View() string {
	var b strings.Builder
	s := m.summary
	fmt.Fprintf(&b, "PR #%d  %s\n", s.ID, s.Title)
	b.WriteString(Faint.Render(fmt.Sprintf("%s  %s → %s", s.Author, s.SourceBranch, s.TargetBranch)))
	b.WriteString("\n\n")

	if m.detail != nil {
		b.WriteString(m.detail.DescriptionMD)
		b.WriteString("\n\n")
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
	b.WriteString("\n── Files ─────────────────────────────────\n")
	if m.loadErr != "" {
		b.WriteString(ErrLine.Render(m.loadErr) + "\n")
	}
	for i, f := range m.files {
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

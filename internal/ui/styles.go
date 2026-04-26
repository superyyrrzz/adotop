package ui

import "github.com/charmbracelet/lipgloss"

var (
	Header   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	Footer   = lipgloss.NewStyle().Faint(true)
	ErrLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	TabOn    = lipgloss.NewStyle().Bold(true).Underline(true)
	TabOff   = lipgloss.NewStyle().Faint(true)
	Selected = lipgloss.NewStyle().Reverse(true)
	Faint    = lipgloss.NewStyle().Faint(true)
	HelpBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	Approve  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	Reject   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	Wait     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	None     = lipgloss.NewStyle().Faint(true)
)

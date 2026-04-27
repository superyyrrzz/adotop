package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ui/theme"
)

// Package-level styles for adotop. Defaulted to a Mocha-derived set so
// any code path that touches them before New() runs (tests, etc.) still
// gets a sensible render. applyStyles overwrites them at app startup
// once the active theme is known.
//
// We keep these as package vars (rather than threading a Styles struct
// through every model) because there's exactly one theme per process —
// the indirection isn't worth ~60 callsites of plumbing.
var (
	Header   lipgloss.Style
	Footer   lipgloss.Style
	ErrLine  lipgloss.Style
	TabOn    lipgloss.Style
	TabOff   lipgloss.Style
	Selected lipgloss.Style
	Faint    lipgloss.Style
	HelpBox  lipgloss.Style
	Approve  lipgloss.Style
	Reject   lipgloss.Style
	Wait     lipgloss.Style
	None     lipgloss.Style

	// PaneBorder is the right-pane frame color in app.go. Exposed as a
	// raw color (not Style) because lipgloss.NewStyle().BorderForeground
	// wants a TerminalColor.
	PaneBorder lipgloss.Color
)

func init() {
	applyStyles(theme.New("dark"))
}

// applyStyles repopulates the package-level styles from a theme. Safe
// to call multiple times; the last call wins. Tests can pin a theme by
// calling applyStyles(theme.New("dark")) in setup.
func applyStyles(t theme.Theme) {
	s := NewStyles(t)
	Header = s.Header
	Footer = s.Footer
	ErrLine = s.ErrLine
	TabOn = s.TabOn
	TabOff = s.TabOff
	Selected = s.Selected
	Faint = s.Faint
	HelpBox = s.HelpBox
	Approve = s.Approve
	Reject = s.Reject
	Wait = s.Wait
	None = s.None
	PaneBorder = s.PaneBorder
}

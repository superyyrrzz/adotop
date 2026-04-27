package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/renzeyu/adotop/internal/ui/theme"
)

// Styles is the full set of lipgloss.Style values derived from a Theme.
// Constructed once at app start (see New()) and threaded into every
// component that renders. Keeping these as concrete fields (not
// functions) lets callers chain .Render(...) without per-keypress work.
type Styles struct {
	Header   lipgloss.Style
	Faint    lipgloss.Style
	ErrLine  lipgloss.Style
	TabOn    lipgloss.Style
	TabOff   lipgloss.Style
	Selected lipgloss.Style
	HelpBox  lipgloss.Style
	Approve  lipgloss.Style
	Reject   lipgloss.Style
	Wait     lipgloss.Style
	None     lipgloss.Style
	Footer   lipgloss.Style

	// Border color used by the right-pane frame in app.go.
	PaneBorder lipgloss.Color
}

// NewStyles builds the style set for a given theme. Add new roles here
// when the UI grows new visual concepts; do NOT scatter
// lipgloss.NewStyle() calls in render code.
func NewStyles(t theme.Theme) Styles {
	return Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.Blue),
		Faint:    lipgloss.NewStyle().Foreground(t.Subtext),
		ErrLine:  lipgloss.NewStyle().Foreground(t.Red),
		TabOn:    lipgloss.NewStyle().Bold(true).Underline(true).Foreground(t.Mauve),
		TabOff:   lipgloss.NewStyle().Foreground(t.Subtext),
		Selected: lipgloss.NewStyle().Reverse(true),
		HelpBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Overlay).
			Padding(0, 1),
		Approve: lipgloss.NewStyle().Foreground(t.Green),
		Reject:  lipgloss.NewStyle().Foreground(t.Red),
		Wait:    lipgloss.NewStyle().Foreground(t.Yellow),
		None:    lipgloss.NewStyle().Foreground(t.Subtext),
		Footer:  lipgloss.NewStyle().Foreground(t.Subtext),

		PaneBorder: t.Overlay,
	}
}

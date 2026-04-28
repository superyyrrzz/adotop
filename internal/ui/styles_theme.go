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

	// Pill* styles render filled-background badges (PR state, etc.).
	// Background is the dominant visual; foreground is forced to
	// black/white so contrast survives any user terminal palette.
	// Padding 0,1 gives one-space horizontal breathing room without
	// adding line height.
	PillGood    lipgloss.Style // OPEN, READY, MERGED -> green-ish bg
	PillBad     lipgloss.Style // CONFLICT, REJECTED, FAILED, BLOCKED -> red bg
	PillWarn    lipgloss.Style // DRAFT, MERGING, CHECKING -> yellow bg
	PillInfo    lipgloss.Style // CHECKING/info-class -> cyan bg
	PillNeutral lipgloss.Style // ABANDONED, unknown -> faint bg
	PillDone    lipgloss.Style // MERGED specifically (distinct from OPEN)
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

		// Pills: bg color does the work. fg pinned to black on bright
		// pills so legibility holds across user terminal palettes; the
		// neutral pill uses overlay-on-text since both ends are quiet.
		PillGood:    pillStyle(t.Green, lipgloss.Color("0")),
		PillBad:     pillStyle(t.Red, lipgloss.Color("15")),
		PillWarn:    pillStyle(t.Yellow, lipgloss.Color("0")),
		PillInfo:    pillStyle(t.Sky, lipgloss.Color("0")),
		PillNeutral: pillStyle(t.Overlay, t.Text),
		PillDone:    pillStyle(t.Mauve, lipgloss.Color("0")),
	}
}

// pillStyle is the shared recipe for chip-style badges: bold, padded,
// solid background. Bold helps the label punch through the bg even when
// the terminal renders ANSI bright codes faintly.
func pillStyle(bg, fg lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Padding(0, 1).Background(bg).Foreground(fg)
}

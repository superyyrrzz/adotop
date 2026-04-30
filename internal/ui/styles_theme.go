package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ui/theme"
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
	// Cursor is rendered on the leftmost column of the highlighted row
	// in the PR list (and any future list). Foreground-only so it doesn't
	// fight with chip backgrounds the way Selected.Reverse did.
	Cursor   lipgloss.Style
	HelpBox  lipgloss.Style
	// ModalBox is the floating loading/info overlay. Differs from
	// HelpBox in that it carries an explicit Background so it reads
	// as opaque against the screen content visible behind/around it.
	ModalBox lipgloss.Style
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
//
// Color discipline (single-accent rule):
//
// The theme's Accent role (Mauve in Catppuccin) is reserved for the
// "you are here / this is selected" vocabulary, and ONLY for that.
// Five legitimate uses today:
//
//  1. Active breadcrumb crumb in the topbar (renderTopbar)
//  2. Active tab pill in the tab strip (TabOn)
//  3. List cursor stripe `▌` in the PR list (Cursor)
//  4. Focused pane border in the detail screen (focusedPaneBorder)
//  5. PillDone for the MERGED PR-state chip and the vote-menu mode pill
//
// New Accent usage anywhere else dilutes the signal. Prefer Header
// (bold Identifier) for labels, Faint (Subtext) for secondary text,
// and the semantic pill styles (PillGood / PillBad / PillWarn /
// PillInfo) for state.
func NewStyles(t theme.Theme) Styles {
	return Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.Identifier),
		Faint:    lipgloss.NewStyle().Foreground(t.Subtext),
		ErrLine:  lipgloss.NewStyle().Foreground(t.Danger),
		TabOn:    pillStyle(t.Accent, t.PillFgOnSaturated),
		TabOff:   lipgloss.NewStyle().Foreground(t.Subtext),
		Selected: lipgloss.NewStyle().Reverse(true),
		Cursor:   lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		HelpBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Overlay).
			Padding(0, 1),
		ModalBox: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(t.Accent).
			Background(t.Surface).
			Foreground(t.Text).
			Padding(1, 4),
		Approve: lipgloss.NewStyle().Foreground(t.Success),
		Reject:  lipgloss.NewStyle().Foreground(t.Danger),
		Wait:    lipgloss.NewStyle().Foreground(t.Attention),
		None:    lipgloss.NewStyle().Foreground(t.Subtext),
		Footer:  lipgloss.NewStyle().Foreground(t.Subtext),

		PaneBorder: t.Overlay,

		// Pills: bg color does the work. fg comes from the theme so each
		// variant picks the foreground that actually contrasts on its
		// own palette (see Theme.PillFgOnSaturated). The neutral pill
		// has its own bg/fg pair so it doesn't degrade to grey-on-grey.
		PillGood:    pillStyle(t.Success, t.PillFgOnSaturated),
		PillBad:     pillStyle(t.Danger, t.PillFgOnSaturated),
		PillWarn:    pillStyle(t.Attention, t.PillFgOnLight),
		PillInfo:    pillStyle(t.Info, t.PillFgOnSaturated),
		PillNeutral: pillStyle(t.PillNeutralBg, t.PillNeutralFg),
		PillDone:    pillStyle(t.Accent, t.PillFgOnSaturated),
	}
}

// pillStyle is the shared recipe for chip-style badges: bold, padded,
// solid background. Bold helps the label punch through the bg even when
// the terminal renders ANSI bright codes faintly.
func pillStyle(bg, fg lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Padding(0, 1).Background(bg).Foreground(fg)
}

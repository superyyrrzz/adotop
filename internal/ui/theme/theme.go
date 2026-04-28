// Package theme provides Catppuccin-based palettes for adotop.
package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Theme holds named colors for the whole TUI. Add new fields here when a
// new semantic role appears; never reach for raw hex in the ui package.
type Theme struct {
	// Base surfaces
	Base    lipgloss.Color // app background
	Surface lipgloss.Color // panes, boxes
	Overlay lipgloss.Color // borders, dividers
	Text    lipgloss.Color // body
	Subtext lipgloss.Color // faint text

	// Accents (semantic uses noted on each callsite, not here)
	Blue   lipgloss.Color
	Mauve  lipgloss.Color
	Green  lipgloss.Color
	Yellow lipgloss.Color
	Sky    lipgloss.Color
	Peach  lipgloss.Color
	Red    lipgloss.Color
	Pink   lipgloss.Color

	// Diff line backgrounds. Kept on the palette so light/dark variants
	// can pick contrast that actually works against the active Base.
	DiffAddBg lipgloss.Color
	DiffDelBg lipgloss.Color

	// PillNeutralBg / PillNeutralFg: the muted "no-status" chip color.
	// Lives on the theme rather than being derived from Overlay+Text
	// because that combination came out grey-on-grey on every variant.
	// Each theme picks a pair with a measurable contrast ratio.
	PillNeutralBg lipgloss.Color
	PillNeutralFg lipgloss.Color

	// PillFgOnSaturated is the foreground used on top of the saturated
	// pill backgrounds (Green/Yellow/Sky/Mauve/Red). On truecolor themes
	// this is black — the bright bg carries the contrast. On the system
	// (ANSI base-16) theme it's bright white, because terminal palettes
	// often pair "0" (black) too closely with their default background
	// for the chip text to register.
	PillFgOnSaturated lipgloss.Color
}

// New resolves a Theme from an explicit override or terminal detection.
//
// override semantics:
//
//	"system"        -> ANSI base-16 (uses your terminal's palette; default)
//	"dark"          -> Catppuccin Mocha
//	"light"         -> Catppuccin Latte
//	"auto"          -> Mocha or Latte via termenv.HasDarkBackground()
//	"" (unset)      -> system (the original look)
//	anything else   -> system (defensive: unknown values shouldn't crash)
func New(override string) Theme {
	switch override {
	case "system":
		return newSystem()
	case "dark":
		return newCatppuccinMocha()
	case "light":
		return newCatppuccinLatte()
	case "auto":
		if termenv.HasDarkBackground() {
			return newCatppuccinMocha()
		}
		return newCatppuccinLatte()
	case "":
		return newSystem()
	default:
		return newSystem()
	}
}

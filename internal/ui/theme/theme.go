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
}

// New resolves a Theme from an explicit override or terminal detection.
//
// override semantics:
//
//	"dark"          -> Mocha
//	"light"         -> Latte
//	"auto", ""      -> termenv.HasDarkBackground()
//	anything else   -> Mocha (defensive: unknown values shouldn't crash)
func New(override string) Theme {
	switch override {
	case "dark":
		return newCatppuccinMocha()
	case "light":
		return newCatppuccinLatte()
	case "auto", "":
		if termenv.HasDarkBackground() {
			return newCatppuccinMocha()
		}
		return newCatppuccinLatte()
	default:
		return newCatppuccinMocha()
	}
}

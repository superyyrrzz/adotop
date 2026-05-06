package theme

import "github.com/charmbracelet/lipgloss"

func newCatppuccinMocha() Theme {
	return Theme{
		Base:    lipgloss.Color("#1e1e2e"),
		Surface: lipgloss.Color("#313244"),
		Overlay: lipgloss.Color("#45475a"),
		Text:    lipgloss.Color("#cdd6f4"),
		Subtext: lipgloss.Color("#a6adc8"),

		Blue:   lipgloss.Color("#89b4fa"),
		Mauve:  lipgloss.Color("#cba6f7"),
		Green:  lipgloss.Color("#a6e3a1"),
		Yellow: lipgloss.Color("#f9e2af"),
		Sky:    lipgloss.Color("#89dceb"),
		Peach:  lipgloss.Color("#fab387"),
		Red:    lipgloss.Color("#f38ba8"),
		Pink:   lipgloss.Color("#f5c2e7"),

		// Low-saturation derivatives chosen to leave syntax-highlighted
		// foregrounds readable on Mocha's #1e1e2e base.
		DiffAddBg: lipgloss.Color("#26343a"),
		DiffDelBg: lipgloss.Color("#3a2638"),

		// Neutral pill: Surface bg gives a clear chip outline against
		// Base, and #ffffff fg drives WCAG-AA contrast (~10:1) so the
		// label reads as text, not "muted noise".
		PillNeutralBg: lipgloss.Color("#45475a"), // Overlay
		PillNeutralFg: lipgloss.Color("#ffffff"),

		PillFgOnSaturated: lipgloss.Color("#11111b"), // Catppuccin "Crust": near-black, reads cleanly on every accent.
		PillFgOnLight:     lipgloss.Color("#11111b"),

		Accent:     lipgloss.Color("#cba6f7"), // Mauve
		Success:    lipgloss.Color("#a6e3a1"), // Green
		Danger:     lipgloss.Color("#f38ba8"), // Red
		Attention:  lipgloss.Color("#f9e2af"), // Yellow
		Info:       lipgloss.Color("#89dceb"), // Sky
		Identifier: lipgloss.Color("#89b4fa"), // Blue
		GlamourStyle: "dark",
		ChromaStyle:  "catppuccin-mocha",
	}
}

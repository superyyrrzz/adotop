package theme

import "github.com/charmbracelet/lipgloss"

func newCatppuccinLatte() Theme {
	return Theme{
		Base:    lipgloss.Color("#eff1f5"),
		Surface: lipgloss.Color("#dce0e8"),
		Overlay: lipgloss.Color("#9ca0b0"),
		Text:    lipgloss.Color("#4c4f69"),
		Subtext: lipgloss.Color("#5c5f77"),

		Blue:   lipgloss.Color("#1e66f5"),
		Mauve:  lipgloss.Color("#8839ef"),
		Green:  lipgloss.Color("#40a02b"),
		Yellow: lipgloss.Color("#df8e1d"),
		Sky:    lipgloss.Color("#04a5e5"),
		Peach:  lipgloss.Color("#fe640b"),
		Red:    lipgloss.Color("#d20f39"),
		Pink:   lipgloss.Color("#ea76cb"),

		DiffAddBg: lipgloss.Color("#d4ead0"),
		DiffDelBg: lipgloss.Color("#f1d4d8"),

		// Neutral pill on light: Latte's Overlay (#9ca0b0) is too washed
		// out to read against Base. Use a darker grey + black fg so the
		// chip is unambiguously a chip; ratio ~7:1.
		PillNeutralBg: lipgloss.Color("#7c7f93"),
		PillNeutralFg: lipgloss.Color("#000000"),

		PillFgOnSaturated: lipgloss.Color("#000000"),
		PillFgOnLight:     lipgloss.Color("#000000"),
	}
}

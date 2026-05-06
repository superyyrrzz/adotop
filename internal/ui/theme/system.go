package theme

import "github.com/charmbracelet/lipgloss"

// newSystem returns a theme that uses ANSI base-16 indices instead of
// truecolor hex. This delegates color choice to the user's terminal
// palette (xterm colors 0-15), which is what adotop shipped with before
// the Catppuccin port. Users who carefully tuned their terminal theme
// (Solarized, Gruvbox, Windows Terminal scheme, etc.) get those colors
// back when they pick this.
//
// We use the standard ANSI mapping:
//
//	0 black   8 bright black
//	1 red     9 bright red
//	2 green  10 bright green
//	3 yellow 11 bright yellow
//	4 blue   12 bright blue
//	5 magenta 13 bright magenta
//	6 cyan   14 bright cyan
//	7 white  15 bright white
//
// Diff backgrounds keep using xterm-256 22/52 (the original deep
// green/red) because there's no ANSI 4-bit equivalent dim enough to
// stay readable behind syntax highlighting.
func newSystem() Theme {
	return Theme{
		Base:    lipgloss.Color(""), // empty = terminal default
		Surface: lipgloss.Color(""),
		Overlay: lipgloss.Color("8"), // bright black for borders
		Text:    lipgloss.Color(""),  // default fg
		Subtext: lipgloss.Color("7"), // ANSI 7 (white/light grey): readable for footer/hints. ANSI 8 was indistinguishable from default bg on many palettes.

		Blue:   lipgloss.Color("12"), // bright blue
		Mauve:  lipgloss.Color("13"), // bright magenta
		Green:  lipgloss.Color("10"), // bright green
		Yellow: lipgloss.Color("11"), // bright yellow
		Sky:    lipgloss.Color("14"), // bright cyan
		Peach:  lipgloss.Color("11"), // closest ANSI: yellow
		Red:    lipgloss.Color("9"),  // bright red
		Pink:   lipgloss.Color("13"), // closest ANSI: bright magenta

		// Use xterm-256 dark green / dark red. These render correctly on
		// any 256-color terminal (which is essentially all of them today)
		// and match the look from before the theme port.
		DiffAddBg: lipgloss.Color("22"),
		DiffDelBg: lipgloss.Color("52"),

		// Neutral pill on system: ANSI 8 (bright black) bg + ANSI 15
		// (bright white) fg. Hard contrast pair that survives any
		// terminal palette tune.
		PillNeutralBg: lipgloss.Color("8"),
		PillNeutralFg: lipgloss.Color("15"),

		// On 4-bit palettes "0" can match the user's terminal bg too
		// closely (translucent backgrounds, low-contrast schemes), so
		// pin chip text to "15" (bright white). Loses the
		// dark-on-bright look but guarantees the label is readable.
		PillFgOnSaturated: lipgloss.Color("15"),

		// Yellow is too bright for white text — black-on-yellow is the
		// universal high-contrast convention for warning chips.
		PillFgOnLight: lipgloss.Color("0"),

		Accent:     lipgloss.Color("13"), // bright magenta
		Success:    lipgloss.Color("10"), // bright green
		Danger:     lipgloss.Color("9"),  // bright red
		Attention:  lipgloss.Color("11"), // bright yellow
		Info:       lipgloss.Color("14"), // bright cyan
		Identifier: lipgloss.Color("12"), // bright blue
		// "dark" is the safer default for system themes — most terminal
		// schemes are dark. Auto-detection lives in the "auto" path
		// (Mocha/Latte) where we know the bg.
		GlamourStyle: "dark",
		ChromaStyle:  "monokai",
	}
}

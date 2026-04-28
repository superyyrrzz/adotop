package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestPillContrastFloor is the regression guard for the "grey on grey"
// bug. Each pill is a fg/bg pair; if the WCAG relative-luminance ratio
// drops below ~3:1, the label fades into the chip and users complain
// (see #neutral-pill thread). 3:1 is the WCAG-AA threshold for "large
// text", which is appropriate for chip labels: short, bold, big enough
// in terminal cells to read at the lower bound.
//
// We only check truecolor themes (Mocha, Latte). The "system" theme
// uses ANSI base-16 indices whose actual rendered colors are chosen by
// the user's terminal; we can't compute contrast for it. That theme is
// covered by TestSystemPillsUseHighContrastIndices below, which checks
// the index pairs are conventionally high-contrast (0/15 black/white,
// 8/15 dark grey/bright white, etc.).
func TestPillContrastFloor(t *testing.T) {
	const minRatio = 3.0
	for _, name := range []string{"dark", "light"} {
		th := New(name)
		cases := []struct {
			label  string
			bg, fg lipgloss.Color
		}{
			{"PillNeutral", th.PillNeutralBg, th.PillNeutralFg},
			{"PillGood", th.Green, th.PillFgOnSaturated},
			{"PillBad", th.Red, th.PillFgOnSaturated},
			{"PillWarn", th.Yellow, th.PillFgOnLight},
			{"PillInfo", th.Sky, th.PillFgOnSaturated},
			{"PillDone", th.Mauve, th.PillFgOnSaturated},
		}
		for _, c := range cases {
			r, err := contrastRatio(c.bg, c.fg)
			if err != nil {
				t.Fatalf("%s/%s ratio: %v", name, c.label, err)
			}
			if r < minRatio {
				t.Fatalf("%s/%s contrast %.2f below %.1f (bg=%s fg=%s)",
					name, c.label, r, minRatio, c.bg, c.fg)
			}
		}
	}
}

// TestSystemPillsUseHighContrastIndices: for the ANSI-indexed system
// theme we can't measure rendered colors, but we CAN assert the index
// pairs follow the convention that survives any reasonable terminal
// palette: chip text on bright pill bg uses 15 (bright white) — never
// 0 (black), which on translucent terminals can match the bg too
// closely. Yellow is the exception — too bright for white text — and
// uses ANSI 0.
func TestSystemPillsUseHighContrastIndices(t *testing.T) {
	th := New("system")
	if th.PillFgOnSaturated != lipgloss.Color("15") {
		t.Fatalf("system PillFgOnSaturated should be ANSI 15 (bright white) for palette safety, got %q", th.PillFgOnSaturated)
	}
	if th.PillFgOnLight != lipgloss.Color("0") {
		t.Fatalf("system PillFgOnLight should be ANSI 0 (black) so DRAFT/yellow chips stay readable, got %q", th.PillFgOnLight)
	}
	// Neutral pill: 8 (bright black) bg + 15 (bright white) fg is the
	// conventional high-contrast greyscale pair.
	if th.PillNeutralBg != lipgloss.Color("8") || th.PillNeutralFg != lipgloss.Color("15") {
		t.Fatalf("system PillNeutral should be ANSI 8/15, got bg=%q fg=%q",
			th.PillNeutralBg, th.PillNeutralFg)
	}
	// Subtext (used for footer/hints/inactive tabs) must be readable.
	// ANSI 8 = bright black is too close to the default bg on common
	// palettes; 7 = white/light grey is the smallest bump that holds.
	if th.Subtext != lipgloss.Color("7") {
		t.Fatalf("system Subtext should be ANSI 7 (light grey/white), got %q", th.Subtext)
	}
}

// contrastRatio computes the WCAG 2.x relative-luminance ratio for two
// hex colors (#rrggbb). Returns (1+L1)/(L2+0.05) where L1 is the
// brighter color's luminance.
func contrastRatio(a, b lipgloss.Color) (float64, error) {
	la, err := luminance(string(a))
	if err != nil {
		return 0, err
	}
	lb, err := luminance(string(b))
	if err != nil {
		return 0, err
	}
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05), nil
}

// luminance returns the WCAG relative luminance of a "#rrggbb" color.
func luminance(hex string) (float64, error) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return 0, fmt.Errorf("not a #rrggbb color: %q", hex)
	}
	parse := func(s string) (float64, error) {
		v, err := strconv.ParseInt(s, 16, 0)
		if err != nil {
			return 0, err
		}
		c := float64(v) / 255.0
		if c <= 0.03928 {
			return c / 12.92, nil
		}
		return math.Pow((c+0.055)/1.055, 2.4), nil
	}
	r, err := parse(h[0:2])
	if err != nil {
		return 0, err
	}
	g, err := parse(h[2:4])
	if err != nil {
		return 0, err
	}
	b, err := parse(h[4:6])
	if err != nil {
		return 0, err
	}
	return 0.2126*r + 0.7152*g + 0.0722*b, nil
}

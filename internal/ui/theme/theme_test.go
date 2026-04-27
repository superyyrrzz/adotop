package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestNewExplicitDarkReturnsMocha(t *testing.T) {
	th := New("dark")
	if th.Base != lipgloss.Color("#1e1e2e") {
		t.Fatalf("dark override: want Mocha base, got %q", th.Base)
	}
}

func TestNewExplicitLightReturnsLatte(t *testing.T) {
	th := New("light")
	if th.Base != lipgloss.Color("#eff1f5") {
		t.Fatalf("light override: want Latte base, got %q", th.Base)
	}
}

func TestNewUnknownFallsBackToSystem(t *testing.T) {
	// Unknown values default to system so we never crash on a typo.
	th := New("blueberry")
	if th.Blue != lipgloss.Color("12") {
		t.Fatalf("unknown name should fall back to system, got Blue %q", th.Blue)
	}
}

func TestNewEmptyDefaultsToSystem(t *testing.T) {
	// Empty (env unset) returns the system palette so users see the
	// original look without configuring anything.
	th := New("")
	if th.Blue != lipgloss.Color("12") {
		t.Fatalf("empty override should default to system, got Blue %q", th.Blue)
	}
}

func TestThemeHasDiffBackgrounds(t *testing.T) {
	th := New("dark")
	if th.DiffAddBg == "" || th.DiffDelBg == "" {
		t.Fatalf("Mocha must define DiffAddBg/DiffDelBg")
	}
	th = New("light")
	if th.DiffAddBg == "" || th.DiffDelBg == "" {
		t.Fatalf("Latte must define DiffAddBg/DiffDelBg")
	}
}

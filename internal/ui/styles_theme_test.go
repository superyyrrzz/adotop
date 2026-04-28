package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ui/theme"
)

func TestNewStylesPopulatesAllRoles(t *testing.T) {
	th := theme.New("dark")
	s := NewStyles(th)
	// Render a sentinel through each style; if any role is the
	// zero-value lipgloss.Style we'd lose color codes silently.
	roles := map[string]string{
		"Header":   s.Header.Render("x"),
		"Faint":    s.Faint.Render("x"),
		"ErrLine":  s.ErrLine.Render("x"),
		"Approve":  s.Approve.Render("x"),
		"Reject":   s.Reject.Render("x"),
		"Wait":     s.Wait.Render("x"),
		"None":     s.None.Render("x"),
		"Selected": s.Selected.Render("x"),
		"HelpBox":  s.HelpBox.Render("x"),
		"TabOn":    s.TabOn.Render("x"),
		"TabOff":   s.TabOff.Render("x"),
		"Footer":   s.Footer.Render("x"),
	}
	for name, out := range roles {
		if !strings.Contains(out, "x") {
			t.Fatalf("style %s did not render input: %q", name, out)
		}
	}
}

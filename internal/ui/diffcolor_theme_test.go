package ui

import (
	"strings"
	"testing"

	"github.com/renzeyu/adotop/internal/ui/theme"
)

func TestApplyDiffThemeChangesAddBg(t *testing.T) {
	before := ansiAddLineBg
	defer func() { ansiAddLineBg = before }()

	applyDiffTheme(theme.New("light"))
	after := ansiAddLineBg
	if before == after {
		t.Fatalf("applyDiffTheme(light) should change ansiAddLineBg; both = %q", before)
	}
	// Sanity: the new value must still be a CSI sequence so Colorize
	// doesn't emit garbage.
	if !strings.HasPrefix(after, "\x1b[") {
		t.Fatalf("ansiAddLineBg after applyDiffTheme not a CSI sequence: %q", after)
	}
}

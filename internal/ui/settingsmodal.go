package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/config"
)

// renderSettingsModal returns a read-only overlay showing the loaded
// config and the path it came from. Edits still go through
// `adotop init` or the TOML file directly — this modal is the "what
// am I running with right now?" answer, not an editor.
func renderSettingsModal(cfg config.Config, path string, termW int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(Cursor.GetForeground())
	keyStyle := lipgloss.NewStyle().Bold(true)
	rows := []settingRow{
		{"org", cfg.Org},
		{"project", cfg.Project},
		{"refresh_interval", cfg.RefreshInterval.String()},
		{"repo_roots", formatRepoRoots(cfg.RepoRoots)},
	}
	keyW := 0
	for _, r := range rows {
		if w := lipgloss.Width(r.key); w > keyW {
			keyW = w
		}
	}
	var lines []string
	lines = append(lines, Faint.Render("─ Values "))
	for _, r := range rows {
		lines = append(lines, "  "+
			lipgloss.NewStyle().Width(keyW).Render(keyStyle.Render(r.key))+
			"   "+r.val)
	}
	pathBlock := Faint.Render("─ Source ") + "\n  " + path
	hint := Faint.Render("read-only · edit via `adotop init` or the file above · , or esc to close")
	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Settings"),
		"",
		strings.Join(lines, "\n"),
		"",
		pathBlock,
		"",
		hint,
	)
	rendered := ModalBox.Render(content)
	if termW > 0 && lipgloss.Width(rendered) > termW {
		return rendered
	}
	return rendered
}

type settingRow struct {
	key string
	val string
}

func formatRepoRoots(roots []string) string {
	if len(roots) == 0 {
		return Faint.Render("(none — local-clone features disabled)")
	}
	return strings.Join(roots, ", ")
}

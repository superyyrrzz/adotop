package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpEntry is one row in the help modal: a key chord and its short
// description. Two-column rendering aligns the keys so the eye scans
// down a single column instead of bouncing across uneven prose.
type helpEntry struct {
	key  string
	desc string
}

// helpSection groups related entries under a heading. Sections render
// stacked top-down with a blank row between, so the modal reads as a
// reference card with clear topic boundaries instead of one long
// catalog the user has to grep visually.
type helpSection struct {
	title   string
	entries []helpEntry
}

// helpSections is the source of truth for what shows up in the help
// modal. Edits here flow through to the rendered overlay; the older
// flat slice in View() is gone. Order is "things you do most often"
// first so the most-used keys are above the fold on a small terminal.
func helpSections() []helpSection {
	return []helpSection{
		{
			title: "Navigation",
			entries: []helpEntry{
				{"j / k or ↓ / ↑", "move cursor (wraps at edges)"},
				{"n / N", "next / prev file (Detail)"},
				{"gg / G", "jump to first / last in focused pane"},
				{"pgup / pgdn", "scroll focused pane"},
				{"tab / shift+tab", "switch Files ↔ Diff focus"},
				{"enter", "drill into Diff focus (from Files)"},
			},
		},
		{
			title: "Threads",
			entries: []helpEntry{
				{"[ / ]", "prev / next thread"},
				{"space", "expand thread under cursor"},
				{"J", "jump to comments block on this file"},
				{"R", "toggle showing resolved comments"},
				{"c", "compose new comment (file or PR-level)"},
				{"C", "reply to selected thread"},
				{"x", "toggle resolve / reactivate selected thread"},
			},
		},
		{
			title: "Actions",
			entries: []helpEntry{
				{"a", "approve PR"},
				{"v", "open vote menu (a/s/w/r/c)"},
				{"X", "abandon PR (confirms)"},
			},
		},
		{
			title: "Views & Modals",
			entries: []helpEntry{
				{"D", "open full PR description in a modal"},
				{"M", "pick a commit to view its diff alone (M again exits)"},
				{"w", "toggle soft-wrap on the diff pane"},
				{"+ / -", "more / less diff context"},
			},
		},
		{
			title: "List screen",
			entries: []helpEntry{
				{"/", "filter PRs by title/author/branch"},
				{"#", "jump to PR by ID"},
			},
		},
		{
			title: "Chrome",
			entries: []helpEntry{
				{"o", "open current PR in browser"},
				{"r", "refresh current screen"},
				{"?", "toggle this help"},
				{"esc", "back / close modal"},
				{"q", "quit (list) or back (detail)"},
				{"ctrl+c", "force quit (always)"},
			},
		},
	}
}

// renderHelpModal returns the bordered overlay used by the ? key.
// Sections render as two-column key→description pairs, grouped under
// a section title. The whole thing is wrapped in ModalBox (the same
// container the description and commits modals use) so the visual
// language stays consistent across overlays.
func renderHelpModal(termW int) string {
	sections := helpSections()

	// Compute the widest key across ALL sections so columns align
	// vertically — otherwise each section's keys would left-align
	// independently and the rendering looks ragged.
	keyW := 0
	for _, sec := range sections {
		for _, e := range sec.entries {
			if w := lipgloss.Width(e.key); w > keyW {
				keyW = w
			}
		}
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(Cursor.GetForeground())
	keyStyle := lipgloss.NewStyle().Bold(true)
	descStyle := lipgloss.NewStyle()

	var blocks []string
	for _, sec := range sections {
		var rows []string
		rows = append(rows, Faint.Render("─ "+sec.title+" "))
		for _, e := range sec.entries {
			row := "  " +
				lipgloss.NewStyle().Width(keyW).Render(keyStyle.Render(e.key)) +
				"   " +
				descStyle.Render(e.desc)
			rows = append(rows, row)
		}
		blocks = append(blocks, lipgloss.JoinVertical(lipgloss.Left, rows...))
	}

	body := strings.Join(blocks, "\n\n")
	header := titleStyle.Render("Help")
	footer := Faint.Render("? or esc to close")
	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		body,
		"",
		footer,
	)
	rendered := ModalBox.Render(content)

	// ModalBox padding is fixed; if the resulting box would be wider
	// than the terminal (very narrow terminal vs. long descriptions)
	// just hand back the unwrapped form — overlayBox will clip rather
	// than break alignment.
	if termW > 0 && lipgloss.Width(rendered) > termW {
		return rendered
	}
	return rendered
}

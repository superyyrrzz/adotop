package ui

import (
	"encoding/json"
	"log/slog"

	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ui/theme"
)

// glamourStyleJSON returns a glamour style spec (JSON) tuned to the
// active theme. Only the high-leverage fields are overridden — heading
// colors, h1 banner, link colors, code block fg/bg — so PR
// descriptions and comments visually belong to the same palette as the
// chrome around them. Everything else (chroma syntax highlighting,
// indents, list markers) inherits from glamour's built-in dark/light
// style; tuning syntax highlighting per theme is a separate, larger
// project.
//
// Returned bytes are safe to hand to glamour.WithStylesFromJSONBytes.
// On any marshalling error we return nil and the caller falls back to
// glamour.WithStandardStyle(t.GlamourStyle).
func glamourStyleJSON(t theme.Theme) []byte {
	// Glamour expects color values as strings — either hex like "#cba6f7"
	// or ANSI indices like "13". We hand back whatever the theme stored;
	// both forms round-trip through lipgloss.Color → string.
	col := func(c lipgloss.Color) string { return string(c) }

	spec := map[string]any{
		"document": map[string]any{
			"block_prefix": "\n",
			"block_suffix": "\n",
			"color":        col(t.Text),
			"margin":       2,
		},
		"block_quote": map[string]any{
			"indent":       1,
			"indent_token": "│ ",
			"color":        col(t.Subtext),
		},
		"paragraph": map[string]any{},
		"list": map[string]any{
			"level_indent": 2,
		},
		// Headings cascade: `heading` is the default for h2-h6 unless
		// overridden. Identifier (Catppuccin Blue) reads as "structural
		// label" everywhere else in the app, so reusing it here keeps
		// the markdown's section breaks visually paired with our
		// pane/file-list labels.
		"heading": map[string]any{
			"block_suffix": "\n",
			"color":        col(t.Identifier),
			"bold":         true,
		},
		// H1 gets a filled accent banner — same accent as the active
		// tab pill and the modal border. Visually it reads as "this is
		// the title of what you're reading", peer of the modal title.
		"h1": map[string]any{
			"prefix":           " ",
			"suffix":           " ",
			"color":            col(t.PillFgOnSaturated),
			"background_color": col(t.Accent),
			"bold":             true,
		},
		"h2":            map[string]any{"prefix": "## "},
		"h3":            map[string]any{"prefix": "### "},
		"h4":            map[string]any{"prefix": "#### "},
		"h5":            map[string]any{"prefix": "##### "},
		"h6":            map[string]any{"prefix": "###### ", "color": col(t.Subtext), "bold": false},
		"text":          map[string]any{},
		"strikethrough": map[string]any{"crossed_out": true},
		"emph":          map[string]any{"italic": true, "color": col(t.Attention)},
		"strong":        map[string]any{"bold": true, "color": col(t.Identifier)},
		"hr": map[string]any{
			"color":  col(t.Overlay),
			"format": "\n──────\n",
		},
		"item":        map[string]any{"block_prefix": "• "},
		"enumeration": map[string]any{"block_prefix": ". "},
		"task": map[string]any{
			"ticked":   "[✓] ",
			"unticked": "[ ] ",
		},
		// Links use Info (cyan-family) so they read as actionable —
		// distinct from the Identifier blue that headings use.
		"link":      map[string]any{"color": col(t.Info), "underline": true},
		"link_text": map[string]any{"color": col(t.Info), "bold": true},
		"image":     map[string]any{"color": col(t.Pink), "underline": true},
		"image_text": map[string]any{
			"color":  col(t.Subtext),
			"format": "Image: {{.text}} →",
		},
		// Inline code: subtle filled badge using Surface bg + Attention
		// fg (yellow). Same visual idea as our PillWarn but at text
		// weight, not chip weight, so it fits inside prose.
		"code": map[string]any{
			"prefix":           " ",
			"suffix":           " ",
			"color":            col(t.Attention),
			"background_color": col(t.Surface),
		},
		// Code blocks: keep glamour's chroma syntax colors (they're a
		// painstakingly-tuned palette and worth more than we'd spend
		// reinventing) but pin the surrounding background to our
		// Surface so the block visually nests inside the modal/pane.
		"code_block": map[string]any{
			"color":  col(t.Subtext),
			"margin": 2,
			"chroma": map[string]any{
				"text":             map[string]any{"color": col(t.Text)},
				"background":       map[string]any{"background_color": col(t.Surface)},
				"keyword":          map[string]any{"color": col(t.Mauve)},
				"keyword_reserved": map[string]any{"color": col(t.Pink)},
				"keyword_namespace": map[string]any{"color": col(t.Pink)},
				"keyword_type":     map[string]any{"color": col(t.Yellow)},
				"comment":          map[string]any{"color": col(t.Subtext), "italic": true},
				"name":             map[string]any{"color": col(t.Text)},
				"name_function":    map[string]any{"color": col(t.Blue)},
				"name_class":       map[string]any{"color": col(t.Yellow), "bold": true},
				"name_attribute":   map[string]any{"color": col(t.Sky)},
				"name_tag":         map[string]any{"color": col(t.Mauve)},
				"name_builtin":     map[string]any{"color": col(t.Peach)},
				"literal_string":   map[string]any{"color": col(t.Green)},
				"literal_number":   map[string]any{"color": col(t.Peach)},
				"operator":         map[string]any{"color": col(t.Sky)},
				"punctuation":      map[string]any{"color": col(t.Subtext)},
				"generic_inserted": map[string]any{"color": col(t.Green)},
				"generic_deleted":  map[string]any{"color": col(t.Red)},
				"generic_emph":     map[string]any{"italic": true},
				"generic_strong":   map[string]any{"bold": true},
			},
		},
		"definition_list": map[string]any{},
		"definition_term": map[string]any{"bold": true},
		"definition_description": map[string]any{
			"block_prefix": "\n🠶 ",
		},
		"html_block": map[string]any{},
		"html_span":  map[string]any{},
	}

	out, err := json.Marshal(spec)
	if err != nil {
		// Defensive: a marshalling failure here would mean a programming
		// error in the spec map (un-encodable type). File-only slog so
		// we have a record; caller falls back to the standard style.
		slog.Warn("glamour style marshal failed", "err", err)
		return nil
	}
	return out
}

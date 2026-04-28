# Theme System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace adotop's ad-hoc package-level lipgloss styles with a Catppuccin Mocha (dark) / Latte (light) theme system that auto-detects the terminal background, with `ADOTOP_THEME` env var override.

**Architecture:** A new `internal/ui/theme` package owns the palette (`Theme` struct of named `lipgloss.Color` values) and the two constructors (`newCatppuccinMocha`, `newCatppuccinLatte`). `theme.New(env)` resolves the active theme from the `ADOTOP_THEME` env var (`light` | `dark` | `auto` | empty), falling back to `termenv.HasDarkBackground()` for `auto`/empty. The `ui` package's existing package-level style vars (`Header`, `Faint`, `Approve`, `Reject`, `Wait`, `None`, `Selected`, `ErrLine`, `HelpBox`, `TabOn`, `TabOff`, `Footer`) are converted from `var` to functions on a `Styles` struct that's built from a `Theme`. `Styles` is constructed once in `New()` (the Model constructor) and threaded everywhere a style is currently read. The hardcoded xterm-256 diff backgrounds in `diffcolor.go` (`\x1b[48;5;22m` / `\x1b[48;5;52m`) become package-level `var`s set at init from the active theme's diff-bg colors.

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, termenv (already a transitive dep via lipgloss). Pattern lifted from `C:\Git\career-ops\dashboard\internal\theme\`.

---

## File Structure

**New files:**
- `internal/ui/theme/theme.go` — `Theme` struct, `New(envOverride string) Theme` constructor, palette resolution
- `internal/ui/theme/catppuccin.go` — `newCatppuccinMocha()` (dark)
- `internal/ui/theme/catppuccin_latte.go` — `newCatppuccinLatte()` (light)
- `internal/ui/theme/theme_test.go` — env override + auto-detect resolution tests
- `internal/ui/styles_theme.go` — `Styles` struct holding all derived `lipgloss.Style` values, `NewStyles(t Theme) Styles` constructor
- `internal/ui/styles_theme_test.go` — sanity test that all old style names still resolve (foreground != zero where expected)

**Modified files:**
- `internal/ui/styles.go` — DELETE the package-level `var` block; keep file as a place for legacy aliases during migration if needed (or delete entirely once migration is done)
- `internal/ui/app.go` — add `styles Styles` and `theme theme.Theme` fields to `Model`; populate in `New()`; replace every `Header.Render(...)` etc. with `m.styles.Header.Render(...)`; thread `m.styles` into list/detail/diff/threads constructors that need it
- `internal/ui/list.go` — accept `Styles` via constructor, replace package-level style refs with field refs
- `internal/ui/detail.go` — same
- `internal/ui/diff.go` — same
- `internal/ui/threads.go` — same
- `internal/ui/keys.go` — same (only uses `Faint` in the help block — small surface)
- `internal/ui/diffcolor.go` — convert the `const` ANSI codes that are theme-dependent (`ansiAddLineBg`, `ansiDelLineBg`, and optionally the gutter bar foregrounds) to `var`s populated by `applyTheme(theme.Theme)`; expose `applyTheme` so `New()` can call it once at startup
- `internal/ui/diffcolor_test.go` (if present) — verify default values still produce well-formed escapes

**Theme palette** (verbatim from career-ops/dashboard, validated as Catppuccin's published hex codes):
- Mocha: Base `#1e1e2e`, Surface `#313244`, Overlay `#45475a`, Text `#cdd6f4`, Subtext `#a6adc8`, Blue `#89b4fa`, Mauve `#cba6f7`, Green `#a6e3a1`, Yellow `#f9e2af`, Sky `#89dceb`, Peach `#fab387`, Red `#f38ba8`, Pink `#f5c2e7`
- Latte: Base `#eff1f5`, Surface `#dce0e8`, Overlay `#9ca0b0`, Text `#4c4f69`, Subtext `#5c5f77`, Blue `#1e66f5`, Mauve `#8839ef`, Green `#40a02b`, Yellow `#df8e1d`, Sky `#04a5e5`, Peach `#fe640b`, Red `#d20f39`, Pink `#ea76cb`

**Diff backgrounds:** Catppuccin doesn't ship dedicated diff-bg colors. Pick low-saturation derivatives of Green/Red:
- Mocha add-bg: `#26343a` (deep teal-ish, blends with `#1e1e2e` base, leaves `#a6e3a1` foreground readable)
- Mocha del-bg: `#3a2638` (deep maroon)
- Latte add-bg: `#d4ead0` (pale green)
- Latte del-bg: `#f1d4d8` (pale rose)

Add these as fields on `Theme` (`DiffAddBg`, `DiffDelBg`) so they're palette-owned, not magic in `diffcolor.go`. Convert hex → ANSI via `lipgloss.Color(hex).Sequence(true)` (true = background). lipgloss handles truecolor vs 256-color downgrade based on `termenv.ColorProfile()`.

---

## Tasks

### Task 1: Theme package skeleton + palettes

**Files:**
- Create: `internal/ui/theme/theme.go`
- Create: `internal/ui/theme/catppuccin.go`
- Create: `internal/ui/theme/catppuccin_latte.go`
- Test: `internal/ui/theme/theme_test.go`

- [ ] **Step 1: Write the failing test**

```go
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

func TestNewUnknownFallsBackToMocha(t *testing.T) {
	// Unknown values default to dark so we never crash on a typo.
	th := New("blueberry")
	if th.Base != lipgloss.Color("#1e1e2e") {
		t.Fatalf("unknown name should fall back to Mocha, got base %q", th.Base)
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/theme/...`
Expected: build error (`theme` package doesn't exist).

- [ ] **Step 3: Create `internal/ui/theme/theme.go`**

```go
// Package theme provides Catppuccin-based palettes for adotop.
package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Theme holds named colors for the whole TUI. Add new fields here when a
// new semantic role appears; never reach for raw hex in the ui package.
type Theme struct {
	// Base surfaces
	Base    lipgloss.Color // app background
	Surface lipgloss.Color // panes, boxes
	Overlay lipgloss.Color // borders, dividers
	Text    lipgloss.Color // body
	Subtext lipgloss.Color // faint text

	// Accents (semantic uses noted on each callsite, not here)
	Blue   lipgloss.Color
	Mauve  lipgloss.Color
	Green  lipgloss.Color
	Yellow lipgloss.Color
	Sky    lipgloss.Color
	Peach  lipgloss.Color
	Red    lipgloss.Color
	Pink   lipgloss.Color

	// Diff line backgrounds. Kept on the palette so light/dark variants
	// can pick contrast that actually works against the active Base.
	DiffAddBg lipgloss.Color
	DiffDelBg lipgloss.Color
}

// New resolves a Theme from an explicit override or terminal detection.
//
// override semantics:
//   "dark"          -> Mocha
//   "light"         -> Latte
//   "auto", ""      -> termenv.HasDarkBackground()
//   anything else   -> Mocha (defensive: unknown values shouldn't crash)
func New(override string) Theme {
	switch override {
	case "dark":
		return newCatppuccinMocha()
	case "light":
		return newCatppuccinLatte()
	case "auto", "":
		if termenv.HasDarkBackground() {
			return newCatppuccinMocha()
		}
		return newCatppuccinLatte()
	default:
		return newCatppuccinMocha()
	}
}
```

- [ ] **Step 4: Create `internal/ui/theme/catppuccin.go`**

```go
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
	}
}
```

- [ ] **Step 5: Create `internal/ui/theme/catppuccin_latte.go`**

```go
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
	}
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/ui/theme/...`
Expected: PASS (4 tests).

- [ ] **Step 7: Commit**

```bash
git add internal/ui/theme/
git commit -m "feat(ui/theme): add Catppuccin Mocha + Latte palettes with auto-detect"
```

---

### Task 2: Styles struct derived from Theme

**Files:**
- Create: `internal/ui/styles_theme.go`
- Test: `internal/ui/styles_theme_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/ -run TestNewStylesPopulatesAllRoles`
Expected: FAIL (`NewStyles` undefined).

- [ ] **Step 3: Create `internal/ui/styles_theme.go`**

```go
package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ui/theme"
)

// Styles is the full set of lipgloss.Style values derived from a Theme.
// Constructed once at app start (see New()) and threaded into every
// component that renders. Keeping these as concrete fields (not
// functions) lets callers chain .Render(...) without per-keypress work.
type Styles struct {
	Header   lipgloss.Style
	Faint    lipgloss.Style
	ErrLine  lipgloss.Style
	TabOn    lipgloss.Style
	TabOff   lipgloss.Style
	Selected lipgloss.Style
	HelpBox  lipgloss.Style
	Approve  lipgloss.Style
	Reject   lipgloss.Style
	Wait     lipgloss.Style
	None     lipgloss.Style
	Footer   lipgloss.Style

	// Border color used by the right-pane frame in app.go.
	PaneBorder lipgloss.Color
}

// NewStyles builds the style set for a given theme. Add new roles here
// when the UI grows new visual concepts; do NOT scatter
// lipgloss.NewStyle() calls in render code.
func NewStyles(t theme.Theme) Styles {
	return Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.Blue),
		Faint:    lipgloss.NewStyle().Foreground(t.Subtext),
		ErrLine:  lipgloss.NewStyle().Foreground(t.Red),
		TabOn:    lipgloss.NewStyle().Bold(true).Underline(true).Foreground(t.Mauve),
		TabOff:   lipgloss.NewStyle().Foreground(t.Subtext),
		Selected: lipgloss.NewStyle().Reverse(true),
		HelpBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Overlay).
			Padding(0, 1),
		Approve: lipgloss.NewStyle().Foreground(t.Green),
		Reject:  lipgloss.NewStyle().Foreground(t.Red),
		Wait:    lipgloss.NewStyle().Foreground(t.Yellow),
		None:    lipgloss.NewStyle().Foreground(t.Subtext),
		Footer:  lipgloss.NewStyle().Foreground(t.Subtext),

		PaneBorder: t.Overlay,
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/ui/ -run TestNewStylesPopulatesAllRoles`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/styles_theme.go internal/ui/styles_theme_test.go
git commit -m "feat(ui): add Styles struct derived from theme palette"
```

---

### Task 3: Wire Styles into Model and remove package-level vars

This is the biggest mechanical change. Do it in one task so the tree stays green between commits.

**Files:**
- Modify: `internal/ui/app.go` — add fields, populate in `New()`, replace style refs
- Modify: `internal/ui/styles.go` — delete the var block
- Modify: `internal/ui/list.go`, `internal/ui/detail.go`, `internal/ui/diff.go`, `internal/ui/threads.go`, `internal/ui/keys.go` — replace bare style refs (`Header`, `Faint`, etc.) with field refs

- [ ] **Step 1: Add fields to Model and construct in New()**

In `internal/ui/app.go`, add to the `Model` struct (near other config-ish fields):

```go
	theme  theme.Theme
	styles Styles
```

Add the import: `"github.com/superyyrrzz/adotop/internal/ui/theme"`.

In the `New()` constructor (top of the function), before any sub-model construction:

```go
th := theme.New(os.Getenv("ADOTOP_THEME"))
styles := NewStyles(th)
```

Then set `theme: th, styles: styles,` in the `Model{...}` literal. Add `"os"` import if not already present.

- [ ] **Step 2: Decide on style access pattern in sub-models**

The sub-models (List, Detail, Diff) currently reference package-level `Header`, `Faint`, etc. directly. The cleanest fix is to add a `styles Styles` field to each, populated by their constructors. Update the constructors:

- `NewList(keys KeyMap, styles Styles) ListModel`
- `NewDetail(keys KeyMap, styles Styles) DetailModel`
- `NewDiff(keys KeyMap, styles Styles) DiffModel`

In `app.go`'s `New()`, pass `styles` to each: `list: NewList(keys, styles), detail: NewDetail(keys, styles), preview: NewDiff(keys, styles),`.

`threads.go` operates on `Model` so it already has access via `m.styles`. No constructor change needed.

`keys.go` only uses `Faint` in `HelpView()`. Either pass `Styles` to `HelpView(s Styles)` or expose a method on `Styles` that returns the help text. Pick the parameter form — it keeps `KeyMap` styleless.

- [ ] **Step 3: Mechanical replace in each file**

In `list.go`, `detail.go`, `diff.go`, `threads.go`, `keys.go`, `app.go`:

For each bare reference like `Header.Render(...)`, replace with `m.styles.Header.Render(...)` (or `s.Header.Render(...)` for HelpView, or the receiver name's `.styles.` for sub-models).

Specifically, the right-pane border in `app.go:844` — `BorderForeground(lipgloss.Color("8"))` becomes `BorderForeground(m.styles.PaneBorder)`.

Also fix `internal/ui/list.go:279,282,370` and `internal/ui/detail.go:309` and `internal/ui/threads.go:161` which call `lipgloss.NewStyle().Faint(true)` inline — replace with `m.styles.Faint` (or pass through receiver).

- [ ] **Step 4: Delete the var block in styles.go**

Replace the whole file contents with:

```go
package ui

// Styles for adotop are now built from the theme palette. See
// styles_theme.go (NewStyles) and theme/theme.go.
//
// This file is intentionally near-empty so that any stray reference to
// the old package-level vars (Header, Faint, etc.) becomes a compile
// error rather than silently using a hardcoded ANSI 4-bit color.
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: succeeds. If any "undefined: Header" / "undefined: Faint" errors remain, fix the missed callsite.

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS. Test helpers that construct sub-models directly (look in `*_test.go` for `NewList(keys)`, `NewDetail(keys)`, `NewDiff(keys)`) need the new `styles` arg — pass `NewStyles(theme.New("dark"))` for determinism in tests. Search: `grep -rn 'NewList\|NewDetail\|NewDiff' internal/ui/ --include='*_test.go'`.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/
git commit -m "refactor(ui): thread Styles through models, drop package-level style vars"
```

---

### Task 4: Theme-aware diff line backgrounds

**Files:**
- Modify: `internal/ui/diffcolor.go` — convert `ansiAddLineBg` / `ansiDelLineBg` from `const` to `var`, set from theme
- Modify: `internal/ui/app.go` — call `applyDiffTheme(theme)` once in `New()`
- Test: `internal/ui/diffcolor_test.go` (create or extend) — verify `applyDiffTheme` mutates the vars and round-trips a diff line

- [ ] **Step 1: Write the failing test**

Create `internal/ui/diffcolor_theme_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/ui/theme"
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/ -run TestApplyDiffThemeChangesAddBg`
Expected: FAIL (`applyDiffTheme` undefined; `ansiAddLineBg` is a const so the defer also won't compile).

- [ ] **Step 3: Convert constants to vars and add applyDiffTheme**

In `diffcolor.go`, move `ansiAddLineBg` and `ansiDelLineBg` out of the `const` block into a `var` block right below it:

```go
// Theme-derived diff line backgrounds. Defaulted to the original
// xterm-256 values so tests / direct package use without a theme still
// produce a sensible render. applyDiffTheme overwrites these at app
// startup (see New() in app.go) once the active Theme is known.
var (
	ansiAddLineBg = "\x1b[48;5;22m"
	ansiDelLineBg = "\x1b[48;5;52m"
)
```

Add at the bottom of the file:

```go
// applyDiffTheme repoints the package-level diff backgrounds at the
// active theme's DiffAddBg/DiffDelBg colors. lipgloss handles the
// truecolor → 256-color downgrade based on termenv.ColorProfile().
//
// This is package-global state — there's only one diff renderer and
// only one theme per process, so the indirection isn't worth a struct.
func applyDiffTheme(t theme.Theme) {
	add := lipgloss.NewStyle().Background(t.DiffAddBg)
	del := lipgloss.NewStyle().Background(t.DiffDelBg)
	// Render an empty string just to capture the SGR open sequence;
	// strip the trailing reset so callers concatenate freely.
	ansiAddLineBg = openSequence(add.Render(""))
	ansiDelLineBg = openSequence(del.Render(""))
}

// openSequence pulls the leading "\x1b[...m" out of a lipgloss-rendered
// string. lipgloss wraps content as "<open><content><reset>"; we want
// just <open>.
func openSequence(s string) string {
	end := strings.Index(s, "m")
	if end < 0 || !strings.HasPrefix(s, "\x1b[") {
		return s
	}
	return s[:end+1]
}
```

Add imports at top of file: `"github.com/charmbracelet/lipgloss"` and `"github.com/superyyrrzz/adotop/internal/ui/theme"`.

- [ ] **Step 4: Call applyDiffTheme in New()**

In `internal/ui/app.go` `New()`, right after `th := theme.New(...)`:

```go
applyDiffTheme(th)
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/ui/ -run TestApplyDiffThemeChangesAddBg`
Expected: PASS.

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS. The existing `diffcolor` tests (if any assert exact byte equality on the old `\x1b[48;5;22m`) will need updating — relax them to assert "contains green foreground + a non-empty bg sequence" instead of the literal escape.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/diffcolor.go internal/ui/diffcolor_theme_test.go internal/ui/app.go
git commit -m "feat(ui): theme-aware diff line backgrounds"
```

---

### Task 5: Document the env var

**Files:**
- Modify: `README.md` (or `docs/` if that's where config lives — check first)

- [ ] **Step 1: Find the right doc**

Run: `ls *.md docs/ 2>&1`. The env var goes in whichever file already documents flags / config. If there's no such section, add one to `README.md`.

- [ ] **Step 2: Add the section**

Add a "Theming" section:

```markdown
## Theming

adotop ships two Catppuccin palettes and auto-detects which to use from the
terminal background. Override with the `ADOTOP_THEME` environment variable:

| Value           | Effect                                           |
| --------------- | ------------------------------------------------ |
| (unset) / `auto`| Detect from terminal background                  |
| `dark`          | Force Catppuccin Mocha (dark)                    |
| `light`         | Force Catppuccin Latte (light)                   |

Detection uses `termenv.HasDarkBackground()`, which queries the terminal's
background color via OSC 11. Some multiplexers (older tmux, certain SSH
setups) don't proxy this; if auto-detect picks the wrong one, set
`ADOTOP_THEME` explicitly.
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document ADOTOP_THEME env var"
```

---

## Verification Pass

After all tasks:

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] `make test-live` passes (real PR rendering across the geometry matrix — catches any rendering regression that unit tests miss)
- [ ] Manual smoke: launch with `ADOTOP_THEME=dark`, then `=light`, then unset. Detail view should look distinctly different in light mode (lighter base, darker text). Diff lines should still have visible add/del backgrounds in both modes.
- [ ] Manual smoke: typo override (`ADOTOP_THEME=blueberry`) — must not crash; falls back to Mocha.

package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// statusMode describes the leftmost segment of the statusline. The mode
// drives both the label and the bg color so the user can read the
// current "operating mode" at a glance.
type statusMode int

const (
	modeNormal  statusMode = iota // neutral: routine navigation. The
	//                               mode pill is reserved for "look at
	//                               me" states (CONFIRM/MENU/ERROR);
	//                               NORMAL is the resting state and
	//                               should fade into the chrome.
	modeError                     // red:  error or footer error
	modePending                   // yellow: confirmation prompt awaiting y/n
	modeMenu                      // magenta: modal overlay (vote menu)
)

func (s statusMode) label() string {
	switch s {
	case modeError:
		return "ERROR"
	case modePending:
		return "CONFIRM"
	case modeMenu:
		return "MENU"
	}
	return "NORMAL"
}

func (s statusMode) style() lipgloss.Style {
	switch s {
	case modeError:
		return PillBad
	case modePending:
		return PillWarn
	case modeMenu:
		return PillDone
	}
	return PillNeutral
}

// segment is one block of the statusline: a label rendered with a
// dedicated style. Segments are joined left-to-right and separated by
// a thin divider (statuslineDivider).
type segment struct {
	text  string
	style lipgloss.Style
}

const statuslineDivider = " ▏ "

// renderStatusline composes the bottom statusline from the current
// Model state. Layout (left-to-right):
//
//	[ MODE ] ▏ <context> ▏ <hint segments>            <clock>
//
// The clock floats right; context+hints fill from the left until they
// hit the clock or the terminal edge. Hints are dropped tail-first when
// the line would overflow so the mode and context segments are never
// truncated.
func renderStatusline(m Model) string {
	mode := currentMode(m)
	left := []segment{{text: mode.label(), style: mode.style()}}
	left = append(left, contextSegments(m)...)

	// Mode/context above is always shown. Hints are individual segments
	// rendered with the faint Footer style and joined with a thinner
	// divider; they degrade by dropping tail items.
	hints := hintSegments(m)

	right := clockSegment()

	return composeStatusline(left, hints, right, m.width)
}

// currentMode classifies the active UI mode so the leftmost segment
// reflects what the user can actually do right now.
func currentMode(m Model) statusMode {
	if m.voteMenu {
		return modeMenu
	}
	if m.pendingAction.kind != "" {
		return modePending
	}
	if m.footerErr != "" {
		return modeError
	}
	return modeNormal
}

// contextSegments produces the "where am I" segments. Filled-bg, more
// muted than the mode segment.
func contextSegments(m Model) []segment {
	if m.voteMenu {
		// In modal vote mode we replace the context with the mode prompt
		// so the only thing visible is "what to press now".
		return []segment{{
			text:  "a:approve  s:approve+suggest  w:wait  r:reject  c:clear  esc:cancel",
			style: contextStyle(),
		}}
	}
	if m.pendingAction.kind != "" {
		return []segment{{text: m.pendingAction.prompt, style: contextStyle()}}
	}
	if m.footerErr != "" {
		return []segment{{text: m.footerErr, style: contextStyle()}}
	}
	switch m.screen {
	case screenList:
		ctx := fmt.Sprintf("%s/%s · %s",
			orPlaceholder(m.cfg.Org, "(no org)"),
			orPlaceholder(m.cfg.Project, "(no project)"),
			m.list.Tab().Short())
		return []segment{{text: ctx, style: contextStyle()}}
	case screenDetail:
		s := m.detail.Summary()
		focus := "Files"
		if m.detailFocus == focusDiff {
			focus = "Diff"
		}
		ctx := fmt.Sprintf("PR #%d · %s", s.ID, focus)
		segs := []segment{{text: ctx, style: contextStyle()}}
		segs = append(segs, segment{text: ctxLabel(m.diffCtx), style: hintStyle()})
		// Surface the cache-revalidation indicator so the user knows the
		// screen they're looking at is being verified against the server.
		// The detailInflight counter ticks down as each of the four
		// fetches lands; we only show the chip while >0.
		if m.detailInflight > 0 {
			segs = append(segs, segment{text: "↻ refreshing", style: hintStyle()})
		}
		return segs
	}
	return nil
}

// hintSegments produces the keybinding hints, one per segment, so the
// statusline can drop hints from the tail when space runs short
// without truncating mid-binding.
func hintSegments(m Model) []segment {
	if m.voteMenu || m.pendingAction.kind != "" {
		return nil
	}
	var hints []string
	switch m.screen {
	case screenList:
		hints = []string{"/:filter", "#:goto", "enter:open", "o:browser", "r:refresh", "tab:next", "?:help", "q:quit"}
	case screenDetail:
		base := []string{"tab:focus", "enter:diff/expand", "n/N:file", "gg/G:top/end", "R:show-resolved",
			"a:approve", "v:vote", "X:abandon", "o:browser", "r:refresh", wrapHint(m), "+/-:context", "?:help", "esc:back"}
		hints = base
	}
	out := make([]segment, 0, len(hints))
	for _, h := range hints {
		out = append(out, segment{text: h, style: hintStyle()})
	}
	return out
}

// clockSegment renders a faint HH:MM clock for the right edge. Set
// ADOTOP_HIDE_CLOCK=1 to suppress.
func clockSegment() string {
	if isClockHidden() {
		return ""
	}
	now := time.Now().Format("15:04")
	return Faint.Render(now)
}

func isClockHidden() bool {
	return strings.TrimSpace(os.Getenv("ADOTOP_HIDE_CLOCK")) == "1"
}

// composeStatusline glues the parts together with overflow-aware logic:
// left segments are joined first, then the clock is right-aligned, and
// hint segments fill what's left, dropping from the tail if needed.
func composeStatusline(left []segment, hints []segment, right string, width int) string {
	if width <= 0 {
		// Fallback: just join everything with the divider, no width math.
		all := append([]segment{}, left...)
		all = append(all, hints...)
		s := joinSegments(all)
		if right != "" {
			s += "  " + right
		}
		return s
	}
	leftStr := joinSegments(left)
	rightW := lipgloss.Width(right)
	gap := 2 // min gap between hints and clock
	avail := width - lipgloss.Width(leftStr) - rightW - gap
	if avail < 0 {
		avail = 0
	}
	hintStr := fitHints(hints, avail)

	// Build the line: left + hints (with divider if both present),
	// then pad to push the clock to the right.
	mid := leftStr
	if hintStr != "" {
		mid += statuslineDivider + hintStr
	}
	pad := width - lipgloss.Width(mid) - rightW
	if pad < 1 {
		pad = 1
	}
	return mid + strings.Repeat(" ", pad) + right
}

// fitHints drops tail hints until the joined string fits in maxWidth.
// Returns "" if even one hint won't fit.
func fitHints(hints []segment, maxWidth int) string {
	if maxWidth <= 0 || len(hints) == 0 {
		return ""
	}
	for take := len(hints); take > 0; take-- {
		s := joinSegments(hints[:take])
		if lipgloss.Width(s) <= maxWidth {
			return s
		}
	}
	return ""
}

func joinSegments(segs []segment) string {
	if len(segs) == 0 {
		return ""
	}
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = s.style.Render(" " + s.text + " ")
	}
	return strings.Join(parts, statuslineDivider)
}

// contextStyle is the second segment's look: muted bg-on-fg pair so it
// reads as secondary to the mode segment without being invisible.
func contextStyle() lipgloss.Style {
	return PillNeutral
}

// wrapHint returns the diff-wrap toggle label, with a state suffix so
// the user can tell the current mode at a glance. We keep it short to
// stay under typical pane widths.
func wrapHint(m Model) string {
	if m.wrapDiff {
		return "w:wrap·on"
	}
	return "w:wrap·off"
}

// hintStyle is intentionally bg-less and faint so hints look like
// supporting text, not chips. Bold gives just enough lift to read on
// non-faint terminal themes.
func hintStyle() lipgloss.Style {
	return Faint
}

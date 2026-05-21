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
	modeNormal statusMode = iota // neutral: routine navigation. The
	//                               mode pill is reserved for "look at
	//                               me" states (CONFIRM/MENU/ERROR);
	//                               NORMAL is the resting state and
	//                               should fade into the chrome.
	modeError   // red:  error or footer error
	modePending // yellow: confirmation prompt awaiting y/n
	modeMenu    // magenta: modal overlay (vote menu)
	modeOK      // green: write-action success banner
)

func (s statusMode) label() string {
	switch s {
	case modeError:
		return "ERROR"
	case modePending:
		return "CONFIRM"
	case modeMenu:
		return "MENU"
	case modeOK:
		return "OK"
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
	case modeOK:
		return PillGood
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

// statuslineGroupDivider separates hint *clusters* — wider than the
// inner divider between mode/context segments, so the eye can tell
// "these hints belong together" at a glance. The double box-drawing
// glyph reads as a stronger boundary without adding bg color.
const statuslineGroupDivider = "  ┃  "

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

	// Mode/context above is always shown. Hints are grouped clusters
	// rendered with the faint Footer style; clusters separate with a
	// wider divider and degrade by dropping tail clusters as a unit.
	hints := hintGroups(m)

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
	if m.footerOK != "" {
		return modeOK
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
		// Errors get the red bg so they read as the failure they are,
		// not as another piece of routine context. Without this, the
		// neutral grey-on-grey pill makes long error messages blend
		// into the chrome and the user sees only the ERROR mode label
		// with no clue what went wrong.
		return []segment{{text: m.footerErr, style: errorContextStyle()}}
	}
	if m.footerOK != "" {
		// Success messages stay neutral — informational, not blocking.
		// The green OK pill in the mode slot already carries the
		// "this is good" signal.
		return []segment{{text: m.footerOK, style: contextStyle()}}
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
		if m.viewingCommit != nil {
			// Per-commit view marker — sticks out in the cursor color
			// so the user can't miss that the diff they're reading is
			// scoped to one commit, not the full PR.
			label := fmt.Sprintf("commit %s · %s", m.viewingCommit.ShortID(), truncCols(m.viewingCommit.Subject, 40))
			segs = append(segs, segment{text: label, style: contextStyle().Foreground(Cursor.GetForeground()).Bold(true)})
		}
		segs = append(segs, segment{text: ctxLabel(m.diffCtx), style: hintStyle()})
		if chip := freshnessSegment(m.detail.LoadedAt(), m.detail.LoadedFromCache()); chip != nil {
			segs = append(segs, *chip)
		}
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

// hintGroups produces grouped keybinding hints. Each inner slice is a
// cluster of related hints rendered with no internal divider so the
// eye reads them as one unit; clusters are separated by the wider
// statuslineGroupDivider when composed.
//
// Drop strategy when the line overflows: clusters fall off the tail as
// a unit, never split mid-cluster — splitting would defeat the whole
// point of grouping.
//
// Hints are context-aware: only show what's actionable in the current
// state. The full reference lives in `?` (help). The goal is "every
// hint here is something you might press right now" — not a wall of
// 30 keys for a new user to memorize.
func hintGroups(m Model) [][]segment {
	if m.voteMenu || m.pendingAction.kind != "" {
		return nil
	}
	if m.footerErr != "" {
		return [][]segment{{{text: "press any key to dismiss", style: hintStyle()}}}
	}
	var groups [][]string
	switch m.screen {
	case screenList:
		hasRows := m.list.Rows() > 0
		_, hasSel := m.list.Selected()
		nav := []string{"j/k:move", "enter:open"}
		if hasRows {
			nav = append(nav, "/:filter", "#:goto")
		}
		groups = append(groups, nav)
		groups = append(groups, []string{"tab:next-tab"})
		if hasSel {
			groups = append(groups, []string{"o:browser"})
		}
		groups = append(groups, []string{"?:help", "q:quit"})
	case screenDetail:
		// nav-move + focus: always relevant
		nav := []string{"j/k:move", "tab:focus", "enter:diff"}
		// File-list jumps only matter in Files focus (where the list
		// actually responds). In Diff focus they'd silently no-op.
		if m.detailFocus == focusFiles {
			nav = append(nav, "n/N:file", "gg/G:top/end")
		} else {
			// Diff focus: surface viewport scroll instead.
			nav = append(nav, "pgup/pgdn:scroll")
		}
		groups = append(groups, nav)

		// Thread keys: only meaningful when threads exist on the PR
		// AND the user is in a context where they fire (Diff focus on
		// a file, or the synthetic Discussion entry selected).
		hasThreads := len(m.threads) > 0
		threadsActive := hasThreads && (m.detailFocus == focusDiff || m.detail.IsDiscussionSelected())
		if threadsActive {
			groups = append(groups, []string{"[/]:thread", "space:expand", "c:new", "C:reply", "x:resolve"})
		}
		// Jump-to-comments works whenever there are threads, in any
		// focus — the J handler auto-flips state to land you somewhere
		// readable, so hiding it would punish the user for being in
		// the "wrong" focus.
		if hasThreads {
			groups = append(groups, []string{"J:jump"})
		}
		// Show-resolved toggle only matters when the PR actually has
		// resolved threads to reveal/hide.
		if hasAnyResolved(m.threads) {
			groups = append(groups, []string{"R:show-resolved"})
		}

		// Always-on: commits picker, PR-level actions, browser, chrome.
		groups = append(groups, []string{"M:commits"})
		groups = append(groups, []string{"a:approve", "v:vote", "X:abandon"})
		// Diff-only view toggles. In Files focus they're no-ops, so
		// hiding them keeps the line honest.
		if m.detailFocus == focusDiff {
			groups = append(groups, []string{wrapHint(m), "+/-:context"})
		}
		groups = append(groups, []string{"o:browser"})
		groups = append(groups, []string{"?:help", "esc:back"})
	}
	out := make([][]segment, 0, len(groups))
	for _, g := range groups {
		segs := make([]segment, 0, len(g))
		for _, h := range g {
			segs = append(segs, segment{text: h, style: hintStyle()})
		}
		out = append(out, segs)
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
// hint clusters fill what's left, dropping from the tail if needed.
func composeStatusline(left []segment, hints [][]segment, right string, width int) string {
	if width <= 0 {
		// Fallback: just join everything with the divider, no width math.
		s := joinSegments(left)
		if hs := joinHintGroups(hints); hs != "" {
			s += statuslineDivider + hs
		}
		if right != "" {
			s += "  " + right
		}
		return s
	}
	leftStr := joinSegments(left)
	rightW := lipgloss.Width(right)
	gap := 2 // min gap between hints and clock
	avail := width - lipgloss.Width(leftStr) - rightW - gap
	if len(hints) > 0 && len(left) > 0 {
		avail -= lipgloss.Width(statuslineDivider)
	}
	if avail < 0 {
		avail = 0
	}
	hintStr := fitHintGroups(hints, avail)

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

// fitHintGroups drops tail clusters until the joined string fits in
// maxWidth. Within-cluster items are atomic: we never split a cluster
// in half, because the whole point of clustering is to keep related
// hints together.
func fitHintGroups(groups [][]segment, maxWidth int) string {
	if maxWidth <= 0 || len(groups) == 0 {
		return ""
	}
	for take := len(groups); take > 0; take-- {
		s := joinHintGroups(groups[:take])
		if lipgloss.Width(s) <= maxWidth {
			return s
		}
	}
	return ""
}

// joinHintGroups renders each cluster (no divider inside) and joins the
// clusters with the wider statuslineGroupDivider. Empty input yields "".
func joinHintGroups(groups [][]segment) string {
	if len(groups) == 0 {
		return ""
	}
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		parts = append(parts, joinHintCluster(g))
	}
	return strings.Join(parts, Faint.Render(statuslineGroupDivider))
}

// joinHintCluster renders the items in a cluster with a single space
// between them — no chip bg, no divider — so the cluster reads as one
// dense, scannable group. Each hint is rendered two-tone: the key
// glyph (text before the first ":") gets the bolder Cursor color so
// the actionable bit pops, the label after gets the faint hintStyle
// so it reads as supporting text. Items without a ":" are rendered
// uniformly faint.
func joinHintCluster(segs []segment) string {
	if len(segs) == 0 {
		return ""
	}
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = renderHintTwoTone(s.text)
	}
	return strings.Join(parts, " ")
}

// renderHintTwoTone splits a "key:label" hint and renders the two
// halves in distinct styles. The split is at the FIRST ":" only so
// labels containing ":" (rare but possible) survive intact. Hints
// without a ":" — like "press any key to dismiss" — fall through to
// uniform faint rendering.
func renderHintTwoTone(text string) string {
	idx := strings.Index(text, ":")
	if idx <= 0 || idx == len(text)-1 {
		return hintStyle().Render(text)
	}
	key := text[:idx]
	label := text[idx+1:]
	return hintKeyStyle().Render(key) + hintStyle().Render(" "+label)
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

// errorContextStyle paints the error message on the red bg so the user
// can see at a glance that the line in front of them IS the error,
// not the usual breadcrumb. Pairs with the ERROR mode pill which
// already uses the same bg.
func errorContextStyle() lipgloss.Style {
	return PillBad
}

// freshnessSegment returns the colored-dot chip that tells the user
// how old the PR snapshot they're reading is. The dot color escalates
// with age so a glance at the statusline answers "is this stale?"
// without parsing the number:
//
//	green  · live, under a minute old   — trust it
//	yellow · live, 1–5 minutes old      — probably fine, R to be sure
//	red    · live, 5+ minutes old       — likely behind the server
//	faint  · loaded from disk cache     — never re-fetched this session
//
// Returns nil when nothing has loaded yet so callers can append-or-skip
// without a placeholder segment cluttering the line. The age text is
// short (just-now / 12s / 5m / 2h) — coarser than the body-line version
// because the statusline has less real estate.
func freshnessSegment(at time.Time, fromCache bool) *segment {
	if at.IsZero() {
		return nil
	}
	age := time.Since(at)
	dotStyle := Approve // green by default
	switch {
	case fromCache:
		dotStyle = Faint
	case age >= 5*time.Minute:
		dotStyle = Reject
	case age >= time.Minute:
		dotStyle = Wait
	}
	var when string
	switch {
	case age < 5*time.Second:
		when = "just now"
	case age < time.Minute:
		when = fmt.Sprintf("%ds", int(age.Seconds()))
	case age < time.Hour:
		when = fmt.Sprintf("%dm", int(age.Minutes()))
	default:
		when = fmt.Sprintf("%dh", int(age.Hours()))
	}
	label := when
	if fromCache {
		label = when + " (cache)"
	}
	// joinSegments wraps the text in " "+text+" " and applies one
	// style to the whole thing — so we can't easily mix two colors
	// inside a segment. Pre-render the dot with its color and the
	// label with the hint style, then hand the composed string to a
	// segment whose style is a no-op passthrough.
	text := dotStyle.Render("●") + hintStyle().Render(" "+label)
	return &segment{text: text, style: passthroughStyle()}
}

// passthroughStyle is a no-op lipgloss.Style — applying it doesn't
// add color or attributes. Used when a segment's `text` is already
// pre-styled (e.g. mixed colors inside one cell) and we just need it
// to flow through joinSegments without being overpainted.
func passthroughStyle() lipgloss.Style {
	return lipgloss.NewStyle()
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

// hintKeyStyle paints the actionable key glyph in the Cursor accent
// color so the eye lands on "what to press" first. Pairs with
// hintStyle for the descriptive label half. No bg fill so it stays
// inline with the surrounding faint text rather than reading as a
// chip.
func hintKeyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Cursor.GetForeground()).Bold(true)
}

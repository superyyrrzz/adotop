package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/superyyrrzz/adotop/internal/ui/theme"
)

// Inline reference highlighting for PR comments and descriptions.
//
// ADO comment bodies are saturated with three kinds of references that
// glamour itself doesn't know about: @user mentions, !12345 PR refs,
// and AB#12345 / #12345 work-item refs. Reviewers scan for these the
// way they scan for code identifiers — making them visually distinct
// from prose meaningfully speeds up triage. Idea borrowed from mdless,
// which highlights @tags and [[wiki-links]] as a pre-render pass.
//
// Strategy: a two-stage substitution that bookends glamour.
//
//  1. Pre-glamour, in the raw markdown text, wrap each ref token in a
//     pair of sentinel bytes (\x01…\x02) that glamour passes through
//     unchanged because they're not markdown syntax.
//  2. Post-glamour, walk the rendered ANSI string and replace each
//     sentinel pair with the right SGR for the token kind. Crucially,
//     we restore the *previous* SGR sequence immediately after the
//     token's reset so glamour's surrounding document-foreground color
//     resumes seamlessly — without this, prose after a ref would lose
//     its color until the next glamour-emitted SGR.
//
// Why not post-glamour-only (regex-replace tokens in the rendered
// output)? Because glamour wraps every prose run in its document
// foreground SGR (e.g. `\x1b[38;2;205;214;243m@alice\x1b[0m`), so a
// "find the token in the rendered string and inject ANSI" pass would
// either fight that wrapping (producing nested SGR that breaks the
// surrounding color) or skip styled runs entirely (which is *every*
// run, since glamour styles all prose). The pre/post sentinel split
// dodges both problems.
//
// Why not pre-glamour-only (inject raw ANSI into the markdown)?
// Because glamour's renderer escapes or strips literal control bytes
// in the source markdown. Sentinels survive; SGR sequences do not.

// inlineRefRE matches the four ref shapes we care about, in priority
// order (longest-prefix first so `AB#123` doesn't get split by the
// `#123` arm). The token boundaries are enforced by the surrounding
// regex shape — each arm starts with a non-word character, so we can't
// match in the middle of an identifier.
//
//	@name      → user mention (letters, digits, dot, dash, underscore)
//	!12345     → PR ref (ADO's own shorthand in commit messages)
//	AB#12345   → cross-project work-item ref
//	#12345     → work-item ref (project-local)
//
// We require ≥2 digits for numeric refs so a bare `#` heading marker
// or a `!` in prose doesn't trip the highlighter. Mentions require ≥2
// chars after the `@` for the same reason.
var inlineRefRE = regexp.MustCompile(
	`(@[A-Za-z][A-Za-z0-9._-]+)` + // @mention
		`|([A-Z]{2,}#\d{2,})` + // AB#12345
		`|(!\d{2,})` + // !12345
		`|(#\d{2,})`, // #12345
)

// Sentinel bytes that bracket a ref token during glamour rendering.
// We use \x01 / \x02 (SOH / STX) because:
//   - they're not valid markdown,
//   - glamour passes them through as raw text (no escaping),
//   - they're not used anywhere else in the codebase,
//   - they're cheap to scan for in the post-render pass.
const (
	refOpen  = "\x01"
	refClose = "\x02"
)

// inlineRefStyles holds the lipgloss styles used to paint the ref
// kinds. Populated by applyInlineRefStyles from the active theme so a
// theme switch propagates the same way glamour's spec does.
//
// Color choices echo the rest of the chrome rather than inventing a
// new vocabulary:
//
//	mention    → Peach (warm, "person-shaped", distinct from links)
//	pr ref     → Identifier (blue — same as PR titles in the queue)
//	work item  → Mauve (accent — matches the pill used for status)
//
// All four are bold so they pop out of body text without needing a
// background fill (which would fight code-span backgrounds visually).
var (
	mentionStyle  lipgloss.Style
	prRefStyle    lipgloss.Style
	workItemStyle lipgloss.Style
)

func init() {
	applyInlineRefStyles(theme.New("dark"))
}

// applyInlineRefStyles repopulates the inline-ref styles from a theme.
// Called from applyStyles alongside the other theme-derived state.
func applyInlineRefStyles(t theme.Theme) {
	mentionStyle = lipgloss.NewStyle().Foreground(t.Peach).Bold(true)
	prRefStyle = lipgloss.NewStyle().Foreground(t.Identifier).Bold(true)
	workItemStyle = lipgloss.NewStyle().Foreground(t.Mauve).Bold(true)
}

// styleForRef picks the right style for a matched ref token. Branch
// order matches inlineRefRE's alternation order.
func styleForRef(tok string) lipgloss.Style {
	switch {
	case strings.HasPrefix(tok, "@"):
		return mentionStyle
	case strings.HasPrefix(tok, "!"):
		return prRefStyle
	case strings.HasPrefix(tok, "#"):
		return workItemStyle
	default:
		// AB#nnn and similar cross-project refs render as work items.
		return workItemStyle
	}
}

// markInlineRefs is the pre-glamour pass. Each ref token is wrapped in
// sentinel bytes so the post-render pass can find them in the styled
// output without doing ANSI parsing tricks.
//
// We skip refs that appear inside code spans (`` `…` ``) or fenced code
// blocks (``` ``` ```). Code is a self-contained context — bot `@names`
// or PR-shaped digits inside an example are noise, and painting them
// fights chroma's syntax highlighting.
func markInlineRefs(md string) string {
	if md == "" || !strings.ContainsAny(md, "@!#") {
		return md
	}
	// Walk line by line so we can flip a "we're inside a fenced block"
	// flag on triple-backtick lines without parsing the whole markdown.
	// Within a non-code line, we still need to skip inline code spans
	// (single-backtick runs); we do that with a per-line state walk.
	var b strings.Builder
	b.Grow(len(md) + 32)
	inFence := false
	for i, line := range strings.SplitAfter(md, "\n") {
		_ = i
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			b.WriteString(line)
			continue
		}
		if inFence {
			b.WriteString(line)
			continue
		}
		b.WriteString(markRefsAvoidingInlineCode(line))
	}
	return b.String()
}

// markRefsAvoidingInlineCode runs the ref regex over `line` but pauses
// inside inline code spans delimited by `` ` `` (single or multi-tick).
// Refs in regular prose are wrapped in sentinels; refs inside a code
// span are passed through unchanged.
func markRefsAvoidingInlineCode(line string) string {
	if !strings.Contains(line, "`") {
		return inlineRefRE.ReplaceAllStringFunc(line, func(tok string) string {
			return refOpen + tok + refClose
		})
	}
	var b strings.Builder
	b.Grow(len(line) + 8)
	i := 0
	for i < len(line) {
		if line[i] == '`' {
			// Count the run of backticks — the closing run must match.
			j := i
			for j < len(line) && line[j] == '`' {
				j++
			}
			run := line[i:j]
			// Find the matching closing run.
			rest := line[j:]
			closeIdx := strings.Index(rest, run)
			if closeIdx < 0 {
				// Unbalanced — treat the rest as prose.
				b.WriteString(inlineRefRE.ReplaceAllStringFunc(line[i:], func(tok string) string {
					return refOpen + tok + refClose
				}))
				return b.String()
			}
			// Emit the open run, the code-span contents, and the close
			// run — all unmarked.
			b.WriteString(line[i : j+closeIdx+len(run)])
			i = j + closeIdx + len(run)
			continue
		}
		// Emit a prose chunk up to the next backtick (or end of line),
		// applying ref marking only to that chunk.
		next := strings.IndexByte(line[i:], '`')
		var chunk string
		if next < 0 {
			chunk = line[i:]
			i = len(line)
		} else {
			chunk = line[i : i+next]
			i += next
		}
		b.WriteString(inlineRefRE.ReplaceAllStringFunc(chunk, func(tok string) string {
			return refOpen + tok + refClose
		}))
	}
	return b.String()
}

// resolveInlineRefs is the post-glamour pass. It walks the rendered
// ANSI string, finds each sentinel-bracketed token, and rewrites the
// region as:
//
//	<token-style-SGR> token <reset-SGR> <restore-prior-SGR>
//
// The restore step is what keeps prose colors continuous after the
// token. Without it, glamour's document foreground would be lost
// between the token's reset and the next SGR glamour emits on its own.
func resolveInlineRefs(rendered string) string {
	if !strings.Contains(rendered, refOpen) {
		return rendered
	}
	var b strings.Builder
	b.Grow(len(rendered) + 64)
	// priorSGR tracks the most recent non-reset SGR sequence we
	// emitted, so we can re-issue it after the token's reset. Empty
	// string means "no active style" — in that case we don't need to
	// restore anything.
	priorSGR := ""
	i := 0
	for i < len(rendered) {
		c := rendered[i]
		// Capture and pass through ANSI CSI sequences, updating
		// priorSGR so we can restore it after each ref token.
		if c == '\x1b' && i+1 < len(rendered) && rendered[i+1] == '[' {
			end := strings.IndexByte(rendered[i:], 'm')
			if end < 0 {
				b.WriteString(rendered[i:])
				return b.String()
			}
			seq := rendered[i : i+end+1]
			b.WriteString(seq)
			if seq == "\x1b[0m" || seq == "\x1b[m" {
				priorSGR = ""
			} else {
				priorSGR = seq
			}
			i += end + 1
			continue
		}
		if c == refOpen[0] {
			// Find the closing sentinel; if absent, treat as literal
			// (defensive: a stray \x01 shouldn't crash rendering).
			closeIdx := strings.IndexByte(rendered[i+1:], refClose[0])
			if closeIdx < 0 {
				// Drop the orphan sentinel rather than emitting a
				// control character into the user's terminal.
				i++
				continue
			}
			tok := rendered[i+1 : i+1+closeIdx]
			b.WriteString(styleForRef(tok).Render(tok))
			// Re-issue the SGR that was active *before* the token so
			// subsequent prose keeps glamour's document color.
			if priorSGR != "" {
				b.WriteString(priorSGR)
			}
			i += 1 + closeIdx + 1
			continue
		}
		if c == refClose[0] {
			// Stray close sentinel — drop it for the same reason.
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// highlightInlineRefs wraps the full lifecycle: caller passes raw
// markdown and a render function; we mark, render, resolve. Kept as a
// single helper so the commentbody pipeline stays one line.
//
// The render closure is parameterized to keep this file independent of
// glamour types — tests can pass a stub that simulates glamour's
// "wrap every prose run in document-fg SGR" behavior without depending
// on the renderer cache.
func highlightInlineRefs(md string, render func(string) (string, error)) (string, error) {
	marked := markInlineRefs(md)
	rendered, err := render(marked)
	if err != nil {
		return "", err
	}
	return resolveInlineRefs(rendered), nil
}

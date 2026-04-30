package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/charmbracelet/glamour"
)

// commentFormat is the result of sniffing a Comment.Content string. ADO
// stores comments as either HTML (web UI rich-text editor) or plain
// text; bots and power users frequently post markdown-as-text. We
// detect each so the renderer can normalize everything to markdown
// before glamour formats it for the terminal.
type commentFormat int

const (
	formatPlain commentFormat = iota
	formatHTML
	formatMarkdown
)

// htmlTagRE matches a likely HTML opening tag. We require an alpha
// character right after `<` so we don't get false-positives from things
// like `<3` or `1 < 2`.
var htmlTagRE = regexp.MustCompile(`<[a-zA-Z]+[\s>/]`)

// markdownHintRE matches common markdown syntax. We deliberately keep
// the set narrow — bot comments routinely emit `**bold**`, fenced
// code blocks, and `- item` lists, so those signals are reliable.
// Headings and inline code are weaker (a single `#` could be a hashtag,
// a single `` ` `` could be a stray backtick), so we require start-of-line
// for headings and don't trigger on a lone backtick.
var markdownHintRE = regexp.MustCompile("(?m)" + strings.Join([]string{
	`\*\*[^*\n]+\*\*`,    // **bold**
	"```",                // fenced code block
	`^\s*[-*]\s+\S`,      // unordered list item
	`^\s*\d+\.\s+\S`,     // ordered list item
	`^#{1,6}\s+\S`,       // heading
	`\[[^\]]+\]\([^)]+\)`, // inline link
}, "|"))

// detectCommentFormat sniffs raw comment text. HTML wins over markdown
// when both are present (HTML is the more invasive format and the
// converter handles inline markdown chars fine).
func detectCommentFormat(s string) commentFormat {
	if htmlTagRE.MatchString(s) {
		return formatHTML
	}
	if markdownHintRE.MatchString(s) {
		return formatMarkdown
	}
	return formatPlain
}

// glamourCache memoizes TermRenderer per width. Constructing a renderer
// is expensive (loads styles, parses CSS-like config) but pure; renders
// are cheap. The detail viewport width changes only on terminal resize,
// so this cache is bounded in practice to a handful of entries.
//
// glamourStyleName is the active glamour style ("dark"/"light") set by
// applyStyles from the theme. The cache key includes it so a theme
// switch mid-session (rare, but possible) doesn't serve a stale
// renderer with the old style.
var (
	glamourMu        sync.Mutex
	glamourCache     = map[glamourCacheKey]*glamour.TermRenderer{}
	glamourStyleName = "dark"
)

type glamourCacheKey struct {
	width int
	style string
}

func glamourRenderer(width int) *glamour.TermRenderer {
	if width <= 0 {
		width = 80
	}
	glamourMu.Lock()
	defer glamourMu.Unlock()
	style := glamourStyleName
	key := glamourCacheKey{width: width, style: style}
	if r, ok := glamourCache[key]; ok {
		return r
	}
	// glamour's WithAutoStyle picks "dark"/"light"/"notty" from the
	// surrounding terminal. The "notty" path (used in tests and pipes)
	// strips emphasis, which makes the Markdown render-loop look like
	// a no-op for `**bold**`. Force the style explicitly so emphasis is
	// always applied, and so light-theme users get a light-styled
	// markdown render that doesn't blast Mocha colors onto Latte.
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	glamourCache[key] = r
	return r
}

// renderCommentBody normalizes a raw comment body into a string ready to
// drop into the diff viewport, indented per `indent`. width bounds the
// rendered line length so the result fits the diff pane.
//
// Pipeline:
//  1. detect format (html / markdown / plain)
//  2. HTML → markdown via html-to-markdown, then unmangle (the converter
//     escapes `[` to `\[` defensively and collapses block-level markdown
//     newlines inside flow tags like <details>; both defeat glamour)
//  3. markdown → ANSI via glamour
//  4. plain → passthrough (still wrapped + indented by the caller)
//
// Returns "" only for an empty input. Errors at any stage fall back to
// the raw string so the user always sees *something* — losing comment
// content silently would be worse than ugly formatting.
func renderCommentBody(raw string, width int, indent string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var md string
	switch detectCommentFormat(raw) {
	case formatHTML:
		pre := promoteEmbeddedMarkdownHeadings(raw)
		out, err := htmltomarkdown.ConvertString(pre)
		if err != nil {
			return wrapBodyLines(raw, indent, width)
		}
		md = unmangleConvertedMarkdown(out)
	case formatMarkdown:
		md = raw
	default:
		// Plain text: skip glamour entirely. glamour would still render
		// it correctly but we'd pay for a parse + ANSI emission to
		// arrive at almost the original bytes, just word-wrapped.
		// wrapBodyLines is good enough and keeps the indent contract.
		return wrapBodyLines(raw, indent, width)
	}

	r := glamourRenderer(width - len(indent))
	if r == nil {
		return wrapBodyLines(md, indent, width)
	}
	out, err := r.Render(md)
	if err != nil {
		return wrapBodyLines(md, indent, width)
	}
	return indentLines(strings.TrimRight(out, "\n"), indent) + "\n"
}

// unmangleConvertedMarkdown post-processes html-to-markdown output to
// undo two failure modes that hurt the GitOps PR Assistant case (and
// any bot that wraps markdown inside HTML <details> / <span> shells):
//
//  1. Escape un-escaping. The converter writes `\[text](url)` whenever
//     the original text contained a `[` it can't prove was a markdown
//     link. Bot text contains lots of those (bracketed link text inside
//     `<small>` etc.). We strip the leading backslash so glamour can
//     render the link.
//  2. Block-level newline restoration. The converter flattens markdown
//     block markers (####, ##### , `- item`) onto a single line when
//     they were inside an inline-flow HTML tag. Glamour requires those
//     markers at the start of a line to recognize them. We re-inject
//     newlines before each `^####+\s` and ` - ` mid-line marker.
//
// This is intentionally narrow: the regexes target the exact patterns
// the converter mangles, not all possible escape/inline patterns.
func unmangleConvertedMarkdown(s string) string {
	// Un-escape `\[` / `\]` / `\_` / `\*` that were defensively escaped.
	s = unescapeRE.ReplaceAllString(s, "$1")
	// Inject newlines before mid-line `####` and `#####` headings.
	s = midlineHeadingRE.ReplaceAllString(s, "\n\n$1 ")
	// Inject newlines before mid-line ` - ` list items so glamour sees
	// them at column 0 of a fresh line.
	s = midlineListRE.ReplaceAllString(s, "\n- ")
	return s
}

var (
	unescapeRE       = regexp.MustCompile("\\\\([\\[\\]_*`])")
	midlineHeadingRE = regexp.MustCompile(` (#{2,6})\s+`)
	midlineListRE    = regexp.MustCompile(` - `)
	// embeddedHeadingRE matches a markdown heading marker that lives
	// inside an HTML body — `\n#### Title\n` (single-newline-terminated,
	// the way bot authors write it inside <details>). html-to-markdown
	// would collapse the trailing single newline into a space, fusing
	// heading text with the following paragraph. We rewrite it to a
	// real <h4>/<h5>/<h6> tag so the converter emits a proper block.
	embeddedHeadingRE = regexp.MustCompile(`(?m)^(#{2,6})\s+([^\n]+?)\n`)
)

// promoteEmbeddedMarkdownHeadings rewrites literal `#### Title\n` markers
// embedded in an HTML body into `<h4>Title</h4>` tags before the
// HTML→markdown conversion runs. This preserves the heading boundary
// that html-to-markdown's whitespace normalization would otherwise lose.
//
// Only triggers on lines whose first non-whitespace is `#` followed by a
// space — too narrow to ever match real prose.
func promoteEmbeddedMarkdownHeadings(s string) string {
	return embeddedHeadingRE.ReplaceAllStringFunc(s, func(m string) string {
		sub := embeddedHeadingRE.FindStringSubmatch(m)
		level := len(sub[1])
		if level < 2 || level > 6 {
			return m
		}
		return fmt.Sprintf("<h%d>%s</h%d>\n", level, sub[2], level)
	})
}

// indentLines prefixes each line of s with indent. Empty lines also
// get the indent so the bg-color rectangle of any styled block (like a
// fenced code block) stays continuous.
func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = indent + ln
	}
	return strings.Join(lines, "\n")
}

// squeezeCommentOneLine is the collapsed-form preview: detect HTML or
// markdown, normalize to a plaintext-ish form, then squeeze to one
// line capped at max chars. Used for the collapsed thread row and the
// PR-level discussion list where space is tight.
//
// Avoids the full glamour pipeline: ANSI escapes inside a one-line
// preview would conflict with the surrounding Faint/Header styles, and
// the user is about to expand to see the real thing anyway.
func squeezeCommentOneLine(raw string, max int) string {
	switch detectCommentFormat(raw) {
	case formatHTML:
		md, err := htmltomarkdown.ConvertString(raw)
		if err == nil {
			return squeezeOneLine(stripMarkdownNoise(md), max)
		}
	case formatMarkdown:
		return squeezeOneLine(stripMarkdownNoise(raw), max)
	}
	return squeezeOneLine(raw, max)
}

// stripMarkdownNoise removes the syntactic punctuation that's only
// useful when rendered (`**`, leading `#`, list bullets, fence ticks).
// We're aiming for a readable ~140-char preview — a soup of asterisks
// and backticks defeats that.
func stripMarkdownNoise(s string) string {
	// Code fences and inline code: keep the content, drop the ticks.
	s = strings.ReplaceAll(s, "```", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	// Heading/list markers at start of line.
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		t := strings.TrimLeft(ln, " ")
		switch {
		case strings.HasPrefix(t, "# "), strings.HasPrefix(t, "## "),
			strings.HasPrefix(t, "### "), strings.HasPrefix(t, "#### "),
			strings.HasPrefix(t, "##### "), strings.HasPrefix(t, "###### "):
			t = strings.TrimLeft(t, "# ")
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "):
			t = "• " + strings.TrimPrefix(strings.TrimPrefix(t, "- "), "* ")
		}
		lines[i] = t
	}
	return strings.Join(lines, "\n")
}

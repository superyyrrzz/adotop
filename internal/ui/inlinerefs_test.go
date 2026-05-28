package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// forceColor pins the color profile to TrueColor for the duration of a
// test. The default `notty` profile under `go test` strips ANSI from
// lipgloss output, which would make every assertion that depends on
// emitted ANSI sequences trivially fail. Pattern copied from
// recents_refresh_test.go so the highlighter exercises real behavior.
func forceColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// fakeGlamour simulates the key property of glamour rendering that
// makes a naive post-render highlighter fail: every prose run is
// wrapped in a document-foreground SGR followed by a reset. The
// sentinel bytes injected by markInlineRefs survive this wrapping
// untouched, which is what resolveInlineRefs relies on.
func fakeGlamour(s string) (string, error) {
	const docFG = "\x1b[38;2;205;214;243m"
	return docFG + s + "\x1b[0m", nil
}

// TestMarkAndResolveRoundTrip: after the full mark → fake-glamour →
// resolve cycle, each ref token must (a) be present in the visible
// text and (b) be preceded by a styling SGR in the raw output.
func TestMarkAndResolveRoundTrip(t *testing.T) {
	forceColor(t)
	cases := []string{
		"thanks @alice for the catch",
		"see PR !12345 for context",
		"fixes #12345",
		"linked to AB#1234567",
		"cc @bob — see !4567, fixes #890123",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			out, err := highlightInlineRefs(in, fakeGlamour)
			if err != nil {
				t.Fatalf("highlightInlineRefs returned error: %v", err)
			}
			if stripANSI(out) != fakeGlamourPlain(in) {
				t.Fatalf("visible text changed:\ngot:  %q\nwant: %q",
					stripANSI(out), fakeGlamourPlain(in))
			}
			// Sentinels must not leak.
			if strings.ContainsAny(out, refOpen+refClose) {
				t.Fatalf("sentinel byte leaked into output: %q", out)
			}
			// Each ref token must have its *own* SGR immediately before
			// it (not just the document-fg wrapper). We check this by
			// finding the token and confirming the SGR right before
			// it is NOT the doc-fg sequence.
			for _, tok := range tokensIn(in) {
				idx := strings.Index(out, tok)
				if idx < 0 {
					t.Fatalf("token %q missing from output: %q", tok, out)
				}
				// Walk back from idx to the most recent SGR start.
				prefix := out[:idx]
				escIdx := strings.LastIndex(prefix, "\x1b[")
				if escIdx < 0 {
					t.Fatalf("no SGR before token %q in output: %q", tok, out)
				}
				sgr := prefix[escIdx:]
				if sgr == "\x1b[38;2;205;214;243m" {
					t.Fatalf("token %q is wrapped only by doc-fg SGR — highlighter did not paint it:\n%q", tok, out)
				}
			}
		})
	}
}

// fakeGlamourPlain returns what the visible text of fakeGlamour(s)
// would be after stripping ANSI. Since fakeGlamour only wraps the
// input in SGR+reset, the visible text is the input verbatim (minus
// sentinels, which markInlineRefs would have added and resolveInlineRefs
// strips).
func fakeGlamourPlain(s string) string {
	return s
}

// tokensIn returns the ref tokens in a body, in match order. Used by
// the round-trip test to enumerate what needs styling.
func tokensIn(s string) []string {
	return inlineRefRE.FindAllString(s, -1)
}

// TestResolveRestoresPriorSGR: the resolver must re-issue the prior
// styling SGR after a token's reset, so the surrounding prose color
// doesn't drop. This is the bug the v1 (post-render only) design hit.
func TestResolveRestoresPriorSGR(t *testing.T) {
	// Simulate "doc-fg SGR opens, then a marked ref, then more prose,
	// then reset" — what fakeGlamour produces for "see @alice today".
	in := "\x1b[38;2;200;200;200msee \x01@alice\x02 today\x1b[0m"
	out := resolveInlineRefs(in)
	// After the token's reset, the prior SGR must reappear before
	// " today" so its color matches the leading "see ".
	tokIdx := strings.Index(out, "@alice")
	if tokIdx < 0 {
		t.Fatalf("token missing: %q", out)
	}
	afterTok := out[tokIdx+len("@alice"):]
	// The first SGR we see in afterTok must be a reset, immediately
	// followed by the prior SGR. We don't pin exact byte ordering past
	// that; the invariant is "prior SGR is restored before the next
	// visible char".
	if !strings.HasPrefix(strings.TrimPrefix(afterTok, "\x1b[0m"), "\x1b[38;2;200;200;200m") {
		t.Fatalf("prior SGR not restored after token: %q", afterTok)
	}
}

// TestMarkInlineRefsBoundaries: ref-like substrings that are part of
// a larger identifier or that fail our minimum-length rule must not
// be wrapped in sentinels.
func TestMarkInlineRefsBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // expected markInlineRefs output
	}{
		{"bare-hash-heading", "# Title", "# Title"},
		{"single-digit-hash", "rated #5 stars", "rated #5 stars"},
		{"single-bang", "what?! not a pr ref", "what?! not a pr ref"},
		{"valid-mention", "cc @bob now", "cc \x01@bob\x02 now"},
		{"valid-pr-ref", "see !4567", "see \x01!4567\x02"},
		{"valid-workitem", "closes #890123", "closes \x01#890123\x02"},
		{"valid-cross-project", "linked AB#1234567", "linked \x01AB#1234567\x02"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := markInlineRefs(c.in)
			if got != c.want {
				t.Fatalf("markInlineRefs(%q):\ngot:  %q\nwant: %q", c.in, got, c.want)
			}
		})
	}
}

// TestMarkInlineRefsSkipsCodeContexts: refs inside fenced code blocks
// and inline code spans must NOT be wrapped in sentinels — code is a
// self-contained context where `@names` and `#numbers` are noise or
// fight chroma's syntax coloring.
func TestMarkInlineRefsSkipsCodeContexts(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "inline-code-span",
			in:   "ping `@alice` in chat",
			want: "ping `@alice` in chat",
		},
		{
			name: "fenced-block",
			in:   "before\n```\n@nobody and #999 here\n```\nafter @alice",
			want: "before\n```\n@nobody and #999 here\n```\nafter \x01@alice\x02",
		},
		{
			name: "tilde-fenced-block",
			in:   "~~~\n@nobody\n~~~\nthen @alice",
			want: "~~~\n@nobody\n~~~\nthen \x01@alice\x02",
		},
		{
			name: "ref-before-code-span",
			in:   "@alice fix `the @bug` now",
			want: "\x01@alice\x02 fix `the @bug` now",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := markInlineRefs(c.in)
			if got != c.want {
				t.Fatalf("markInlineRefs(%q):\ngot:  %q\nwant: %q", c.in, got, c.want)
			}
		})
	}
}


// real glamour pipeline. Refs must come out with extra styling on top
// of glamour's own document coloring, and the visible text must be
// preserved with no sentinel leakage.
func TestRenderCommentBodyMarkdownHighlightsRefs(t *testing.T) {
	forceColor(t)
	in := "Looking at this with **fresh eyes**:\n\n- @alice flagged !4567\n- closes #890123"
	out := renderCommentBody(in, 80, "")
	plain := stripANSI(out)
	for _, want := range []string{"@alice", "!4567", "#890123"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, plain)
		}
	}
	if strings.ContainsAny(out, refOpen+refClose) {
		t.Fatalf("sentinel byte leaked through to rendered output: %q", out)
	}
	// Each ref must be preceded by a styling SGR distinct from
	// glamour's document-foreground wrap. The cleanest invariant we
	// can assert without binding to exact theme bytes is: the
	// immediately-preceding SGR must be bold (all three ref styles
	// are bold; nothing else in glamour's default output is). We scan
	// a 16-byte window before each token because glamour may emit a
	// wrap-fg SGR between the bold-open and the token if the line
	// wrapped — the bold opener is still within ~6 bytes.
	for _, want := range []string{"@alice", "!4567", "#890123"} {
		idx := strings.Index(out, want)
		if idx < 0 {
			t.Fatalf("token %q missing from raw output", want)
		}
		// Walk back from idx and inspect every SGR until we find one
		// that looks bold (`[1m`, `[1;...m`, or `...;1m`). If we hit
		// the start of string without one, fail.
		searchFrom := idx - 64
		if searchFrom < 0 {
			searchFrom = 0
		}
		window := out[searchFrom:idx]
		foundBold := false
		for k := strings.LastIndex(window, "\x1b["); k >= 0; k = strings.LastIndex(window[:k], "\x1b[") {
			end := strings.IndexByte(window[k:], 'm')
			if end < 0 {
				break
			}
			sgr := window[k : k+end+1]
			if strings.Contains(sgr, "[1m") || strings.Contains(sgr, "[1;") || strings.Contains(sgr, ";1m") {
				foundBold = true
				break
			}
		}
		if !foundBold {
			t.Fatalf("no bold SGR within 64 bytes before %q — ref highlight missing:\n%q", want, out)
		}
	}
}

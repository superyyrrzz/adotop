# Syntax Highlighting in Diff Viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add language-aware syntax highlighting to the diff preview pane on Detail screen. Each `+`/`-`/context line's code body gets tokenized with chroma using the monokai theme; the existing diff markers (gutter bar, +/- char, hunk headers, file headers) keep their current colors. Delta output remains untouched (delta already does its own highlighting).

**Architecture:** Add a thin `internal/ui/syntax.go` wrapper around chroma that returns a per-line highlighter for a given file path. `Colorize()` in `diffcolor.go` calls it once per body line right before writing the body, using the most recent file path seen in a `+++ b/...` header (so a single Colorize call across multi-file diffs still picks the right lexer per file). If the path has no matching lexer, the body is written unmodified — no failure mode.

**Tech Stack:** Go 1.26, `github.com/alecthomas/chroma/v2` (lexers + monokai style + ANSI 256-color formatter).

---

## File Structure

- `go.mod` / `go.sum` (modify) — add chroma dependency.
- `internal/ui/syntax.go` (create) — `HighlightLine(path, code string) string` plus `lexerFor(path)` helper, both pure functions; cache lexers per extension.
- `internal/ui/syntax_test.go` (create) — pin behavior: known extensions emit ANSI, unknown extensions pass through, empty input is safe.
- `internal/ui/diffcolor.go` (modify) — track the current `+++ b/<path>` header; pass body lines through `HighlightLine` before writing.
- `internal/ui/diffcolor_test.go` (modify) — extend `TestColorizeDiffMarksAddDelete` to assert that body code in a `.go` diff carries chroma-style ANSI, and add a test that unknown-extension diffs still emit plain text bodies.

No new packages outside `internal/ui`.

---

## Task 1: Add chroma dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/alecthomas/chroma/v2@latest`
Expected: `go.mod` updated with a new `require github.com/alecthomas/chroma/v2 vX.Y.Z` line; `go.sum` populated.

- [ ] **Step 2: Tidy and verify it builds**

Run: `go mod tidy && go build ./...`
Expected: no errors. The chroma binary footprint will be visible in the final `adotop.exe` (~3 MB larger).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add chroma dependency for syntax highlighting"
```

---

## Task 2: Write `HighlightLine` with extension-based lexer cache

**Files:**
- Create: `internal/ui/syntax.go`
- Create: `internal/ui/syntax_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/syntax_test.go`:

```go
package ui

import (
	"strings"
	"testing"
)

func TestHighlightLineAddsAnsiForKnownExtension(t *testing.T) {
	out := HighlightLine("foo.go", "func main() {}")
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escapes for .go input, got %q", out)
	}
	if !strings.Contains(out, "func") {
		t.Fatalf("expected literal text preserved, got %q", out)
	}
}

func TestHighlightLineUnknownExtensionIsPassthrough(t *testing.T) {
	in := "anything goes"
	out := HighlightLine("notes.zzz", in)
	if out != in {
		t.Fatalf("expected unknown extension to pass through, got %q", out)
	}
}

func TestHighlightLineEmptyInputSafe(t *testing.T) {
	if HighlightLine("foo.go", "") != "" {
		t.Fatalf("empty input should round-trip empty")
	}
}

func TestHighlightLineStripsTrailingResetNewline(t *testing.T) {
	// chroma appends a newline by default; we want a single line for inline use.
	out := HighlightLine("foo.go", "x := 1")
	if strings.HasSuffix(out, "\n") {
		t.Fatalf("expected no trailing newline, got %q", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run TestHighlightLine -v`
Expected: FAIL with `undefined: HighlightLine`.

- [ ] **Step 3: Implement `syntax.go`**

Create `internal/ui/syntax.go`:

```go
package ui

import (
	"bytes"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	lexerCache   sync.Map // map[string]chroma.Lexer (nil when none matches)
	highlighter  = formatters.Get("terminal256")
	highlightSty = styles.Get("monokai")
)

// HighlightLine tokenizes a single line of code from `path` and returns it
// wrapped in ANSI escapes using the monokai theme. If no lexer matches the
// file extension (or chroma errors), it returns `code` unchanged.
//
// Empty `code` round-trips empty. The result never contains a trailing newline,
// so callers can compose it with their own line terminators.
func HighlightLine(path, code string) string {
	if code == "" {
		return ""
	}
	lex := lexerFor(path)
	if lex == nil {
		return code
	}
	it, err := lex.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var buf bytes.Buffer
	if err := highlighter.Format(&buf, highlightSty, it); err != nil {
		return code
	}
	out := buf.String()
	out = strings.TrimRight(out, "\n")
	return out
}

func lexerFor(path string) chroma.Lexer {
	ext := strings.ToLower(filepath.Ext(path))
	if cached, ok := lexerCache.Load(ext); ok {
		if cached == nil {
			return nil
		}
		return cached.(chroma.Lexer)
	}
	lex := lexers.Match(path)
	if lex != nil {
		lex = chroma.Coalesce(lex)
	}
	if lex == nil {
		lexerCache.Store(ext, (chroma.Lexer)(nil))
		return nil
	}
	lexerCache.Store(ext, lex)
	return lex
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run TestHighlightLine -v`
Expected: all four PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/syntax.go internal/ui/syntax_test.go
git commit -m "feat(ui): add chroma-backed HighlightLine for diff bodies"
```

---

## Task 3: Track current file in `Colorize` and highlight body lines

**Files:**
- Modify: `internal/ui/diffcolor.go`
- Modify: `internal/ui/diffcolor_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/diffcolor_test.go`:

```go
func TestColorizeHighlightsCodeBodiesByPath(t *testing.T) {
	in := []byte("--- a/foo.go\n+++ b/foo.go\n@@ -1,2 +1,2 @@\n-old := 1\n+new := 2\n")
	out := string(Colorize(in))
	plus := indexLine(out, "new := 2")
	if plus == "" {
		t.Fatalf("missing plus line:\n%s", out)
	}
	// HighlightLine inserts at least one extra ANSI escape *inside* the body
	// (besides the existing red/green wrapping). Look for an SGR foreground
	// color sequence after the `+` marker.
	body := plus[strings.Index(plus, "+"):]
	if !strings.Contains(body, "\x1b[38;5;") {
		t.Fatalf("expected 256-color SGR inside .go body:\n%q", body)
	}
}

func TestColorizeUnknownExtensionStillWorks(t *testing.T) {
	in := []byte("--- a/notes.zzz\n+++ b/notes.zzz\n@@ -1 +1 @@\n+plain text body\n")
	out := string(Colorize(in))
	if !strings.Contains(out, "plain text body") {
		t.Fatalf("body missing for unknown extension:\n%s", out)
	}
	// Should still have the green gutter bar from the + line.
	if !strings.Contains(out, "\x1b[32") {
		t.Fatalf("missing green wrap on + line:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run TestColorizeHighlights -v`
Expected: FAIL — current `Colorize` writes the body verbatim without the 256-color SGR.

- [ ] **Step 3: Track path and highlight body in `Colorize`**

Replace the body of `Colorize` in `internal/ui/diffcolor.go`:

```go
func Colorize(in []byte) []byte {
	if bytes.Contains(in, []byte(headerEscape)) {
		return in
	}
	var out bytes.Buffer
	out.Grow(len(in) + len(in)/4)
	lines := strings.SplitAfter(string(in), "\n")
	currentPath := ""
	for _, line := range lines {
		body := line
		nl := ""
		if strings.HasSuffix(body, "\n") {
			body = body[:len(body)-1]
			nl = "\n"
		}
		switch {
		case strings.HasPrefix(body, "+++ "):
			currentPath = stripDiffPathPrefix(body[4:])
			out.WriteString(ansiBold)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "--- "):
			out.WriteString(ansiBold)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "@@"):
			out.WriteString(hunkBar)
			out.WriteString(ansiCyan)
			out.WriteString(body)
			out.WriteString(ansiReset)
		case strings.HasPrefix(body, "+"):
			out.WriteString(addBar)
			out.WriteString(ansiGreen)
			out.WriteString("+")
			out.WriteString(ansiReset)
			out.WriteString(HighlightLine(currentPath, body[1:]))
		case strings.HasPrefix(body, "-"):
			out.WriteString(deleteBar)
			out.WriteString(ansiRed)
			out.WriteString("-")
			out.WriteString(ansiReset)
			out.WriteString(HighlightLine(currentPath, body[1:]))
		case strings.HasPrefix(body, "diff ") || strings.HasPrefix(body, "index "):
			out.WriteString(ansiDim)
			out.WriteString(body)
			out.WriteString(ansiReset)
		default:
			out.WriteString(contextBar)
			if len(body) > 0 && body[0] == ' ' {
				out.WriteString(" ")
				out.WriteString(HighlightLine(currentPath, body[1:]))
			} else {
				out.WriteString(HighlightLine(currentPath, body))
			}
		}
		out.WriteString(nl)
	}
	return out.Bytes()
}

// stripDiffPathPrefix removes a leading "a/" or "b/" if present and trims
// trailing whitespace, leaving something filepath.Ext can use.
func stripDiffPathPrefix(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "a/") || strings.HasPrefix(s, "b/") {
		return s[2:]
	}
	return s
}
```

Note: the `+` and `-` markers are still wrapped in their own red/green ANSI so they remain visually distinct from the highlighted body. The marker is emitted, then `ansiReset`, then the highlighted body — this prevents the body's chroma colors from being clobbered by the surrounding red/green.

The original code wrapped the WHOLE body line in red/green. Existing tests like `TestColorizeDiffMarksAddDelete` look for `\x1b[31` or `\x1b[32` somewhere in the line, which is still satisfied by the marker wrap.

- [ ] **Step 4: Run all colorizer tests**

Run: `go test ./internal/ui/ -run TestColorize -v`
Expected: all PASS, including the previously-passing `TestColorizeDiffMarksAddDelete`, `TestColorizeLeavesContextUntouched`, `TestColorizeSkipsIfAlreadyColored`, `TestColorizeFileHeadersAreBold`.

If `TestColorizeLeavesContextUntouched` fails, that test feeds ` plain context line\n` with no preceding `+++` header, so `currentPath` stays empty → `lexerFor("")` returns nil → body passes through unmodified. Verify the test still passes; if not, the empty-path early-return in `lexerFor` is missing.

- [ ] **Step 5: Run the full UI suite**

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/diffcolor.go internal/ui/diffcolor_test.go
git commit -m "feat(ui): syntax-highlight diff bodies via chroma"
```

---

## Task 4: Rebuild the binary and smoke test

**Files:** none (build only)

- [ ] **Step 1: Build**

Run: `go build -o adotop.exe ./cmd/adotop`
Expected: `adotop.exe` rebuilt, ~3 MB larger.

- [ ] **Step 2: Manual smoke test**

Launch `./adotop.exe`, open a PR with a `.go` file change, and confirm:
- `+`/`-` lines have green/red gutter bars (unchanged).
- Code on those lines now shows keyword/string/comment colors from monokai.
- A `.txt` or other unknown-extension file in the same PR shows plain bodies (no chroma colors).
- `delta` output (when local clone + delta installed) is unchanged — it still has its own highlighting and was passed through by the early-return in `Colorize`.

If any of those fails, that's a bug; fix and re-run before declaring done.

---

## Self-Review Notes

- **Spec coverage:** chroma → Task 1+2. Monokai theme → hard-coded in `syntax.go` Task 2. Body-only highlighting → marker emitted before reset, body emitted after, in Task 3. Delta passthrough → preserved by the existing `bytes.Contains(in, headerEscape)` short-circuit at the top of `Colorize`, untouched by Task 3.
- **Type consistency:** `HighlightLine(path, code string) string` defined in Task 2, called with `(currentPath, body[1:])` in Task 3 — both strings, matches.
- **Placeholders:** none.
- **Risk:** chroma `lexers.Match` falls back on shebangs/content if extension fails, which would be wasted work for diff bodies. Mitigated by the per-extension cache that stores `nil` on first miss so we never call `Match` twice for the same extension.
- **Risk 2:** Some diffs have no `+++ b/...` line (delta output we already short-circuit; but raw `git diff --no-prefix`?). In that case `currentPath` stays empty and `lexerFor("")` returns nil → safe pass-through.

---

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

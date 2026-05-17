package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Org             string   `toml:"org"`
	Project         string   `toml:"project"`
	RefreshInterval Duration `toml:"refresh_interval"`
	RepoRoots       []string `toml:"repo_roots"`
	// PRIDForLiveTest, when non-zero, names the PR that the
	// build-tagged live tests under internal/ui/...live_test.go
	// should target. Lets each contributor point the test suite at
	// a PR their account can actually read, instead of hardcoding
	// an ID that's only meaningful to one tenant.
	PRIDForLiveTest int `toml:"pr_id_for_live_test"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func Default() Config {
	return Config{
		RefreshInterval: Duration{60 * time.Second},
	}
}

// Path returns the config file path. Same on every OS: ~/.adotop/config.toml.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".adotop", "config.toml"), nil
}

// Load reads the config file. Missing file returns defaults.
func Load() (Config, string, error) {
	p, err := Path()
	if err != nil {
		return Config{}, "", err
	}
	cfg := Default()
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, p, nil
		}
		return Config{}, p, fmt.Errorf("read config: %w", err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, p, fmt.Errorf("parse config %s: %w", p, err)
	}
	if cfg.RefreshInterval.Duration <= 0 {
		cfg.RefreshInterval = Default().RefreshInterval
	}
	return cfg, p, nil
}

// Exists reports whether a config file is on disk. Used by `adotop`
// (no args) to decide whether to drop into the init flow on first run
// — Load() returns defaults for a missing file, so this distinguishes
// "no config" from "config with empty org" (the latter is a user
// choice we should respect, not silently overwrite).
func Exists() (bool, error) {
	p, err := Path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Write atomically persists the given config to disk under
// ~/.adotop/config.toml. Creates the parent directory as needed. The
// rendered TOML is hand-written rather than marshaled because
// BurntSushi/toml has no encoder; this keeps the dependency surface
// minimal and the on-disk format predictable for users who want to
// edit by hand later.
//
// Unknown top-level keys and tables from the existing file (e.g. a
// field added by a newer adotop version, or a user's own scratch
// section) are preserved verbatim, appended after the rendered known
// fields. This guarantees that an older binary writing a newer file
// doesn't silently drop data.
func Write(cfg Config) (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return p, fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	var prior string
	if existing, err := os.ReadFile(p); err == nil {
		prior = string(existing)
	}
	body := renderTOML(cfg)
	if extra := extractUnknownBlocks(prior); extra != "" {
		body += "\n# Preserved from prior config (keys this adotop build doesn't recognize).\n"
		body += "# Newer adotop versions may consume these — leaving them in place.\n"
		body += extra
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return p, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return p, fmt.Errorf("rename %s -> %s: %w", tmp, p, err)
	}
	return p, nil
}

// renderTOML formats a Config as the same shape Load() reads. Only
// non-zero fields are emitted so a fresh config doesn't ship with
// `pr_id_for_live_test = 0` or `repo_roots = []` lines that just
// confuse new users.
func renderTOML(cfg Config) string {
	var b strings.Builder
	b.WriteString("# adotop config — see https://github.com/superyyrrzz/adotop\n\n")
	if cfg.Org != "" {
		fmt.Fprintf(&b, "org              = %q\n", cfg.Org)
	}
	if cfg.Project != "" {
		fmt.Fprintf(&b, "project          = %q\n", cfg.Project)
	}
	if cfg.RefreshInterval.Duration > 0 {
		fmt.Fprintf(&b, "refresh_interval = %q\n", cfg.RefreshInterval.String())
	}
	if len(cfg.RepoRoots) > 0 {
		b.WriteString("repo_roots       = [")
		for i, r := range cfg.RepoRoots {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", r)
		}
		b.WriteString("]\n")
	}
	if cfg.PRIDForLiveTest != 0 {
		fmt.Fprintf(&b, "pr_id_for_live_test = %d\n", cfg.PRIDForLiveTest)
	}
	return b.String()
}

// knownKeys is the set of top-level TOML names this binary understands.
// extractUnknownBlocks consults this when deciding what to drop versus
// preserve verbatim. Keep in sync with the Config struct's toml tags.
var knownKeys = map[string]bool{
	"org":                 true,
	"project":             true,
	"refresh_interval":    true,
	"repo_roots":          true,
	"pr_id_for_live_test": true,
}

// extractUnknownBlocks scans the prior on-disk config and returns the
// verbatim text of every top-level block whose key (or table header)
// is not in knownKeys. The goal is round-trip safety: an older binary
// writing the file must not silently drop fields a newer one added.
//
// A "block" is the line starting a top-level assignment or table
// header, plus any continuation lines (multi-line arrays, table body
// rows) up to but not including the next top-level key, table header,
// or end of file. Blank/comment lines preceding a block are attached
// to it as leading context so user-written comments travel with their
// key when preserved.
//
// Inputs we don't try to parse: inline tables on the right-hand side
// of a known key (we never emit those), and any TOML construct that
// the BurntSushi loader would have rejected outright (Load returns an
// error before Write runs, so malformed input never reaches here).
func extractUnknownBlocks(prior string) string {
	if strings.TrimSpace(prior) == "" {
		return ""
	}
	lines := strings.Split(prior, "\n")
	// Strip a trailing empty element introduced by a final "\n".
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	type block struct {
		key      string // top-level key name or table header name; "" until known
		isTable  bool
		lines    []string
		bracket  int // running [ ] depth for multi-line arrays
		braceDep int // running { } depth for multi-line inline tables (rare)
	}
	var blocks []block
	var pending []string // blank/comment lines waiting for the next block

	flush := func(b block) {
		blocks = append(blocks, b)
	}

	cur := block{}
	inBlock := false
	for _, raw := range lines {
		trim := strings.TrimSpace(raw)
		if !inBlock {
			// Blank or pure-comment lines: hold until the next real block.
			if trim == "" || strings.HasPrefix(trim, "#") {
				pending = append(pending, raw)
				continue
			}
			// Table header?
			if strings.HasPrefix(trim, "[") {
				name := tableName(trim)
				cur = block{key: name, isTable: true, lines: append(append([]string{}, pending...), raw)}
				pending = nil
				inBlock = true
				continue
			}
			// Top-level assignment?
			if k, ok := topLevelKey(trim); ok {
				cur = block{key: k, lines: append(append([]string{}, pending...), raw)}
				pending = nil
				cur.bracket = bracketDelta(raw)
				cur.braceDep = braceDelta(raw)
				if cur.bracket > 0 || cur.braceDep > 0 {
					inBlock = true
				} else {
					flush(cur)
					cur = block{}
				}
				continue
			}
			// Unrecognized stray line — keep it as pending so it stays
			// attached to the next block (or trails harmlessly if EOF).
			pending = append(pending, raw)
			continue
		}
		// Inside a block. Continuation until brackets/braces close, or
		// until we hit a new top-level key / table header (which means
		// the prior block was the last assignment in a table body).
		if cur.isTable {
			if strings.HasPrefix(trim, "[") {
				flush(cur)
				name := tableName(trim)
				cur = block{key: name, isTable: true, lines: []string{raw}}
				continue
			}
			cur.lines = append(cur.lines, raw)
			continue
		}
		// Scalar/array assignment block — track bracket depth.
		cur.lines = append(cur.lines, raw)
		cur.bracket += bracketDelta(raw)
		cur.braceDep += braceDelta(raw)
		if cur.bracket <= 0 && cur.braceDep <= 0 {
			flush(cur)
			cur = block{}
			inBlock = false
		}
	}
	if inBlock {
		flush(cur)
	}
	// Trailing pending lines (orphan comments at EOF) are dropped —
	// they belong to no key, and rewriting them under the "preserved"
	// banner would just confuse the user.

	var b strings.Builder
	for _, blk := range blocks {
		if knownKeys[blk.key] {
			continue
		}
		for _, l := range blk.lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// topLevelKey returns the bare key name from a TOML assignment line
// like `foo = 1` or `foo.bar = 1`. Returns ("", false) if the line is
// not a top-level assignment. We treat dotted keys as belonging to
// their first segment, since that's how knownKeys is indexed.
func topLevelKey(line string) (string, bool) {
	// Strip any trailing comment so we don't include it in the key scan.
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	eq := strings.Index(line, "=")
	if eq <= 0 {
		return "", false
	}
	key := strings.TrimSpace(line[:eq])
	if key == "" {
		return "", false
	}
	// Dotted-key: take the first segment.
	if dot := strings.Index(key, "."); dot >= 0 {
		key = key[:dot]
	}
	// Quoted keys: strip the quotes.
	key = strings.Trim(key, `"'`)
	// Reject anything that doesn't look like a bare ident — we don't
	// want to mis-detect e.g. an array element line.
	for _, r := range key {
		if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return "", false
		}
	}
	return key, true
}

// tableName extracts the bare header name from a `[name]` or
// `[name.sub]` line. We track tables by their first segment because
// knownKeys is flat; a sub-table under a known table still counts as
// known (we'd never expect a partial schema split).
func tableName(line string) string {
	// Strip trailing comment.
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "[[")
	line = strings.TrimPrefix(line, "[")
	line = strings.TrimSuffix(line, "]]")
	line = strings.TrimSuffix(line, "]")
	name := strings.TrimSpace(line)
	if dot := strings.Index(name, "."); dot >= 0 {
		name = name[:dot]
	}
	return strings.Trim(name, `"'`)
}

// bracketDelta counts `[` minus `]` in a line, ignoring those inside
// string literals and trailing comments. Used to track when a multi-
// line array opens and closes across lines.
func bracketDelta(line string) int {
	return delimDelta(line, '[', ']')
}

// braceDelta is the same for `{` and `}` so multi-line inline tables
// (rare but legal) don't terminate the block early.
func braceDelta(line string) int {
	return delimDelta(line, '{', '}')
}

func delimDelta(line string, open, close rune) int {
	depth := 0
	inStr := false
	var strQuote rune
	for i := 0; i < len(line); i++ {
		c := rune(line[i])
		if inStr {
			if c == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if c == strQuote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			strQuote = c
		case '#':
			return depth // comment — rest of line ignored
		case open:
			depth++
		case close:
			depth--
		}
	}
	return depth
}

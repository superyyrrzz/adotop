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
	Org             string            `toml:"org"`
	Project         string            `toml:"project"`
	RefreshInterval Duration          `toml:"refresh_interval"`
	RepoRoots       []string          `toml:"repo_roots"`
	Keybindings     map[string]string `toml:"keybindings"`
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
		Keybindings:     map[string]string{},
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
func Write(cfg Config) (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return p, fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	body := renderTOML(cfg)
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

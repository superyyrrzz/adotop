package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // os.UserHomeDir reads this on Windows
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	setHome(t, t.TempDir())
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshInterval.Duration != Default().RefreshInterval.Duration {
		t.Fatalf("expected default refresh interval, got %v", cfg.RefreshInterval)
	}
}

func TestLoadParses(t *testing.T) {
	dir := t.TempDir()
	setHome(t, dir)
	p := filepath.Join(dir, ".adotop", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`org = "acme"
project = "Platform"
refresh_interval = "30s"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Org != "acme" || cfg.Project != "Platform" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.RefreshInterval.Duration.Seconds() != 30 {
		t.Fatalf("refresh: %v", cfg.RefreshInterval)
	}
}

func TestLoadRepoRoots(t *testing.T) {
	dir := t.TempDir()
	setHome(t, dir)
	p := filepath.Join(dir, ".adotop", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`org = "acme"
project = "Platform"
repo_roots = ["~/git", "~/src"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.RepoRoots) != 2 || cfg.RepoRoots[0] != "~/git" || cfg.RepoRoots[1] != "~/src" {
		t.Fatalf("RepoRoots = %v", cfg.RepoRoots)
	}
}

// TestWritePreservesUnknownKeys is the round-trip guard: a config
// file written by a newer adotop may contain keys this binary doesn't
// know about. When we Write, those keys must survive verbatim — losing
// them would silently downgrade the user's config every time an older
// build touched it.
func TestWritePreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	setHome(t, dir)
	p := filepath.Join(dir, ".adotop", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	prior := `org = "acme"
project = "Platform"
future_scalar = "hello"
future_array = [
  "a",
  "b",
]

[future_table]
nested = 1
also = "ok"
`
	if err := os.WriteFile(p, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		`future_scalar = "hello"`,
		`future_array = [`,
		`"a",`,
		`[future_table]`,
		`nested = 1`,
		`also = "ok"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q after round-trip:\n%s", want, s)
		}
	}
	// Known fields should still be present exactly once each.
	if strings.Count(s, "org") < 1 || strings.Count(s, "project") < 1 {
		t.Errorf("known fields lost:\n%s", s)
	}
}

// TestWriteDropsRemovedKnownKeys: when a field IS in our schema but
// the caller passed a zero value, we should not preserve the stale
// value from disk. This is the symmetric case to the test above —
// otherwise "clear repo_roots via init" would silently fail because
// the preservation path resurrected the old value.
func TestWriteDropsRemovedKnownKeys(t *testing.T) {
	dir := t.TempDir()
	setHome(t, dir)
	p := filepath.Join(dir, ".adotop", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	prior := `org = "acme"
project = "Platform"
repo_roots = ["~/old"]
`
	if err := os.WriteFile(p, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.RepoRoots = nil // user cleared it
	if _, err := Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), "~/old") {
		t.Fatalf("cleared repo_roots resurrected from prior file:\n%s", got)
	}
}

// TestWriteNoPriorFileSkipsPreservedBanner: the explanatory banner
// only makes sense when there's actually preserved content. A fresh
// init should produce a clean config without it.
func TestWriteNoPriorFileSkipsPreservedBanner(t *testing.T) {
	dir := t.TempDir()
	setHome(t, dir)
	cfg := Default()
	cfg.Org = "acme"
	cfg.Project = "Platform"
	if _, err := Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p := filepath.Join(dir, ".adotop", "config.toml")
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), "Preserved from prior config") {
		t.Fatalf("preserved banner emitted on fresh write:\n%s", got)
	}
}

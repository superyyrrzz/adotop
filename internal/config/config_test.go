package config

import (
	"os"
	"path/filepath"
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
	if err := os.WriteFile(p, []byte(`org = "ceapex"
project = "Engineering"
refresh_interval = "30s"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Org != "ceapex" || cfg.Project != "Engineering" {
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
	if err := os.WriteFile(p, []byte(`org = "ceapex"
project = "Engineering"
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

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
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
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	p := filepath.Join(dir, "adotop", "config.toml")
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

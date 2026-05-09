package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superyyrrzz/adotop/internal/config"
)

// TestPrintUsageShape covers the smoke contract of `adotop --help`:
// the printed text must list each top-level invocation we promise so
// users learn what's there. Catches a regression where someone adds
// a subcommand and forgets to advertise it.
func TestPrintUsageShape(t *testing.T) {
	var out bytes.Buffer
	printUsage(&out)
	want := []string{
		"adotop init",
		"adotop --version",
		"adotop --help",
		"<pr-id>",
		"<pr-url>",
		"~/.adotop/logs/adotop.log",
	}
	for _, w := range want {
		if !strings.Contains(out.String(), w) {
			t.Fatalf("usage missing %q:\n%s", w, out.String())
		}
	}
}

// TestInitWritesConfig: piping org + project answers through stdin
// must produce a readable config.toml with those values. Exercises
// the full prompt → write → reload contract.
func TestInitWritesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	in := strings.NewReader("acme\nPlatform\n")
	var out bytes.Buffer
	if err := runInitWith(in, &out); err != nil {
		t.Fatalf("runInitWith: %v  out=%q", err, out.String())
	}

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Org != "acme" || cfg.Project != "Platform" {
		t.Fatalf("config not persisted: %+v", cfg)
	}
}

// TestInitPreservesExistingConfigOnBareEnters: with a config already
// present, hitting enter through both prompts must keep the existing
// org/project unchanged AND the prompts must show those values in
// [brackets] so the user knows what they're keeping. The earlier
// "Overwrite? (y/N)" gate is gone — editing-by-default is the UX,
// since unprompted fields are preserved either way.
func TestInitPreservesExistingConfigOnBareEnters(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cfgPath := filepath.Join(dir, ".adotop", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`org = "acme"`+"\n"+`project = "Platform"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := strings.NewReader("\n\n")
	var out bytes.Buffer
	if err := runInitWith(in, &out); err != nil {
		t.Fatalf("runInitWith: %v  out=%q", err, out.String())
	}

	cfg, _, _ := config.Load()
	if cfg.Org != "acme" || cfg.Project != "Platform" {
		t.Fatalf("existing values not preserved on bare enter: %+v", cfg)
	}
	if !strings.Contains(out.String(), "[acme]") || !strings.Contains(out.String(), "[Platform]") {
		t.Fatalf("expected current values in [brackets]; got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Editing") {
		t.Fatalf("expected 'Editing' header; got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Overwrite?") {
		t.Fatalf("Overwrite? prompt should be gone; got:\n%s", out.String())
	}
}

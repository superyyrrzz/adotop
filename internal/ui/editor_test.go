package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// We can't actually launch vim in unit tests, so we point composeWithEditor
// at a tiny shell/batch script that writes a known string to the temp
// file. The contract under test: caller seeds → editor mutates → caller
// reads back the mutation.
func TestComposeWithEditor_ReadsBackEditedFile(t *testing.T) {
	dir := t.TempDir()
	var editor string
	if runtime.GOOS == "windows" {
		editor = filepath.Join(dir, "fake-editor.bat")
		if err := os.WriteFile(editor, []byte("@echo off\r\necho edited body> %1\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		editor = filepath.Join(dir, "fake-editor.sh")
		if err := os.WriteFile(editor, []byte("#!/bin/sh\necho 'edited body' > \"$1\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := composeWithEditor("seed text\n", editor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "edited body") {
		t.Fatalf("expected edited body, got %q", got)
	}
}

func TestComposeWithEditor_EmptyResultReturnsBlank(t *testing.T) {
	dir := t.TempDir()
	var editor string
	if runtime.GOOS == "windows" {
		editor = filepath.Join(dir, "noop.bat")
		_ = os.WriteFile(editor, []byte("@echo off\r\ntype nul > %1\r\n"), 0o755)
	} else {
		editor = filepath.Join(dir, "noop.sh")
		_ = os.WriteFile(editor, []byte("#!/bin/sh\n: > \"$1\"\n"), 0o755)
	}
	got, err := composeWithEditor("seed", editor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("expected empty when editor cleared file, got %q", got)
	}
}

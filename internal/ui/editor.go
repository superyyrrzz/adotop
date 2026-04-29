package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// composeWithEditor seeds a temp file with `seed`, launches `editor` on
// it, then returns the file's contents after the editor exits. The
// editor string is split on whitespace so flag-bearing commands like
// "code -w" or "vim -c 'set ft=markdown'" work — first token is the
// program, rest are flags, the temp path is appended last.
//
// NOTE: this is a synchronous foreground call. Bubble Tea callers MUST
// use tea.ExecProcess (or equivalent) so the TUI surrenders the TTY
// before this runs; otherwise the editor and the renderer fight for it.
func composeWithEditor(seed, editor string) (string, error) {
	if strings.TrimSpace(editor) == "" {
		return "", fmt.Errorf("no editor configured")
	}
	f, err := os.CreateTemp("", "adotop-comment-*.md")
	if err != nil {
		return "", err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if _, err := f.WriteString(seed); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	parts := strings.Fields(editor)
	args := append(parts[1:], tmpPath)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %q exited with error: %w", filepath.Base(parts[0]), err)
	}
	b, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// resolveEditor returns the user's editor command. Honors $VISUAL then
// $EDITOR, falling back to platform defaults so the feature works
// without configuration.
func resolveEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

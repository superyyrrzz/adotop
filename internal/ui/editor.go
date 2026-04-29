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
// The seedAndCmd / readEditedFile pair below is what the Bubble Tea
// integration uses.
func composeWithEditor(seed, editor string) (string, error) {
	tmpPath, err := writeSeedFile(seed)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)
	cmd, err := buildEditorCmd(editor, tmpPath)
	if err != nil {
		return "", err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %q exited with error: %w", filepath.Base(strings.Fields(editor)[0]), err)
	}
	b, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// writeSeedFile creates a temp markdown file with the given seed content
// and returns its path. Caller is responsible for os.Remove on the path.
func writeSeedFile(seed string) (string, error) {
	f, err := os.CreateTemp("", "adotop-comment-*.md")
	if err != nil {
		return "", err
	}
	tmpPath := f.Name()
	if _, err := f.WriteString(seed); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

// buildEditorCmd parses an editor command string ("vim", "code -w",
// etc.) and returns an *exec.Cmd ready to run with the temp path
// appended as the final argument. Returns an error when the editor
// string is empty.
func buildEditorCmd(editor, tmpPath string) (*exec.Cmd, error) {
	if strings.TrimSpace(editor) == "" {
		return nil, fmt.Errorf("no editor configured")
	}
	parts := strings.Fields(editor)
	args := append(parts[1:], tmpPath)
	return exec.Command(parts[0], args...), nil
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

// trimSeedAndComments strips HTML-style comment lines (the seed
// instructions we wrote in) from the editor output and trims surrounding
// whitespace. Empty result == cancel.
func trimSeedAndComments(body string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "<!--") && strings.HasSuffix(t, "-->") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

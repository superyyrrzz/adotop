package gitlocal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffProducesUnifiedDiff(t *testing.T) {
	gitOK(t)
	dir := t.TempDir()
	mustRun(t, "", "git", "init", "-b", "main", dir)
	mustRun(t, dir, "git", "config", "user.email", "t@t")
	mustRun(t, dir, "git", "config", "user.name", "t")

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", "f.txt")
	mustRun(t, dir, "git", "commit", "-m", "init")
	tgt := commitSha(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nB\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", "f.txt")
	mustRun(t, dir, "git", "commit", "-m", "change")
	src := commitSha(t, dir)

	out, err := Diff(context.Background(), dir, tgt, src, "f.txt", false, 3)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "-b") || !strings.Contains(s, "+B") {
		t.Fatalf("diff missing changes: %s", s)
	}
}

func commitSha(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

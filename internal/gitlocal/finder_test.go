package gitlocal

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func TestFindMatchesByRemote(t *testing.T) {
	gitOK(t)
	root := t.TempDir()
	repo := filepath.Join(root, "MyRepo")
	mustRun(t, "", "git", "init", "-b", "main", repo)
	mustRun(t, repo, "git", "remote", "add", "origin", "https://dev.azure.com/acme/Platform/_git/MyRepo")

	f := New([]string{root})
	got, ok := f.Find("MyRepo", "acme")
	if !ok {
		t.Fatal("expected to find MyRepo")
	}
	if got != repo {
		t.Fatalf("got %q want %q", got, repo)
	}
}

func TestFindRejectsWrongRemote(t *testing.T) {
	gitOK(t)
	root := t.TempDir()
	repo := filepath.Join(root, "MyRepo")
	mustRun(t, "", "git", "init", "-b", "main", repo)
	mustRun(t, repo, "git", "remote", "add", "origin", "https://github.com/x/MyRepo")

	f := New([]string{root})
	if _, ok := f.Find("MyRepo", "acme"); ok {
		t.Fatal("expected no match — remote points elsewhere")
	}
}

func TestFindCachesLookup(t *testing.T) {
	gitOK(t)
	root := t.TempDir()
	repo := filepath.Join(root, "MyRepo")
	mustRun(t, "", "git", "init", "-b", "main", repo)
	mustRun(t, repo, "git", "remote", "add", "origin", "https://dev.azure.com/acme/_git/MyRepo")

	f := New([]string{root})
	if _, ok := f.Find("MyRepo", "acme"); !ok {
		t.Fatal("first lookup failed")
	}
	mustRun(t, repo, "git", "remote", "remove", "origin")
	got, ok := f.Find("MyRepo", "acme")
	if !ok || got != repo {
		t.Fatalf("cached lookup lost: ok=%v got=%q", ok, got)
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}

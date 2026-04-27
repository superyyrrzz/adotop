package ui

import (
	"strings"
	"testing"
)

func TestDiffViewShowsRendererBadge(t *testing.T) {
	m := NewDiff(DefaultKeys())
	m = m.SetHeader("/src/login.go", "local+delta")
	m, _ = m.Update(diffLoadedMsg{content: []byte("--- a/src/login.go\n+++ b/src/login.go\n-old\n+new\n")})
	out := m.View()
	if !strings.Contains(out, "/src/login.go") || !strings.Contains(out, "local+delta") {
		t.Fatalf("header missing:\n%s", out)
	}
	if !strings.Contains(out, "+new") {
		t.Fatalf("body missing:\n%s", out)
	}
}

func TestDiffViewShowsErrorOnFailure(t *testing.T) {
	m := NewDiff(DefaultKeys())
	m = m.SetHeader("/x", "rest")
	m, _ = m.Update(diffLoadedMsg{err: errString("boom")})
	out := m.View()
	if !strings.Contains(out, "boom") {
		t.Fatalf("error not shown:\n%s", out)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestDiffSetHeaderKeepsPriorContentWhenReloading(t *testing.T) {
	m := NewDiff(DefaultKeys()).SetSize(40, 10)
	m, _ = m.Update(diffLoadedMsg{
		content: []byte("--- a/x\n+++ b/x\n+hi\n"),
		target:  diffTargetPreview,
	})
	if !strings.Contains(m.vp.View(), "hi") {
		t.Fatalf("preload setup failed:\n%s", m.vp.View())
	}
	m = m.SetHeader("/x", "local")
	if !strings.Contains(m.vp.View(), "hi") {
		t.Fatalf("expected prior content to remain after SetHeader; got:\n%s", m.vp.View())
	}
	if !m.reloading {
		t.Fatalf("expected reloading flag to be true while waiting for new diff")
	}
}

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestWrapKeyTogglesFlag is the regression guard for the `w` binding.
// Each press flips m.wrapDiff. Off by default per the design (most
// diffs don't need wrap; users opt in for files with long lines).
func TestWrapKeyTogglesFlag(t *testing.T) {
	m := newDetailModel(t)
	if m.wrapDiff {
		t.Fatalf("wrapDiff should default to off")
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = mm.(Model)
	if !m.wrapDiff {
		t.Fatalf("first w press did not enable wrap")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = mm.(Model)
	if m.wrapDiff {
		t.Fatalf("second w press did not disable wrap")
	}
}

// TestWrapHintReflectsState ensures the statusline shows the current
// mode so users can see whether wrap is on without trial-and-error.
func TestWrapHintReflectsState(t *testing.T) {
	m := newTestModel()
	if got := wrapHint(m); !strings.Contains(got, "off") {
		t.Fatalf("hint should say off when wrap disabled; got %q", got)
	}
	m.wrapDiff = true
	if got := wrapHint(m); !strings.Contains(got, "on") {
		t.Fatalf("hint should say on when wrap enabled; got %q", got)
	}
}

// TestRefreshPreviewWithWrapWritesWrappedContent is the wiring guard:
// after a diff is loaded into the preview cache, toggling wrap=on must
// re-feed the viewport with wrapped content. Without this the toggle
// would flip the flag but leave the screen unchanged until the next
// j/k.
func TestRefreshPreviewWithWrapWritesWrappedContent(t *testing.T) {
	m := newDetailModel(t)
	// Stage a tiny preview cache entry so refreshPreview has something
	// to render. Use a long + line that we can confirm wraps.
	body := strings.Repeat("X", 200)
	raw := []byte("--- a/x\n+++ b/x\n@@ -1 +1 @@\n+" + body + "\n")
	prID := m.detail.Summary().ID
	m.previewKey = "test-key"
	m.previewCache.Set(prID, m.previewKey, raw)
	// Mark preview as loaded with the unwrapped content so the early-
	// return short-circuit is the one we're exercising.
	m.preview = m.preview.SetSize(40, 20)
	m.preview.loaded = true
	rendered, _ := m.previewCache.Rendered(m.previewKey)
	m.preview.vp.SetContent(rendered)

	// Sanity: viewport has the long line as one row right now.
	preWrapView := m.preview.vp.View()

	// Flip wrap on and let refreshPreview rewrite the content.
	m.wrapDiff = true
	m = m.refreshPreview()
	wrappedView := m.preview.vp.View()

	if wrappedView == preWrapView {
		t.Fatalf("refreshPreview did not rewrite viewport when wrap turned on")
	}
	// After wrap, the viewport's rendered content should contain at
	// least one continuation marker.
	if !strings.Contains(wrappedView, "…") {
		t.Fatalf("wrapped viewport missing continuation marker:\n%s", wrappedView)
	}
}

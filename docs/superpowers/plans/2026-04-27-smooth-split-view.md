# Smooth Split-View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Detail screen the only diff surface — never push a second screen — and make file-to-file movement feel native via focus toggling, per-file scroll memory, neighbor prefetch, and non-jarring loading states.

**Architecture:** Detail keeps its existing left file list + right preview viewport, but gains a focus model (`focusFiles` ↔ `focusDiff`) toggled by `tab`. When diff has focus, scroll keys route to the preview viewport and `n`/`N` walks files without losing focus. The full-screen `screenDiff` is removed entirely. Per-file viewport y-offsets are remembered in a `map[string]int`. A small `map[string][]byte` cache holds the last-N diff bodies so cursor moves usually paint instantly; on every cursor move we kick off prefetches for the file immediately above and below. The preview keeps its previous content visible while a new load is in flight (no flash to "loading…").

**Tech Stack:** Go 1.26, Bubble Tea (charmbracelet/bubbletea), bubbles/viewport, lipgloss, runewidth.

---

## File Structure

- `internal/ui/app.go` (modify) — focus state, key routing, prefetch cache, scroll memory; rip out screenDiff.
- `internal/ui/diff.go` (modify) — `SetHeader` no longer blanks viewport; add `MarkLoading`/`Reloading` flag; render a faint title-row hint while reloading.
- `internal/ui/detail.go` (modify) — render a focus chevron next to the "Files" header.
- `internal/ui/app_test.go` (create) — coordinator-level tests for focus toggle, n/N walk, scroll memory, prefetch reuse.
- `internal/ui/diff_test.go` (modify) — keep-stale behavior test.

No new packages.

---

## Task 1: Add focus enum and `tab`/`shift+tab` toggle

**Files:**
- Modify: `internal/ui/app.go` (add field + key handling in `updateDetailScreen`)
- Create: `internal/ui/app_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/app_test.go`:

```go
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/renzeyu/adotop/internal/ado"
	"github.com/renzeyu/adotop/internal/config"
)

func newDetailModel(t *testing.T) Model {
	t.Helper()
	m := New(config.Config{}, nil)
	m.screen = screenDetail
	m.detail = m.detail.SetSummary(ado.PRSummary{ID: 1, Title: "x"})
	d, _ := m.detail.Update(detailLoadedMsg{detail: &ado.PRDetail{
		PRSummary: ado.PRSummary{ID: 1, Title: "x"},
		SourceSha: "src", TargetSha: "tgt",
	}})
	m.detail = d
	d, _ = m.detail.Update(filesLoadedMsg{files: []ado.FileChange{
		{Path: "/a.go", ChangeType: "edit"},
		{Path: "/b.go", ChangeType: "edit"},
		{Path: "/c.go", ChangeType: "edit"},
	}})
	m.detail = d
	return m
}

func TestDetailTabTogglesFocus(t *testing.T) {
	m := newDetailModel(t)
	if m.detailFocus != focusFiles {
		t.Fatalf("expected default focusFiles, got %v", m.detailFocus)
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	if m.detailFocus != focusDiff {
		t.Fatalf("tab should switch to focusDiff, got %v", m.detailFocus)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = mm.(Model)
	if m.detailFocus != focusFiles {
		t.Fatalf("shift+tab should switch back to focusFiles, got %v", m.detailFocus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDetailTabTogglesFocus -v`
Expected: FAIL with `m.detailFocus undefined` / `focusFiles undefined`.

- [ ] **Step 3: Add the focus type and field**

In `internal/ui/app.go`, add near the `screen` constants:

```go
type detailFocus int

const (
	focusFiles detailFocus = iota
	focusDiff
)
```

In the `Model` struct, add the field (after `previewKey`):

```go
	detailFocus detailFocus
```

- [ ] **Step 4: Wire `tab`/`shift+tab` in `updateDetailScreen`**

In `updateDetailScreen` (`internal/ui/app.go`), at the top of the switch, add:

```go
	case msg.Type == tea.KeyTab:
		if m.detailFocus == focusFiles {
			m.detailFocus = focusDiff
		} else {
			m.detailFocus = focusFiles
		}
		return m, nil
	case msg.Type == tea.KeyShiftTab:
		if m.detailFocus == focusDiff {
			m.detailFocus = focusFiles
		} else {
			m.detailFocus = focusDiff
		}
		return m, nil
```

Note: the surrounding switch uses `keyMatches(msg, m.keys.X)`; the new cases use bare `msg.Type ==` because tab/shift+tab aren't (yet) in `KeyMap`. That's intentional — tab is overloaded with NextTab on the list screen, but Detail has no tabs to switch.

- [ ] **Step 5: Reset focus when leaving Detail**

In `updateDetailScreen`, where `keyMatches(msg, m.keys.Back)` returns to list, also reset:

```go
	case keyMatches(msg, m.keys.Back):
		m.screen = screenList
		m.detailFocus = focusFiles
		return m, nil
```

And in `updateListScreen` where `Open` enters Detail, set focus explicitly:

```go
		if s, ok := m.list.Selected(); ok {
			m.detail = m.detail.SetSummary(s)
			m.preview = m.sizeDiffModel(NewDiff(m.keys), diffTargetPreview)
			m.previewKey = ""
			m.detailFocus = focusFiles
			m.screen = screenDetail
			return m, m.loadDetail(s)
		}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDetailTabTogglesFocus -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): add tab/shift+tab focus toggle on Detail screen"
```

---

## Task 2: Route scroll keys to the preview pane when diff has focus

**Files:**
- Modify: `internal/ui/app.go` (`updateDetailScreen`)
- Modify: `internal/ui/app_test.go` (add test)

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestDetailDiffFocusRoutesScrollKeys(t *testing.T) {
	m := newDetailModel(t)
	// Pre-fill preview with multi-line content so PgDn moves the offset.
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content:   []byte(strings.Repeat("ctx\n", 200)),
		target:    diffTargetPreview,
		requestID: m.previewReqID,
	})
	m.preview = m.preview.SetSize(40, 5)

	// In files focus, j must move the file cursor (not scroll preview).
	beforeOffset := m.preview.vp.YOffset
	beforeCursor := m.detail.cursor
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = mm.(Model)
	if m.detail.cursor == beforeCursor {
		t.Fatalf("expected file cursor to advance in files focus")
	}
	if m.preview.vp.YOffset != beforeOffset {
		t.Fatalf("preview scroll should not move in files focus")
	}

	// Switch to diff focus; j must scroll preview, not move file cursor.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	cursorAfterTab := m.detail.cursor
	offsetBefore := m.preview.vp.YOffset
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = mm.(Model)
	if m.detail.cursor != cursorAfterTab {
		t.Fatalf("file cursor must not move when diff has focus")
	}
	if m.preview.vp.YOffset == offsetBefore {
		t.Fatalf("preview should have scrolled in diff focus")
	}
}
```

Add `"strings"` to the imports of `internal/ui/app_test.go` if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDetailDiffFocusRoutesScrollKeys -v`
Expected: FAIL — j moves the file cursor regardless of focus.

- [ ] **Step 3: Route scroll keys based on focus**

Replace the trailing `before, beforeOK := m.detail.SelectedFile()` block in `updateDetailScreen` with this focus-aware dispatch:

```go
	if m.detailFocus == focusDiff {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}
	before, beforeOK := m.detail.SelectedFile()
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	after, afterOK := m.detail.SelectedFile()
	if afterOK && (!beforeOK || before.Path != after.Path) {
		m, previewCmd := m.queuePreviewForSelection()
		return m, tea.Batch(cmd, previewCmd)
	}
	return m, cmd
```

Also delete the standalone `case keyMatches(msg, m.keys.PgUp), keyMatches(msg, m.keys.PgDn), keyMatches(msg, m.keys.GotoTop), keyMatches(msg, m.keys.GotoEnd):` block from the same function — those keys are now handled implicitly by the focus dispatch above (when focus is `focusDiff` they route to preview; when focus is `focusFiles` they fall through to `m.detail.Update(msg)` where they're harmless no-ops).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDetailDiffFocusRoutesScrollKeys -v`
Expected: PASS. Also run the full UI suite to ensure nothing else broke:

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): route scroll keys to preview when diff has focus"
```

---

## Task 3: `n`/`N` walks files from inside diff focus

**Files:**
- Modify: `internal/ui/keys.go` (add bindings)
- Modify: `internal/ui/app.go` (`updateDetailScreen`)
- Modify: `internal/ui/app_test.go` (add test)

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestDetailNextPrevFileInDiffFocus(t *testing.T) {
	m := newDetailModel(t)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // focus diff
	m = mm.(Model)

	if m.detail.cursor != 0 {
		t.Fatalf("cursor should start at 0")
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mm.(Model)
	if m.detail.cursor != 1 {
		t.Fatalf("n should advance file cursor while keeping diff focus, got %d", m.detail.cursor)
	}
	if m.detailFocus != focusDiff {
		t.Fatalf("focus should remain on diff after n")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = mm.(Model)
	if m.detail.cursor != 0 {
		t.Fatalf("N should retreat file cursor, got %d", m.detail.cursor)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDetailNextPrevFileInDiffFocus -v`
Expected: FAIL — `n` is consumed by viewport (no-op), cursor doesn't advance.

- [ ] **Step 3: Add the key bindings**

In `internal/ui/keys.go`, add to `KeyMap`:

```go
	NextFile, PrevFile key.Binding
```

And in `DefaultKeys()`:

```go
		NextFile: key.NewBinding(key.WithKeys("n")),
		PrevFile: key.NewBinding(key.WithKeys("N")),
```

- [ ] **Step 4: Handle `n`/`N` before the focus dispatch**

In `updateDetailScreen`, add these cases before the `m.detailFocus == focusDiff` block (so they take precedence over scroll routing):

```go
	case keyMatches(msg, m.keys.NextFile):
		if m.detail.cursor < len(m.detail.files)-1 {
			m.detail.cursor++
		}
		m, previewCmd := m.queuePreviewForSelection()
		return m, previewCmd
	case keyMatches(msg, m.keys.PrevFile):
		if m.detail.cursor > 0 {
			m.detail.cursor--
		}
		m, previewCmd := m.queuePreviewForSelection()
		return m, previewCmd
```

These cases are inside the existing `switch { ... }` at the top of `updateDetailScreen`, alongside `keyMatches(msg, m.keys.Back)` etc. They run regardless of focus, so `n`/`N` works from either pane.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDetailNextPrevFileInDiffFocus -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/keys.go internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): n/N walks files from inside diff focus"
```

---

## Task 4: Per-file scroll memory

**Files:**
- Modify: `internal/ui/app.go` (Model + `queuePreviewForSelection` + `diffLoadedMsg` handler)
- Modify: `internal/ui/app_test.go` (add test)

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestDetailRestoresPerFileScrollOffset(t *testing.T) {
	m := newDetailModel(t)
	body := []byte(strings.Repeat("ctx\n", 200))
	// Land on /a.go preview with offset 0.
	mfocus, _ := m.queuePreviewForSelection()
	m = mfocus
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: body, target: diffTargetPreview, requestID: m.previewReqID,
	})
	m.preview = m.preview.SetSize(40, 5)
	// Scroll preview down, then move to /b.go.
	for i := 0; i < 5; i++ {
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = mm.(Model)
		mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = mm.(Model)
		mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = mm.(Model)
	}
	scrolledA := m.preview.vp.YOffset
	if scrolledA == 0 {
		t.Fatalf("expected non-zero scroll on /a.go")
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = mm.(Model)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: body, target: diffTargetPreview, requestID: m.previewReqID,
	})
	if m.preview.vp.YOffset != 0 {
		t.Fatalf("expected /b.go to start at top, got %d", m.preview.vp.YOffset)
	}
	// Go back to /a.go — saved offset must be restored when its diff arrives.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = mm.(Model)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: body, target: diffTargetPreview, requestID: m.previewReqID,
	})
	if m.preview.vp.YOffset != scrolledA {
		t.Fatalf("expected /a.go scroll to be restored to %d, got %d", scrolledA, m.preview.vp.YOffset)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDetailRestoresPerFileScrollOffset -v`
Expected: FAIL — preview always starts at top after each load.

- [ ] **Step 3: Add scroll memory map**

In `Model` (`internal/ui/app.go`), add:

```go
	scrollMem map[string]int
```

Initialize it in `New()`:

```go
		scrollMem: map[string]int{},
```

- [ ] **Step 4: Save offset before swapping files; restore after diff loads**

In `queuePreviewForSelection`, save the previous file's offset before mutating `previewKey`:

```go
	if m.previewKey != "" {
		m.scrollMem[m.previewKey] = m.preview.vp.YOffset
	}
	m.previewKey = key
```

In the `diffLoadedMsg` handler in `Update`, after the `m.preview, cmd = m.preview.Update(msg)` line for `diffTargetPreview`, restore the saved offset using the request's path. Replace the `case diffTargetPreview:` branch with:

```go
		case diffTargetPreview:
			if msg.requestID != m.previewReqID {
				return m, nil
			}
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			if off, ok := m.scrollMem[m.previewKey]; ok {
				m.preview.vp.SetYOffset(off)
			}
			return m, cmd
```

Note: `SetYOffset` exists on `viewport.Model`. If your bubbles version exposes only `vp.YOffset = off`, use direct assignment.

- [ ] **Step 5: Reset scroll memory when leaving the PR**

In `updateListScreen` `Open` handler (where you reset preview), also reset:

```go
			m.scrollMem = map[string]int{}
```

And in `updateDetailScreen` `Refresh` handler:

```go
			m.scrollMem = map[string]int{}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDetailRestoresPerFileScrollOffset -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): remember per-file scroll offset in preview"
```

---

## Task 5: Keep last diff visible during reload (no flash to "loading…")

**Files:**
- Modify: `internal/ui/diff.go`
- Modify: `internal/ui/diff_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/diff_test.go` (create the file if it doesn't exist with package `ui`):

```go
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
```

If `diff_test.go` does not yet exist, prepend:

```go
package ui

import (
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDiffSetHeaderKeepsPriorContentWhenReloading -v`
Expected: FAIL — `m.reloading undefined`, content was overwritten with "loading…".

- [ ] **Step 3: Add `reloading` field and stop blanking content**

In `internal/ui/diff.go`, add `reloading bool` to `DiffModel`. Replace `SetHeader`:

```go
func (m DiffModel) SetHeader(file, renderer string) DiffModel {
	m.file = file
	m.renderer = renderer
	m.loadErr = ""
	if !m.loaded {
		m.vp.SetContent("loading…")
	}
	m.reloading = m.loaded
	return m
}
```

In the `diffLoadedMsg` success branch, clear the flag:

```go
		} else {
			m.loaded = true
			m.reloading = false
			m.vp.SetContent(string(Colorize(msg.content)))
			m.vp.GotoTop()
		}
```

- [ ] **Step 4: Show a faint reload hint in the title row**

Replace `View()`:

```go
func (m DiffModel) View() string {
	var b strings.Builder
	suffix := "[" + m.renderer + "]"
	if m.reloading {
		suffix += " (reloading…)"
	}
	fmt.Fprintf(&b, "%s  %s\n", Header.Render(m.file), Faint.Render(suffix))
	b.WriteString(m.vp.View())
	if m.loadErr != "" {
		b.WriteString("\n" + ErrLine.Render(m.loadErr))
	}
	return b.String()
}
```

- [ ] **Step 5: Suppress automatic GotoTop on reload so scroll memory wins**

The current `diffLoadedMsg` branch calls `m.vp.GotoTop()` unconditionally. Scroll memory restoration runs in `app.go` after the model update, so leave `GotoTop()` here — Task 4 calls `SetYOffset` after this runs and overrides it. No additional change required.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDiffSetHeaderKeepsPriorContentWhenReloading -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/diff.go internal/ui/diff_test.go
git commit -m "feat(ui): keep prior diff visible while a new one loads"
```

---

## Task 6: ±1 neighbor prefetch with body cache

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/app_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestDetailServesPrefetchedNeighborInstantly(t *testing.T) {
	m := newDetailModel(t)
	// Simulate that /b.go was prefetched: stuff its body into the cache.
	bKey := diffSelectionKey("src", "tgt", "/b.go")
	m.previewBodies = map[string][]byte{bKey: []byte("--- a/b.go\n+++ b/b.go\n+B\n")}

	// Move cursor to /b.go — preview should render the cached body without a fetch.
	mfocus, _ := m.queuePreviewForSelection()
	m = mfocus
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = mm.(Model)

	if !strings.Contains(m.preview.vp.View(), "B") {
		t.Fatalf("expected cached body to render immediately:\n%s", m.preview.vp.View())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDetailServesPrefetchedNeighborInstantly -v`
Expected: FAIL — `m.previewBodies undefined`.

- [ ] **Step 3: Add the body cache and prefetch helpers**

In `Model`:

```go
	previewBodies map[string][]byte
```

In `New()`:

```go
		previewBodies: map[string][]byte{},
```

Add a helper near `queuePreviewForSelection`:

```go
func (m Model) prefetchNeighbors() (Model, tea.Cmd) {
	if m.detail.Detail() == nil {
		return m, nil
	}
	d := m.detail.Detail()
	files := m.detail.files
	cur := m.detail.cursor
	var cmds []tea.Cmd
	for _, idx := range []int{cur - 1, cur + 1} {
		if idx < 0 || idx >= len(files) {
			continue
		}
		f := files[idx]
		key := diffSelectionKey(d.SourceSha, d.TargetSha, f.Path)
		if _, ok := m.previewBodies[key]; ok {
			continue
		}
		// Reserve cache slot so we don't re-issue while the load is in flight.
		m.previewBodies[key] = nil
		cmds = append(cmds, m.prefetchOne(f, key))
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

type prefetchLoadedMsg struct {
	key     string
	content []byte
	err     error
}

func (m Model) prefetchOne(file ado.FileChange, key string) tea.Cmd {
	s := m.detail.Summary()
	d := m.detail.Detail()
	var clonePath string
	if p, ok := m.git.Find(s.Repo, m.cfg.Org); ok {
		clonePath = p
	}
	useDelta := m.useDelta
	sourceSha, targetSha := d.SourceSha, d.TargetSha
	repoID := s.RepoID
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if clonePath != "" {
			out, err := gitlocal.Diff(ctx, clonePath, targetSha, sourceSha, strings.TrimPrefix(file.Path, "/"), useDelta)
			return prefetchLoadedMsg{key: key, content: out, err: err}
		}
		src, tgt, err := client.GetFileContents(ctx, repoID, file.Path, sourceSha, targetSha)
		if err != nil {
			return prefetchLoadedMsg{key: key, err: err}
		}
		return prefetchLoadedMsg{key: key, content: simpleDiff(tgt, src, file.Path)}
	}
}
```

Handle `prefetchLoadedMsg` in `Update`'s switch (add a case beside `diffLoadedMsg`):

```go
	case prefetchLoadedMsg:
		if msg.err == nil {
			m.previewBodies[msg.key] = msg.content
		} else {
			delete(m.previewBodies, msg.key)
		}
		return m, nil
```

- [ ] **Step 4: Use cached body in `queuePreviewForSelection`**

Replace `queuePreviewForSelection` with:

```go
func (m Model) queuePreviewForSelection() (Model, tea.Cmd) {
	if m.screen != screenDetail || m.detail.Detail() == nil {
		return m, nil
	}
	f, ok := m.detail.SelectedFile()
	if !ok {
		return m, nil
	}
	key := diffSelectionKey(m.detail.Detail().SourceSha, m.detail.Detail().TargetSha, f.Path)
	if key == m.previewKey {
		return m, nil
	}
	if m.previewKey != "" {
		m.scrollMem[m.previewKey] = m.preview.vp.YOffset
	}
	m.previewKey = key

	// Cache hit (non-nil body): render immediately, then prefetch new neighbors.
	if body, ok := m.previewBodies[key]; ok && body != nil {
		renderer := "rest"
		if p, ok := m.git.Find(m.detail.Summary().Repo, m.cfg.Org); ok && p != "" {
			if m.useDelta {
				renderer = "local+delta"
			} else {
				renderer = "local"
			}
		}
		m.preview = m.sizeDiffModel(m.preview.SetHeader(f.Path, renderer), diffTargetPreview)
		m.previewReqID++
		// Inject the cached body via a synthesized msg so all the loaded/scroll-restore plumbing fires.
		reqID := m.previewReqID
		var cmds []tea.Cmd
		cmds = append(cmds, func() tea.Msg {
			return diffLoadedMsg{content: body, target: diffTargetPreview, requestID: reqID}
		})
		mm, prefetchCmd := m.prefetchNeighbors()
		m = mm
		if prefetchCmd != nil {
			cmds = append(cmds, prefetchCmd)
		}
		return m, tea.Batch(cmds...)
	}

	m.previewReqID++
	pm, cmd := m.loadDiff(diffTargetPreview, m.preview, m.previewReqID, m.detail.Summary(), f, m.detail.Detail().SourceSha, m.detail.Detail().TargetSha)
	m.preview = pm
	mm, prefetchCmd := m.prefetchNeighbors()
	m = mm
	return m, tea.Batch(cmd, prefetchCmd)
}
```

Also, when the foreground load completes (in `diffLoadedMsg` `case diffTargetPreview`), populate the cache so a re-visit is a hit. Update that branch:

```go
		case diffTargetPreview:
			if msg.requestID != m.previewReqID {
				return m, nil
			}
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			if msg.err == nil && m.previewKey != "" {
				m.previewBodies[m.previewKey] = msg.content
			}
			if off, ok := m.scrollMem[m.previewKey]; ok {
				m.preview.vp.SetYOffset(off)
			}
			return m, cmd
```

- [ ] **Step 5: Reset cache when leaving PR or refreshing**

In `updateListScreen` `Open` and `updateDetailScreen` `Refresh`:

```go
			m.previewBodies = map[string][]byte{}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDetailServesPrefetchedNeighborInstantly -v`
Expected: PASS.

Run the full UI suite:

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): prefetch ±1 neighbor diff and serve from cache"
```

---

## Task 7: Remove the full-screen Diff screen

**Files:**
- Modify: `internal/ui/app.go`

- [ ] **Step 1: Verify no test depends on screenDiff**

Run: `grep -n "screenDiff\|updateDiffScreen\|m\.diff\b\|diffReqID" internal/ui/`
Expected: results only in `app.go`. If any test references these symbols, that test should be deleted as part of this task.

- [ ] **Step 2: Remove `screenDiff` const and the `diff`/`diffReqID` fields**

In `internal/ui/app.go`:

```go
const (
	screenList screen = iota
	screenDetail
)
```

Remove the `diff DiffModel` field and `diffReqID int` field from `Model`. Remove `diff: NewDiff(keys),` from `New()`.

- [ ] **Step 3: Remove the `screenDiff` branches from `Update` and `View`**

In `Update`:
- Delete the `case screenDiff:` branch from the keyed switch.
- Delete the `case screenDiff:` branch from the trailing fallthrough switch.
- Remove the `m.diff = m.sizeDiffModel(m.diff, diffTargetFull)` line in the `tea.WindowSizeMsg` handler.
- In the `diffLoadedMsg` switch, remove the `default:` (full-diff) branch, leaving only the `case diffTargetPreview:` branch. The whole `diffLoadedMsg` handler becomes:

```go
	case diffLoadedMsg:
		if msg.target != diffTargetPreview {
			return m, nil
		}
		if msg.requestID != m.previewReqID {
			return m, nil
		}
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		if msg.err == nil && m.previewKey != "" {
			m.previewBodies[m.previewKey] = msg.content
		}
		if off, ok := m.scrollMem[m.previewKey]; ok {
			m.preview.vp.SetYOffset(off)
		}
		return m, cmd
```

In `View`:
- Delete the `case screenDiff:` branch.

- [ ] **Step 4: Remove `updateDiffScreen` and the `Open` branch in Detail that pushed it**

Delete the entire `updateDiffScreen` function. In `updateDetailScreen`, remove the `case keyMatches(msg, m.keys.Open):` branch — Enter no longer has a Detail-screen action. (Skip the diff-zoom feature; we're committing to "no second screen.")

- [ ] **Step 5: Update help text and footer hints**

In `View`'s help block, replace the relevant lines:

```go
		body = HelpBox.Render(strings.Join([]string{
			"Help",
			"",
			"  ?           toggle this help",
			"  q / ctrl+c  quit",
			"  r           refresh current screen",
			"  o           open in browser",
			"  /           filter (list)",
			"  tab/shift+tab  switch focus (Detail)",
			"  n / N       next / prev file (Detail)",
			"  ↑↓ pgup/pgdn g/G  scroll focused pane",
			"  esc         back",
		}, "\n"))
```

In `footerHints`, remove the `screenDiff` case and update `screenDetail`:

```go
func footerHints(s screen) string {
	switch s {
	case screenList:
		return "/:filter  enter:open  o:browser  r:refresh  tab:next  ?:help  q:quit"
	case screenDetail:
		return "tab:focus  n/N:file  ↑↓ pgup/pgdn g/G:scroll  o:browser  esc:back  r:refresh  ?:help  q:quit"
	}
	return ""
}
```

- [ ] **Step 6: Build and test**

Run: `go build ./...`
Expected: success.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app.go
git commit -m "refactor(ui): remove full-screen Diff; Detail is the only diff surface"
```

---

## Task 8: Render focus indicator on Detail panes

**Files:**
- Modify: `internal/ui/app.go` (`detailPreviewView` + `previewPaneView`)
- Modify: `internal/ui/detail.go` (expose helper)

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestDetailFocusIndicatorMovesWithFocus(t *testing.T) {
	m := newDetailModel(t)
	m.width, m.height = 140, 40
	m.preview = m.preview.SetSize(60, 20)
	m.preview, _ = m.preview.Update(diffLoadedMsg{
		content: []byte("--- a/x\n+++ b/x\n+hi\n"), target: diffTargetPreview,
	})

	out := m.detailPreviewView()
	if !strings.Contains(out, "● Files") {
		t.Fatalf("expected files header to show focus dot:\n%s", out)
	}
	if strings.Contains(out, "● Diff Preview") {
		t.Fatalf("diff header should not show focus dot when files focused:\n%s", out)
	}

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	out = m.detailPreviewView()
	if !strings.Contains(out, "● Diff Preview") {
		t.Fatalf("expected diff header to show focus dot:\n%s", out)
	}
	if strings.Contains(out, "● Files") {
		t.Fatalf("files header should not show focus dot when diff focused:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDetailFocusIndicatorMovesWithFocus -v`
Expected: FAIL — neither header carries `●`.

- [ ] **Step 3: Pass the focus state into the renderers**

Replace `previewPaneView` and the file-list header.

In `internal/ui/app.go` `previewPaneView`:

```go
func (m Model) previewPaneView() string {
	dot := "○ "
	if m.detailFocus == focusDiff {
		dot = "● "
	}
	title := Header.Render(dot + "Diff Preview")
	if m.preview.file == "" {
		if _, ok := m.detail.SelectedFile(); !ok {
			return title + "\n" + Faint.Render("No changed files available.")
		}
		return title + "\n" + Faint.Render("Loading selected file diff…")
	}
	return title + "\n" + m.preview.View()
}
```

In `internal/ui/detail.go` `View`, replace the `── Files ──` divider line with a focus-aware header. Add a method on `DetailModel`:

```go
func (m DetailModel) FilesHeader(focused bool) string {
	dot := "○ "
	if focused {
		dot = "● "
	}
	return Header.Render(dot+"Files") + "\n"
}
```

Then change the corresponding line in `View()` from:

```go
	b.WriteString("\n── Files ─────────────────────────────────\n")
```

to expose the divider via an injected boolean. Since `DetailModel.View()` doesn't know focus, change `detailPreviewView` in `app.go` to compose the file pane manually:

Replace the body of `detailPreviewView`:

```go
func (m Model) detailPreviewView() string {
	layout := m.detailLayout()
	left := m.detail.ViewWithFocus(m.detailFocus == focusFiles)
	right := m.previewPaneView()
	if !layout.split {
		return strings.Join([]string{left, "", right}, "\n")
	}
	leftPane := lipgloss.NewStyle().
		Width(layout.leftWidth).
		MaxWidth(layout.leftWidth).
		Height(layout.bodyHeight).
		Render(left)
	rightPane := lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(lipgloss.Color("8")).
		PaddingLeft(1).
		Width(layout.rightWidth).
		MaxWidth(layout.rightWidth).
		Height(layout.bodyHeight).
		Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}
```

Add `ViewWithFocus` to `DetailModel` (`internal/ui/detail.go`) by copying the body of `View()` and replacing the `── Files ──` divider with `m.FilesHeader(focused)`. Have the existing `View()` delegate:

```go
func (m DetailModel) View() string { return m.ViewWithFocus(true) }
```

(That keeps existing tests like `TestDetailRendersDescriptionAndFiles` passing.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestDetailFocusIndicatorMovesWithFocus -v`
Expected: PASS.

Run the full suite to be sure existing detail tests still pass:

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/detail.go internal/ui/app_test.go
git commit -m "feat(ui): show ●/○ focus indicator on Detail pane headers"
```

---

## Self-Review Notes

- **Spec coverage:** #1 (no second screen) → Task 7. #2 (focus indicator) → Tasks 1+8. #5 (n/N walk) → Task 3. #9 (per-file scroll memory) → Task 4. #13 (±1 prefetch) → Task 6. #14 (no flash on reload) → Task 5. All MVP picks have a task.
- **Type consistency:** `detailFocus`/`focusFiles`/`focusDiff` defined in Task 1 used in Tasks 2, 3, 6, 8. `previewBodies map[string][]byte` defined in Task 6 used by `prefetchOne`/`queuePreviewForSelection`. `scrollMem map[string]int` defined in Task 4 used in Task 6. `reloading` field defined in Task 5 used only in Task 5. `NextFile`/`PrevFile` keys defined in Task 3 used only there.
- **Placeholders:** none. Every code step shows the actual code; commands have expected results.
- **Risk:** Task 6 synthesizes a `diffLoadedMsg` from a goroutine to reuse the success path. That's intentional — it avoids duplicating the scroll-restore + cache-write logic. If the bubbletea version in use has stricter type checks, replace the synthesized msg with a direct call to `m.preview, _ = m.preview.Update(...)` in `queuePreviewForSelection` and run scroll restore inline.

---

Plan complete and saved to `docs/superpowers/plans/2026-04-27-smooth-split-view.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?

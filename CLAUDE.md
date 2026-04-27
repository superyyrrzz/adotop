# adotop — Project Instructions

## TUI Testing Protocol

This project ships a Bubble Tea TUI. View bugs are easy to introduce
and easy to "fix" without actually fixing — because the test geometry
rarely matches what the user sees. Follow these rules.

### 1. Test through the real layout, not the bare model

A `DetailModel.View()` rendered with `tea.WindowSizeMsg{Width:80}` is
NOT what the user sees in split-pane mode (left pane is ~40 cols).
For any test that asserts on rendered output, use the shared helper:

```go
out := renderDetailInLayout(t, m, termW, termH)
```

It mirrors `app.go`'s `detailPreviewView` — computes `detailLayout`,
calls `SetPaneSize(leftWidth, bodyHeight)`, returns the composed
string. If you call `m.View()` directly in a test, justify it in a
comment.

### 2. Run the geometry matrix

When a user reports "X is missing/wrong/clipped in the rendered view,"
the first response is a failing table-driven test that iterates over
realistic pane sizes:

```go
forEachPaneSize(t, func(t *testing.T, w, h int) {
    out := renderDetailInLayout(t, m, w, h)
    assertHeaderVisible(t, out, summary, h)
})
```

The matrix lives in `forEachPaneSize` (currently 30×24, 40×26, 50×35,
60×30, 80×40, 40×50). Don't fix view bugs before adding the failing
case.

### 3. Measure the final composed string

Use `lipgloss.Height/Width` on the **final** rendered string, never on
intermediate fragments. Wrapping happens at composition; an
intermediate header that's 6 source lines may render as 14 visual rows
inside a 40-col pane.

### 4. Keep the live test honest

`internal/ui/detail_live_test.go` (build tag `live`) hits real PR
#1145087 across the geometry matrix. Run it before declaring any
"PR view" bug fixed:

```sh
make test-live
```

### 5. New invariants go in a helper

If you find yourself asserting "repo line is present AND title is
present AND files sub-header is present AND height ≤ pane" in more
than one place, add it to `assertHeaderVisible` (or similar). One
edit must be enough to enforce a new invariant everywhere.

## Git Operations

Do NOT push to remote unless explicitly asked. Local commits only.

## Azure DevOps

Use the ADO MCP server. Default org `ceapex`, project `Engineering`.
Never WebFetch ADO URLs.

package ui

import "fmt"

// ctxLadder is the cycle for `+`/`-` on the diff preview. 0 stands in for
// the default (3 lines around each change, matching `git diff` and the
// ADO web UI). -1 stands in for "all" (entire file, no folding).
//
// Order is intentional: + steps right, - steps left, both wrap.
var ctxLadder = []int{0, 10, 25, -1}

// nextCtx returns the next ladder value after current. If current isn't
// on the ladder, jumps to the first rung (0 = default).
func nextCtx(current int) int {
	for i, v := range ctxLadder {
		if v == current {
			return ctxLadder[(i+1)%len(ctxLadder)]
		}
	}
	return ctxLadder[0]
}

// prevCtx is nextCtx in reverse.
func prevCtx(current int) int {
	for i, v := range ctxLadder {
		if v == current {
			return ctxLadder[(i-1+len(ctxLadder))%len(ctxLadder)]
		}
	}
	return ctxLadder[0]
}

// ctxLabel renders the current rung for the statusline.
func ctxLabel(c int) string {
	switch c {
	case 0:
		return "ctx:3"
	case -1:
		return "ctx:all"
	default:
		return fmt.Sprintf("ctx:%d", c)
	}
}

// ctxLines is the actual context-line count to pass to the diff layer.
// 0 → 3 (the default). -1 → a very large number so the unified-diff
// emitter folds nothing.
func ctxLines(c int) int {
	switch c {
	case 0:
		return 3
	case -1:
		return 1 << 30
	default:
		return c
	}
}

package ui

// Layout primitives. Three tiny helpers that name the width/height
// math the screens and modals do repeatedly. Goal isn't generality —
// it's removing the "wait, what does this clamp do again?" question
// from every Read at the existing call sites. Inspired by FTXUI's
// declarative layout vocabulary, scaled down to what adotop actually
// uses.

// clamp returns n constrained to the inclusive range [lo, hi]. When
// hi < lo (a misconfigured caller), the result is hi — bias toward
// the upper bound matches how most TUI sizing wants to behave when
// the terminal is comically narrow: pick the larger value and let
// the renderer truncate, rather than collapsing to the lower bound
// and showing nothing.
//
// Parameter order matches the math.Clamp proposals and common usage
// in other languages (n first, then bounds), so call sites read as
// "clamp this value between these limits" rather than the ambiguous
// "clamp(40, 100, w)" form an earlier draft used.
func clamp(n, lo, hi int) int {
	if n < lo {
		n = lo
	}
	if n > hi {
		n = hi
	}
	return n
}

// modalSize computes the overlay box dimensions for a centered modal.
// wRatio/hRatio are the fractions of terminal width/height the modal
// would prefer; wMin/wMax/hMin/hMax clamp the result so the box stays
// readable on huge terminals and present on tiny ones. When termW or
// termH is zero (pre-resize window, briefly visible before the first
// WindowSizeMsg lands) the result falls back to the upper bounds —
// the modal may briefly clip, but it won't render as a collapsed
// sliver, which is the more jarring failure mode.
//
// Centralizes the pattern that descModalSize and composeModalSize were
// re-implementing with slightly different constants.
func modalSize(termW, termH int, wRatio, hRatio float64, wMin, wMax, hMin, hMax int) (w, h int) {
	if termW <= 0 || termH <= 0 {
		return wMax, hMax
	}
	w = int(float64(termW) * wRatio)
	h = int(float64(termH) * hRatio)
	return clamp(w, wMin, wMax), clamp(h, hMin, hMax)
}

package ui

import "testing"

func TestClamp(t *testing.T) {
	cases := []struct {
		name     string
		n, lo, hi int
		want     int
	}{
		{"in range", 50, 0, 100, 50},
		{"at lower bound", 0, 0, 100, 0},
		{"at upper bound", 100, 0, 100, 100},
		{"below range", -5, 0, 100, 0},
		{"above range", 200, 0, 100, 100},
		{"negative range", -10, -100, -1, -10},
		// Misconfigured caller (hi < lo): result should be hi, per the
		// doc-comment contract — TUI sizing prefers "render too wide,
		// let it truncate" over "render to nothing."
		{"inverted bounds", 50, 100, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clamp(c.n, c.lo, c.hi); got != c.want {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", c.n, c.lo, c.hi, got, c.want)
			}
		})
	}
}

func TestModalSize(t *testing.T) {
	cases := []struct {
		name                                   string
		termW, termH                           int
		wRatio, hRatio                         float64
		wMin, wMax, hMin, hMax                 int
		wantW, wantH                           int
	}{
		{
			name:  "preferred ratios within bounds",
			termW: 120, termH: 40,
			wRatio: 0.8, hRatio: 0.6,
			wMin: 40, wMax: 100, hMin: 12, hMax: 24,
			wantW: 96, wantH: 24,
		},
		{
			name:  "wide terminal clamps to wMax",
			termW: 300, termH: 40,
			wRatio: 0.8, hRatio: 0.6,
			wMin: 40, wMax: 100, hMin: 12, hMax: 24,
			wantW: 100, wantH: 24,
		},
		{
			name:  "narrow terminal clamps to wMin",
			termW: 30, termH: 40,
			wRatio: 0.8, hRatio: 0.6,
			wMin: 40, wMax: 100, hMin: 12, hMax: 24,
			wantW: 40, wantH: 24,
		},
		{
			name:  "short terminal clamps to hMin",
			termW: 120, termH: 10,
			wRatio: 0.8, hRatio: 0.6,
			wMin: 40, wMax: 100, hMin: 12, hMax: 24,
			wantW: 96, wantH: 12,
		},
		{
			// Pre-resize fallback. Both bounds at max so the modal
			// briefly takes the whole box rather than collapsing —
			// see the doc comment for why.
			name:  "zero terminal width falls back to maxes",
			termW: 0, termH: 0,
			wRatio: 0.8, hRatio: 0.6,
			wMin: 40, wMax: 100, hMin: 12, hMax: 24,
			wantW: 100, wantH: 24,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, h := modalSize(c.termW, c.termH, c.wRatio, c.hRatio, c.wMin, c.wMax, c.hMin, c.hMax)
			if w != c.wantW || h != c.wantH {
				t.Errorf("modalSize(%d, %d, %.1f, %.1f, %d, %d, %d, %d) = (%d, %d), want (%d, %d)",
					c.termW, c.termH, c.wRatio, c.hRatio, c.wMin, c.wMax, c.hMin, c.hMax,
					w, h, c.wantW, c.wantH)
			}
		})
	}
}

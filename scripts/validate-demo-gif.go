//go:build ignore

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

type bounds struct {
	minX int
	minY int
	maxX int
	maxY int
}

type result struct {
	ok       bool
	messages []string
}

func main() {
	gifPath := flag.String("gif", "assets/adotop-demo.gif", "path to demo GIF")
	maxOuterMargin := flag.Int("max-outer-margin", 12, "maximum background margin around terminal")
	maxInkBottomMargin := flag.Int("max-ink-bottom-margin", 16, "maximum blank bottom inside terminal content")
	maxVerticalGap := flag.Int("max-vertical-gap", 2, "maximum missing pixels inside expected vertical box stroke")
	maxInnerRightMargin := flag.Int("max-inner-right-margin", 20, "maximum blank right side inside terminal content")
	flag.Parse()

	if err := run(*gifPath, *maxOuterMargin, *maxInkBottomMargin, *maxVerticalGap, *maxInnerRightMargin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(gifPath string, maxOuterMargin, maxInkBottomMargin, maxVerticalGap, maxInnerRightMargin int) error {
	data, err := os.ReadFile(gifPath)
	if err != nil {
		return err
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return err
	}
	frames, err := sourceFrames()
	if err != nil {
		return err
	}
	if len(decoded.Image) == 0 {
		return errors.New("GIF has no frames")
	}

	cfg := imageConfig(decoded)
	fmt.Printf("GIF: %dx%d, frames=%d\n", cfg.W, cfg.H, len(decoded.Image))

	termX, termY := 8, 8
	termW, termH := 1012, 524
	pad := 10
	cellW, cellH := 8.4, 14.0
	termBgSample := rgb{0x11, 0x11, 0x1b}

	failures := []string{}
	limit := min(len(decoded.Image), len(frames))
	for i := 0; i < limit; i++ {
		img := decoded.Image[i]
		outerBg := rgbAt(img, 0, 0)
		termBg := rgbAt(img, termX+pad+2, termY+pad+2)

		outer, ok := findBounds(img, image.Rect(0, 0, cfg.W, cfg.H), outerBg, 5)
		if ok {
			left, top := outer.minX, outer.minY
			right, bottom := cfg.W-1-outer.maxX, cfg.H-1-outer.maxY
			fmt.Printf("frame %d: outer margins L=%d T=%d R=%d B=%d\n", i, left, top, right, bottom)
			if max4(left, top, right, bottom) > maxOuterMargin {
				failures = append(failures, fmt.Sprintf("frame %d outer margin exceeds %d", i, maxOuterMargin))
			}
		}

		innerRect := image.Rect(termX+pad, termY+pad, min(cfg.W, termX+termW-pad+1), min(cfg.H, termY+termH-pad+1))
		inner, ok := findBounds(img, innerRect, termBg, 20)
		if ok {
			inkBottom := innerRect.Max.Y - 1 - inner.maxY
			inkRight := innerRect.Max.X - 1 - inner.maxX
			fmt.Printf("frame %d: ink bottom margin=%d, ink right margin=%d\n", i, inkBottom, inkRight)
			if inkBottom > maxInkBottomMargin {
				failures = append(failures, fmt.Sprintf("frame %d ink bottom margin %d exceeds %d", i, inkBottom, maxInkBottomMargin))
			}
			if i == 0 && inkRight > maxInnerRightMargin {
				failures = append(failures, fmt.Sprintf("frame %d ink right margin %d exceeds %d", i, inkRight, maxInnerRightMargin))
			}
		}

		strokeFailures := validateVerticalStrokes(img, frames[i], termX, termY, pad, cellW, cellH, termBgSample, maxVerticalGap)
		for _, f := range strokeFailures {
			failures = append(failures, fmt.Sprintf("frame %d %s", i, f))
		}
	}

	if len(failures) > 0 {
		fmt.Println("Failures:")
		for i, failure := range failures {
			if i >= 40 {
				fmt.Printf("  ... %d more\n", len(failures)-i)
				break
			}
			fmt.Println("  " + failure)
		}
		return errors.New("demo GIF validation failed")
	}
	fmt.Println("Demo GIF validation passed")
	return nil
}

func sourceFrames() ([]string, error) {
	cmd := exec.Command("go", "run", "./cmd/adotop", "demo", "--frames")
	cmd.Env = append(os.Environ(), "ADOTOP_THEME=dark")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile("\\x1b\\[[0-9;]*m")
	parts := strings.Split(string(out), "\f")
	frames := make([]string, 0, len(parts))
	for _, part := range parts {
		frames = append(frames, re.ReplaceAllString(strings.TrimRight(part, "\r\n\t "), ""))
	}
	return frames, nil
}

func validateVerticalStrokes(img image.Image, frame string, termX, termY, pad int, cellW, cellH float64, bg rgb, maxGap int) []string {
	failures := []string{}
	lines := strings.Split(frame, "\n")
	for row, line := range lines {
		col := 0
		for len(line) > 0 {
			r, size := utf8.DecodeRuneInString(line)
			if r == utf8.RuneError && size == 0 {
				break
			}
			if r == '│' || r == '┃' {
				x := float64(termX+pad) + float64(col)*cellW + cellW/2
				y0 := float64(termY+pad) + float64(row)*cellH - 1
				y1 := float64(termY+pad) + float64(row+1)*cellH + 1
				worstGap, hits := verticalGap(img, x, y0, y1, bg)
				if worstGap > maxGap {
					failures = append(failures, fmt.Sprintf("vertical stroke row=%d col=%d hits=%d worstGap=%d", row, col, hits, worstGap))
				}
			}
			line = line[size:]
			col++
		}
	}
	return failures
}

func verticalGap(img image.Image, centerX, topY, bottomY float64, bg rgb) (int, int) {
	x := int(math.Round(centerX))
	startY := int(math.Floor(topY))
	endY := int(math.Ceil(bottomY))
	currentGap, worstGap, hits := 0, 0, 0
	for y := startY; y <= endY; y++ {
		hit := false
		for dx := -1; dx <= 1; dx++ {
			p := rgbAt(img, x+dx, y)
			if colorDistance(p, bg) > 30 {
				hit = true
				break
			}
		}
		if hit {
			hits++
			if currentGap > worstGap {
				worstGap = currentGap
			}
			currentGap = 0
		} else {
			currentGap++
		}
	}
	if currentGap > worstGap {
		worstGap = currentGap
	}
	return worstGap, hits
}

func findBounds(img image.Image, rect image.Rectangle, base rgb, threshold int) (bounds, bool) {
	b := bounds{minX: rect.Max.X, minY: rect.Max.Y, maxX: -1, maxY: -1}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if colorDistance(rgbAt(img, x, y), base) > threshold {
				if x < b.minX {
					b.minX = x
				}
				if y < b.minY {
					b.minY = y
				}
				if x > b.maxX {
					b.maxX = x
				}
				if y > b.maxY {
					b.maxY = y
				}
			}
		}
	}
	return b, b.maxX >= 0
}

type rgb struct{ r, g, b int }
type imageCfg struct{ W, H int }

func imageConfig(g *gif.GIF) imageCfg {
	if len(g.Image) == 0 {
		return imageCfg{}
	}
	r := g.Image[0].Bounds()
	return imageCfg{W: r.Dx(), H: r.Dy()}
}

func rgbAt(img image.Image, x, y int) rgb {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return rgb{}
	}
	c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	return rgb{int(c.R), int(c.G), int(c.B)}
}

func colorDistance(a, b rgb) int {
	return abs(a.r-b.r) + abs(a.g-b.g) + abs(a.b-b.b)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max4(a, b, c, d int) int {
	m := a
	for _, v := range []int{b, c, d} {
		if v > m {
			m = v
		}
	}
	return m
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package perch draws the bird by the taskbar (barahn/chirp#136): the
// warm package's poses, which are the site's and the icon's bird, without
// the perch line -- by the taskbar the bird lands on the taskbar itself --
// on a transparent canvas, and the flights in and out. The windows that
// show it are per OS, in cmd/chirp.
package perch

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"regexp"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// The canvas, in the art's units (its 120-unit viewBox). It ends at the
// bird's feet, so the window's bottom edge is where it stands, and leaves
// room above and to the right for the flight in.
const (
	canvasX = 0.0
	canvasY = -62.0
	canvasW = 164.0
	canvasH = 164.0 // to y = 102, just under the feet
)

// Height is the canvas's height for a width, in pixels.
func Height(width int) int {
	return int(math.Round(float64(width) * canvasH / canvasW))
}

// Motion places the bird in flight: moved by DX, DY in the art's units,
// turned by Rotate degrees about its middle, and drawn at Alpha, 0 to 1.
type Motion struct {
	DX, DY, Rotate, Alpha float64
}

// Perched is the bird at rest.
var Perched = Motion{Alpha: 1}

// perchLine is the line under the feet, which the site and the pose SVGs
// draw and the taskbar replaces.
var perchLine = regexp.MustCompile(`<line[^>]*\by1="103"[^>]*\by2="103"[^>]*/>`)

// inner is the drawing inside the <svg> element of an SVG file.
var inner = regexp.MustCompile(`(?s)<svg[^>]*>(.*)</svg>`)

// Frame draws pose, moved by m, on a transparent canvas width pixels wide
// and Height(width) high. The pixels are premultiplied, as image.RGBA's are.
func Frame(pose warm.Pose, m Motion, width int) (*image.RGBA, error) {
	src := warm.Bird(pose).Content()
	match := inner.FindSubmatch(src)
	if match == nil {
		return nil, fmt.Errorf("perch: %s is not an SVG", pose)
	}
	drawing := perchLine.ReplaceAll(match[1], nil)
	height := Height(width)
	var svg bytes.Buffer
	// The viewBox starts at 0, 0 and the canvas's origin moves the drawing
	// instead: oksvg takes a viewBox's origin in pixels, not in its units.
	fmt.Fprintf(&svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" width="%d" height="%d">`,
		canvasW, canvasH, width, height)
	fmt.Fprintf(&svg, `<g transform="translate(%g %g) rotate(%g 60 60)">`, m.DX-canvasX, m.DY-canvasY, m.Rotate)
	svg.Write(drawing)
	svg.WriteString(`</g></svg>`)

	icon, err := oksvg.ReadIconStream(&svg)
	if err != nil {
		return nil, fmt.Errorf("perch: %s: %w", pose, err)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	icon.SetTarget(0, 0, float64(width), float64(height))
	icon.Draw(rasterx.NewDasher(width, height, rasterx.NewScannerGV(width, height, img, img.Bounds())), 1)
	fade(img, m.Alpha)
	return img, nil
}

// fade multiplies img's premultiplied pixels by alpha.
func fade(img *image.RGBA, alpha float64) {
	if alpha >= 1 {
		return
	}
	alpha = math.Max(0, alpha)
	for i, v := range img.Pix {
		img.Pix[i] = uint8(math.Round(float64(v) * alpha))
	}
}

// Landing is the flight in, at fps frames a second: the site's land
// animation (site/assets/site.css), from 40 right and 60 up, turned -12
// degrees, to perched, over 0.9 s, eased by cubic-bezier(.2, .9, .3, 1.3),
// which overshoots a little before it settles.
func Landing(fps int) []Motion {
	ease := cubicBezier(.2, .9, .3, 1.3)
	return flight(fps, 0.9, func(t float64) Motion {
		left := 1 - ease(t)
		return Motion{DX: 40 * left, DY: -60 * left, Rotate: -12 * left, Alpha: 1}
	})
}

// FlyingOff is the flight out, at fps frames a second: up and to the right,
// the way it came, fading out over 0.6 s, eased in.
func FlyingOff(fps int) []Motion {
	ease := cubicBezier(.4, 0, 1, 1)
	return flight(fps, 0.6, func(t float64) Motion {
		p := ease(t)
		return Motion{DX: 60 * p, DY: -90 * p, Rotate: -15 * p, Alpha: 1 - p}
	})
}

// flight samples at over seconds, at fps frames a second, from t = 0 to 1
// inclusive.
func flight(fps int, seconds float64, at func(t float64) Motion) []Motion {
	n := max(2, int(math.Round(float64(fps)*seconds)))
	ms := make([]Motion, n)
	for i := range ms {
		ms[i] = at(float64(i) / float64(n-1))
	}
	return ms
}

// cubicBezier is CSS's cubic-bezier(x1, y1, x2, y2) timing function: for a
// time t from 0 to 1, the progress.
func cubicBezier(x1, y1, x2, y2 float64) func(t float64) float64 {
	curve := func(a, b, s float64) float64 {
		return 3*a*s*(1-s)*(1-s) + 3*b*s*s*(1-s) + s*s*s
	}
	return func(t float64) float64 {
		if t <= 0 {
			return 0
		}
		if t >= 1 {
			return 1
		}
		// x is monotonic in s for 0 <= x1, x2 <= 1: find s by bisection.
		lo, hi := 0.0, 1.0
		for range 50 {
			mid := (lo + hi) / 2
			if curve(x1, x2, mid) < t {
				lo = mid
			} else {
				hi = mid
			}
		}
		return curve(y1, y2, (lo+hi)/2)
	}
}

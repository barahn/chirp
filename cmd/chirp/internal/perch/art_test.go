// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package perch

import (
	"image"
	"math"
	"testing"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

const testWidth = 296 // two pixels per unit of the art

// at is the pixel of img at (x, y) in the art's units.
func at(t *testing.T, img *image.RGBA, x, y float64) (r, g, b, a uint8) {
	t.Helper()
	px := int(math.Round((x - canvasX) * float64(img.Bounds().Dx()) / canvasW))
	py := int(math.Round((y - canvasY) * float64(img.Bounds().Dy()) / canvasH))
	c := img.RGBAAt(px, py)
	return c.R, c.G, c.B, c.A
}

func frame(t *testing.T, p warm.Pose, m Motion) *image.RGBA {
	t.Helper()
	img, err := Frame(p, m, testWidth)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// The bird by the taskbar is the site's and the icon's bird: the same body,
// in the same yellow.
func TestItIsTheSitesBird(t *testing.T) {
	img := frame(t, warm.Perched, Motion{Alpha: 1})
	if r, g, b, a := at(t, img, 40, 80); r != 0xF5 || g != 0xB8 || b != 0x3D || a != 0xFF {
		t.Fatalf("body is #%02X%02X%02X alpha %d, want #F5B83D opaque", r, g, b, a)
	}
}

// It lands on the taskbar itself: the perch line the site draws is left
// out, and the canvas ends at its feet. The legs stay.
func TestTheBirdStandsOnTheTaskbarWithoutAPerch(t *testing.T) {
	img := frame(t, warm.Perched, Motion{Alpha: 1})
	if _, _, _, a := at(t, img, 20, 102); a != 0 {
		t.Fatalf("the perch line is drawn (alpha %d under the tail)", a)
	}
	if _, _, _, a := at(t, img, 52, 98); a == 0 {
		t.Fatal("the legs are gone")
	}
	if got := canvasY + canvasH; got < 101 || got > 104 {
		t.Fatalf("the canvas ends at %v, want at the feet, about 102", got)
	}
}

// Around the bird there is nothing: the window is see-through.
func TestTheBackgroundIsTransparent(t *testing.T) {
	img := frame(t, warm.Perched, Motion{Alpha: 1})
	for _, p := range []image.Point{{0, 0}, {img.Bounds().Dx() - 1, 0}, {0, img.Bounds().Dy() - 1}} {
		if a := img.RGBAAt(p.X, p.Y).A; a != 0 {
			t.Fatalf("pixel %v has alpha %d", p, a)
		}
	}
}

// The landing is the site's: from 40 right and 60 up, turned -12 degrees,
// to perched, overshooting a little on the way, as cubic-bezier(.2, .9, .3,
// 1.3) does.
func TestTheLandingIsTheSites(t *testing.T) {
	ms := Landing(30)
	if n := len(ms); n < 25 || n > 29 {
		t.Fatalf("%d frames at 30 fps, want about 0.9 s worth", n)
	}
	first, last := ms[0], ms[len(ms)-1]
	if first.DX != 40 || first.DY != -60 || first.Rotate != -12 {
		t.Fatalf("starts at %+v, want 40, -60, -12", first)
	}
	if last.DX != 0 || last.DY != 0 || last.Rotate != 0 || last.Alpha != 1 {
		t.Fatalf("ends at %+v, want perched", last)
	}
	overshoot := false
	for _, m := range ms {
		overshoot = overshoot || m.DX < 0
	}
	if !overshoot {
		t.Fatal("no overshoot: the bird does not settle as on the site")
	}
}

// Flying off, it rises the way it came and fades out.
func TestFlyingOffFadesAway(t *testing.T) {
	ms := FlyingOff(30)
	first, last := ms[0], ms[len(ms)-1]
	if first.DX != 0 || first.Alpha != 1 {
		t.Fatalf("starts at %+v, want perched", first)
	}
	if last.DX <= 0 || last.DY >= 0 || last.Alpha != 0 {
		t.Fatalf("ends at %+v, want up and to the right, gone", last)
	}
	img := frame(t, warm.Flying, last)
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			t.Fatal("the last frame of the flight still shows the bird")
		}
	}
}

// The whole flight stays inside the canvas: no frame is cut at its edge.
func TestTheFlightFitsTheCanvas(t *testing.T) {
	for _, m := range Landing(30) {
		img := frame(t, warm.Perched, m)
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		for x := 0; x < w; x++ {
			for _, y := range []int{0} {
				if img.RGBAAt(x, y).A != 0 {
					t.Fatalf("frame %+v is cut at the top edge", m)
				}
			}
		}
		for y := 0; y < h; y++ {
			if img.RGBAAt(w-1, y).A != 0 {
				t.Fatalf("frame %+v is cut at the right edge", m)
			}
		}
	}
}

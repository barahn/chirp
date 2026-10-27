// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package perch

import (
	"image"
	"math"
)

// margin is the gap, in pixels, between the bird and the tray or the edge.
const margin = 8

// Spot is where a bird window w by h pixels goes: standing on the taskbar's
// top edge, just left of the tray, when the taskbar is at the bottom; at the
// bottom right of the work area otherwise, or when no taskbar is found.
// All rectangles are in screen pixels.
func Spot(taskbar, tray, work image.Rectangle, w, h int) image.Rectangle {
	bottomBar := !taskbar.Empty() && taskbar.Dx() > taskbar.Dy() && taskbar.Min.Y >= work.Max.Y-1
	if bottomBar {
		right := taskbar.Max.X - margin
		if !tray.Empty() {
			right = tray.Min.X - margin
		}
		return image.Rect(right-w, taskbar.Min.Y-h, right, taskbar.Min.Y)
	}
	return image.Rect(work.Max.X-margin-w, work.Max.Y-h, work.Max.X-margin, work.Max.Y)
}

// HeadY is the screen row of the top of the bird's head in a bird window at
// bird: the body's top, at y = 34 in the art.
func HeadY(bird image.Rectangle) int {
	return bird.Min.Y + int(math.Round((34-canvasY)*float64(bird.Dy())/canvasH))
}

// BalloonAt is where a balloon of size goes: just above the bird's head,
// its right edge over the bird's, kept inside the work area.
func BalloonAt(bird image.Rectangle, size image.Point, work image.Rectangle) image.Rectangle {
	r := image.Rectangle{Max: size}.Add(image.Pt(bird.Max.X-size.X, HeadY(bird)-4-size.Y))
	if r.Max.X > work.Max.X {
		r = r.Add(image.Pt(work.Max.X-r.Max.X, 0))
	}
	if r.Min.X < work.Min.X {
		r = r.Add(image.Pt(work.Min.X-r.Min.X, 0))
	}
	if r.Min.Y < work.Min.Y {
		r = r.Add(image.Pt(0, work.Min.Y-r.Min.Y))
	}
	return r
}

// Hit is the box, in a bird window width pixels wide, that takes clicks:
// the perched bird's, beak and tail included. The rest of the window lets
// clicks through, where the system needs telling (X11's input shape).
func Hit(width int) image.Rectangle {
	scale := float64(width) / canvasW
	at := func(x, y float64) image.Point {
		return image.Pt(int(math.Round((x-canvasX)*scale)), int(math.Round((y-canvasY)*scale)))
	}
	// The perched bird spans x 12 to 101 and y 34 to 101 in the art.
	return image.Rectangle{Min: at(12, 34), Max: at(101, 102)}
}

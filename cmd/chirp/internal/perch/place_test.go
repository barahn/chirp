// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package perch

import (
	"image"
	"testing"
)

// A 1920x1080 screen with a 48-pixel taskbar at the bottom, its tray from
// x = 1700.
var (
	screen  = image.Rect(0, 0, 1920, 1080)
	taskbar = image.Rect(0, 1032, 1920, 1080)
	tray    = image.Rect(1700, 1032, 1920, 1080)
	work    = image.Rect(0, 0, 1920, 1032)
)

// The bird stands on the taskbar's top edge, just left of the tray.
func TestTheBirdStandsOnTheTaskbarByTheTray(t *testing.T) {
	r := Spot(taskbar, tray, work, 110, 110)
	if r.Max.Y != taskbar.Min.Y {
		t.Fatalf("feet at y %d, want on the taskbar's edge, %d", r.Max.Y, taskbar.Min.Y)
	}
	if r.Max.X > tray.Min.X || r.Max.X < tray.Min.X-40 {
		t.Fatalf("right edge at x %d, want just left of the tray at %d", r.Max.X, tray.Min.X)
	}
	if r.Dx() != 110 || r.Dy() != 110 {
		t.Fatalf("size %v, want 110x110", r.Size())
	}
}

// With the taskbar elsewhere, or none found, the bird stands at the bottom
// right of the work area, on whatever is below it.
func TestWithoutABottomTaskbarTheBirdStandsInTheCorner(t *testing.T) {
	left := image.Rect(0, 0, 48, 1080)
	r := Spot(left, image.Rectangle{}, image.Rect(48, 0, 1920, 1080), 110, 110)
	if r.Max.Y != 1080 || r.Max.X != 1920-8 {
		t.Fatalf("bird at %v, want standing in the bottom right corner", r)
	}
	r = Spot(image.Rectangle{}, image.Rectangle{}, screen, 110, 110)
	if r.Max.Y != 1080 {
		t.Fatalf("bird at %v without a taskbar, want at the bottom", r)
	}
}

// The balloon sits just above the bird's head, its right edge over the
// bird's, and never off the work area.
func TestTheBalloonSitsAboveTheBird(t *testing.T) {
	bird := Spot(taskbar, tray, work, 110, 110)
	b := BalloonAt(bird, image.Pt(360, 200), work)
	head := HeadY(bird)
	if b.Max.Y > head || b.Max.Y < head-16 {
		t.Fatalf("balloon bottom at %d, want just above the head at %d", b.Max.Y, head)
	}
	if !b.In(work) {
		t.Fatalf("balloon %v leaves the work area %v", b, work)
	}
	wide := BalloonAt(image.Rect(0, 920, 110, 1032), image.Pt(360, 200), work)
	if wide.Min.X < 0 {
		t.Fatalf("balloon %v off the left edge", wide)
	}
	tall := BalloonAt(bird, image.Pt(360, 2000), work)
	if tall.Min.Y < 0 {
		t.Fatalf("balloon %v off the top", tall)
	}
}

// The bird's head is where its body's top is drawn in the canvas.
func TestTheHeadIsWhereTheBodyStarts(t *testing.T) {
	bird := image.Rect(0, 0, 164, 164)
	if got := HeadY(bird); got < 94 || got > 98 {
		t.Fatalf("head at %d in a 164-pixel canvas, want about 96 (the body's top at y = 34)", got)
	}
}

// Only the perched bird takes clicks: its box holds its body and stops
// well short of the canvas's empty top and right, where the flight in
// starts.
func TestOnlyThePerchedBirdTakesClicks(t *testing.T) {
	const w = 164
	hit := Hit(w)
	body := image.Pt(58, 64-int(canvasY)) // the body's middle, in canvas pixels
	if !body.In(hit) {
		t.Fatalf("hit box %v misses the body at %v", hit, body)
	}
	if hit.Min.Y < 90 || hit.Max.X > 110 {
		t.Fatalf("hit box %v reaches into the empty canvas", hit)
	}
}

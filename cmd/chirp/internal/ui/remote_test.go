// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"image"
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/barahn/remotekit/bark"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// A 16:9 screen in a square area is drawn full width, with bands above and
// below; positions on the bands are not on the screen.
func TestPositionsMapOntoTheScreenNotTheBands(t *testing.T) {
	area, pic := fyne.NewSize(400, 400), image.Pt(1600, 900) // drawn 400x225, from y 87.5
	for _, tc := range []struct {
		p      fyne.Position
		x, y   float64
		inside bool
	}{
		{fyne.NewPos(0, 87.5), 0, 0, true},
		{fyne.NewPos(200, 200), 0.5, 0.5, true},
		{fyne.NewPos(400, 312.5), 1, 1, true},
		{fyne.NewPos(200, 40), 0.5, -0.211111, false},
	} {
		x, y, ok := fitPosition(tc.p, area, pic)
		if ok != tc.inside || !near(x, tc.x) || math.Abs(y-tc.y) > 1e-5 {
			t.Errorf("fitPosition(%v) = %.4f, %.4f, %v; want %.4f, %.4f, %v", tc.p, x, y, ok, tc.x, tc.y, tc.inside)
		}
	}
	if _, _, ok := fitPosition(fyne.NewPos(1, 1), area, image.Point{}); ok {
		t.Error("a position mapped before any frame arrived")
	}
}

func TestKeysGoOverAsTheirPhysicalDOMCodes(t *testing.T) {
	for name, want := range map[fyne.KeyName]string{
		fyne.KeyA:             "KeyA",
		fyne.Key0:             "Digit0",
		fyne.KeyReturn:        "Enter",
		fyne.KeyBackspace:     "Backspace",
		fyne.KeyF12:           "F12",
		fyne.KeyF1:            "F1",
		fyne.KeyPageUp:        "PageUp",
		desktop.KeyShiftLeft:  "ShiftLeft",
		desktop.KeySuperRight: "MetaRight",
		fyne.KeyLeftBracket:   "BracketLeft",
	} {
		if code, _, ok := domKey(name); !ok || code != want {
			t.Errorf("%s -> %q, %v; want %q", name, code, ok, want)
		}
	}
	if _, key, _ := domKey(fyne.KeyQ); key != "q" {
		t.Errorf("Q's key value is %q, want the unshifted %q", key, "q")
	}
	if _, _, ok := domKey(fyne.KeyName("NoSuchKey")); ok {
		t.Error("an unknown key mapped to something")
	}
}

func TestMouseButtonsUseTheDOMNumbering(t *testing.T) {
	if mouseButton(desktop.MouseButtonPrimary) != 0 || mouseButton(desktop.MouseButtonTertiary) != 1 || mouseButton(desktop.MouseButtonSecondary) != 2 {
		t.Error("mouse buttons are not 0 primary, 1 middle, 2 secondary")
	}
}

func frame(w, h int, luma byte) *image.YCbCr {
	f := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for i := range f.Y {
		f.Y[i] = luma
	}
	return f
}

// Frames faster than the window draws are dropped, only the latest is drawn,
// and buffers are recycled rather than allocated per frame.
func TestTheMailboxKeepsTheLatestFrame(t *testing.T) {
	var m frameMailbox
	if !m.post(frame(4, 4, 1)) {
		t.Fatal("the first frame did not ask for a draw")
	}
	if m.post(frame(4, 4, 2)) || m.post(frame(4, 4, 3)) {
		t.Fatal("a second draw was scheduled while one was pending")
	}
	got := m.take()
	if got == nil || got.Y[0] != 3 {
		t.Fatalf("drew %v, want the latest frame", got)
	}
	if m.take() != nil {
		t.Error("the same frame was handed over twice")
	}
	seen := map[*image.YCbCr]bool{got: true}
	for i := 0; i < 20; i++ {
		m.post(frame(4, 4, byte(i)))
		if f := m.take(); f != nil {
			seen[f] = true
		}
	}
	if len(seen) > 3 {
		t.Errorf("%d buffers were allocated, want at most 3", len(seen))
	}
}

func TestCopyFrameIsIndependentOfItsSource(t *testing.T) {
	src := frame(6, 4, 9)
	dst := copyFrame(nil, src)
	src.Y[0], src.Cb[0] = 1, 1
	if dst.Y[0] != 9 || dst.Cb[0] != 0 || dst.Rect != src.Rect {
		t.Errorf("the copy changed with its source, or lost its size: %v", dst.Rect)
	}
}

// What the user does over the remote screen goes out as input events, mapped
// onto the picture; End gives the session up.
func TestTheRemoteScreenSendsInputAndEnds(t *testing.T) {
	w := newWindow(t)
	ends := 0
	var sent []bark.InputEvent
	r := w.OpenRemote(func(ev bark.InputEvent) { sent = append(sent, ev) }, func() { ends++ })
	r.view.Resize(fyne.NewSize(400, 225))
	r.view.setFrame(frame(1600, 900, 50))

	r.view.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 50)}})
	r.view.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 50)}, Button: desktop.MouseButtonSecondary})
	r.view.KeyDown(&fyne.KeyEvent{Name: fyne.KeyA})
	r.view.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(10, 10)}, Scrolled: fyne.Delta{DY: 3}})
	if len(sent) != 4 {
		t.Fatalf("sent %d events, want 4: %+v", len(sent), sent)
	}
	if mv := sent[0]; mv.Type != "mouse_move" || !near(mv.X, 0.25) || !near(mv.Y, 50.0/225) {
		t.Errorf("move sent as %+v", mv)
	}
	if down := sent[1]; down.Type != "mouse_down" || down.Button != 2 {
		t.Errorf("right click sent as %+v", down)
	}
	if k := sent[2]; k.Type != "key_down" || k.Code != "KeyA" {
		t.Errorf("A sent as %+v", k)
	}
	if s := sent[3]; s.Type != "scroll" || s.DeltaY >= 0 {
		t.Errorf("scrolling up sent as %+v, want a negative deltaY", s)
	}

	test.Tap(r.stop)
	test.Tap(r.stop)
	if ends != 1 {
		t.Errorf("End ended the session %d times, want once", ends)
	}
	r.Close()
}

// A button released off the picture, over the bands beside it, is still
// released at the other end -- at the picture's edge -- so it is not left
// held. Other events off the picture are not sent.
func TestAReleaseOffThePictureStillReleases(t *testing.T) {
	w := newWindow(t)
	var sent []bark.InputEvent
	r := w.OpenRemote(func(ev bark.InputEvent) { sent = append(sent, ev) }, func() {})
	defer r.Close()
	r.view.Resize(fyne.NewSize(400, 400)) // a 16:9 picture: bands above and below
	r.view.setFrame(frame(1600, 900, 50))

	off := &desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(200, 10)}}
	r.view.MouseDown(off)
	r.view.MouseUp(off)
	if len(sent) != 1 || sent[0].Type != "mouse_up" || sent[0].Y != 0 || !near(sent[0].X, 0.5) {
		t.Fatalf("off the picture sent %+v, want only the release, at the top edge", sent)
	}
}

// A frame arriving clears a notice that said why there was none.
func TestAFrameClearsTheNotice(t *testing.T) {
	w := newWindow(t)
	r := w.OpenRemote(func(bark.InputEvent) {}, func() {})
	defer r.Close()
	r.notice.SetText(remoteNotice("capture_unavailable"))
	r.notice.Show()
	r.Show(frame(64, 48, 10))
	waitUntil(t, func() bool { return !r.notice.Visible() })
}

// waitUntil polls cond until it holds, failing the test after a few seconds.
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

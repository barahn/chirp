// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"image"
	"io"
	"slices"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/barahn/remotekit/bark"
)

// Remote is the controller's window onto the other computer (docs/ux.md,
// During a session): its screen fills the window, under a toolbar with Send a
// file and End. Files dropped on the window are sent too. Closing the window
// ends the session. The clipboard needs no button: cmd/chirp syncs it both
// ways while the other person allows it (#134).
//
// Show, Notice, Granted and FileReceived may be called from any goroutine;
// everything else from Fyne's.
type Remote struct {
	// SendFile is how Send a file reaches the other computer: cmd/chirp
	// sets it on Fyne's goroutine, right after OpenRemote. It is called off
	// Fyne's goroutine, and returns once the other end has saved the file or
	// refused it.
	SendFile func(name string, f io.ReadSeeker, progress func(sent, total int64)) error

	w      *Window
	end    func()
	app    fyne.App
	win    fyne.Window
	tools  *remoteTools
	view   *remoteView
	notice *widget.Label
	// limits says what the other person has not allowed; it stays while
	// the picture plays, unlike notice. control is whether they allow the
	// mouse and keyboard, read on Fyne's goroutine as input arrives.
	limits  *widget.Label
	control atomic.Bool
	stop    *widget.Button
	frames  frameMailbox
	closed  bool
}

// OpenRemote opens the remote screen. input receives what the user does over
// it, as remotekit input events, on Fyne's goroutine. End, or closing the
// window, calls end once: it ends this session and no other, so a window
// still closing from an earlier session cannot cancel a newer attempt.
func (w *Window) OpenRemote(input func(bark.InputEvent), end func()) *Remote {
	r := &Remote{w: w, app: w.app, win: w.app.NewWindow(tr(remoteTitle))}
	ended := false
	finish := func() {
		if ended {
			return
		}
		ended = true
		end()
	}
	// Until the other computer says what it allows, input is sent: an older
	// Chirp never says, and its runner drops what it does not allow anyway.
	r.control.Store(true)
	r.view = newRemoteView(func(ev bark.InputEvent) {
		if r.control.Load() {
			input(ev)
		}
	})
	r.notice = widget.NewLabel("")
	r.notice.Wrapping = fyne.TextWrapWord
	r.notice.Hide()
	r.limits = widget.NewLabel("")
	r.limits.Wrapping = fyne.TextWrapWord
	r.limits.Importance = widget.WarningImportance
	r.limits.Hide()
	r.stop = widget.NewButton(tr(remoteEnd), finish)
	r.stop.Importance = widget.DangerImportance
	r.tools = newRemoteTools(r)
	bar := container.NewVBox(
		container.NewBorder(nil, nil, nil, r.stop, r.tools.buttons()),
		r.limits, r.notice, r.tools.status,
	)
	r.win.SetContent(container.NewBorder(bar, nil, nil, nil, r.view))
	r.win.SetOnDropped(r.tools.dropped)
	r.win.SetCloseIntercept(finish)
	r.win.Resize(fyne.NewSize(1024, 640))
	r.win.Show()
	r.win.Canvas().Focus(r.view)
	r.end = finish
	w.remote = r
	w.updateTray()
	return r
}

// Show draws a frame of the remote screen. img is copied, so the caller may
// reuse it as soon as Show returns; frames that arrive faster than the window
// draws are skipped.
func (r *Remote) Show(img *image.YCbCr) {
	if r.frames.post(img) {
		fyne.Do(func() {
			if f := r.frames.take(); f != nil && !r.closed {
				r.view.setFrame(f)
				// The picture arriving answers whatever was said
				// instead of it.
				if r.notice.Visible() {
					r.notice.Hide()
					r.win.Content().Refresh()
				}
			}
		})
	}
}

// Notice shows what the other computer said instead of its screen: kind is
// remotekit's notice type.
func (r *Remote) Notice(kind string) {
	fyne.Do(func() {
		if text := remoteNotice(kind); text != "" && !r.closed {
			r.notice.SetText(text)
			r.notice.Show()
			r.win.Content().Refresh()
		}
	})
}

// Granted takes what the other person allows, as their runner announces it:
// at the start and whenever they take a permission back. Without the mouse
// and keyboard the window says so and sends no input; Send a file works only
// while files are allowed. It may be called from any goroutine.
func (r *Remote) Granted(permissions []string) {
	control := slices.Contains(permissions, "remote_control")
	r.control.Store(control)
	fyne.Do(func() {
		if r.closed {
			return
		}
		r.tools.granted(permissions)
		if text := limitsText(permissions); text != "" {
			r.limits.SetText(text)
			r.limits.Show()
		} else {
			r.limits.Hide()
		}
		r.win.Content().Refresh()
	})
}

// FileReceived says a file the other person sent was saved in folder. It
// may be called from any goroutine.
func (r *Remote) FileReceived(name, folder string) {
	fyne.Do(func() {
		if !r.closed {
			r.tools.received(name, folder)
		}
	})
}

// Close closes the window.
func (r *Remote) Close() {
	if r.closed {
		return
	}
	r.closed = true
	r.win.Close()
	if r.w.remote == r {
		r.w.remote = nil
		r.w.updateTray()
	}
}

// frameMailbox hands frames from the decoder's goroutine to Fyne's through
// three recycled buffers: one on screen, one being written, one waiting. A
// frame that waits is replaced by the next if Fyne has not taken it yet.
type frameMailbox struct {
	mu        sync.Mutex
	free      []*image.YCbCr
	latest    *image.YCbCr
	shown     *image.YCbCr
	scheduled bool
}

// post copies img in, and reports whether a draw needs scheduling.
func (m *frameMailbox) post(img *image.YCbCr) bool {
	m.mu.Lock()
	var buf *image.YCbCr
	if n := len(m.free); n > 0 {
		buf, m.free = m.free[n-1], m.free[:n-1]
	} else if m.latest != nil {
		buf, m.latest = m.latest, nil // not drawn yet: overwrite it
	}
	m.mu.Unlock()

	buf = copyFrame(buf, img)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.latest != nil {
		m.free = append(m.free, m.latest)
	}
	m.latest = buf
	if m.scheduled {
		return false
	}
	m.scheduled = true
	return true
}

// take returns the frame to draw, and recycles the one it replaces.
func (m *frameMailbox) take() *image.YCbCr {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduled = false
	f := m.latest
	if f == nil {
		return nil
	}
	m.latest = nil
	if m.shown != nil {
		m.free = append(m.free, m.shown)
	}
	m.shown = f
	return f
}

// copyFrame copies src's visible pixels into dst, reallocating dst if its size
// differs, and returns it.
func copyFrame(dst, src *image.YCbCr) *image.YCbCr {
	r := image.Rect(0, 0, src.Rect.Dx(), src.Rect.Dy())
	if dst == nil || dst.Rect != r || dst.SubsampleRatio != src.SubsampleRatio {
		dst = image.NewYCbCr(r, src.SubsampleRatio)
	}
	w, h := r.Dx(), r.Dy()
	for y := 0; y < h; y++ {
		copy(dst.Y[y*dst.YStride:y*dst.YStride+w], src.Y[src.YOffset(src.Rect.Min.X, src.Rect.Min.Y+y):])
	}
	cw, ch := (w+1)/2, (h+1)/2
	for y := 0; y < ch; y++ {
		so := src.COffset(src.Rect.Min.X, src.Rect.Min.Y+2*y)
		copy(dst.Cb[y*dst.CStride:y*dst.CStride+cw], src.Cb[so:])
		copy(dst.Cr[y*dst.CStride:y*dst.CStride+cw], src.Cr[so:])
	}
	return dst
}

// remoteView draws the remote screen, scaled to fit with its proportions
// kept, and turns what the user does over it into input events.
type remoteView struct {
	widget.BaseWidget
	img   *canvas.Image
	input func(bark.InputEvent)
	size  image.Point // the remote screen's size, for mapping positions
}

func newRemoteView(input func(bark.InputEvent)) *remoteView {
	v := &remoteView{input: input, img: canvas.NewImageFromImage(nil)}
	v.img.FillMode = canvas.ImageFillContain
	v.img.ScaleMode = canvas.ImageScaleSmooth
	v.ExtendBaseWidget(v)
	return v
}

func (v *remoteView) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(v.img) }

func (v *remoteView) MinSize() fyne.Size { return fyne.NewSize(320, 200) }

func (v *remoteView) setFrame(f *image.YCbCr) {
	v.size = f.Rect.Size()
	v.img.Image = f
	v.img.Refresh()
}

// position maps a point in the widget to a fraction of the remote screen, and
// reports whether it falls on the picture rather than on the bands beside it.
func (v *remoteView) position(p fyne.Position) (x, y float64, ok bool) {
	return fitPosition(p, v.Size(), v.size)
}

// fitPosition is position for a widget of size area showing a picture of size
// pic scaled to fit.
func fitPosition(p fyne.Position, area fyne.Size, pic image.Point) (x, y float64, ok bool) {
	if pic.X <= 0 || pic.Y <= 0 || area.Width <= 0 || area.Height <= 0 {
		return 0, 0, false
	}
	scale := min(float64(area.Width)/float64(pic.X), float64(area.Height)/float64(pic.Y))
	w, h := float64(pic.X)*scale, float64(pic.Y)*scale
	ox, oy := (float64(area.Width)-w)/2, (float64(area.Height)-h)/2
	x, y = (float64(p.X)-ox)/w, (float64(p.Y)-oy)/h
	return x, y, x >= 0 && x <= 1 && y >= 0 && y <= 1
}

func (v *remoteView) send(ev bark.InputEvent) {
	if v.input != nil {
		v.input(ev)
	}
}

func (v *remoteView) mouse(kind string, e *desktop.MouseEvent) {
	x, y, ok := v.position(e.Position)
	if !ok {
		// A release off the picture still releases, at its edge: dropped,
		// it would leave the button held at the other end.
		if kind != "mouse_up" || v.size == (image.Point{}) {
			return
		}
		x, y = max(0, min(1, x)), max(0, min(1, y))
	}
	v.send(bark.InputEvent{Type: kind, X: x, Y: y, Button: mouseButton(e.Button)})
}

// mouseButton maps Fyne's buttons to the DOM numbering remotekit uses: 0 is
// the primary button, 1 the middle one, 2 the secondary.
func mouseButton(b desktop.MouseButton) int {
	switch b {
	case desktop.MouseButtonSecondary:
		return 2
	case desktop.MouseButtonTertiary:
		return 1
	default:
		return 0
	}
}

// desktop.Hoverable

func (v *remoteView) MouseIn(e *desktop.MouseEvent)    { v.mouse("mouse_move", e) }
func (v *remoteView) MouseMoved(e *desktop.MouseEvent) { v.mouse("mouse_move", e) }
func (v *remoteView) MouseOut()                        {}

// desktop.Mouseable

func (v *remoteView) MouseDown(e *desktop.MouseEvent) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(v); c != nil {
		c.Focus(v)
	}
	v.mouse("mouse_down", e)
}

func (v *remoteView) MouseUp(e *desktop.MouseEvent) { v.mouse("mouse_up", e) }

// fyne.Scrollable. Fyne's DY is positive scrolling up; the DOM's deltaY,
// which remotekit takes, is positive scrolling down.
func (v *remoteView) Scrolled(e *fyne.ScrollEvent) {
	x, y, ok := v.position(e.Position)
	if !ok {
		return
	}
	v.send(bark.InputEvent{Type: "scroll", X: x, Y: y, DeltaX: float64(-e.Scrolled.DX), DeltaY: float64(-e.Scrolled.DY)})
}

// fyne.Focusable and desktop.Keyable: keys go over as physical key presses,
// down and up, so modifiers and repeats behave as they would at the other
// keyboard. Typed runes are not sent separately.

func (v *remoteView) FocusGained()             {}
func (v *remoteView) FocusLost()               {}
func (v *remoteView) TypedRune(rune)           {}
func (v *remoteView) TypedKey(*fyne.KeyEvent)  {}
func (v *remoteView) AcceptsTab() bool         { return true }
func (v *remoteView) KeyDown(e *fyne.KeyEvent) { v.key("key_down", e) }
func (v *remoteView) KeyUp(e *fyne.KeyEvent)   { v.key("key_up", e) }

func (v *remoteView) key(kind string, e *fyne.KeyEvent) {
	code, key, ok := domKey(e.Name)
	if !ok {
		return
	}
	v.send(bark.InputEvent{Type: kind, Code: code, Key: key})
}

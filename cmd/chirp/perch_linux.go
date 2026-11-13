// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"context"
	"fmt"
	"image"
	"math/rand/v2"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/shape"
	"github.com/jezek/xgb/xproto"

	"github.com/barahn/chirp/cmd/chirp/internal/perch"
	"github.com/barahn/chirp/cmd/chirp/internal/ui"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// The bird by the taskbar on X11 (barahn/chirp#136): an override-redirect
// window with a 32-bit ARGB visual, so the window manager leaves it alone
// -- no frame, no taskbar entry, above the managed windows -- and a
// compositor blends it, so only the bird shows. Its input shape is the
// perched bird's box: clicks anywhere else in it go to what is under it.
// Perched, it walks along the panel (stroll), and onto the tops of windows,
// moving its window.
// Without a compositor there is no transparency, and so no bird; under
// Wayland there is no X11 at all.

const (
	fps       = 30
	birdWidth = 110 // the window's width, as on Windows
)

// x11Perch is ui.Perch on X11.
type x11Perch struct {
	conn  *xgb.Conn
	win   xproto.Window
	gc    xproto.Gcontext
	depth byte

	mu     sync.Mutex
	tapped func()
	width  int
	at     perch.Moment // where it stands now
	shown  bool
	stop   chan struct{} // ends the flight in progress
	stroll *stroll
}

// newPerch makes the bird's window, or returns nil where X11 has no
// compositor or no ARGB visual, or is not there.
func newPerch() ui.Perch {
	p, err := newX11Perch()
	if err != nil {
		fyne.LogError("chirp: no bird by the taskbar", err)
		return nil
	}
	return p
}

func newX11Perch() (*x11Perch, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)
	cm, err := atom(conn, fmt.Sprintf("_NET_WM_CM_S%d", conn.DefaultScreen))
	if err != nil {
		conn.Close()
		return nil, err
	}
	if owner, err := xproto.GetSelectionOwner(conn, cm).Reply(); err != nil || owner.Owner == 0 {
		conn.Close()
		return nil, fmt.Errorf("no compositor")
	}
	visual := xproto.Visualid(0)
	for _, d := range screen.AllowedDepths {
		if d.Depth != 32 {
			continue
		}
		for _, v := range d.Visuals {
			if v.Class == xproto.VisualClassTrueColor {
				visual = v.VisualId
				break
			}
		}
	}
	if visual == 0 {
		conn.Close()
		return nil, fmt.Errorf("no 32-bit visual")
	}
	if err := shape.Init(conn); err != nil {
		conn.Close()
		return nil, err
	}
	_ = randr.Init(conn) // without it, the whole screen is the monitor

	colormap, _ := xproto.NewColormapId(conn)
	_ = xproto.CreateColormapChecked(conn, xproto.ColormapAllocNone, colormap, screen.Root, visual).Check()
	win, _ := xproto.NewWindowId(conn)
	mask := uint32(xproto.CwBackPixel | xproto.CwBorderPixel | xproto.CwOverrideRedirect | xproto.CwEventMask | xproto.CwColormap)
	values := []uint32{0, 0, 1, xproto.EventMaskButtonRelease | xproto.EventMaskEnterWindow, uint32(colormap)}
	if err := xproto.CreateWindowChecked(conn, 32, win, screen.Root, 0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, visual, mask, values).Check(); err != nil {
		conn.Close()
		return nil, err
	}
	gc, _ := xproto.NewGcontextId(conn)
	if err := xproto.CreateGCChecked(conn, gc, xproto.Drawable(win), 0, nil).Check(); err != nil {
		conn.Close()
		return nil, err
	}
	p := &x11Perch{conn: conn, win: win, gc: gc, depth: 32}
	go p.events()
	return p, nil
}

// events delivers clicks on the bird, and the pointer passing over it,
// until the connection closes.
func (p *x11Perch) events() {
	for {
		ev, err := p.conn.WaitForEvent()
		if ev == nil && err == nil {
			return
		}
		switch e := ev.(type) {
		case xproto.ButtonReleaseEvent:
			if e.Detail != xproto.ButtonIndex1 {
				continue
			}
			p.mu.Lock()
			tapped := p.tapped
			p.mu.Unlock()
			if tapped != nil {
				fyne.Do(tapped)
			}
		case xproto.EnterNotifyEvent:
			p.mu.Lock()
			s := p.stroll
			p.mu.Unlock()
			if s != nil {
				s.pass(time.Now())
			}
		}
	}
}

// Land flies the bird in to its spot, and sets it walking.
func (p *x11Perch) Land(tapped func()) {
	spot := p.findSpot()
	scale := perch.Scale(spot.Dx())
	home := perch.Ledge{Spot: spot, Bounds: perch.Stretch(spot, p.workArea(), scale)}
	walker := perch.NewWalker(home, scale, fps, perch.Site, rand.Float64)
	p.mu.Lock()
	p.tapped, p.width, p.at = tapped, spot.Dx(), walker.Still(warm.Perched)
	if p.stroll != nil {
		p.stroll.stop()
	}
	p.mu.Unlock()
	p.shapeInput(false)
	xproto.ConfigureWindow(p.conn, p.win,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowStackMode,
		[]uint32{uint32(spot.Min.X), uint32(spot.Min.Y), uint32(spot.Dx()), uint32(spot.Dy()), xproto.StackModeAbove}) // #nosec G115 -- screen coordinates
	landing := perch.Landing(fps)
	p.play(warm.Perched, landing, false)
	s := startStroll(walker, time.Second/fps, time.Duration(len(landing))*time.Second/fps+settleAfterLanding, reducedMotion(), p.ledges, p.showMoment)
	p.mu.Lock()
	p.stroll = s
	p.mu.Unlock()
}

// showMoment draws the walking bird where it stands now.
func (p *x11Perch) showMoment(m perch.Moment) {
	p.mu.Lock()
	flying, w, was := p.stop != nil, p.width, p.at
	if !flying {
		p.at = m
	}
	p.mu.Unlock()
	if flying {
		return
	}
	if m.At != was.At {
		xproto.ConfigureWindow(p.conn, p.win, xproto.ConfigWindowX|xproto.ConfigWindowY,
			[]uint32{uint32(int32(m.At.X)), uint32(int32(m.At.Y))}) // #nosec G115 -- screen coordinates
	}
	if m.Motion.Left != was.Motion.Left {
		p.shapeInput(m.Motion.Left)
	}
	if img, err := perch.Frame(m.Pose, m.Motion, w); err == nil {
		p.blit(img)
	}
}

// ledges are the tops of the windows on the current desktop where the bird
// may stand: normal windows and dialogs, not Chirp's, not minimized, with
// their frames, top first.
func (p *x11Perch) ledges() []perch.Ledge {
	screen := xproto.Setup(p.conn).DefaultScreen(p.conn)
	desktop, _ := p.cardinals(screen.Root, "_NET_CURRENT_DESKTOP")
	stacking, err := p.cardinals(screen.Root, "_NET_CLIENT_LIST_STACKING")
	if err != nil {
		return nil
	}
	hidden, _ := atom(p.conn, "_NET_WM_STATE_HIDDEN")
	normal, _ := atom(p.conn, "_NET_WM_WINDOW_TYPE_NORMAL")
	dialog, _ := atom(p.conn, "_NET_WM_WINDOW_TYPE_DIALOG")
	var tops []image.Rectangle
	for _, id := range slices.Backward(stacking) {
		w := xproto.Window(id)
		if on, err := p.cardinals(w, "_NET_WM_DESKTOP"); err == nil && len(on) == 1 && len(desktop) == 1 && on[0] != desktop[0] && on[0] != 0xFFFFFFFF {
			continue
		}
		if state, _ := p.cardinals(w, "_NET_WM_STATE"); slices.Contains(state, uint32(hidden)) {
			continue
		}
		if kinds, _ := p.cardinals(w, "_NET_WM_WINDOW_TYPE"); len(kinds) > 0 && !slices.Contains(kinds, uint32(normal)) && !slices.Contains(kinds, uint32(dialog)) {
			continue
		}
		if pid, _ := p.cardinals(w, "_NET_WM_PID"); len(pid) == 1 && int(pid[0]) == os.Getpid() {
			continue
		}
		geo, err := xproto.GetGeometry(p.conn, xproto.Drawable(w)).Reply()
		if err != nil {
			continue
		}
		at, err := xproto.TranslateCoordinates(p.conn, w, screen.Root, 0, 0).Reply()
		if err != nil {
			continue
		}
		r := image.Rect(int(at.DstX), int(at.DstY), int(at.DstX)+int(geo.Width), int(at.DstY)+int(geo.Height))
		if frame, _ := p.cardinals(w, "_NET_FRAME_EXTENTS"); len(frame) == 4 {
			r.Min.X -= int(frame[0])
			r.Max.X += int(frame[1])
			r.Min.Y -= int(frame[2])
			r.Max.Y += int(frame[3])
		}
		tops = append(tops, r)
	}
	p.mu.Lock()
	w := p.width
	p.mu.Unlock()
	return perch.Ledges(tops, p.workArea(), w, perch.Height(w))
}

// cardinals reads w's property name as 32-bit numbers: CARDINAL, WINDOW and
// ATOM properties alike.
func (p *x11Perch) cardinals(w xproto.Window, name string) ([]uint32, error) {
	a, err := atom(p.conn, name)
	if err != nil {
		return nil, err
	}
	r, err := xproto.GetProperty(p.conn, false, w, a, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	if err != nil {
		return nil, err
	}
	if r.Format != 32 {
		return nil, fmt.Errorf("%s is not 32-bit", name)
	}
	values := make([]uint32, r.ValueLen)
	for i := range values {
		values[i] = xgb.Get32(r.Value[i*4:])
	}
	return values, nil
}

// shapeInput lets only the bird's box, facing left or right, take clicks.
func (p *x11Perch) shapeInput(left bool) {
	p.mu.Lock()
	w := p.width
	p.mu.Unlock()
	hit := perch.Hit(w, left)
	_ = shape.RectanglesChecked(p.conn, shape.SoSet, shape.SkInput, xproto.ClipOrderingUnsorted, p.win, 0, 0,
		[]xproto.Rectangle{{X: int16(hit.Min.X), Y: int16(hit.Min.Y), Width: uint16(hit.Dx()), Height: uint16(hit.Dy())}}).Check() // #nosec G115 -- a box inside a window about 110 pixels wide
}

// Pose shows the perched bird in pose.
func (p *x11Perch) Pose(name string) {
	pose := warm.Perched
	if name == "chirping" {
		pose = warm.Chirping
	}
	p.mu.Lock()
	s := p.stroll
	p.mu.Unlock()
	if s != nil {
		s.pose(pose)
	}
}

// Hold keeps the bird still under its balloon.
func (p *x11Perch) Hold(held bool) {
	p.mu.Lock()
	s := p.stroll
	p.mu.Unlock()
	if s != nil {
		s.hold(held)
	}
}

// FlyOff sends the bird away, the way it came.
func (p *x11Perch) FlyOff() {
	p.mu.Lock()
	shown, s := p.shown, p.stroll
	p.stroll = nil
	p.mu.Unlock()
	if s != nil {
		s.stop()
	}
	if shown {
		p.play(warm.Flying, perch.FlyingOff(fps), true)
	}
}

// Place puts the balloon just above the bird's head.
func (p *x11Perch) Place(balloon fyne.Window) {
	p.mu.Lock()
	spot := image.Rectangle{Max: image.Pt(p.width, perch.Height(p.width))}.Add(p.at.At)
	p.mu.Unlock()
	nw, ok := balloon.(driver.NativeWindow)
	if !ok || spot.Empty() {
		return
	}
	nw.RunNative(func(ctx any) {
		xc, ok := ctx.(driver.X11WindowContext)
		if !ok || xc.WindowHandle == 0 {
			return
		}
		w := xproto.Window(xc.WindowHandle) // #nosec G115 -- an X11 window ID
		geo, err := xproto.GetGeometry(p.conn, xproto.Drawable(w)).Reply()
		if err != nil {
			return
		}
		at := perch.BalloonAt(spot, image.Pt(int(geo.Width), int(geo.Height)), p.workArea())
		xproto.ConfigureWindow(p.conn, w, xproto.ConfigWindowX|xproto.ConfigWindowY,
			[]uint32{uint32(at.Min.X), uint32(at.Min.Y)}) // #nosec G115 -- screen coordinates
	})
}

// play shows pose through motions, a frame at a time, on a goroutine of
// its own; leaving unmaps the window at the end. A flight in progress is
// cut short.
func (p *x11Perch) play(pose warm.Pose, motions []perch.Motion, leaving bool) {
	p.mu.Lock()
	if p.stop != nil {
		close(p.stop)
	}
	stop := make(chan struct{})
	p.stop = stop
	w := p.width
	p.mu.Unlock()
	go func() {
		t := time.NewTicker(time.Second / fps)
		defer t.Stop()
		for _, m := range motions {
			img, err := perch.Frame(pose, m, w)
			if err != nil {
				return
			}
			p.blit(img)
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
		p.mu.Lock()
		if p.stop == stop {
			p.stop = nil
		}
		p.mu.Unlock()
		if leaving {
			xproto.UnmapWindow(p.conn, p.win)
			p.mu.Lock()
			p.shown = false
			p.mu.Unlock()
		}
	}()
}

// blit shows img in the window, mapping it first if need be.
func (p *x11Perch) blit(img *image.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	// X wants the ARGB visual's pixels as BGRA bytes, premultiplied, as
	// image.RGBA's are.
	data := make([]byte, len(img.Pix))
	for i := 0; i < len(data); i += 4 {
		data[i], data[i+1], data[i+2], data[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	p.mu.Lock()
	shown := p.shown
	p.shown = true
	p.mu.Unlock()
	if !shown {
		xproto.MapWindow(p.conn, p.win)
		xproto.ConfigureWindow(p.conn, p.win, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove})
	}
	xproto.PutImage(p.conn, xproto.ImageFormatZPixmap, xproto.Drawable(p.win), p.gc,
		uint16(w), uint16(h), 0, 0, 0, p.depth, data) // #nosec G115 -- a window about 110 pixels wide
}

// findSpot is where the bird stands now, on the primary monitor.
func (p *x11Perch) findSpot() image.Rectangle {
	work := p.workArea()
	monitor := p.primary()
	var taskbar image.Rectangle
	if work.Max.Y < monitor.Max.Y {
		taskbar = image.Rect(monitor.Min.X, work.Max.Y, monitor.Max.X, monitor.Max.Y)
	}
	return perch.Spot(taskbar, image.Rectangle{}, work, birdWidth, perch.Height(birdWidth))
}

// primary is the primary monitor's rectangle, or the whole screen.
func (p *x11Perch) primary() image.Rectangle {
	screen := xproto.Setup(p.conn).DefaultScreen(p.conn)
	whole := image.Rect(0, 0, int(screen.WidthInPixels), int(screen.HeightInPixels))
	mons, err := randr.GetMonitors(p.conn, screen.Root, true).Reply()
	if err != nil {
		return whole
	}
	for _, m := range mons.Monitors {
		if m.Primary {
			return image.Rect(int(m.X), int(m.Y), int(m.X)+int(m.Width), int(m.Y)+int(m.Height))
		}
	}
	return whole
}

// workArea is the primary monitor's area the panels leave free:
// _NET_WORKAREA, which spans every monitor, cut to the primary one.
func (p *x11Perch) workArea() image.Rectangle {
	monitor := p.primary()
	screen := xproto.Setup(p.conn).DefaultScreen(p.conn)
	a, err := atom(p.conn, "_NET_WORKAREA")
	if err != nil {
		return monitor
	}
	r, err := xproto.GetProperty(p.conn, false, screen.Root, a, xproto.AtomCardinal, 0, 4).Reply()
	if err != nil || r.ValueLen < 4 {
		return monitor
	}
	v := func(i int) int { return int(xgb.Get32(r.Value[i*4:])) }
	work := image.Rect(v(0), v(1), v(0)+v(2), v(1)+v(3)).Intersect(monitor)
	if work.Empty() {
		return monitor
	}
	return work
}

// atom interns name.
func atom(conn *xgb.Conn, name string) (xproto.Atom, error) {
	r, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply() // #nosec G115 -- constant names
	if err != nil {
		return 0, err
	}
	return r.Atom, nil
}

// reducedMotion reports whether the desktop asks for less motion: GNOME's
// and its kin's enable-animations turned off.
func reducedMotion() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "enable-animations").Output()
	return err == nil && strings.TrimSpace(string(out)) == "false"
}

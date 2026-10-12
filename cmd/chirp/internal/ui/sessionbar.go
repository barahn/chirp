// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/barahn/chirp/client"
)

// The owner's session bar (barahn/chirp#77, docs/ux.md, During a session):
// for as long as someone is let in, a window of its own says who, for how
// long, and offers End. Like the consent prompt it carries no bird and no
// brand word. End acts at once, without asking, and closing the bar ends the
// session too: a bar the owner wants gone is a session they want over.
//
// Fyne cannot keep a window above every other one; until a per-OS shim does
// (#77), the bar opens as its own small window and asks for focus.

type sessionBar struct {
	w       *Window
	win     fyne.Window
	session client.ActiveSession
	start   time.Time
	now     func() time.Time
	clock   *widget.Label
	end     *widget.Button
	closed  func()
	done    bool
	stop    chan struct{}
}

// showSessionBar opens the bar for s, replacing one still open.
func (w *Window) showSessionBar(s client.ActiveSession) {
	if w.bar != nil {
		w.bar.dismiss()
	}
	b := &sessionBar{w: w, win: w.app.NewWindow(barTitle), session: s, now: time.Now, stop: make(chan struct{})}
	b.start = b.now()
	w.bar = b

	dot := canvas.NewCircle(theme.Color(theme.ColorNameError))
	dot.Resize(fyne.NewSize(10, 10))
	who := widget.NewLabelWithStyle(barText(s.Name), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	b.clock = widget.NewLabel(elapsedText(0))
	b.end = widget.NewButton(barEnd, b.endNow)
	b.end.Importance = widget.DangerImportance
	b.win.SetContent(container.NewPadded(container.NewHBox(
		container.NewGridWrap(fyne.NewSize(10, 10), dot), who, b.clock, b.end,
	)))
	b.closed = b.endNow
	b.win.SetCloseIntercept(b.closed)
	b.win.SetFixedSize(true)
	b.win.Show()
	b.win.RequestFocus()

	if w.animating {
		go b.run()
	}
}

// run ticks the clock once a second until the bar goes.
func (b *sessionBar) run() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			fyne.Do(b.tick)
		}
	}
}

// tick redraws the clock from the time the session started, not from the
// ticks seen: after the machine sleeps, or the window stalls, it still says
// how long the session has really lasted.
func (b *sessionBar) tick() {
	if b.done {
		return
	}
	b.clock.SetText(elapsedText(int(b.now().Sub(b.start).Seconds())))
}

// endNow is End, or the bar's close button: the session ends at once.
func (b *sessionBar) endNow() {
	if b.done {
		return
	}
	b.dismiss()
	b.w.endTheSession()
}

// dismiss closes the bar without ending anything: the session is already
// over, or another replaces it.
func (b *sessionBar) dismiss() {
	if b.done {
		return
	}
	b.done = true
	close(b.stop)
	b.win.Close()
	if b.w.bar == b {
		b.w.bar = nil
	}
}

// barText says who is in, as a claim.
func barText(name string) string {
	if c := claim(name); c != "" {
		return "“" + c + "” is controlling this computer"
	}
	return "Someone is controlling this computer"
}

// elapsedText is the bar's clock, as minutes and seconds.
func elapsedText(seconds int) string {
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

const (
	barTitle = "Session in progress"
	barEnd   = "End"
)

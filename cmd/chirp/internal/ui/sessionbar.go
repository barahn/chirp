// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"fmt"
	"slices"
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
// Under that line, each permission allowed besides Screen view is a switch
// that only turns off (barahn/chirp#52): the owner can take one back at any
// moment, and it stops at once, but turning one on again needs a new
// request. Taking Screen view back is End.
//
// While files are allowed, the bar has Send a file, and it says when a file
// arrives, with Open folder (#134). Everything else the person helping does
// with the clipboard and files is told in system notifications (Alert).
//
// Fyne cannot keep a window above every other one; until a per-OS shim does
// (#77), the bar opens as its own small window and asks for focus.

type sessionBar struct {
	w       *Window
	win     fyne.Window
	session client.ActiveSession
	start   time.Time
	now     func() time.Time
	who     *widget.Label
	clock   *widget.Label
	end     *widget.Button
	perms   []*permSwitch
	tools   *ownerTools
	granted []string
	closed  func()
	done    bool
	stop    chan struct{}
}

// showSessionBar opens the bar for s, replacing one still open.
func (w *Window) showSessionBar(s client.ActiveSession) {
	if w.bar != nil {
		w.bar.dismiss()
	}
	b := &sessionBar{w: w, win: w.app.NewWindow(tr(barTitle)), session: s, now: time.Now, stop: make(chan struct{})}
	b.start = b.now()
	w.bar = b

	dot := canvas.NewCircle(theme.Color(theme.ColorNameError))
	dot.Resize(fyne.NewSize(10, 10))
	b.granted = append([]string(nil), s.Permissions...)
	b.who = widget.NewLabelWithStyle(barText(s.Name, b.granted), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	b.clock = widget.NewLabel(elapsedText(0))
	b.end = widget.NewButton(tr(barEnd), b.endNow)
	b.end.Importance = widget.DangerImportance
	top := container.NewHBox(container.NewGridWrap(fyne.NewSize(10, 10), dot), b.who, b.clock, b.end)
	content := container.NewVBox(top)
	for _, name := range s.Permissions {
		if name == permScreenView {
			continue
		}
		sw := newPermSwitch(name, true, nil)
		sw.offOnly = true
		sw.changed = func(on bool) {
			if on {
				return
			}
			b.granted = slices.DeleteFunc(b.granted, func(p string) bool { return p == sw.name })
			b.who.SetText(barText(s.Name, b.granted))
			b.tools.granted(b.granted)
			if w.OnRevoke != nil {
				w.OnRevoke(s.ID, sw.name)
			}
		}
		b.perms = append(b.perms, sw)
	}
	if len(b.perms) > 0 {
		hint := widget.NewLabel(tr(barRevokeHint))
		hint.Importance = widget.LowImportance
		content.Add(hint)
		for _, sw := range b.perms {
			content.Add(sw)
		}
	}
	b.tools = newOwnerTools(b)
	content.Add(b.tools.content())
	b.win.SetContent(container.NewPadded(content))
	b.closed = b.endNow
	b.win.SetCloseIntercept(b.closed)
	b.win.SetFixedSize(true)
	b.win.Show()
	b.win.RequestFocus()
	w.updateTray()

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
		b.w.updateTray()
	}
}

// recolor redraws the switches in the theme's current colours.
func (b *sessionBar) recolor() {
	for _, sw := range b.perms {
		sw.Refresh()
	}
}

// barText says who is in, as a claim, and whether they can use the computer
// or only see it.
func barText(name string, granted []string) string {
	c := claim(name)
	control := slices.Contains(granted, "remote_control")
	switch {
	case control && c != "":
		return trf("“%s” is controlling this computer", c)
	case control:
		return tr("Someone is controlling this computer")
	case c != "":
		return trf("“%s” is seeing this screen", c)
	}
	return tr("Someone is seeing this screen")
}

// elapsedText is the bar's clock, as minutes and seconds.
func elapsedText(seconds int) string {
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

const (
	barTitle      = "Session in progress"
	barEnd        = "End"
	barRevokeHint = "Turn one off to take it back. It stays off until the next request."
)

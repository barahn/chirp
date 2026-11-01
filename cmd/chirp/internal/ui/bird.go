// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/chirpsound"

	"fyne.io/fyne/v2"
)

// The bird by the taskbar (barahn/chirp#136, docs/ux.md, During a session).
//
// During a session the owner sees no session window: the bird flies in and
// perches by the tray, and its balloon is the session bar -- who is in, the
// clock, End and the switches that only turn off. The balloon opens on
// landing to say who is in, and for each alert in turn, then folds away;
// tapping the bird opens it until tapped again. When the session ends the
// bird flies off and the main window comes back.
//
// The bird itself is drawn by cmd/chirp, in a window of its own shape where
// the system allows one (Perch). Without it -- Wayland and macOS for now --
// the balloon is an ordinary window that stays out, as the session bar did,
// and alerts are system notifications.

// Perch is the bird by the taskbar. Its methods are called on Fyne's
// goroutine.
type Perch interface {
	// Land flies the bird in and perches it by the tray. tapped runs on
	// Fyne's goroutine each time the bird is clicked.
	Land(tapped func())
	// Pose shows the bird in one of its poses: "perched", "chirping".
	Pose(name string)
	// Place puts the balloon window just above the bird.
	Place(balloon fyne.Window)
	// FlyOff sends the bird away.
	FlyOff()
	// Hold keeps the bird still on its spot while held, under its open
	// balloon; let go, it walks along the taskbar again.
	Hold(held bool)
}

// Prefs is where the one thing Chirp remembers between runs is kept:
// whether the bird is hidden (#49 keeps everything else out of it).
// fyne.Preferences is one.
type Prefs interface {
	BoolWithFallback(key string, fallback bool) bool
	SetBool(key string, value bool)
}

// birdHiddenKey is the bird's one remembered setting.
const birdHiddenKey = "bird.hidden"

// How long the balloon stays open by itself: on landing, long enough to read
// who is in; for an alert, long enough to read a sentence.
const (
	landShow  = 8 * time.Second
	alertShow = 6 * time.Second
)

// Poses the window asks of the bird.
const (
	posePerched  = "perched"
	poseChirping = "chirping"
)

// afterFunc runs f on Fyne's goroutine after d, unless the returned stop is
// called first.
func afterFunc(d time.Duration, f func()) func() {
	t := time.AfterFunc(d, func() { fyne.Do(f) })
	return func() { t.Stop() }
}

// birdHidden reports whether the owner hid the bird, now or in an earlier
// run.
func (w *Window) birdHidden() bool {
	return w.Prefs != nil && w.Prefs.BoolWithFallback(birdHiddenKey, false)
}

// birdOut reports whether the bird is by the taskbar: there is one, and the
// owner has not hidden it.
func (w *Window) birdOut() bool { return w.Perch != nil && !w.birdHidden() }

// setBirdHidden is the tray's Hide and Show the Chirp bird.
func (w *Window) setBirdHidden(hidden bool) {
	if w.Prefs != nil {
		w.Prefs.SetBool(birdHiddenKey, hidden)
	}
	if b := w.bar; b != nil {
		if hidden {
			b.flyOff()
			b.hide()
		} else {
			b.land()
		}
	}
	w.updateTray()
}

// birdItem is the tray's item for the bird, worded by what it will do. It
// is there only while the owner's session has a bird to hide or show.
func (w *Window) birdItem() *fyne.MenuItem {
	if w.Perch == nil || w.bar == nil {
		return nil
	}
	if w.birdHidden() {
		return fyne.NewMenuItem(tr("Show the Chirp bird"), func() { w.setBirdHidden(false) })
	}
	return fyne.NewMenuItem(tr("Hide the Chirp bird"), func() { w.setBirdHidden(true) })
}

// play plays s, if Chirp can.
func (w *Window) play(s chirpsound.Sound) {
	if w.Sound != nil {
		w.Sound(s)
	}
}

// land brings the bird in, with the balloon open to say who is in.
func (b *sessionBar) land() {
	p := b.w.Perch
	if p == nil || b.landed {
		return
	}
	b.landed = true
	p.Land(b.tapped)
	b.w.play(chirpsound.Landed)
	b.show()
	b.folding = b.w.after(landShow, b.fold)
}

// flyOff sends the bird away, if it is out.
func (b *sessionBar) flyOff() {
	if !b.landed {
		return
	}
	b.landed = false
	b.w.Perch.FlyOff()
	b.w.play(chirpsound.Leaving)
}

// tapped is a click on the bird: it opens the balloon, or closes one the
// owner opened.
func (b *sessionBar) tapped() {
	if b.done {
		return
	}
	if b.shown && b.held {
		b.held = false
		b.hide()
		return
	}
	b.held = true
	b.show()
}

// announce says text in the balloon, after whatever it is saying now.
func (b *sessionBar) announce(text string) {
	b.queue = append(b.queue, text)
	if !b.speaking {
		b.next()
	}
}

// next says the next queued alert, or, with none left, folds the balloon.
func (b *sessionBar) next() {
	if b.done {
		return
	}
	if len(b.queue) == 0 {
		b.speaking = false
		b.news.SetText("")
		b.news.Hide()
		b.w.Perch.Pose(posePerched)
		b.fold()
		return
	}
	b.speaking = true
	text := b.queue[0]
	b.queue = b.queue[1:]
	b.news.SetText(text)
	b.news.Show()
	b.w.Perch.Pose(poseChirping)
	b.w.play(chirpsound.Alert)
	// Fit first: show places the balloon above the bird by its size.
	b.fit()
	b.show()
	b.w.after(alertShow, b.next)
}

// fold closes the balloon, unless the owner opened it or it has more to say.
func (b *sessionBar) fold() {
	if b.done || b.held || b.speaking {
		return
	}
	b.hide()
}

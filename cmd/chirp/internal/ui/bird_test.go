// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// fakePerch records what the window asks of the bird by the taskbar.
type fakePerch struct {
	lands, flights int
	poses          []string
	placed         []fyne.Window
	sizes          []fyne.Size // the balloon's size as each was placed
	tapped         func()
	perched        bool
}

func (p *fakePerch) Land(tapped func()) { p.lands++; p.tapped = tapped; p.perched = true }
func (p *fakePerch) Pose(name string)   { p.poses = append(p.poses, name) }
func (p *fakePerch) Place(w fyne.Window) {
	p.placed = append(p.placed, w)
	p.sizes = append(p.sizes, w.Canvas().Size())
}
func (p *fakePerch) FlyOff() { p.flights++; p.perched = false }

// fakePrefs records what the window remembers between runs.
type fakePrefs map[string]bool

func (p fakePrefs) BoolWithFallback(key string, fallback bool) bool {
	if v, ok := p[key]; ok {
		return v
	}
	return fallback
}
func (p fakePrefs) SetBool(key string, v bool) { p[key] = v }

// clock runs the window's timers by hand.
type clock struct{ due []func() }

func (c *clock) after(_ time.Duration, f func()) func() {
	i := len(c.due)
	c.due = append(c.due, f)
	return func() { c.due[i] = nil }
}

// fire runs the timers due so far, in order, once.
func (c *clock) fire() {
	due := c.due
	c.due = nil
	for _, f := range due {
		if f != nil {
			f()
		}
	}
}

// birdWindow is a window with a bird, its notifications, tray, prefs and
// timers recorded.
func birdWindow(t *testing.T) (*Window, *fakePerch, *[]*fyne.Notification, *fakeTray, fakePrefs, *clock) {
	t.Helper()
	w, sent, tray := alertWindow(t)
	p, prefs, c := &fakePerch{}, fakePrefs{}, &clock{}
	w.Perch, w.Prefs, w.after = p, prefs, c.after
	return w, p, sent, tray, prefs, c
}

// menuItem finds the tray menu's item labelled label.
func menuItem(t *testing.T, tray *fakeTray, label string) *fyne.MenuItem {
	t.Helper()
	if tray.menu == nil {
		t.Fatal("no tray menu")
	}
	for _, it := range tray.menu.Items {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("no tray item %q", label)
	return nil
}

// During a session the bird is the session's presence (barahn/chirp#136): it
// lands, and its balloon -- who is in, the clock, End, the switches -- is
// the only session window, borderless above it. The main window is in the
// tray.
func TestTheBirdIsTheOnlySessionWindow(t *testing.T) {
	w, p, _, _, _, _ := birdWindow(t)
	started(w, "Ana")
	if p.lands != 1 || w.bar == nil {
		t.Fatalf("lands %d, balloon %v: want the bird landed with its balloon", p.lands, w.bar != nil)
	}
	if len(p.placed) == 0 || p.placed[len(p.placed)-1] != w.bar.win {
		t.Fatal("the balloon was not placed above the bird")
	}
	if !w.mainHidden {
		t.Fatal("the main window is still out during a session")
	}
	if !w.bar.shown {
		t.Fatal("no balloon on landing to say who is in")
	}
}

// Each alert is a balloon, one at a time and in order, instead of a system
// notification; then the balloon folds away, and the bird stays.
func TestEachAlertBecomesABalloonInOrder(t *testing.T) {
	w, p, sent, _, _, c := birdWindow(t)
	started(w, "Ana")
	c.fire() // the landing balloon folds away
	if w.bar.shown {
		t.Fatal("the landing balloon stayed open")
	}
	w.Alert("s1", Alert{Kind: AlertPastedIn, Size: 5})
	w.Alert("s1", Alert{Kind: AlertCopiedOut, Size: 3})
	if len(*sent) != 0 {
		t.Fatalf("%d system notifications while the bird is out, want none", len(*sent))
	}
	if !w.bar.shown || w.bar.news.Text != "Text pasted into this computer by “Ana”." {
		t.Fatalf("balloon shown %v saying %q, want the paste first", w.bar.shown, w.bar.news.Text)
	}
	c.fire()
	if !w.bar.shown || w.bar.news.Text != "Text you copied went to “Ana”." {
		t.Fatalf("balloon shown %v saying %q, want the copy next", w.bar.shown, w.bar.news.Text)
	}
	c.fire()
	if w.bar.shown {
		t.Fatal("the balloon stayed open after the last alert")
	}
	if !p.perched || len(p.poses) == 0 || p.poses[0] != "chirping" {
		t.Fatalf("perched %v, poses %v: want the bird chirping at an alert, still perched", p.perched, p.poses)
	}
}

// Tapping the bird opens its balloon, which stays until tapped again.
func TestTappingTheBirdOpensAndClosesItsBalloon(t *testing.T) {
	w, p, _, _, _, c := birdWindow(t)
	started(w, "Ana")
	c.fire()
	p.tapped()
	if !w.bar.shown {
		t.Fatal("tapping the bird did not open its balloon")
	}
	c.fire()
	if !w.bar.shown {
		t.Fatal("a balloon the owner opened folded away by itself")
	}
	p.tapped()
	if w.bar.shown {
		t.Fatal("tapping the bird again did not close its balloon")
	}
}

// The balloon does what the session bar did: End ends the session at once,
// and a switch turned off takes the permission back.
func TestTheBirdsBalloonEndsTheSessionAndTakesPermissionsBack(t *testing.T) {
	w, p, _, _, _, _ := birdWindow(t)
	var revoked []string
	w.OnRevoke = func(_, perm string) { revoked = append(revoked, perm) }
	ended := 0
	w.OnEndSession = func() { ended++ }
	started(w, "Ana")
	w.bar.perms[0].toggle()
	if len(revoked) != 1 {
		t.Fatalf("revoked %v, want one permission taken back", revoked)
	}
	w.bar.end.OnTapped()
	if ended != 1 || w.bar != nil {
		t.Fatalf("ended %d, balloon left %v: want End to end the session at once", ended, w.bar != nil)
	}
	if p.flights != 1 {
		t.Fatalf("the bird flew off %d times, want once", p.flights)
	}
}

// When the session ends the bird flies off and the main window comes back:
// one window is left.
func TestTheBirdFliesOffAndTheMainWindowComesBack(t *testing.T) {
	w, p, _, tray, _, _ := birdWindow(t)
	started(w, "Ana")
	w.bar.endNow()
	if p.flights != 1 || p.perched {
		t.Fatalf("flights %d, perched %v: want the bird gone", p.flights, p.perched)
	}
	if w.mainHidden || tray.shown {
		t.Fatalf("main hidden %v, tray shown %v: want the main window back and no tray icon", w.mainHidden, tray.shown)
	}
}

// The tray item for the bird says what it will do. With the bird hidden,
// the alerts are system notifications again, and the tray still has Open
// Chirp and End session.
func TestTheTrayItemHidesAndShowsTheBirdByItsState(t *testing.T) {
	w, p, sent, tray, _, _ := birdWindow(t)
	started(w, "Ana")
	menuItem(t, tray, "Hide the Chirp bird").Action()
	if p.perched || w.bar.shown {
		t.Fatalf("perched %v, balloon %v after Hide: want both gone", p.perched, w.bar.shown)
	}
	menuItem(t, tray, "Open Chirp")
	menuItem(t, tray, "End session")
	w.Alert("s1", Alert{Kind: AlertPastedIn, Size: 5})
	if len(*sent) != 1 {
		t.Fatalf("%d notifications with the bird hidden, want 1", len(*sent))
	}
	menuItem(t, tray, "Show the Chirp bird").Action()
	if !p.perched || p.lands != 2 {
		t.Fatalf("perched %v after %d landings: want the bird back", p.perched, p.lands)
	}
}

// Whether the bird is shown is the one thing remembered between runs, and
// it holds nothing about any session (#49).
func TestTheBirdsShownOrHiddenIsRememberedAndNothingElse(t *testing.T) {
	w, _, _, tray, prefs, _ := birdWindow(t)
	started(w, "Ana")
	menuItem(t, tray, "Hide the Chirp bird").Action()
	if len(prefs) != 1 || !prefs[birdHiddenKey] {
		t.Fatalf("remembered %v, want only that the bird is hidden", prefs)
	}

	next, p, _, _, _, _ := birdWindow(t)
	next.Prefs = prefs
	started(next, "Bia")
	if p.lands != 0 {
		t.Fatal("the bird landed though it was hidden last time")
	}
}

// Without a bird -- a system Chirp cannot draw one on yet -- the balloon is
// the session window as the session bar was: always out, with notifications
// for the alerts, and no tray item for a bird.
func TestWithoutABirdTheBalloonStaysOut(t *testing.T) {
	w, sent, tray := alertWindow(t)
	started(w, "Ana")
	w.Alert("s1", Alert{Kind: AlertPastedIn, Size: 5})
	if !w.bar.shown || len(*sent) != 1 {
		t.Fatalf("balloon shown %v, %d notifications: want the window out and a notification", w.bar.shown, len(*sent))
	}
	for _, it := range tray.menu.Items {
		if it.Label == "Hide the Chirp bird" || it.Label == "Show the Chirp bird" {
			t.Fatalf("tray item %q without a bird", it.Label)
		}
	}
}

// The balloon is placed above the bird once it has its size: grown for an
// alert after it was placed, it would hang down over the bird (seen on X11).
func TestTheBalloonIsPlacedAtItsFinalSize(t *testing.T) {
	w, p, _, _, _, c := birdWindow(t)
	started(w, "Ana")
	c.fire()
	w.Alert("s1", Alert{Kind: AlertPastedIn, Size: 5})
	got := p.sizes[len(p.sizes)-1]
	if want := w.bar.win.Canvas().Size(); got != want {
		t.Fatalf("placed at %v, then grew to %v: placed before it fit", got, want)
	}
}

// So is a balloon that grows while open, for a file sent or received.
func TestABalloonThatGrowsWhileOpenIsPlacedAgain(t *testing.T) {
	w, p, _, _, _, c := birdWindow(t)
	started(w, "Ana")
	c.fire()
	p.tapped()
	w.bar.tools.received("photo.jpg", "/home/ana/Downloads/Chirp")
	got := p.sizes[len(p.sizes)-1]
	if want := w.bar.win.Canvas().Size(); got != want {
		t.Fatalf("placed at %v, then grew to %v", got, want)
	}
}

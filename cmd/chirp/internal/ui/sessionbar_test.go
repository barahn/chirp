// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/barahn/chirp/client"
)

func started(w *Window, name string) {
	w.ApplyOwner(client.OwnerEvent{Started: &client.ActiveSession{ID: "s1", Name: name}})
}

func TestTheBarOpensWhenASessionStarts(t *testing.T) {
	w := newWindow(t)
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
	started(w, "João da TI")
	if w.bar == nil {
		t.Fatal("no bar")
	}
	got := strings.Join(labels(w.bar.win.Content()), " | ")
	if !strings.Contains(got, "“João da TI” is controlling this computer") || !strings.Contains(got, "00:00") {
		t.Errorf("the bar says %q", got)
	}
	// A session event leaves the code and status alone.
	if w.code.Text != "482-913" || w.status.Text != "Ready" {
		t.Errorf("the session event changed the window: %q, %q", w.code.Text, w.status.Text)
	}
}

func TestTheBarCountsAndCleansTheName(t *testing.T) {
	w := newWindow(t)
	started(w, "Ana\nVerified by your bank")
	for i := 0; i < 65; i++ {
		w.bar.tick()
	}
	if w.bar.clock.Text != "01:05" {
		t.Errorf("after 65 s the clock says %q", w.bar.clock.Text)
	}
	if got := barText("Ana\nVerified by your bank"); strings.Contains(got, "\n") {
		t.Errorf("the bar shows a line break: %q", got)
	}
	if got := barText(""); got != "Someone is controlling this computer" {
		t.Errorf("an empty name shows %q", got)
	}
}

func TestEndOnTheBarEndsTheSessionAtOnce(t *testing.T) {
	w := newWindow(t)
	ended := 0
	w.OnEndSession = func() { ended++ }
	started(w, "Ana")
	test.Tap(w.bar.end)
	if ended != 1 || w.bar != nil {
		t.Errorf("End: OnEndSession called %d times, bar still open %v", ended, w.bar != nil)
	}
}

func TestClosingTheBarEndsTheSession(t *testing.T) {
	w := newWindow(t)
	ended := 0
	w.OnEndSession = func() { ended++ }
	started(w, "Ana")
	// What the window manager's close button runs; Fyne's test window never
	// calls the intercept itself.
	bar := w.bar
	bar.closed()
	bar.closed()
	if ended != 1 {
		t.Errorf("closing the bar ended the session %d times, want once", ended)
	}
}

func TestTheBarGoesWhenTheSessionEndsElsewhere(t *testing.T) {
	w := newWindow(t)
	w.OnEndSession = func() { t.Error("a session ended by the other side was ended again") }
	started(w, "Ana")
	bar := w.bar
	w.ApplyOwner(client.OwnerEvent{Ended: true})
	if w.bar != nil || !bar.done || w.request.Text != sessionOverNote || w.endSession.Visible() {
		t.Errorf("after the end: bar %v, note %q", w.bar != nil, w.request.Text)
	}
	// Ticks after it closed change nothing.
	bar.tick()
	if bar.elapsed != 0 {
		t.Error("a closed bar kept counting")
	}
}

func TestANewSessionReplacesTheBar(t *testing.T) {
	w := newWindow(t)
	w.OnEndSession = func() { t.Error("replacing the bar ended a session") }
	started(w, "Ana")
	first := w.bar
	started(w, "Bo")
	if w.bar == first || !first.done || w.bar.session.Name != "Bo" {
		t.Error("the new session did not replace the bar")
	}
}

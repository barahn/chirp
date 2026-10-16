// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/barahn/chirp/client"
	"github.com/barahn/remotekit/bark"
)

// switchFor is the prompt's switch for permission.
func switchFor(t *testing.T, p *prompt, permission string) *permSwitch {
	t.Helper()
	for _, s := range p.switches {
		if s.name == permission {
			return s
		}
	}
	t.Fatalf("the prompt has no switch for %s", permission)
	return nil
}

// unlock lets the seconds Allow is locked for pass.
func unlock(p *prompt) {
	for i := 0; i < allowLock; i++ {
		p.tick()
	}
}

// Only Screen view starts on, and it cannot be turned off: a hurried Allow
// lets in the picture and nothing else (barahn/chirp#52).
func TestOnlyScreenViewStartsOn(t *testing.T) {
	w, _, grants := askedGrants(t, request())
	p := w.prompt
	for _, name := range allFour {
		if on := switchFor(t, p, name).on; on != (name == "screen_view") {
			t.Errorf("%s starts on = %v", name, on)
		}
	}
	screen := switchFor(t, p, "screen_view")
	test.Tap(screen)
	if !screen.on {
		t.Error("a tap turned Screen view off")
	}
	unlock(p)
	test.Tap(p.allow)
	if len(*grants) != 1 || !slices.Equal((*grants)[0], []string{"screen_view"}) {
		t.Fatalf("Allow granted %v, want only screen_view", *grants)
	}
}

// Allow grants what is on, in the order asked for, and tapping a switch does
// not take the focus from Decline.
func TestAllowGrantsWhatIsOn(t *testing.T) {
	w, _, grants := askedGrants(t, request())
	p := w.prompt
	test.Tap(switchFor(t, p, "file_transfer"))
	test.Tap(switchFor(t, p, "remote_control"))
	test.Tap(switchFor(t, p, "file_transfer")) // and off again
	if p.win.Canvas().Focused() != p.decline {
		t.Error("tapping a switch moved the focus from Decline")
	}
	unlock(p)
	test.Tap(p.allow)
	if want := []string{"screen_view", "remote_control"}; len(*grants) != 1 || !slices.Equal((*grants)[0], want) {
		t.Fatalf("Allow granted %v, want %v", *grants, want)
	}
}

// Space turns the focused switch, as a keyboard user reaches it with Tab.
func TestSpaceTurnsTheFocusedSwitch(t *testing.T) {
	w, _ := asked(t, request())
	s := switchFor(t, w.prompt, "clipboard")
	w.prompt.win.Canvas().Focus(s)
	s.TypedRune(' ')
	if !s.on {
		t.Error("Space did not turn the clipboard on")
	}
}

// With nothing on there is nothing to allow: a request that does not ask
// for Screen view leaves Allow disabled until something is turned on.
func TestAllowNeedsSomethingOn(t *testing.T) {
	req := request()
	req.Permissions = []string{"clipboard"}
	w, answers := asked(t, req)
	p := w.prompt
	unlock(p)
	if !p.allow.Disabled() {
		t.Fatal("Allow is enabled with every switch off")
	}
	test.Tap(p.allow)
	if len(*answers) != 0 {
		t.Fatal("Allow answered with nothing on")
	}
	test.Tap(switchFor(t, p, "clipboard"))
	if p.allow.Disabled() {
		t.Error("Allow stayed disabled with the clipboard on")
	}
}

// The bar's switches only turn off: each permission allowed besides Screen
// view can be taken back once, and not given again.
func TestTheBarTakesPermissionsBack(t *testing.T) {
	w := newWindow(t)
	var revoked []string
	w.OnRevoke = func(sessionID, permission string) {
		if sessionID != "s1" {
			t.Errorf("revoked on session %q", sessionID)
		}
		revoked = append(revoked, permission)
	}
	w.ApplyOwner(client.OwnerEvent{Started: &client.ActiveSession{ID: "s1", Name: "Ana", Permissions: []string{"screen_view", "remote_control"}}})
	if w.bar == nil || len(w.bar.perms) != 1 || w.bar.perms[0].name != "remote_control" {
		t.Fatalf("the bar's switches: %+v", w.bar)
	}
	sw := w.bar.perms[0]
	test.Tap(sw)
	test.Tap(sw)
	if sw.on || !slices.Equal(revoked, []string{"remote_control"}) {
		t.Errorf("after two taps: on %v, revoked %v", sw.on, revoked)
	}
	if got := w.bar.who.Text; got != "“Ana” is seeing this screen" {
		t.Errorf("after taking the mouse and keyboard back the bar says %q", got)
	}
}

// Without the mouse and keyboard the remote screen says so and sends no
// input; when they are allowed it sends again and the line goes.
func TestTheRemoteScreenFollowsWhatIsGranted(t *testing.T) {
	w := newWindow(t)
	var sent []bark.InputEvent
	r := w.OpenRemote(func(ev bark.InputEvent) { sent = append(sent, ev) }, func() {})
	defer r.Close()
	r.view.Resize(fyne.NewSize(400, 225))
	r.view.setFrame(frame(1600, 900, 50))
	move := func() {
		r.view.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 50)}})
	}

	r.Granted([]string{"screen_view"})
	move()
	if len(sent) != 0 {
		t.Errorf("sent %d events without remote_control", len(sent))
	}
	if !r.limits.Visible() || r.limits.Text != limitsText([]string{"screen_view"}) {
		t.Errorf("limits %q, visible %v", r.limits.Text, r.limits.Visible())
	}

	r.Granted(allFour)
	move()
	if len(sent) != 1 || r.limits.Visible() {
		t.Errorf("with everything allowed: sent %d, limits visible %v", len(sent), r.limits.Visible())
	}
}

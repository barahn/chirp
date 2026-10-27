// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"testing"

	"github.com/barahn/remotekit/bark"
)

// twoScreens is a computer with two monitors, the second one streamed. The
// indexes are the agent's, which need not run from 0.
func twoScreens() bark.DisplaysPayload {
	return bark.DisplaysPayload{
		Active: 7,
		Displays: []bark.DisplayInfo{
			{Index: 3, Name: "DP-1", Width: 1920, Height: 1080, Primary: true},
			{Index: 7, Name: "HDMI-1", Width: 1280, Height: 1024},
		},
	}
}

func openRemote(t *testing.T) *Remote {
	t.Helper()
	inPlace(t)
	w := newWindow(t)
	r := w.OpenRemote(func(bark.InputEvent) {}, func() {})
	t.Cleanup(r.Close)
	return r
}

// The controller picks which of the other computer's monitors to see
// (barahn/chirp#79). With one there is nothing to pick, and no picker.
func TestTheMonitorPickerShowsOnlyWithSeveralScreens(t *testing.T) {
	r := openRemote(t)
	if r.tools.screens.Visible() {
		t.Fatal("a monitor picker before the other computer said what it has")
	}
	r.Displays(bark.DisplaysPayload{Active: 0, Displays: []bark.DisplayInfo{{Index: 0, Width: 1920, Height: 1080}}})
	if r.tools.screens.Visible() {
		t.Fatal("a monitor picker for a computer with one monitor")
	}
	r.Displays(twoScreens())
	if !r.tools.screens.Visible() {
		t.Fatal("no monitor picker for a computer with two monitors")
	}
	if got, want := r.tools.screens.Options, []string{"Screen 1 of 2", "Screen 2 of 2"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("options %q, want %q", got, want)
	}
	if got := r.tools.screens.Selected; got != "Screen 2 of 2" {
		t.Fatalf("selected %q, want the one being streamed, Screen 2 of 2", got)
	}
}

// Picking a screen asks the other computer for it, by its own index. What the
// other computer says is streamed moves the picker without asking again.
func TestPickingAScreenAsksForIt(t *testing.T) {
	r := openRemote(t)
	var asked []int
	r.SelectDisplay = func(index int) error { asked = append(asked, index); return nil }
	r.Displays(twoScreens())
	if len(asked) != 0 {
		t.Fatalf("asked for %v when the other computer only said what it has", asked)
	}
	r.tools.screens.SetSelected("Screen 1 of 2")
	if len(asked) != 1 || asked[0] != 3 {
		t.Fatalf("asked for %v, want [3]: the first monitor's own index", asked)
	}
	p := twoScreens()
	p.Active = 3
	r.Displays(p)
	if len(asked) != 1 {
		t.Fatalf("asked again, %v, when the other computer confirmed the switch", asked)
	}
}

// A switch the other computer refuses is said, and the picker goes back to
// the screen still being streamed.
func TestARefusedSwitchIsSaid(t *testing.T) {
	r := openRemote(t)
	r.SelectDisplay = func(int) error { return nil }
	r.Displays(twoScreens())
	r.tools.screens.SetSelected("Screen 1 of 2")
	p := twoScreens()
	p.Error = "monitor switched too recently; try again"
	r.Displays(p)
	if !r.tools.status.Visible() || r.tools.status.Text != "That screen can't be shown right now. Try again." {
		t.Fatalf("status %q (visible %v), want the refusal said", r.tools.status.Text, r.tools.status.Visible())
	}
	if got := r.tools.screens.Selected; got != "Screen 2 of 2" {
		t.Fatalf("selected %q, want the screen still streamed, Screen 2 of 2", got)
	}
}

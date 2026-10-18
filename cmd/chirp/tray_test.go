// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
)

// trayCalls records what systemTray does, in order.
type trayCalls struct{ calls []string }

func (c *trayCalls) SetSystemTrayMenu(m *fyne.Menu) { c.calls = append(c.calls, "menu:"+m.Label) }
func (c *trayCalls) SetSystemTrayIcon(fyne.Resource) { c.calls = append(c.calls, "icon") }
func (c *trayCalls) SetSystemTrayWindow(fyne.Window) {}
func (c *trayCalls) visible(v bool) {
	if v {
		c.calls = append(c.calls, "show")
	} else {
		c.calls = append(c.calls, "hide")
	}
}

// Fyne starts its tray only when the app starts running, so the tray is set
// up before then -- hidden first, so no icon ever shows without a session --
// and a session only shows it.
func TestTheTrayIsSetUpHiddenBeforeRunAndShownForASession(t *testing.T) {
	c := &trayCalls{}
	tr := &systemTray{app: c, setVisible: c.visible}
	tr.prepare()
	if want := []string{"hide", "icon", "menu:Chirp"}; !slices.Equal(c.calls, want) {
		t.Fatalf("prepare did %v, want %v", c.calls, want)
	}
	c.calls = nil
	tr.Show(fyne.NewMenu("Session"))
	tr.Hide()
	if want := []string{"menu:Session", "show", "hide"}; !slices.Equal(c.calls, want) {
		t.Fatalf("a session did %v, want %v", c.calls, want)
	}
}

func TestNoTrayIsQuietlyNothing(t *testing.T) {
	var tr *systemTray
	tr.prepare()
	tr.Show(fyne.NewMenu("x"))
	tr.Hide()
}

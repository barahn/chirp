// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// systemTray is the tray icon, there only while a session is on
// (barahn/chirp#134).
//
// Fyne starts its tray only when the app starts running, and cannot take it
// down afterwards. So prepare sets the tray up before ShowAndRun, hidden
// first, and sessions show and hide it with SetVisible, from Barahn's fork
// of fyne.io/systray until upstream has it.
type systemTray struct {
	app        desktop.App
	setVisible func(bool)
}

// newSystemTray returns the tray for a, or nil where the system has none.
func newSystemTray(a fyne.App) *systemTray {
	d, ok := a.(desktop.App)
	if !ok {
		return nil
	}
	return &systemTray{app: d, setVisible: systray.SetVisible}
}

// prepare sets the tray up, hidden. Call it before the app runs.
func (t *systemTray) prepare() {
	if t == nil {
		return
	}
	t.setVisible(false)
	t.app.SetSystemTrayIcon(warm.AppIcon)
	t.app.SetSystemTrayMenu(fyne.NewMenu("Chirp"))
}

func (t *systemTray) Show(m *fyne.Menu) {
	if t == nil {
		return
	}
	t.app.SetSystemTrayMenu(m)
	t.setVisible(true)
}

func (t *systemTray) Hide() {
	if t == nil {
		return
	}
	t.setVisible(false)
}

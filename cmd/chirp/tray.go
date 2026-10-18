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
// (barahn/chirp#134). Fyne starts its tray on the first menu it is given and
// cannot take it down again, so the icon is hidden between sessions with
// SetVisible, from Barahn's fork of fyne.io/systray until upstream has it.
// No tray is started before the first session.
type systemTray struct {
	app     desktop.App
	started bool
}

// newSystemTray returns the tray for a, or nil where the system has none.
func newSystemTray(a fyne.App) *systemTray {
	d, ok := a.(desktop.App)
	if !ok {
		return nil
	}
	return &systemTray{app: d}
}

func (t *systemTray) Show(m *fyne.Menu) {
	if t == nil {
		return
	}
	if !t.started {
		t.app.SetSystemTrayIcon(warm.AppIcon)
		t.started = true
	}
	t.app.SetSystemTrayMenu(m)
	systray.SetVisible(true)
}

func (t *systemTray) Hide() {
	if t == nil || !t.started {
		return
	}
	systray.SetVisible(false)
}

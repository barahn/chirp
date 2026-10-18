// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/systray"
)

// newNotifier returns how alerts are shown on Windows: as a balloon from the
// tray icon, which Windows presents as a notification from Chirp and
// dismisses on its own. Fyne's toasts need an AppUserModelID registered with
// a Start menu shortcut, which a downloaded Chirp does not install (#49), and
// without one Windows drops them silently: seen on Windows 11 with
// v0.5.0-rc.1. They remain the fallback for when the balloon cannot be shown.
func newNotifier(a fyne.App) func(*fyne.Notification) {
	return func(n *fyne.Notification) {
		if err := systray.ShowNotification(n.Title, n.Content); err != nil {
			a.SendNotification(n)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !linux

package main

import "fyne.io/fyne/v2"

// newNotifier returns how alerts are shown: Fyne's own notifications, which
// Windows and macOS dismiss on their own.
func newNotifier(a fyne.App) func(*fyne.Notification) { return a.SendNotification }

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !linux && !windows

package main

import "fyne.io/fyne/v2"

// newNotifier returns how alerts are shown on macOS: Fyne's notifications,
// which macOS dismisses on its own.
func newNotifier(a fyne.App) func(*fyne.Notification) { return a.SendNotification }

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !linux && !windows

package main

import "fyne.io/fyne/v2"

// pinBar does nothing on macOS yet: Chirp has no macOS build.
func pinBar(_, _ fyne.Window) {}

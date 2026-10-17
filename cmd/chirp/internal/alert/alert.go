// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package alert tells the user something when Chirp's own window cannot: a
// native message box on Windows, where a double-clicked chirp.exe has no
// console to print to, and standard error elsewhere. It does not use Fyne,
// since what it reports is usually that Fyne could not open a window.
package alert

// Show reports title and text to the user and returns once they have seen it,
// or at once where there is nothing to wait on.
func Show(title, text string) { show(title, text) }

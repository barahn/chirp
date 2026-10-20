// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows && !darwin

package main

// displayLanguages is nil outside Windows and macOS, where Fyne's
// lang.SystemLocale is already the display language.
func displayLanguages() []string { return nil }

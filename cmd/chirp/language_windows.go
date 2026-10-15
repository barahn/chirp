// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import "golang.org/x/sys/windows"

// displayLanguages is the user's Windows display languages, most preferred
// first, as language names such as "pt-BR". It is nil if Windows does not
// say, and Chirp then follows Fyne's lang.SystemLocale.
func displayLanguages() []string {
	list, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return list
}

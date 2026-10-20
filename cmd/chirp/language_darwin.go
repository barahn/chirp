// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import "os/exec"

// displayLanguages is the languages the Mac shows itself in, most preferred
// first, from the user's AppleLanguages. Fyne reads AppleLocale instead, the
// region, whenever the bundle declares no localizations (barahn/chirp#152).
// It is nil if macOS does not say, and Chirp then follows Fyne.
func displayLanguages() []string {
	out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return nil
	}
	return appleLanguages(string(out))
}

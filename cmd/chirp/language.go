// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"strings"

	"github.com/barahn/chirp/cmd/chirp/internal/ui"
)

// The language Chirp speaks is the one the system shows itself in
// (barahn/chirp#80, docs/ux.md, Languages). Fyne's lang.SystemLocale gives it
// on Linux. On Windows and macOS it gives the regional format -- how dates and
// numbers are written -- which need not be the display language: an English
// Windows set to Brazilian formats would show Chirp in Portuguese, and a
// Portuguese one set to US formats in English (barahn/chirp#104, #152). There,
// displayLanguages reads the display languages instead.

// chooseLanguage picks the first of the display languages, in the user's
// order, that Chirp speaks, so a user who puts English first gets English
// even if Portuguese follows. With none Chirp speaks, or none known, it falls
// back to system, the locale Fyne reports.
func chooseLanguage(display []string, system string) string {
	for _, l := range display {
		if ui.Speaks(l) {
			return l
		}
	}
	return system
}

// appleLanguages reads the output of `defaults read -g AppleLanguages`, the
// languages a Mac shows itself in, most preferred first: a property list such
// as ("en-BR", "pt-BR"), where defaults leaves bare codes unquoted. Anything
// else, as when nothing is set, gives nil.
func appleLanguages(out string) []string {
	out = strings.TrimSpace(out)
	inner, ok := strings.CutPrefix(out, "(")
	if !ok {
		return nil
	}
	if inner, ok = strings.CutSuffix(inner, ")"); !ok {
		return nil
	}
	var list []string
	for _, l := range strings.Split(inner, ",") {
		if l = strings.Trim(strings.TrimSpace(l), `"`); l != "" {
			list = append(list, l)
		}
	}
	return list
}

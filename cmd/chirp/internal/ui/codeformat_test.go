// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// A code typed as digits gets its dashes as it grows, as the other person's
// window shows it: 482-913-5071. Anything else -- a link, a token -- is left
// as typed.
func TestCodesGetTheirDashesAsTheyAreTyped(t *testing.T) {
	for in, want := range map[string]string{
		"":                           "",
		"4":                          "4",
		"482":                        "482",
		"4829":                       "482-9",
		"482913":                     "482-913",
		"4829135":                    "482-913-5",
		"4829135071":                 "482-913-5071",
		"482 913 5071":               "482-913-5071",
		"482-913-5071":               "482-913-5071",
		"482.913":                    "482-913",
		"48291350712":                "48291350712",
		"https://usechirp.app/l#abc": "https://usechirp.app/l#abc",
		"abc123":                     "abc123",
	} {
		if got := formatCode(in); got != want {
			t.Errorf("formatCode(%q) = %q, want %q", in, got, want)
		}
	}
}

// Typing into the field formats it, and leaves the cursor at the end.
func TestTheCodeFieldAddsTheDashes(t *testing.T) {
	w := newWindow(t)
	test.Type(w.target, "4829135071")
	if w.target.Text != "482-913-5071" || w.target.CursorColumn != len("482-913-5071") {
		t.Errorf("typed digits show as %q, cursor at %d", w.target.Text, w.target.CursorColumn)
	}
}

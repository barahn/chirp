// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import "testing"

func TestTheDisplayLanguageWinsOverTheRegionalFormat(t *testing.T) {
	for _, tc := range []struct {
		display []string
		system  string
		want    string
	}{
		// English Windows with Brazilian formats: English.
		{[]string{"en-US"}, "pt-BR", "en-US"},
		// Portuguese Windows with US formats: Portuguese.
		{[]string{"pt-BR"}, "en-US", "pt-BR"},
		// The user's order decides between two Chirp speaks.
		{[]string{"en-GB", "pt-BR"}, "pt-BR", "en-GB"},
		// A display language Chirp lacks is passed over for the next.
		{[]string{"fr-FR", "pt-PT"}, "en-US", "pt-PT"},
		// None Chirp speaks, or none known: the system locale.
		{[]string{"fr-FR"}, "pt-BR", "pt-BR"},
		{nil, "pt-BR", "pt-BR"},
	} {
		if got := chooseLanguage(tc.display, tc.system); got != tc.want {
			t.Errorf("chooseLanguage(%v, %q) = %q, want %q", tc.display, tc.system, got, tc.want)
		}
	}
}

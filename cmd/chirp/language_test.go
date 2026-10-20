// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"slices"
	"testing"
)

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

func TestAppleLanguagesAreReadInTheUsersOrder(t *testing.T) {
	for _, tc := range []struct {
		out  string
		want []string
	}{
		// What `defaults read -g AppleLanguages` prints on a Mac set up in
		// English with Brazilian formats, Portuguese added second.
		{"(\n    \"en-BR\",\n    \"pt-BR\"\n)\n", []string{"en-BR", "pt-BR"}},
		// defaults leaves a bare code unquoted.
		{"(\n    en,\n    \"pt-BR\"\n)\n", []string{"en", "pt-BR"}},
		// Script subtags stay.
		{"(\n    \"zh-Hans-CN\"\n)\n", []string{"zh-Hans-CN"}},
		// Nothing set, or not a list: nil, and Fyne's locale decides.
		{"", nil},
		{"()\n", nil},
		{"The domain/default pair of (kCFPreferencesAnyApplication, AppleLanguages) does not exist\n", nil},
	} {
		got := appleLanguages(tc.out)
		if !slices.Equal(got, tc.want) {
			t.Errorf("appleLanguages(%q) = %q, want %q", tc.out, got, tc.want)
		}
	}
}

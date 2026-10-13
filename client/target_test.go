// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	token := strings.Repeat("aZ0-_", 8) + "abc" // 43 characters
	for _, tc := range []struct {
		in   string
		want Target
	}{
		{"482913", Target{Code: "482913"}},
		{" 482 913 ", Target{Code: "482913"}},
		{"482-913", Target{Code: "482913"}},
		{"482.913", Target{Code: "482913"}},
		{token, Target{Token: token}},
		{"https://chirp.example/l#" + token, Target{Token: token}},
		{"  https://chirp.example/#" + token + "\n", Target{Token: token}},
		{"482-913-5071", Target{Code: "482913", Secret: "5071"}},
		{"482 913 5071", Target{Code: "482913", Secret: "5071"}},
		{"4829135071", Target{Code: "482913", Secret: "5071"}},
		{token + ".0042", Target{Token: token, Secret: "0042"}},
		{"https://chirp.example/l#" + token + ".5071", Target{Token: token, Secret: "5071"}},
	} {
		got, err := ParseTarget(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseTarget(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{
		"",
		"48291",                            // five digits
		"4829135",                          // seven
		"482 91a",                          // a letter
		"٤٨٢٩١٣",                           // digits from another script
		token[:42],                         // a token one short
		token + "x",                        // one long
		"https://chirp.example/l/" + token, // the token outside the fragment
		"https://chirp.example/l#",
		"hello",
		"482-913-507",   // nine digits
		"482-913-50712", // eleven
		token + ".507",  // a short secret
		token + ".50a1", // a letter in it
		token + ".",     // a dot and nothing
		"https://chirp.example/l#" + token + ".12345",
	} {
		if _, err := ParseTarget(in); !errors.Is(err, ErrNotATarget) {
			t.Errorf("ParseTarget(%q) = %v, want ErrNotATarget", in, err)
		}
	}
}

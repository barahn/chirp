// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"os"
	"strings"
	"testing"
)

// Fyne keeps preferences under the app ID, and macOS knows Chirp by its
// bundle ID: one name, so neither moves without the other.
func TestTheAppIDIsTheBundleID(t *testing.T) {
	src, err := os.ReadFile("../../scripts/macapp/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if want := `bundleID = "` + appID + `"`; !strings.Contains(string(src), want) {
		t.Fatalf("scripts/macapp does not say %s", want)
	}
}

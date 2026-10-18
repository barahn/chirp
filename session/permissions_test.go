// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session

import (
	"slices"
	"testing"
)

// A session asks for everything the free core offers, the clipboard included
// (barahn/chirp#134); the owner turns on what the call needs, and every switch
// but the screen starts off.
func TestASessionAsksForTheWholeCore(t *testing.T) {
	want := []string{"screen_view", "remote_control", "clipboard", "file_transfer"}
	if !slices.Equal(DefaultPermissions, want) {
		t.Fatalf("DefaultPermissions = %v, want %v", DefaultPermissions, want)
	}
}

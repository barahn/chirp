// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/barahn/remotekit/bark"
)

func TestGrantableWithholdsTheClipboard(t *testing.T) {
	got := Grantable([]string{"screen_view", "clipboard", "remote_control", "file_transfer", "webcam"})
	want := []string{"screen_view", "remote_control", "file_transfer", "webcam"}
	if !slices.Equal(got, want) {
		t.Fatalf("Grantable = %v, want %v", got, want)
	}
	if got := Grantable(nil); got == nil || len(got) != 0 {
		t.Fatalf("Grantable(nil) = %#v, want an empty list", got)
	}
}

// A relay writes the request on the trusted path. One that still asks for the
// clipboard -- an older one, or one that means harm -- must not get it shown
// in the prompt, and so not granted.
func TestARelayCannotRequestTheClipboard(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"type": TypeConsentRequest,
		"request": bark.ConsentRequestPayload{
			SessionID:            "s1",
			TechnicianName:       "Ana",
			RequestedPermissions: []string{"screen_view", "remote_control", "clipboard", "file_transfer"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := decode(raw)
	if m.Consent == nil {
		t.Fatal("no consent request decoded")
	}
	if slices.Contains(m.Consent.Permissions, "clipboard") {
		t.Fatalf("the request asks the owner for %v", m.Consent.Permissions)
	}
	if !slices.Contains(m.Consent.Permissions, "screen_view") {
		t.Fatalf("the request lost what it may ask for: %v", m.Consent.Permissions)
	}
}

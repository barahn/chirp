// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/barahn/remotekit/bark"
)

func TestGrantableKeepsWhatIsAsked(t *testing.T) {
	asked := []string{"screen_view", "clipboard", "remote_control", "file_transfer", "webcam"}
	if got := Grantable(asked); !slices.Equal(got, asked) {
		t.Fatalf("Grantable = %v, want %v", got, asked)
	}
	if got := Grantable(nil); got == nil || len(got) != 0 {
		t.Fatalf("Grantable(nil) = %#v, want an empty list", got)
	}
}

// What a relay asks for reaches the prompt whole, the clipboard included: the
// owner decides, switch by switch, and the clipboard switch starts off.
func TestARelaysRequestReachesThePrompt(t *testing.T) {
	asked := []string{"screen_view", "remote_control", "clipboard", "file_transfer"}
	raw, err := json.Marshal(map[string]any{
		"type": TypeConsentRequest,
		"request": bark.ConsentRequestPayload{
			SessionID:            "s1",
			TechnicianName:       "Ana",
			RequestedPermissions: asked,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := decode(raw)
	if m.Consent == nil {
		t.Fatal("no consent request decoded")
	}
	if !slices.Equal(m.Consent.Permissions, asked) {
		t.Fatalf("the request asks the owner for %v, want %v", m.Consent.Permissions, asked)
	}
}

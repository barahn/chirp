// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"reflect"
	"testing"
)

func TestICEConfigIsExplicitEvenWhenEmpty(t *testing.T) {
	for _, tc := range []struct {
		list string
		want []string
	}{
		// Nothing given is no servers, not remotekit's default ones.
		{"", []string{}},
		{" , ", []string{}},
		{"stun:turn.example.org:3478", []string{"stun:turn.example.org:3478"}},
		{" stun:a:3478 ,turn:a:3478,, turns:a:5349 ", []string{"stun:a:3478", "turn:a:3478", "turns:a:5349"}},
	} {
		got := iceConfig(tc.list, "u", "p")
		if got.ICEServers == nil || !reflect.DeepEqual(got.ICEServers, tc.want) {
			t.Errorf("iceConfig(%q).ICEServers = %#v, want %#v", tc.list, got.ICEServers, tc.want)
		}
		if got.TURNUsername != "u" || got.TURNPassword != "p" {
			t.Errorf("iceConfig(%q) lost the TURN credential", tc.list)
		}
	}
}

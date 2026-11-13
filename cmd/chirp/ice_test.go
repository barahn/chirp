// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"slices"
	"testing"

	"github.com/barahn/chirp/client"
	"github.com/barahn/remotekit/webrtc"
)

var sessionTURN = &client.TURNCredential{
	URLs:       []string{"turn:relay.example:3478?transport=udp", "turn:relay.example:3478?transport=tcp"},
	Username:   "1793500000:3f9c1a7e",
	Credential: "c2Vzc2lvbg==",
}

func TestASessionWithoutTURNKeepsTheConfiguredServers(t *testing.T) {
	base := iceConfig("stun:relay.example:3478", "", "")
	got := sessionICE(base, nil)
	if !slices.Equal(got.ICEServers, base.ICEServers) || got.TURNUsername != "" || got.TURNPassword != "" {
		t.Fatalf("sessionICE without TURN = %+v, want %+v", got, base)
	}
}

func TestASessionsTURNCredentialIsAddedToTheConfiguredSTUN(t *testing.T) {
	base := iceConfig("stun:relay.example:3478", "", "")
	got := sessionICE(base, sessionTURN)
	want := append([]string{"stun:relay.example:3478"}, sessionTURN.URLs...)
	if !slices.Equal(got.ICEServers, want) {
		t.Errorf("ICE servers = %v, want %v", got.ICEServers, want)
	}
	if got.TURNUsername != sessionTURN.Username || got.TURNPassword != sessionTURN.Credential {
		t.Errorf("TURN credential = %q/%q, want the session's", got.TURNUsername, got.TURNPassword)
	}
	if len(base.ICEServers) != 1 {
		t.Errorf("the configured servers were changed: %v", base.ICEServers)
	}
}

// PeerConfig carries one TURN credential for every server in it. A TURN
// server configured with its own credential keeps it, rather than be handed
// a credential meant for another.
func TestAConfiguredTURNCredentialIsKept(t *testing.T) {
	base := iceConfig("turn:own.example:3478", "own-user", "own-pass")
	got := sessionICE(base, sessionTURN)
	if !slices.Equal(got.ICEServers, base.ICEServers) || got.TURNUsername != "own-user" || got.TURNPassword != "own-pass" {
		t.Fatalf("sessionICE with a configured TURN = %+v, want %+v", got, base)
	}
}

func TestBothEndsUseTheSessionsTURNCredential(t *testing.T) {
	conn := &client.SessionConn{ID: "s", TURN: sessionTURN}
	base := iceConfig("stun:relay.example:3478", "", "")
	for side, cfg := range map[string]*webrtc.PeerConfig{
		"runner": runnerConfig(conn, base).PeerConfig,
		"viewer": viewerConfig(conn, base).PeerConfig,
	} {
		if cfg == nil || cfg.TURNUsername != sessionTURN.Username || !slices.Contains(cfg.ICEServers, sessionTURN.URLs[0]) {
			t.Errorf("the %s's PeerConfig = %+v, want the session's TURN", side, cfg)
		}
	}
}

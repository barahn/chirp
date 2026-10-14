// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/barahn/chirp/client"
	"github.com/barahn/remotekit/stream"
	"github.com/barahn/remotekit/webrtc"
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

// pipe is an in-memory stream.SignalConn pair.
type pipe struct {
	in, out chan []byte
	done    chan struct{}
	once    *sync.Once
}

func newPipe() (*pipe, *pipe) {
	a, b, done, once := make(chan []byte, 64), make(chan []byte, 64), make(chan struct{}), &sync.Once{}
	return &pipe{in: a, out: b, done: done, once: once}, &pipe{in: b, out: a, done: done, once: once}
}

func (p *pipe) ReadMessage() ([]byte, error) {
	select {
	case m := <-p.in:
		return m, nil
	case <-p.done:
		return nil, errors.New("closed")
	}
}

func (p *pipe) WriteMessage(b []byte) error {
	select {
	case p.out <- append([]byte(nil), b...):
		return nil
	case <-p.done:
		return errors.New("closed")
	}
}

func (p *pipe) Close() error { p.once.Do(func() { close(p.done) }); return nil }

// A viewer that is not served over WebRTC is refused the JPEG fallback rather
// than sent the screen through the relay.
func TestTheRunnerNeverSendsTheScreenThroughTheRelay(t *testing.T) {
	agent, viewer := newPipe()
	r := newRunner(&client.SessionConn{ID: "session", Permissions: []string{stream.PermissionScreenView}}, webrtc.PeerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Serve(ctx, agent); close(done) }()
	defer func() { cancel(); <-done }()

	// session_start with no SDP: no WebRTC, so only the fallback could
	// carry the screen.
	if err := viewer.WriteMessage([]byte(`{"type":"session_start","viewer_id":"v"}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(15 * time.Second)
	for {
		var m []byte
		select {
		case m = <-viewer.in:
		case <-deadline:
			t.Fatal("neither a refusal nor a frame arrived")
		}
		var head struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(m, &head)
		switch head.Type {
		case "relay_fallback_refused":
			return
		case "frame", "relay_mode":
			t.Fatalf("the runner sent %q through the relay", head.Type)
		case "capture_unavailable":
			t.Skip("no screen to capture here")
		}
	}
}

// A session signalled over Nostr refuses input sent beside its signals: it
// must come over the data channel, and never through the relays.
func TestASessionOverNostrTakesInputOnlyOnTheDataChannel(t *testing.T) {
	agent, viewer := newPipe()
	r := newRunner(&client.SessionConn{ID: "session", Permissions: []string{stream.PermissionScreenView, stream.PermissionRemoteControl}, Nostr: true}, webrtc.PeerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Serve(ctx, agent); close(done) }()
	defer func() { cancel(); <-done }()

	if err := viewer.WriteMessage([]byte(`{"type":"input","viewer_id":"v","payload":{"type":"mouse_move","x":0.5,"y":0.5}}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		var m []byte
		select {
		case m = <-viewer.in:
		case <-deadline:
			t.Fatal("the input was not refused")
		}
		var head struct {
			Type    string `json:"type"`
			MsgType string `json:"message_type"`
		}
		_ = json.Unmarshal(m, &head)
		if head.Type == "data_channel_required" && head.MsgType == "input" {
			return
		}
	}
}

func TestNostrIsOnlyWhenRelaysAreGiven(t *testing.T) {
	if c := nostrConfig(" , "); c != nil {
		t.Errorf("nostrConfig of no relays = %+v, want nil", c)
	}
	c := nostrConfig("wss://a.example.org, wss://b.example.org")
	if c == nil || len(c.Relays) != 2 || c.Relays[1] != "wss://b.example.org" {
		t.Errorf("nostrConfig = %+v", c)
	}
}

// With keys the PAKE proved, the runner acts only on offers the controller
// signed: an unsigned one, as the relays could forge, gets no answer.
func TestASessionWithProvedKeysRefusesUnsignedOffers(t *testing.T) {
	_, ownerKey, _ := ed25519.GenerateKey(nil)
	controllerPub, _, _ := ed25519.GenerateKey(nil)
	agent, viewer := newPipe()
	r := newRunner(&client.SessionConn{
		ID: "session", Permissions: []string{stream.PermissionScreenView}, Nostr: true,
		SigningKey: ownerKey, PeerKey: controllerPub,
	}, webrtc.PeerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Serve(ctx, agent); close(done) }()
	defer func() { cancel(); <-done }()

	if err := viewer.WriteMessage([]byte(`{"type":"offer","viewer_id":"v","sdp":"v=0"}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case m := <-viewer.in:
			var head struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(m, &head)
			if head.Type == "answer" {
				t.Fatalf("the runner answered an unsigned offer: %s", m)
			}
		case <-deadline:
			return
		}
	}
}

func TestTheViewerPinsTheProvedKeys(t *testing.T) {
	ownerPub, _, _ := ed25519.GenerateKey(nil)
	_, controllerKey, _ := ed25519.GenerateKey(nil)
	cfg := viewerConfig(&client.SessionConn{ID: "s", ViewerID: "npub-of-controller", SigningKey: controllerKey, PeerKey: ownerPub}, webrtc.PeerConfig{})
	if cfg.ID != "npub-of-controller" || !cfg.AgentKey.Equal(ownerPub) || cfg.SigningKey == nil {
		t.Errorf("viewerConfig = %+v", cfg)
	}
	plain := viewerConfig(&client.SessionConn{ID: "s"}, webrtc.PeerConfig{})
	if plain.ID != "chirp-controller" || plain.AgentKey != nil || plain.SigningKey != nil {
		t.Errorf("viewerConfig without keys = %+v", plain)
	}
}

// Granted, remotekit's runner reads the owner's clipboard on its own and sends
// every change to the controller (decision 4 forbids it): no session conn,
// however it was built, gets the runner to grant it.
func TestTheRunnerIsNeverGrantedTheClipboard(t *testing.T) {
	r := newRunner(&client.SessionConn{ID: "session", Permissions: []string{
		stream.PermissionScreenView, stream.PermissionRemoteControl, stream.PermissionClipboard,
	}}, webrtc.PeerConfig{})
	if r.Granted(stream.PermissionClipboard) {
		t.Fatal("the runner was granted the clipboard")
	}
	if !r.Granted(stream.PermissionRemoteControl) {
		t.Fatal("the runner lost what it may be granted")
	}
}

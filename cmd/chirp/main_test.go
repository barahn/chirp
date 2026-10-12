// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

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
	r := newRunner("session", []string{stream.PermissionScreenView}, webrtc.PeerConfig{})
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

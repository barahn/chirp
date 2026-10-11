// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"image"
	"testing"
	"time"

	"github.com/barahn/remotekit/stream"
	"github.com/barahn/remotekit/webrtc"
)

// help runs Help as the controller, through its own client, until the test
// ends, and returns its events.
func help(t *testing.T, c *Client, shown Peer) <-chan HelperEvent {
	t.Helper()
	events := make(chan HelperEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Help(ctx, shown.Link, "Ana", func(ev HelperEvent) { events <- ev })
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return events
}

func TestAnAllowedHelperViewsOverItsSocket(t *testing.T) {
	r := newRelay(t)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, newClient(t, r), serve)
	shown := await(t, events, "a code", ready).Peer

	cc := newClient(t, r)
	viewed := make(chan *SessionConn, 1)
	cc.View = func(_ context.Context, conn *SessionConn) {
		viewed <- conn
		<-conn.Done()
	}
	help(t, cc, shown)

	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	select {
	case <-viewed:
		t.Fatal("the helper started viewing before the owner allowed it")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	owner := await(t, served, "the owner's session", func(*SessionConn) bool { return true })
	view := await(t, viewed, "the helper's session", func(*SessionConn) bool { return true })
	if view.ID != owner.ID {
		t.Errorf("the helper views %q, the owner serves %q", view.ID, owner.ID)
	}

	if err := view.WriteMessage([]byte(`{"type":"offer","sdp":"v=0"}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, owner); got != `{"type":"offer","sdp":"v=0"}` {
		t.Errorf("the owner read %s", got)
	}
	if err := owner.WriteMessage([]byte(`{"type":"answer","sdp":"v=0"}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, view); got != `{"type":"answer","sdp":"v=0"}` {
		t.Errorf("the helper read %s", got)
	}

	// The owner ending the session ends the helper's view of it.
	o.NewCode()
	ended(t, view, "the owner ended the session")
}

// barahn/chirp#12's "done when", on one machine: the owner's Chirp serves its
// screen with remotekit's runner, the helper's Chirp shows it with remotekit's
// viewer, and everything between them goes through Chirp's relay.
func TestTheHelperSeesTheOwnersScreenThroughTheRelay(t *testing.T) {
	r := newRelay(t)
	o, events, _, _ := runOwnerServing(t, newClient(t, r), func(ctx context.Context, conn *SessionConn) {
		runner := stream.New(stream.Config{ID: conn.ID, PeerConfig: &webrtc.PeerConfig{}})
		runner.RequireScreenViewConsent(true)
		// Screen only: granting remote control here would let the test
		// drive the real pointer.
		runner.Grant(stream.PermissionScreenView)
		_ = runner.Serve(ctx, conn)
	})
	shown := await(t, events, "a code", ready).Peer

	frames := make(chan image.Rectangle, 8)
	notices := make(chan stream.Notice, 8)
	cc := newClient(t, r)
	cc.View = func(ctx context.Context, conn *SessionConn) {
		v := stream.NewViewer(stream.ViewerConfig{ID: "helper", AgentID: conn.ID, PeerConfig: &webrtc.PeerConfig{}})
		v.OnFrame(func(img *image.YCbCr) {
			select {
			case frames <- img.Rect:
			default:
			}
		})
		v.OnNotice(func(n stream.Notice) { notices <- n })
		_ = v.Serve(ctx, conn)
	}
	help(t, cc, shown)

	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	select {
	case rect := <-frames:
		if rect.Dx() < 16 || rect.Dy() < 16 {
			t.Fatalf("the helper decoded a %v picture", rect)
		}
	case n := <-notices:
		if n.Type == "capture_unavailable" {
			t.Skipf("no screen to capture here: %s", n.Detail)
		}
		t.Fatalf("the owner's Chirp sent %+v instead of its screen", n)
	case <-time.After(30 * time.Second):
		t.Fatal("no frame reached the helper within 30s")
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/barahn/remotekit/stream"
	"github.com/barahn/remotekit/webrtc"
)

// serving records each session the Owner hands to Serve, and keeps it open
// until the session ends.
func serving() (func(context.Context, *SessionConn), <-chan *SessionConn) {
	served := make(chan *SessionConn, 4)
	return func(ctx context.Context, conn *SessionConn) {
		served <- conn
		select {
		case <-conn.Done():
		case <-ctx.Done():
		}
	}, served
}

// controller redeems shown's code as Ana and joins the session, without
// following it: the test drives the socket itself.
func controller(t *testing.T, c *Client, shown Peer) (Session, *Socket) {
	t.Helper()
	ctx := context.Background()
	sess, err := c.Redeem(ctx, shown.Formatted, "Ana")
	if err != nil {
		t.Fatal(err)
	}
	sock, err := c.DialController(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sock.Close() })
	return sess, sock
}

// signal returns the next message on s that the relay did not send itself.
func signal(t *testing.T, s *Socket) string {
	t.Helper()
	for {
		if m := next(t, s); !m.FromRelay() {
			return string(m.Raw)
		}
	}
}

func read(t *testing.T, conn *SessionConn) string {
	t.Helper()
	got := make(chan []byte, 1)
	go func() {
		if m, err := conn.ReadMessage(); err == nil {
			got <- m
		}
	}()
	select {
	case m := <-got:
		return string(m)
	case <-time.After(5 * time.Second):
		t.Fatal("no signal reached the session")
		return ""
	}
}

func ended(t *testing.T, conn *SessionConn, why string) {
	t.Helper()
	select {
	case <-conn.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("the session was still served after %s", why)
	}
	if _, err := conn.ReadMessage(); !errors.Is(err, ErrSessionOver) {
		t.Errorf("ReadMessage after %s = %v, want ErrSessionOver", why, err)
	}
	if err := conn.WriteMessage([]byte(`{}`)); !errors.Is(err, ErrSessionOver) {
		t.Errorf("WriteMessage after %s = %v, want ErrSessionOver", why, err)
	}
}

func TestAnAllowedSessionIsServedOverTheOwnerSocket(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, c, serve)
	shown := await(t, events, "a code", ready).Peer

	sess, ctrl := controller(t, c, shown)
	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent

	// Nothing is served before the owner answers, and what the controller
	// sends meanwhile goes nowhere.
	if err := ctrl.Send([]byte(`{"type":"offer","sdp":"too early"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-served:
		t.Fatal("a session was served before the owner allowed it")
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	conn := await(t, served, "the allowed session", func(*SessionConn) bool { return true })
	if conn.ID != sess.ID {
		t.Errorf("served session %q, want %q", conn.ID, sess.ID)
	}
	if len(conn.Permissions) == 0 {
		t.Error("the session was served with no permissions")
	}

	if err := ctrl.Send([]byte(`{"type":"offer","sdp":"v=0"}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, conn); got != `{"type":"offer","sdp":"v=0"}` {
		t.Errorf("the session read %s, want the offer sent after consent", got)
	}
	if err := conn.WriteMessage([]byte(`{"type":"answer","sdp":"v=0"}`)); err != nil {
		t.Fatal(err)
	}
	if got := signal(t, ctrl); got != `{"type":"answer","sdp":"v=0"}` {
		t.Errorf("the controller got %s, want the answer", got)
	}

	// The controller leaving ends the session.
	_ = ctrl.Close()
	ended(t, conn, "the controller left")
}

func TestADeclinedSessionIsNotServed(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, c, serve)
	shown := await(t, events, "a code", ready).Peer

	controller(t, c, shown)
	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-served:
		t.Fatal("a declined session was served")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestEndingTheSessionStopsServingIt(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	serve, served := serving()
	o, events, stop, done := runOwnerServing(t, c, serve)
	shown := await(t, events, "a code", ready).Peer

	controller(t, c, shown)
	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	conn := await(t, served, "the allowed session", func(*SessionConn) bool { return true })

	// The window's End session asks for a new code.
	o.NewCode()
	ended(t, conn, "the owner ended it")

	// A new code is a new registration, so wait for the new peer, not for
	// the code that replaced the one the first session spent.
	shown = await(t, events, "the new registration", func(ev OwnerEvent) bool { return ready(ev) && ev.Peer.ID != shown.ID }).Peer
	controller(t, c, shown)
	asked = await(t, events, "the second request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	conn = await(t, served, "the second session", func(*SessionConn) bool { return true })

	// Quitting ends it too, and Run waits for Serve to return.
	stop()
	<-done
	ended(t, conn, "the owner quit")
}

// remotekit's runner, served over a SessionConn, talks to the controller
// through the relay: what it writes arrives, and what the controller sends is
// read and gated by consent.
func TestRemotekitsRunnerServesOverTheOwnerSocket(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	o, events, _, _ := runOwnerServing(t, c, func(ctx context.Context, conn *SessionConn) {
		// Granted nothing, so the screen stays withheld whatever is asked.
		runner := stream.New(stream.Config{ID: conn.ID, PeerConfig: &webrtc.PeerConfig{}})
		runner.RequireScreenViewConsent(true)
		_ = runner.Serve(ctx, conn)
	})
	shown := await(t, events, "a code", ready).Peer

	sess, ctrl := controller(t, c, shown)
	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}

	var hello struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(signal(t, ctrl)), &hello); err != nil || hello.Type != "agent_ready" || hello.SessionID != sess.ID {
		t.Fatalf("the controller got %+v (%v), want the runner's agent_ready for %s", hello, err, sess.ID)
	}

	if err := ctrl.Send([]byte(`{"type":"session_start","viewer_id":"ana"}`)); err != nil {
		t.Fatal(err)
	}
	var refused struct {
		Type       string `json:"type"`
		Permission string `json:"permission"`
	}
	if err := json.Unmarshal([]byte(signal(t, ctrl)), &refused); err != nil || refused.Type != "consent_required" || refused.Permission != stream.PermissionScreenView {
		t.Fatalf("the controller got %+v (%v), want the screen refused for want of consent", refused, err)
	}
}

// A controller cannot end the owner's session by forging the relay's leave
// notice: the relay refuses to forward it, tells the controller so, and the
// session goes on.
func TestAForgedLeaveNoticeDoesNotEndTheSession(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, c, serve)
	shown := await(t, events, "a code", ready).Peer

	_, ctrl := controller(t, c, shown)
	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	conn := await(t, served, "the allowed session", func(*SessionConn) bool { return true })

	if err := ctrl.Send([]byte(`{"type":"chirp.peer_left","role":"controller"}`)); err != nil {
		t.Fatal(err)
	}
	for {
		m := next(t, ctrl)
		if m.Type == TypeRelayError {
			break
		}
	}
	select {
	case <-conn.Done():
		t.Fatal("a forged leave notice ended the owner's session")
	case <-time.After(200 * time.Millisecond):
	}
	if err := ctrl.Send([]byte(`{"type":"offer","sdp":"v=0"}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, conn); got != `{"type":"offer","sdp":"v=0"}` {
		t.Errorf("after the forgery the owner read %s", got)
	}
}

// Signals the runner does not read are bounded in bytes as well as in number:
// a controller sending large ones cannot make this machine hold more than
// signalBudget of them.
func TestQueuedSignalsAreBoundedInBytes(t *testing.T) {
	conn := newSessionConn(nil, &ConsentRequest{SessionID: "s"})
	big := make([]byte, signalBudget/4)
	for i := 0; i < 8; i++ {
		conn.deliver(big)
	}
	if n := len(conn.in); n != 4 {
		t.Fatalf("%d signals of %d bytes queued, want the 4 that fit in %d", n, len(big), signalBudget)
	}
	conn.deliver([]byte(`{"type":"candidate"}`))
	if n := len(conn.in); n != 4 {
		t.Fatalf("a full budget took another signal: %d queued", n)
	}
	if _, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	conn.deliver(big)
	if n := len(conn.in); n != 4 {
		t.Fatalf("after one read, %d signals queued, want its room taken again (4)", n)
	}
}

// A controller flooding the owner with signals its runner does not read
// cannot keep the owner from ending the session.
func TestAFloodOfSignalsDoesNotHoldTheSessionOpen(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	// A Serve that never reads, as a runner stuck on a slow negotiation.
	served := make(chan *SessionConn, 1)
	o, events, _, _ := runOwnerServing(t, c, func(ctx context.Context, conn *SessionConn) {
		served <- conn
		<-conn.Done()
	})
	shown := await(t, events, "a code", ready).Peer

	_, ctrl := controller(t, c, shown)
	asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
	if _, err := o.Answer(context.Background(), asked.SessionID, true); err != nil {
		t.Fatal(err)
	}
	conn := await(t, served, "the allowed session", func(*SessionConn) bool { return true })
	for i := 0; i < 4*signalQueue; i++ {
		if err := ctrl.Send([]byte(`{"type":"candidate","candidate":"{}"}`)); err != nil {
			t.Fatal(err)
		}
	}
	o.NewCode()
	ended(t, conn, "the owner ended it under a flood")
}

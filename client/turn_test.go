// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/barahn/chirp/signalling"
)

var testTURNURLs = []string{"turn:relay.example:3478?transport=udp"}

func withTURN(srv *signalling.Server) {
	srv.TURN = &signalling.TURN{Secret: "shared with coturn", URLs: testTURNURLs}
}

func viewing(c *Client) <-chan *SessionConn {
	viewed := make(chan *SessionConn, 1)
	c.View = func(_ context.Context, conn *SessionConn) {
		viewed <- conn
		<-conn.Done()
	}
	return viewed
}

func checkTURN(t *testing.T, side string, got *TURNCredential) {
	t.Helper()
	if got == nil {
		t.Fatalf("the %s's session has no TURN credential", side)
	}
	if !slices.Equal(got.URLs, testTURNURLs) {
		t.Errorf("the %s's TURN credential is for %v, want %v", side, got.URLs, testTURNURLs)
	}
	if got.Username == "" || got.Credential == "" {
		t.Errorf("the %s's TURN credential has no username or password: %+v", side, got)
	}
}

// Over the relay, the owner gets both credentials when it allows the session
// and hands the controller its own over the session; that message is
// Chirp's, not remotekit's, so the viewer never reads it.
func TestBothEndsOfASessionThroughTheRelayGetATURNCredential(t *testing.T) {
	r := newRelay(t, withTURN)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, newClient(t, r), serve)
	shown := await(t, events, "a code", ready).Peer

	hc := newClient(t, r)
	viewed := viewing(hc)
	helped := helpWith(t, hc, shown.Formatted, "Ana")
	req := await(t, events, "the request", asked).Consent
	if _, err := o.Answer(context.Background(), req.SessionID, req.Permissions); err != nil {
		t.Fatal(err)
	}
	await(t, helped, "the helper to be let in", func(ev HelperEvent) bool { return ev.Phase == HelperAllowed })
	owner := await(t, served, "the owner's session", func(*SessionConn) bool { return true })
	view := await(t, viewed, "the helper's session", func(*SessionConn) bool { return true })

	checkTURN(t, "owner", owner.TURN)
	checkTURN(t, "helper", view.TURN)
	if owner.TURN.Username == view.TURN.Username {
		t.Error("both ends were given the same TURN credential")
	}

	if err := owner.WriteMessage([]byte(`{"type":"agent_ready"}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, view); got != `{"type":"agent_ready"}` {
		t.Errorf("the viewer read %s first, want the runner's first signal", got)
	}
}

func TestBothEndsOfASessionOverNostrGetATURNCredential(t *testing.T) {
	r, nr := newRelay(t, withTURN), newNostrRelay(t)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, newNostrClient(t, r, nr), serve)
	shown := await(t, events, "a code", ready).Peer

	hc := newNostrClient(t, r, nr)
	viewed := viewing(hc)
	helped := helpWith(t, hc, shown.Spoken(), "Ana")
	req, _ := askedAndReplaced(t, events, shown.Code)
	await(t, helped, "the helper to be asking", func(ev HelperEvent) bool { return ev.Phase == HelperAsking })
	if _, err := o.Answer(context.Background(), req.SessionID, req.Permissions); err != nil {
		t.Fatal(err)
	}
	owner := await(t, served, "the owner's session", func(*SessionConn) bool { return true })
	view := await(t, viewed, "the helper's session", func(*SessionConn) bool { return true })

	checkTURN(t, "owner", owner.TURN)
	checkTURN(t, "helper", view.TURN)
	if owner.TURN.Username == view.TURN.Username {
		t.Error("both ends were given the same TURN credential")
	}
}

// A relay without TURN answers 404, and the session goes on with STUN: the
// owner still says so, so the helper is let in without waiting.
func TestARelayWithoutTURNLeavesTheSessionOnSTUN(t *testing.T) {
	r := newRelay(t)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, newClient(t, r), serve)
	shown := await(t, events, "a code", ready).Peer

	hc := newClient(t, r)
	hc.turnWait = time.Minute
	viewed := viewing(hc)
	helpWith(t, hc, shown.Formatted, "Ana")
	req := await(t, events, "the request", asked).Consent
	if _, err := o.Answer(context.Background(), req.SessionID, req.Permissions); err != nil {
		t.Fatal(err)
	}
	owner := await(t, served, "the owner's session", func(*SessionConn) bool { return true })
	view := await(t, viewed, "the helper's session, well before turnWait", func(*SessionConn) bool { return true })
	if owner.TURN != nil || view.TURN != nil {
		t.Errorf("TURN credentials from a relay without TURN: owner %+v, helper %+v", owner.TURN, view.TURN)
	}
}

// An owner from before TURN never says whether there is a credential. Its
// helper waits turnWait for word, then is let in on STUN.
func TestAHelperOfAnOwnerWithoutTURNSupportIsLetInAfterAWait(t *testing.T) {
	r := newRelay(t, withTURN)
	oc := newClient(t, r)
	ctx := context.Background()
	p, err := oc.Register(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	sock, err := oc.DialOwner(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sock.Close() })

	hc := newClient(t, r)
	hc.turnWait = 300 * time.Millisecond
	viewed := viewing(hc)
	helped := helpWith(t, hc, p.Formatted, "Ana")
	sess := await(t, helped, "the helper to be asking", func(ev HelperEvent) bool { return ev.Phase == HelperAsking }).Session
	allowedAt := time.Now()
	if _, err := oc.Consent(ctx, p.ID, sess.ID, true, "allowed by the owner"); err != nil {
		t.Fatal(err)
	}
	view := await(t, viewed, "the helper's session", func(*SessionConn) bool { return true })
	if waited := time.Since(allowedAt); waited < hc.turnWait {
		t.Errorf("the helper was let in after %v, before turnWait (%v) gave the owner a chance to send its credential", waited, hc.turnWait)
	}
	if view.TURN != nil {
		t.Errorf("the helper has a TURN credential %+v nobody sent", view.TURN)
	}
}

func TestAnAllowedSessionWaitsForTheOwnersTURNWord(t *testing.T) {
	start := time.Now()
	g := turnGate{wait: time.Second}
	if g.ready(Session{ID: "s"}, start) {
		t.Fatal("shown before the owner said anything about TURN")
	}
	cred := &TURNCredential{Username: "u"}
	if waiting := g.hear(cred); waiting == nil || waiting.ID != "s" {
		t.Fatalf("hearing the owner let in %v, want the session waiting", waiting)
	}
	if !g.ready(Session{ID: "s"}, start) || g.cred != cred {
		t.Error("not shown, with the owner's credential, once the owner spoke")
	}
}

func TestAnOwnerFromBeforeTURNIsWaitedForOnlySoLong(t *testing.T) {
	start := time.Now()
	g := turnGate{wait: time.Second}
	if g.hear(nil) != nil {
		t.Error("a word before any allowed session let something in")
	}
	g = turnGate{wait: time.Second}
	g.ready(Session{ID: "s"}, start)
	if g.ready(Session{ID: "s"}, start.Add(999*time.Millisecond)) {
		t.Error("shown before the wait was over")
	}
	if !g.ready(Session{ID: "s"}, start.Add(time.Second)) {
		t.Error("still held back once the wait was over")
	}
}

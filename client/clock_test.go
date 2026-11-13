// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"
)

// relayAhead stands in front of r and says, in every answer's Date header,
// that the time is off later than it is: a relay whose clock is the one
// both ends trust, on machines whose own clocks say otherwise.
func relayAhead(t *testing.T, r *relay, off time.Duration) string {
	t.Helper()
	target, _ := url.Parse(r.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Date", time.Now().Add(off).UTC().Format(http.TimeFormat))
		return nil
	}
	ts := httptest.NewServer(proxy)
	t.Cleanup(ts.Close)
	return ts.URL
}

func near(got, want time.Time) bool {
	d := got.Sub(want)
	return d > -3*time.Second && d < 3*time.Second
}

// The client keeps the relay's time, from the Date of its answers, and
// until it has one, the machine's own (barahn/remotekit#179).
func TestTheClientKeepsTheRelaysTime(t *testing.T) {
	r := newRelay(t)
	c, err := New(relayAhead(t, r, 4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !near(c.Now(), time.Now()) {
		t.Fatalf("before asking the relay anything, Now is %v off the machine's clock", time.Until(c.Now()))
	}
	if _, err := c.Register(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if !near(c.Now(), time.Now().Add(4*time.Hour)) {
		t.Fatalf("after the relay answered, Now is %v off the machine's clock, want the relay's 4h ahead", time.Until(c.Now()))
	}
}

// Over Nostr, two Chirps write and check their messages on their relay's
// time, so they talk whatever their machines' clocks say; their sessions
// carry that clock too, for the signed signals. Here both machines are 4h
// behind their relay. A helper that did not take the relay's time would
// have its knocks dropped as stale.
func TestChirpsOnTheRelaysTimeTalkOverNostr(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	ahead := relayAhead(t, r, 4*time.Hour)
	chirp := func(addr string) *Client {
		c, err := New(addr)
		if err != nil {
			t.Fatal(err)
		}
		c.pollEvery = 20 * time.Millisecond
		c.Nostr = &NostrConfig{Relays: []string{nr.url()}}
		return c
	}

	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, chirp(ahead), serve)
	shown := await(t, events, "a code", ready).Peer

	// The link, not the code: each use mints a code of its own, so the
	// stray helper's lookup leaves something for the next one.
	link := shown.LinkFragment()
	helpWith(t, chirp(r.URL), link, "Ana")
	select {
	case ev := <-events:
		if ev.Consent != nil {
			t.Fatal("a helper 4h off its owner's clock was asked about")
		}
	case <-time.After(3 * time.Second):
	}

	helper := chirp(ahead)
	viewed := make(chan *SessionConn, 1)
	helper.View = func(_ context.Context, conn *SessionConn) {
		viewed <- conn
		<-conn.Done()
	}
	helpWith(t, helper, link, "Ana")
	req := await(t, events, "the request", asked).Consent
	if _, err := o.Answer(context.Background(), req.SessionID, req.Permissions); err != nil {
		t.Fatal(err)
	}
	owner := await(t, served, "the owner's session", func(*SessionConn) bool { return true })
	view := await(t, viewed, "the helper's session", func(*SessionConn) bool { return true })
	for side, conn := range map[string]*SessionConn{"owner": owner, "helper": view} {
		if conn.Clock == nil || !near(conn.Clock(), time.Now().Add(4*time.Hour)) {
			t.Fatalf("the %s's session is not on the relay's time", side)
		}
	}
}

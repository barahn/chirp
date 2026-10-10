// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"errors"
	"testing"
	"time"
)

// await returns the first value from ch that ok accepts, failing the test if
// none arrives in time.
func await[T any](t *testing.T, ch <-chan T, what string, ok func(T) bool) T {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case v := <-ch:
			if ok(v) {
				return v
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// runOwner starts an Owner with test-sized timings and returns its events,
// a cancel that stops it, and a channel closed when Run returns.
func runOwner(t *testing.T, c *Client) (*Owner, <-chan OwnerEvent, context.CancelFunc, <-chan struct{}) {
	t.Helper()
	return runOwnerServing(t, c, nil)
}

// runOwnerServing is runOwner with serve as the Owner's Serve.
func runOwnerServing(t *testing.T, c *Client, serve func(context.Context, *SessionConn)) (*Owner, <-chan OwnerEvent, context.CancelFunc, <-chan struct{}) {
	t.Helper()
	events := make(chan OwnerEvent, 64)
	o := NewOwner(c, "", func(ev OwnerEvent) { events <- ev })
	o.Poll, o.Retry, o.Grace = 50*time.Millisecond, 20*time.Millisecond, 5*time.Second
	o.Serve = serve
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = o.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return o, events, cancel, done
}

func ready(ev OwnerEvent) bool { return ev.Phase == OwnerReady && ev.Peer.Code != "" }

func TestOwnerShowsACodeReportsTheRequestAndReplacesTheSpentCode(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	_, events, _, _ := runOwner(t, c)

	first := await(t, events, "the first code", ready)
	if first.Replaced {
		t.Error("the first code is reported as a replacement")
	}

	helped := make(chan HelperEvent, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Help(ctx, first.Peer.Formatted, "Ana", func(ev HelperEvent) { helped <- ev })

	asked := await(t, events, "the consent request", func(ev OwnerEvent) bool { return ev.Consent != nil })
	if asked.Consent.Name != "Ana" || asked.Consent.NameVerified {
		t.Errorf("consent request = %+v", asked.Consent)
	}
	next := await(t, events, "a new code", func(ev OwnerEvent) bool { return ev.Replaced })
	if next.Peer.Code == first.Peer.Code {
		t.Error("the spent code is still the one shown")
	}
	await(t, helped, "the helper asking", func(ev HelperEvent) bool { return ev.Phase == HelperAsking })
}

func TestStoppingTheOwnerReleasesItsCodeAndLink(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	_, events, cancel, done := runOwner(t, c)
	shown := await(t, events, "a code", ready).Peer

	cancel()
	<-done
	if _, err := c.Peer(context.Background(), shown.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after stopping, the peer is %v; want it released", err)
	}
	if _, err := c.RedeemLink(context.Background(), shown.Link, "Ana"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after stopping, the link gives %v; want ErrNotFound", err)
	}
}

func TestNewCodeRevokesTheOldCodeAndLink(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	o, events, _, _ := runOwner(t, c)
	old := await(t, events, "a code", ready).Peer

	o.NewCode()
	fresh := await(t, events, "the new code", func(ev OwnerEvent) bool { return ev.Replaced }).Peer
	if fresh.Code == old.Code || fresh.Link == old.Link {
		t.Fatalf("new code %+v repeats the old %+v", fresh, old)
	}
	if _, err := c.RedeemLink(context.Background(), old.Link, "Ana"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the old link gives %v; want ErrNotFound", err)
	}
}

func TestADroppedSocketKeepsTheCodeAndComesBack(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	_, events, _, _ := runOwner(t, c)
	shown := await(t, events, "a code", ready).Peer

	// The code is shown as soon as it is registered, before the socket is
	// up, so cut until the socket is among what was cut.
	var away OwnerEvent
	for away.Phase != OwnerReconnecting {
		r.ln.cut()
		select {
		case ev := <-events:
			away = ev
		case <-time.After(100 * time.Millisecond):
		}
	}
	if away.Peer.Code != shown.Code {
		t.Errorf("while reconnecting the code shown is %q, want %q", away.Peer.Code, shown.Code)
	}
	back := await(t, events, "ready again", func(ev OwnerEvent) bool { return ev.Phase == OwnerReady })
	if back.Peer.ID != shown.ID {
		t.Errorf("came back as another peer: %+v", back.Peer)
	}
}

func TestAnUnreachableRelayShowsOffline(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	r.Close()
	_, events, _, _ := runOwner(t, c)
	ev := await(t, events, "offline", func(ev OwnerEvent) bool { return ev.Phase == OwnerOffline })
	if ev.Lost || !errors.Is(ev.Err, ErrUnavailable) {
		t.Errorf("offline event = %+v", ev)
	}
}

func TestHelpFollowsTheOwnersDecision(t *testing.T) {
	for _, tc := range []struct {
		granted bool
		want    HelperPhase
	}{
		{false, HelperDeclined},
		{true, HelperAllowed},
	} {
		r := newRelay(t)
		c := newClient(t, r)
		o, events, _, _ := runOwner(t, c)
		shown := await(t, events, "a code", ready).Peer

		helped := make(chan HelperEvent, 64)
		ctx, cancel := context.WithCancel(context.Background())
		go c.Help(ctx, shown.Link, "Ana", func(ev HelperEvent) { helped <- ev })

		asked := await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil }).Consent
		if asked.ExpiresIn <= 0 {
			t.Errorf("the request arrived without its deadline: %+v", asked)
		}
		await(t, helped, "asking", func(ev HelperEvent) bool { return ev.Phase == HelperAsking })
		s, err := o.Answer(context.Background(), asked.SessionID, tc.granted)
		if err != nil {
			t.Fatal(err)
		}
		if want := map[bool]string{true: StateConsentGranted, false: StateConsentDenied}[tc.granted]; s.State != want {
			t.Errorf("after answering %v the session is %q, want %q", tc.granted, s.State, want)
		}
		await(t, helped, "the decision", func(ev HelperEvent) bool { return ev.Phase == tc.want })
		// A decision is final: the relay refuses a second one.
		if _, err := o.Answer(context.Background(), asked.SessionID, !tc.granted); err == nil {
			t.Error("a second answer was accepted")
		}
		cancel()
	}
}

func TestAnswerNeedsARegistration(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	o := NewOwner(c, "", func(OwnerEvent) {})
	if _, err := o.Answer(context.Background(), "s", true); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("Answer before Run = %v, want ErrNotRegistered", err)
	}
}

func TestAnAnswerToAnotherSessionIsRefused(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	o, events, _, _ := runOwner(t, c)
	shown := await(t, events, "a code", ready).Peer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Help(ctx, shown.Code, "Ana", func(HelperEvent) {})
	await(t, events, "the request", func(ev OwnerEvent) bool { return ev.Consent != nil })
	if _, err := o.Answer(context.Background(), "not-this-session", true); !errors.Is(err, ErrBusy) {
		t.Errorf("answering another session = %v, want ErrBusy (409)", err)
	}
}

func TestHelpRefusesBadInputWithoutAskingTheRelay(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	r.Close() // anything that reached the relay would fail as unavailable

	for _, tc := range []struct {
		input, name string
		want        error
	}{
		{"482 913", "", ErrNoName},
		{"hello", "Ana", ErrNotATarget},
	} {
		var got HelperEvent
		c.Help(context.Background(), tc.input, tc.name, func(ev HelperEvent) { got = ev })
		if got.Phase != HelperFailed || !errors.Is(got.Err, tc.want) {
			t.Errorf("Help(%q, %q) = %+v, want failed with %v", tc.input, tc.name, got, tc.want)
		}
	}
}

func TestCancellingHelpReportsNothing(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	_, events, _, _ := runOwner(t, c)
	shown := await(t, events, "a code", ready).Peer

	helped := make(chan HelperEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		c.Help(ctx, shown.Code, "Ana", func(ev HelperEvent) { helped <- ev })
	}()
	await(t, helped, "asking", func(ev HelperEvent) bool { return ev.Phase == HelperAsking })
	cancel()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Help did not return after cancel")
	}
	for {
		select {
		case ev := <-helped:
			if ev.Phase != HelperAsking {
				t.Errorf("after cancel, Help reported %+v", ev)
			}
		default:
			return
		}
	}
}

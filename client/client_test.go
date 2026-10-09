// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/chirp/signalling"
	"github.com/barahn/remotekit/bark"
)

// relay is a real signalling server on a listener whose connections the test
// can cut, so a dropped socket can be staged.
type relay struct {
	*httptest.Server
	ln *cuttable
}

func newRelay(t *testing.T) *relay {
	t.Helper()
	sm := session.NewSessionManager(10 * time.Minute)
	t.Cleanup(sm.Close)
	ts := httptest.NewUnstartedServer(signalling.NewServer(sm).Handler())
	ln := &cuttable{Listener: ts.Listener}
	ts.Listener = ln
	ts.Start()
	t.Cleanup(ts.Close)
	return &relay{Server: ts, ln: ln}
}

func newClient(t *testing.T, r *relay) *Client {
	t.Helper()
	c, err := New(r.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.pollEvery = 20 * time.Millisecond
	return c
}

// cuttable is a listener that remembers what it accepted, including the
// connections the server hijacks for WebSockets, so cut can drop them all.
type cuttable struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (l *cuttable) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.mu.Lock()
		l.conns = append(l.conns, c)
		l.mu.Unlock()
	}
	return c, err
}

func (l *cuttable) cut() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.conns {
		_ = c.Close()
	}
	l.conns = nil
}

// consent answers the prompt as the owner's machine would. Drawing the prompt
// and answering it is barahn/chirp#11; the test plays that part directly.
func consent(t *testing.T, r *relay, peerID, sessionID string, granted bool) {
	t.Helper()
	body, _ := json.Marshal(bark.ConsentResponsePayload{SessionID: sessionID, Granted: granted, Timestamp: time.Now().UTC()})
	resp, err := http.Post(r.URL+"/api/peers/"+peerID+"/consent", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("consent: status %d", resp.StatusCode)
	}
}

// next reads one message, failing the test if none arrives in time.
func next(t *testing.T, s *Socket) Message {
	t.Helper()
	got := make(chan Message, 1)
	go func() {
		m, err := s.Next()
		if err == nil {
			got <- m
		}
	}()
	select {
	case m := <-got:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no message on the socket")
		return Message{}
	}
}

func TestRelayMessageTypesMatchTheRelay(t *testing.T) {
	for got, want := range map[string]string{
		TypeRelayError:     signalling.RelayErrorType,
		TypePeerJoined:     signalling.PeerJoinedType,
		TypePeerLeft:       signalling.PeerLeftType,
		TypeConsentRequest: signalling.ConsentRequestType,
	} {
		if got != want {
			t.Errorf("client says %q, the relay says %q", got, want)
		}
	}
}

func TestNewRefusesWhatIsNotAnHTTPAddress(t *testing.T) {
	for _, addr := range []string{"", "localhost:8080", "ftp://relay.example", "https://", "wss://relay.example"} {
		if _, err := New(addr); err == nil {
			t.Errorf("New(%q) accepted it", addr)
		}
	}
	if _, err := New(" https://relay.example/base/ "); err != nil {
		t.Errorf("New refused a good address: %v", err)
	}
}

func TestACodeReachesTheOwnerAsAConsentRequest(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	ctx := context.Background()

	owner, err := c.Register(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if owner.ID == "" || len(owner.Code) != 6 || owner.Link == "" {
		t.Fatalf("registration is missing something: %+v", owner)
	}
	ownerSock, err := c.DialOwner(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ownerSock.Close() }()

	sess, err := c.Redeem(ctx, owner.Formatted, "Ana")
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := c.DialController(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ctrl.Close() }()

	if m := next(t, ctrl); m.Type != TypePeerJoined || m.Role != "owner" || !m.FromRelay() {
		t.Fatalf("controller got %+v, want the owner's arrival", m)
	}
	var req *ConsentRequest
	for req == nil {
		m := next(t, ownerSock)
		req = m.Consent
	}
	if req.SessionID != sess.ID || req.Name != "Ana" || req.NameVerified || len(req.Permissions) == 0 {
		t.Fatalf("consent request = %+v", req)
	}
}

func TestRelayRefusalsMapToErrorsAWindowCanName(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	ctx := context.Background()

	owner, err := c.Register(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Redeem(ctx, owner.Code, "Ana"); err != nil {
		t.Fatal(err)
	}
	// The start code was spent by that session.
	if _, err := c.Redeem(ctx, owner.Code, "Bo"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a spent code: %v, want ErrNotFound", err)
	}
	// The owner is in a session; its link mints a code that finds it busy.
	if _, err := c.RedeemLink(ctx, owner.Link, "Bo"); !errors.Is(err, ErrBusy) {
		t.Errorf("a busy owner: %v, want ErrBusy", err)
	}
	var re *RelayError
	if _, err := c.Session(ctx, "no-such-session"); !errors.As(err, &re) || re.Status != http.StatusNotFound {
		t.Errorf("an unknown session: %v, want a 404 RelayError", err)
	}
	// Guessing is metered per origin; past the limit even a guess is refused.
	for i := 0; i < session.DefaultMaxRedemptionFailures+1; i++ {
		_, err = c.Redeem(ctx, "000000", "Guess")
	}
	if !errors.Is(err, ErrTooMany) {
		t.Errorf("past the guessing limit: %v, want ErrTooMany", err)
	}
}

func TestAnUnreachableRelayIsUnavailable(t *testing.T) {
	r := newRelay(t)
	c := newClient(t, r)
	r.Close()
	if _, err := c.Register(context.Background(), ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Register on a closed relay: %v, want ErrUnavailable", err)
	}
	if _, err := c.DialOwner(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("DialOwner on a closed relay: %v, want ErrUnavailable", err)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/barahn/remotekit/stream"
	"github.com/barahn/remotekit/webrtc"
	"github.com/gorilla/websocket"
)

// nostrRelay is just enough of a NIP-01 relay for these tests: it stores each
// event, answers a subscription with what it has and then streams new
// matches, and remembers the kind of every event published to it, which is
// all a relay ever learns about a message.
type nostrRelay struct {
	srv *httptest.Server

	mu     sync.Mutex
	events []nostrEvent
	kinds  []int
	subs   map[*nostrPeer]map[string]nostrFilter
}

type nostrEvent struct {
	raw       json.RawMessage
	kind      int
	p         string
	createdAt int64
}

type nostrFilter struct {
	Kinds []int    `json:"kinds"`
	P     []string `json:"#p"`
	Since int64    `json:"since"`
}

func (f nostrFilter) matches(ev nostrEvent) bool {
	return slices.Contains(f.Kinds, ev.kind) && slices.Contains(f.P, ev.p) && ev.createdAt >= f.Since
}

type nostrPeer struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (p *nostrPeer) send(msg ...any) {
	data, _ := json.Marshal(msg)
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.ws.WriteMessage(websocket.TextMessage, data)
}

func newNostrRelay(t *testing.T) *nostrRelay {
	t.Helper()
	nr := &nostrRelay{subs: map[*nostrPeer]map[string]nostrFilter{}}
	var upgrader websocket.Upgrader
	nr.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		p := &nostrPeer{ws: ws}
		nr.mu.Lock()
		nr.subs[p] = map[string]nostrFilter{}
		nr.mu.Unlock()
		defer func() {
			nr.mu.Lock()
			delete(nr.subs, p)
			nr.mu.Unlock()
			_ = ws.Close()
		}()
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			nr.handle(p, data)
		}
	}))
	t.Cleanup(func() {
		nr.mu.Lock()
		for p := range nr.subs {
			_ = p.ws.Close()
		}
		nr.mu.Unlock()
		nr.srv.Close()
	})
	return nr
}

func (nr *nostrRelay) url() string { return "ws" + strings.TrimPrefix(nr.srv.URL, "http") }

func (nr *nostrRelay) handle(p *nostrPeer, data []byte) {
	var msg []json.RawMessage
	if json.Unmarshal(data, &msg) != nil || len(msg) < 2 {
		return
	}
	var label string
	_ = json.Unmarshal(msg[0], &label)
	switch label {
	case "EVENT":
		var head struct {
			ID        string     `json:"id"`
			Kind      int        `json:"kind"`
			CreatedAt int64      `json:"created_at"`
			Tags      [][]string `json:"tags"`
		}
		if json.Unmarshal(msg[1], &head) != nil {
			return
		}
		ev := nostrEvent{raw: msg[1], kind: head.Kind, createdAt: head.CreatedAt}
		for _, tag := range head.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				ev.p = tag[1]
			}
		}
		p.send("OK", head.ID, true, "")
		nr.mu.Lock()
		nr.events = append(nr.events, ev)
		nr.kinds = append(nr.kinds, ev.kind)
		type delivery struct {
			to  *nostrPeer
			sub string
		}
		var out []delivery
		for peer, subs := range nr.subs {
			for sub, f := range subs {
				if f.matches(ev) {
					out = append(out, delivery{peer, sub})
				}
			}
		}
		nr.mu.Unlock()
		for _, d := range out {
			d.to.send("EVENT", d.sub, ev.raw)
		}
	case "REQ":
		var sub string
		var f nostrFilter
		if len(msg) < 3 || json.Unmarshal(msg[1], &sub) != nil || json.Unmarshal(msg[2], &f) != nil {
			return
		}
		nr.mu.Lock()
		nr.subs[p][sub] = f
		var stored []json.RawMessage
		for _, ev := range nr.events {
			if f.matches(ev) {
				stored = append(stored, ev.raw)
			}
		}
		nr.mu.Unlock()
		for _, ev := range stored {
			p.send("EVENT", sub, ev)
		}
		p.send("EOSE", sub)
	case "CLOSE":
		var sub string
		_ = json.Unmarshal(msg[1], &sub)
		nr.mu.Lock()
		delete(nr.subs[p], sub)
		nr.mu.Unlock()
	}
}

// published returns the kind of every event published so far.
func (nr *nostrRelay) published() []int {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	return append([]int(nil), nr.kinds...)
}

// newNostrClient is newClient with signalling over nr.
func newNostrClient(t *testing.T, r *relay, nr *nostrRelay) *Client {
	t.Helper()
	c := newClient(t, r)
	c.Nostr = &NostrConfig{Relays: []string{nr.url()}}
	return c
}

// helpWith runs Help with input as the controller until the test ends.
func helpWith(t *testing.T, c *Client, input, name string) <-chan HelperEvent {
	t.Helper()
	events := make(chan HelperEvent, 256)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Help(ctx, input, name, func(ev HelperEvent) { events <- ev })
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return events
}

func asked(ev OwnerEvent) bool { return ev.Consent != nil }

func fieldsOf(t *testing.T, msg string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		t.Fatalf("%s is not a JSON object: %v", msg, err)
	}
	return m
}

// barahn/chirp#43: the server only looks the code up. Asking, the answer and
// the session's signals all go between the two Chirps over a Nostr relay,
// which sees nothing but gift wraps.
func TestASessionIsSignalledOverNostr(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, newNostrClient(t, r, nr), serve)
	shown := await(t, events, "a code", ready).Peer

	cc := newNostrClient(t, r, nr)
	viewed := make(chan *SessionConn, 1)
	cc.View = func(_ context.Context, conn *SessionConn) {
		viewed <- conn
		<-conn.Done()
	}
	helped := helpWith(t, cc, shown.Formatted, "Ana")

	req := await(t, events, "the request", asked).Consent
	if req.Name != "Ana" || req.NameVerified {
		t.Errorf("the owner was asked by %q (verified %v), want an unverified Ana", req.Name, req.NameVerified)
	}
	if len(req.Permissions) == 0 || req.ExpiresIn <= 0 {
		t.Errorf("the request asks for %v with %ds left", req.Permissions, req.ExpiresIn)
	}
	await(t, helped, "the helper to be asking", func(ev HelperEvent) bool { return ev.Phase == HelperAsking })
	// The lookup spent the code, as a redemption does.
	await(t, events, "a new code", func(ev OwnerEvent) bool { return ev.Replaced && ev.Peer.Code != shown.Code })

	if _, err := o.Answer(context.Background(), req.SessionID, true); err != nil {
		t.Fatal(err)
	}
	await(t, helped, "the helper to be let in", func(ev HelperEvent) bool { return ev.Phase == HelperAllowed })
	owner := await(t, served, "the owner's session", func(*SessionConn) bool { return true })
	view := await(t, viewed, "the helper's session", func(*SessionConn) bool { return true })
	if !owner.Nostr || !view.Nostr {
		t.Errorf("Nostr = %v on the owner's side and %v on the helper's, want both", owner.Nostr, view.Nostr)
	}
	if view.ID != owner.ID || owner.ID != req.SessionID {
		t.Errorf("the helper views %q, the owner serves %q, the prompt showed %q", view.ID, owner.ID, req.SessionID)
	}

	if err := view.WriteMessage([]byte(`{"type":"offer","sdp":"v=0","viewer_id":"helper"}`)); err != nil {
		t.Fatal(err)
	}
	got := fieldsOf(t, read(t, owner))
	if got["type"] != "offer" || got["sdp"] != "v=0" || got["viewer_id"] == "helper" {
		t.Errorf("the owner read %v; want the offer, labelled with the helper's key", got)
	}
	if err := owner.WriteMessage([]byte(`{"type":"answer","sdp":"v=0","viewer_id":"someone-else"}`)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, view); got != `{"sdp":"v=0","type":"answer"}` {
		t.Errorf("the helper read %s", got)
	}

	// The owner ending the session ends the helper's view of it.
	o.NewCode()
	ended(t, view, "the owner ended the session")
	await(t, helped, "Help to end", func(ev HelperEvent) bool { return ev.Phase == HelperEnded })

	kinds := nr.published()
	if len(kinds) == 0 {
		t.Fatal("nothing went over the Nostr relay")
	}
	for _, k := range kinds {
		if k != 1059 {
			t.Fatalf("the Nostr relay saw an event of kind %d; want only gift wraps (1059): %v", k, kinds)
		}
	}
}

func TestADeclineOverNostrReachesTheHelper(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	serve, served := serving()
	o, events, _, _ := runOwnerServing(t, newNostrClient(t, r, nr), serve)
	shown := await(t, events, "a code", ready).Peer

	helped := helpWith(t, newNostrClient(t, r, nr), shown.Link, "Ana")
	req := await(t, events, "the request", asked).Consent
	s, err := o.Answer(context.Background(), req.SessionID, false)
	if err != nil || s.State != StateConsentDenied {
		t.Fatalf("Answer = %+v, %v", s, err)
	}
	await(t, helped, "the decline", func(ev HelperEvent) bool { return ev.Phase == HelperDeclined })
	select {
	case <-served:
		t.Fatal("a declined session was served")
	case <-time.After(200 * time.Millisecond):
	}
	// A declined request cannot be allowed afterwards.
	if _, err := o.Answer(context.Background(), req.SessionID, true); err == nil {
		select {
		case <-served:
			t.Fatal("allowing a declined request served it")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// One controller at a time, as over the relay: a second one is told the owner
// is busy, and the first keeps its place.
func TestASecondHelperOverNostrIsToldTheOwnerIsBusy(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	_, events, _, _ := runOwner(t, newNostrClient(t, r, nr))
	shown := await(t, events, "a code", ready).Peer

	helpWith(t, newNostrClient(t, r, nr), shown.Formatted, "Ana")
	await(t, events, "the first request", asked)
	next := await(t, events, "the next code", func(ev OwnerEvent) bool { return ev.Peer.Code != shown.Code && ev.Peer.Code != "" }).Peer

	second := helpWith(t, newNostrClient(t, r, nr), next.Formatted, "Bruno")
	ev := await(t, second, "the second helper's answer", func(ev HelperEvent) bool { return ev.Phase != HelperAsking })
	if ev.Phase != HelperFailed || !errors.Is(ev.Err, ErrBusy) {
		t.Fatalf("the second helper got %+v, want busy", ev)
	}
}

// A helper that gives up while asking frees the owner for the next one.
func TestAHelperGivingUpOverNostrFreesTheOwner(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	_, events, _, _ := runOwner(t, newNostrClient(t, r, nr))
	shown := await(t, events, "a code", ready).Peer

	ctx, cancel := context.WithCancel(context.Background())
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		newNostrClient(t, r, nr).Help(ctx, shown.Formatted, "Ana", func(HelperEvent) {})
	}()
	first := await(t, events, "the first request", asked).Consent
	next := await(t, events, "the next code", func(ev OwnerEvent) bool { return ev.Peer.Code != shown.Code && ev.Peer.Code != "" }).Peer
	cancel()
	<-gone

	// The owner hears the helper leave, then a new one may ask.
	time.Sleep(300 * time.Millisecond)
	helpWith(t, newNostrClient(t, r, nr), next.Formatted, "Bruno")
	second := await(t, events, "the second request", asked).Consent
	if second.Name != "Bruno" || second.SessionID == first.SessionID {
		t.Fatalf("the second request is %+v", second)
	}
}

// A helper using Nostr cannot reach an owner that is not listening there, and
// says so instead of waiting.
func TestAnOwnerWithoutNostrCannotBeReachedOverIt(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	_, events, _, _ := runOwner(t, newClient(t, r))
	shown := await(t, events, "a code", ready).Peer

	helped := helpWith(t, newNostrClient(t, r, nr), shown.Formatted, "Ana")
	ev := await(t, helped, "the failure", func(ev HelperEvent) bool { return ev.Phase == HelperFailed })
	if !errors.Is(ev.Err, ErrNoNostr) {
		t.Fatalf("Help failed with %v, want ErrNoNostr", ev.Err)
	}
}

// An owner listening on Nostr can still be helped through the relay by a Chirp
// that does not use Nostr: the old path stays for those who prefer it.
func TestAnOwnerOnNostrCanStillBeHelpedThroughTheRelay(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	o, events, _, _ := runOwner(t, newNostrClient(t, r, nr))
	shown := await(t, events, "a code", ready).Peer

	helped := helpWith(t, newClient(t, r), shown.Formatted, "Ana")
	req := await(t, events, "the request", asked).Consent
	if _, err := o.Answer(context.Background(), req.SessionID, true); err != nil {
		t.Fatal(err)
	}
	await(t, helped, "the helper to be let in", func(ev HelperEvent) bool { return ev.Phase == HelperAllowed })
}

func TestALookupRefusesAnUnusableContact(t *testing.T) {
	for _, body := range []string{
		`{"key":"not-a-key","relays":["wss://relay.example.org"]}`,
		`{"key":"` + strings.Repeat("a", 64) + `","relays":["http://relay.example.org"]}`,
		`{"key":"` + strings.Repeat("a", 64) + `","relays":[]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		c, err := New(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Lookup(context.Background(), "123456"); !errors.Is(err, ErrBadContact) {
			t.Errorf("Lookup answered with %s = %v, want ErrBadContact", body, err)
		}
		srv.Close()
	}
}

// barahn/chirp#43's "done when": a session between two Chirps negotiates over
// a Nostr relay -- remotekit's runner on one side, its viewer on the other --
// and the relay sees only gift wraps.
func TestTheHelperSeesTheOwnersScreenOverNostr(t *testing.T) {
	r, nr := newRelay(t), newNostrRelay(t)
	o, events, _, _ := runOwnerServing(t, newNostrClient(t, r, nr), func(ctx context.Context, conn *SessionConn) {
		runner := stream.New(stream.Config{ID: conn.ID, PeerConfig: &webrtc.PeerConfig{}})
		runner.RequireScreenViewConsent(true)
		runner.DisableRelayFallback(true)
		runner.RequireDataChannel(conn.Nostr)
		// Screen only: granting remote control here would let the test
		// drive the real pointer.
		runner.Grant(stream.PermissionScreenView)
		_ = runner.Serve(ctx, conn)
	})
	shown := await(t, events, "a code", ready).Peer

	frames := make(chan image.Rectangle, 8)
	notices := make(chan stream.Notice, 8)
	cc := newNostrClient(t, r, nr)
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
	helpWith(t, cc, shown.Formatted, "Ana")

	req := await(t, events, "the request", asked).Consent
	if _, err := o.Answer(context.Background(), req.SessionID, true); err != nil {
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
	for _, k := range nr.published() {
		if k != 1059 {
			t.Fatalf("the Nostr relay saw an event of kind %d; want only gift wraps", k)
		}
	}
}

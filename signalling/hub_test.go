// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

// fakeConn records what the hub sends it. The Conn interface exists so that
// routing can be tested without a browser, a socket or a WebRTC stack.
type fakeConn struct {
	mu     sync.Mutex
	sent   [][]byte
	closed bool
	err    error
}

func (f *fakeConn) Send(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, append([]byte(nil), data...))
	return nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeConn) messages() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.sent...)
}

// isRelayMessage reports whether data is one of the relay's own messages
// rather than a signal forwarded from the other end.
func isRelayMessage(data []byte) bool {
	var m struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(data, &m) == nil && strings.HasPrefix(m.Type, "chirp.")
}

// signals returns only what the other end sent, without the relay's notices,
// so assertions about forwarding stay exact.
func (f *fakeConn) signals() [][]byte {
	var out [][]byte
	for _, m := range f.messages() {
		if !isRelayMessage(m) {
			out = append(out, m)
		}
	}
	return out
}

// notices returns only the relay's own messages, decoded.
func (f *fakeConn) notices() []map[string]string {
	var out []map[string]string
	for _, m := range f.messages() {
		if isRelayMessage(m) {
			var n map[string]string
			_ = json.Unmarshal(m, &n)
			out = append(out, n)
		}
	}
	return out
}

func (f *fakeConn) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// newSession returns a manager and a session in the given state, so each test
// starts from a real session rather than a stub: the hub's admission rules are
// the session manager's rules, and testing them against a fake would test the
// fake.
func newSession(t *testing.T) (*session.SessionManager, *session.Session) {
	t.Helper()
	sm := session.NewSessionManager(10 * time.Minute)
	t.Cleanup(sm.Close)
	p, err := sm.RegisterPeer("Ana")
	if err != nil {
		t.Fatalf("RegisterPeer: %v", err)
	}
	sess, err := sm.Redeem(string(p.Code), "Bruno", &session.ClientInfo{RemoteIP: "192.0.2.1"})
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	return sm, sess
}

func TestForwardReachesTheOppositeEnd(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl, owner := &fakeConn{}, &fakeConn{}
	if err := h.Join(sess.ID, RoleController, ctrl); err != nil {
		t.Fatalf("join controller: %v", err)
	}
	if err := h.Join(sess.ID, RoleOwner, owner); err != nil {
		t.Fatalf("join owner: %v", err)
	}

	if err := h.Forward(sess.ID, RoleController, []byte(`{"type":"offer"}`)); err != nil {
		t.Fatalf("forward from controller: %v", err)
	}
	if err := h.Forward(sess.ID, RoleOwner, []byte(`{"type":"answer"}`)); err != nil {
		t.Fatalf("forward from owner: %v", err)
	}

	if got := owner.signals(); len(got) != 1 || string(got[0]) != `{"type":"offer"}` {
		t.Errorf("owner received %q, want the controller's offer", got)
	}
	if got := ctrl.signals(); len(got) != 1 || string(got[0]) != `{"type":"answer"}` {
		t.Errorf("controller received %q, want the owner's answer", got)
	}
	// A peer must never be handed its own message: that would loop an offer
	// straight back into the sender's stack.
	if len(ctrl.signals()) != 1 || len(owner.signals()) != 1 {
		t.Error("a message was delivered to its own sender")
	}
}

func TestForwardWithoutTheOtherEndFails(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl := &fakeConn{}
	if err := h.Join(sess.ID, RoleController, ctrl); err != nil {
		t.Fatalf("join: %v", err)
	}

	// The controller often connects before the owner has joined the
	// binary. Sending then must report that nobody is there, not pretend the
	// message was delivered.
	if err := h.Forward(sess.ID, RoleController, []byte("x")); err != ErrNoPeer {
		t.Errorf("forward with no owner = %v, want ErrNoPeer", err)
	}
}

func TestSecondPeerInTheSameRoleIsRejected(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	first, second := &fakeConn{}, &fakeConn{}
	if err := h.Join(sess.ID, RoleOwner, first); err != nil {
		t.Fatalf("join: %v", err)
	}
	// Someone else holding the code must not be able to displace the owner
	// already in the session by connecting after it.
	if err := h.Join(sess.ID, RoleOwner, second); err != ErrRoleTaken {
		t.Fatalf("second owner = %v, want ErrRoleTaken", err)
	}

	if err := h.Join(sess.ID, RoleController, second); err != nil {
		t.Fatalf("controller should still be able to join: %v", err)
	}
	if err := h.Forward(sess.ID, RoleController, []byte("ping")); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if len(first.signals()) != 1 {
		t.Error("the original owner stopped receiving after the rejected join")
	}
}

func TestLeaveFreesTheRoleAndDropsThePair(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	if err := h.Join(sess.ID, RoleOwner, &fakeConn{}); err != nil {
		t.Fatalf("join: %v", err)
	}
	h.Leave(sess.ID, RoleOwner)

	if _, owner := h.Connected(sess.ID); owner {
		t.Error("owner still connected after Leave")
	}
	// Reconnecting after a dropped socket is the common case and must work.
	if err := h.Join(sess.ID, RoleOwner, &fakeConn{}); err != nil {
		t.Errorf("rejoin after leave: %v", err)
	}
}

func TestEndedSessionRefusesPeers(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	if _, err := sm.EndSession(sess.ID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	// Failing at join, rather than at the first message, is what puts the
	// error in front of the person while they are still looking at the page.
	if err := h.Join(sess.ID, RoleOwner, &fakeConn{}); err != ErrNotPermitted {
		t.Errorf("join on ended session = %v, want ErrNotPermitted", err)
	}
}

func TestUnknownSessionIsRejected(t *testing.T) {
	sm, _ := newSession(t)
	h := NewHub(sm)

	if err := h.Join("no-such-session", RoleOwner, &fakeConn{}); err == nil {
		t.Error("join on unknown session succeeded")
	}
}

func TestCloseDropsEveryPeer(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl, owner := &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleController, ctrl)
	_ = h.Join(sess.ID, RoleOwner, owner)

	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !ctrl.isClosed() || !owner.isClosed() {
		t.Error("Close left a connection open")
	}
	if err := h.Join(sess.ID, RoleOwner, &fakeConn{}); err != ErrHubClosed {
		t.Errorf("join after close = %v, want ErrHubClosed", err)
	}
}

func TestConcurrentForwardIsSafe(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl, owner := &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleController, ctrl)
	_ = h.Join(sess.ID, RoleOwner, owner)

	// ICE candidates arrive in bursts from both ends at once; this is the
	// normal case, not an edge case.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = h.Forward(sess.ID, RoleController, []byte("c")) }()
		go func() { defer wg.Done(); _ = h.Forward(sess.ID, RoleOwner, []byte("c")) }()
	}
	wg.Wait()

	if len(owner.signals()) != 50 || len(ctrl.signals()) != 50 {
		t.Errorf("delivered %d to owner and %d to controller, want 50 each",
			len(owner.signals()), len(ctrl.signals()))
	}
}

// When the second end arrives, both are told who is there -- the one that
// waited, so it knows it can send, and the one that arrived, so it knows it is
// registered. Each notice names the other end, and nothing else is sent.
func TestArrivalIsAnnouncedToBothEnds(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl, owner := &fakeConn{}, &fakeConn{}
	if err := h.Join(sess.ID, RoleController, ctrl); err != nil {
		t.Fatalf("join controller: %v", err)
	}
	if n := ctrl.notices(); len(n) != 0 {
		t.Fatalf("a peer alone was sent %v", n)
	}
	if err := h.Join(sess.ID, RoleOwner, owner); err != nil {
		t.Fatalf("join owner: %v", err)
	}

	want := func(c *fakeConn, role Role) {
		t.Helper()
		n := c.notices()
		if len(n) != 1 || n[0]["type"] != PeerJoinedType || n[0]["role"] != string(role) {
			t.Errorf("notices = %v, want one %s naming %s", n, PeerJoinedType, role)
		}
	}
	want(ctrl, RoleOwner)
	want(owner, RoleController)
}

// A peer turned away with ErrRoleTaken never took a place, so nobody is told
// it arrived.
func TestRejectedPeerIsNotAnnounced(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl, owner, intruder := &fakeConn{}, &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleController, ctrl)
	_ = h.Join(sess.ID, RoleOwner, owner)
	if err := h.Join(sess.ID, RoleOwner, intruder); err != ErrRoleTaken {
		t.Fatalf("second owner = %v, want ErrRoleTaken", err)
	}

	if n := ctrl.notices(); len(n) != 1 {
		t.Errorf("controller got %v; the rejected owner was announced", n)
	}
	if n := intruder.notices(); len(n) != 0 {
		t.Errorf("the rejected owner was told of a peer: %v", n)
	}
}

// The end that stays is told when the other leaves, so it stops sending into a
// session nobody is listening to.
func TestDepartureIsAnnouncedToTheEndThatStays(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	ctrl, owner := &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleController, ctrl)
	_ = h.Join(sess.ID, RoleOwner, owner)
	h.Leave(sess.ID, RoleOwner)

	n := ctrl.notices()
	if len(n) != 2 || n[1]["type"] != PeerLeftType || n[1]["role"] != string(RoleOwner) {
		t.Fatalf("controller notices = %v, want %s naming owner last", n, PeerLeftType)
	}
	if n := owner.notices(); len(n) != 1 {
		t.Errorf("the departing owner was sent its own departure: %v", n)
	}

	// The last one out tells nobody, and does not fail.
	h.Leave(sess.ID, RoleController)
}

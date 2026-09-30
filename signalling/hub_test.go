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
	sess, err := sm.CreateSession("tech-1", "Ana", 10*time.Minute)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sm, sess
}

func TestForwardReachesTheOppositeEnd(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech, agent := &fakeConn{}, &fakeConn{}
	if err := h.Join(sess.ID, RoleTechnician, tech); err != nil {
		t.Fatalf("join technician: %v", err)
	}
	if err := h.Join(sess.ID, RoleAgent, agent); err != nil {
		t.Fatalf("join agent: %v", err)
	}

	if err := h.Forward(sess.ID, RoleTechnician, []byte(`{"type":"offer"}`)); err != nil {
		t.Fatalf("forward from technician: %v", err)
	}
	if err := h.Forward(sess.ID, RoleAgent, []byte(`{"type":"answer"}`)); err != nil {
		t.Fatalf("forward from agent: %v", err)
	}

	if got := agent.signals(); len(got) != 1 || string(got[0]) != `{"type":"offer"}` {
		t.Errorf("agent received %q, want the technician's offer", got)
	}
	if got := tech.signals(); len(got) != 1 || string(got[0]) != `{"type":"answer"}` {
		t.Errorf("technician received %q, want the agent's answer", got)
	}
	// A peer must never be handed its own message: that would loop an offer
	// straight back into the sender's stack.
	if len(tech.signals()) != 1 || len(agent.signals()) != 1 {
		t.Error("a message was delivered to its own sender")
	}
}

func TestForwardWithoutTheOtherEndFails(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech := &fakeConn{}
	if err := h.Join(sess.ID, RoleTechnician, tech); err != nil {
		t.Fatalf("join: %v", err)
	}

	// The technician often connects before the helped person has run the
	// binary. Sending then must report that nobody is there, not pretend the
	// message was delivered.
	if err := h.Forward(sess.ID, RoleTechnician, []byte("x")); err != ErrNoPeer {
		t.Errorf("forward with no agent = %v, want ErrNoPeer", err)
	}
}

func TestSecondPeerInTheSameRoleIsRejected(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	first, second := &fakeConn{}, &fakeConn{}
	if err := h.Join(sess.ID, RoleAgent, first); err != nil {
		t.Fatalf("join: %v", err)
	}
	// Someone else holding the code must not be able to displace the agent
	// already in the session by connecting after it.
	if err := h.Join(sess.ID, RoleAgent, second); err != ErrRoleTaken {
		t.Fatalf("second agent = %v, want ErrRoleTaken", err)
	}

	if err := h.Join(sess.ID, RoleTechnician, second); err != nil {
		t.Fatalf("technician should still be able to join: %v", err)
	}
	if err := h.Forward(sess.ID, RoleTechnician, []byte("ping")); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if len(first.signals()) != 1 {
		t.Error("the original agent stopped receiving after the rejected join")
	}
}

func TestLeaveFreesTheRoleAndDropsThePair(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	if err := h.Join(sess.ID, RoleAgent, &fakeConn{}); err != nil {
		t.Fatalf("join: %v", err)
	}
	h.Leave(sess.ID, RoleAgent)

	if _, agent := h.Connected(sess.ID); agent {
		t.Error("agent still connected after Leave")
	}
	// Reconnecting after a dropped socket is the common case and must work.
	if err := h.Join(sess.ID, RoleAgent, &fakeConn{}); err != nil {
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
	if err := h.Join(sess.ID, RoleAgent, &fakeConn{}); err != ErrNotPermitted {
		t.Errorf("join on ended session = %v, want ErrNotPermitted", err)
	}
}

func TestUnknownSessionIsRejected(t *testing.T) {
	sm, _ := newSession(t)
	h := NewHub(sm)

	if err := h.Join("no-such-session", RoleAgent, &fakeConn{}); err == nil {
		t.Error("join on unknown session succeeded")
	}
}

func TestCloseDropsEveryPeer(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech, agent := &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleTechnician, tech)
	_ = h.Join(sess.ID, RoleAgent, agent)

	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !tech.isClosed() || !agent.isClosed() {
		t.Error("Close left a connection open")
	}
	if err := h.Join(sess.ID, RoleAgent, &fakeConn{}); err != ErrHubClosed {
		t.Errorf("join after close = %v, want ErrHubClosed", err)
	}
}

func TestConcurrentForwardIsSafe(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech, agent := &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleTechnician, tech)
	_ = h.Join(sess.ID, RoleAgent, agent)

	// ICE candidates arrive in bursts from both ends at once; this is the
	// normal case, not an edge case.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = h.Forward(sess.ID, RoleTechnician, []byte("c")) }()
		go func() { defer wg.Done(); _ = h.Forward(sess.ID, RoleAgent, []byte("c")) }()
	}
	wg.Wait()

	if len(agent.signals()) != 50 || len(tech.signals()) != 50 {
		t.Errorf("delivered %d to agent and %d to technician, want 50 each",
			len(agent.signals()), len(tech.signals()))
	}
}

// When the second end arrives, both are told who is there -- the one that
// waited, so it knows it can send, and the one that arrived, so it knows it is
// registered. Each notice names the other end, and nothing else is sent.
func TestArrivalIsAnnouncedToBothEnds(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech, agent := &fakeConn{}, &fakeConn{}
	if err := h.Join(sess.ID, RoleTechnician, tech); err != nil {
		t.Fatalf("join technician: %v", err)
	}
	if n := tech.notices(); len(n) != 0 {
		t.Fatalf("a peer alone was sent %v", n)
	}
	if err := h.Join(sess.ID, RoleAgent, agent); err != nil {
		t.Fatalf("join agent: %v", err)
	}

	want := func(c *fakeConn, role Role) {
		t.Helper()
		n := c.notices()
		if len(n) != 1 || n[0]["type"] != PeerJoinedType || n[0]["role"] != string(role) {
			t.Errorf("notices = %v, want one %s naming %s", n, PeerJoinedType, role)
		}
	}
	want(tech, RoleAgent)
	want(agent, RoleTechnician)
}

// A peer turned away with ErrRoleTaken never took a place, so nobody is told
// it arrived.
func TestRejectedPeerIsNotAnnounced(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech, agent, intruder := &fakeConn{}, &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleTechnician, tech)
	_ = h.Join(sess.ID, RoleAgent, agent)
	if err := h.Join(sess.ID, RoleAgent, intruder); err != ErrRoleTaken {
		t.Fatalf("second agent = %v, want ErrRoleTaken", err)
	}

	if n := tech.notices(); len(n) != 1 {
		t.Errorf("technician got %v; the rejected agent was announced", n)
	}
	if n := intruder.notices(); len(n) != 0 {
		t.Errorf("the rejected agent was told of a peer: %v", n)
	}
}

// The end that stays is told when the other leaves, so it stops sending into a
// session nobody is listening to.
func TestDepartureIsAnnouncedToTheEndThatStays(t *testing.T) {
	sm, sess := newSession(t)
	h := NewHub(sm)

	tech, agent := &fakeConn{}, &fakeConn{}
	_ = h.Join(sess.ID, RoleTechnician, tech)
	_ = h.Join(sess.ID, RoleAgent, agent)
	h.Leave(sess.ID, RoleAgent)

	n := tech.notices()
	if len(n) != 2 || n[1]["type"] != PeerLeftType || n[1]["role"] != string(RoleAgent) {
		t.Fatalf("technician notices = %v, want %s naming agent last", n, PeerLeftType)
	}
	if n := agent.notices(); len(n) != 1 {
		t.Errorf("the departing agent was sent its own departure: %v", n)
	}

	// The last one out tells nobody, and does not fail.
	h.Leave(sess.ID, RoleTechnician)
}

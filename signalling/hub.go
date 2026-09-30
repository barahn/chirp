// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package signalling routes WebRTC signalling between the two ends of a Chirp
// session: the technician who opened it and the ephemeral agent the helped
// person runs.
//
// It is deliberately small. Barahn's control plane signals for a fleet of
// standing agents and needs tenants, roles and persistence to do it; Chirp
// signals for one session at a time, between two peers who found each other
// with a six-digit code that expires. None of that machinery applies, and
// none of it is borrowed -- this is written against the wire format in
// remotekit/webrtc, which is Apache-2.0, and shares no code with the
// proprietary implementation.
//
// The hub holds no session state of its own. Whether a peer may connect at
// all is session.SessionManager's decision, and this package asks rather than
// duplicating the rules.
package signalling

import (
	"errors"
	"sync"

	"github.com/barahn/chirp/session"
)

// Role identifies which end of a session a peer is.
type Role string

const (
	// RoleTechnician is the operator who created the session.
	RoleTechnician Role = "technician"
	// RoleAgent is the ephemeral binary the helped person runs.
	RoleAgent Role = "agent"
)

var (
	ErrRoleTaken    = errors.New("chirp/signalling: that role is already connected for this session")
	ErrNoPeer       = errors.New("chirp/signalling: the other end is not connected")
	ErrUnknownRole  = errors.New("chirp/signalling: unknown role")
	ErrHubClosed    = errors.New("chirp/signalling: hub is closed")
	ErrNotPermitted = errors.New("chirp/signalling: session does not permit signalling")
)

// Conn is the transport a peer is reachable on. A WebSocket satisfies it; so
// does a pipe in a test, which is why signalling is testable without a browser
// or a network.
type Conn interface {
	// Send delivers one encoded signal message to the peer. It must be safe
	// to call from another peer's goroutine.
	Send(data []byte) error
	// Close releases the transport.
	Close() error
}

// pair is the two ends of one session. Either may be absent: peers arrive in
// whichever order the humans happen to act.
type pair struct {
	technician Conn
	agent      Conn
}

func (p *pair) get(r Role) Conn {
	if r == RoleTechnician {
		return p.technician
	}
	return p.agent
}

func (p *pair) set(r Role, c Conn) {
	if r == RoleTechnician {
		p.technician = c
	} else {
		p.agent = c
	}
}

func (p *pair) empty() bool { return p.technician == nil && p.agent == nil }

// Admissible reports whether a session is in a state that may still signal.
//
// It lives here, and is exported, so that the HTTP layer can refuse before
// upgrading a WebSocket rather than after: a peer rejected post-upgrade has to
// read the reason off a socket, where the same refusal before it is a status
// code every client already understands. Two copies of this rule would
// eventually disagree, and the one that mattered would be whichever ran last.
func Admissible(s *session.Session) error {
	switch s.State {
	case session.StateExpired, session.StateTerminated, session.StateConsentDenied:
		return ErrNotPermitted
	}
	return nil
}

// Hub routes messages between the peers of each session.
type Hub struct {
	sessions *session.SessionManager

	mu     sync.Mutex
	pairs  map[string]*pair
	closed bool
}

// NewHub returns a hub routing for the sessions sm knows about.
func NewHub(sm *session.SessionManager) *Hub {
	return &Hub{sessions: sm, pairs: make(map[string]*pair)}
}

// Join registers conn as role for the session identified by sessionID.
//
// The session must exist and must not have expired or ended: a code that has
// run out of time should fail here rather than at the first message, when the
// human on the other end is already waiting.
//
// Join returns an error if that role is already connected. Two technicians on
// one session, or a second agent, means either a stale connection or someone
// else holding the code, and silently replacing the first peer would hand the
// session over without telling anyone.
func (h *Hub) Join(sessionID string, role Role, conn Conn) error {
	if role != RoleTechnician && role != RoleAgent {
		return ErrUnknownRole
	}
	if conn == nil {
		return errors.New("chirp/signalling: nil conn")
	}

	sess, err := h.sessions.GetSession(sessionID)
	if err != nil {
		return err
	}
	if err := Admissible(sess); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ErrHubClosed
	}

	p, ok := h.pairs[sessionID]
	if !ok {
		p = &pair{}
		h.pairs[sessionID] = p
	}
	if p.get(role) != nil {
		return ErrRoleTaken
	}
	p.set(role, conn)
	return nil
}

// Leave removes role from the session and drops the pair once both ends are
// gone, so a hub that has been busy does not keep growing.
func (h *Hub) Leave(sessionID string, role Role) {
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.pairs[sessionID]
	if !ok {
		return
	}
	p.set(role, nil)
	if p.empty() {
		delete(h.pairs, sessionID)
	}
}

// Forward delivers data to the peer opposite from, for sessionID.
//
// The hub does not inspect or validate the payload. Signalling messages are
// SDP and ICE candidates that mean something to the two WebRTC stacks and
// nothing here, and a relay that parses what it forwards acquires opinions
// about protocol versions it has no way to keep current.
func (h *Hub) Forward(sessionID string, from Role, data []byte) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrHubClosed
	}
	p, ok := h.pairs[sessionID]
	if !ok {
		h.mu.Unlock()
		return ErrNoPeer
	}
	var target Conn
	if from == RoleTechnician {
		target = p.agent
	} else {
		target = p.technician
	}
	h.mu.Unlock()

	if target == nil {
		return ErrNoPeer
	}
	return target.Send(data)
}

// Connected reports which roles are currently attached to sessionID.
func (h *Hub) Connected(sessionID string) (technician, agent bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.pairs[sessionID]
	if !ok {
		return false, false
	}
	return p.technician != nil, p.agent != nil
}

// Close drops every peer. Connections are closed while the lock is not held,
// so a transport that blocks on Close cannot stall the whole hub.
func (h *Hub) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	var conns []Conn
	for _, p := range h.pairs {
		if p.technician != nil {
			conns = append(conns, p.technician)
		}
		if p.agent != nil {
			conns = append(conns, p.agent)
		}
	}
	h.pairs = make(map[string]*pair)
	h.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
	return nil
}

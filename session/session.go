// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package session is Chirp's product layer over remotekit's rendezvous
// mechanism: who owns a code, who redeems it, and what a session between them
// may do.
//
// The direction follows docs/product-model.md. The machine to be controlled --
// the owner -- registers as a peer and shows its code. Whoever redeems that
// code becomes the controller of a new session with the owner, and nothing
// happens until the owner consents on its own machine.
//
// Codes, links, redemption and the per-origin throttle are remotekit's
// (github.com/barahn/remotekit/rendezvous). This package supplies the values --
// how long a code lives, how many failures an origin gets -- and the session
// state machine.
package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/barahn/remotekit/rendezvous"
	"github.com/google/uuid"
)

// SessionState represents the lifecycle phase of a Chirp session.
type SessionState string

const (
	StatePending          SessionState = "pending"
	StateConnected        SessionState = "connected"
	StateConsentRequested SessionState = "consent_requested"
	StateConsentGranted   SessionState = "consent_granted"
	StateConsentDenied    SessionState = "consent_denied"
	StateActive           SessionState = "active"
	StateExpired          SessionState = "expired"
	StateTerminated       SessionState = "terminated"
)

// Chirp's values for the rendezvous mechanism. They are product decisions, so
// they live here rather than in remotekit, which picks none.
const (
	// DefaultMaxRedemptionFailures and DefaultRedemptionWindow bound how many
	// failed redemptions one origin may make: small enough to stop a sweep of
	// the 900,000-code space, large enough for someone who mistypes.
	DefaultMaxRedemptionFailures = 5
	DefaultRedemptionWindow      = 10 * time.Minute

	// DefaultCodeTTL is how long a code stays redeemable once shown or minted.
	DefaultCodeTTL = 10 * time.Minute

	// DefaultMaxSessionsPerOrigin bounds how many sessions one origin may hold
	// open at once as controller. A maintainer decision: 30.
	DefaultMaxSessionsPerOrigin = 30

	// DefaultMaxPeersPerOrigin bounds how many peers registered from one
	// origin may hold a live code at once. Every registration holds a code, so
	// without it one origin could fill the 900,000-code space. It counts live
	// codes rather than registrations so that a Chirp which exits without
	// releasing itself gives its place back when its code expires. A
	// maintainer decision: 30.
	DefaultMaxPeersPerOrigin = 30
)

var (
	ErrSessionNotFound   = errors.New("chirp: session not found")
	ErrSessionExpired    = errors.New("chirp: session has expired")
	ErrSessionEnded      = errors.New("chirp: session ended without a consent decision")
	ErrSessionNotPending = errors.New("chirp: session is no longer pending")
	ErrConsentAlreadySet = errors.New("chirp: consent decision has already been submitted")
	ErrInvalidState      = errors.New("chirp: invalid state transition")
	ErrPeerNotFound      = errors.New("chirp: peer not found")
	// ErrPeerBusy reports that the owner already has a session in progress.
	// An owner serves one controller at a time.
	ErrPeerBusy = errors.New("chirp: that peer is already in a session")
	// ErrTooManySessions reports that the controller's origin already holds
	// the maximum number of open sessions.
	ErrTooManySessions = errors.New("chirp: too many open sessions from this origin")
	// ErrTooManyPeers reports that the origin a peer registered from already
	// has the maximum number of peers holding a live code.
	ErrTooManyPeers = errors.New("chirp: too many peers registered from this origin")
)

// Rendezvous errors a caller may need to tell apart, re-exported so callers
// need not import remotekit to classify an error.
var (
	ErrCodeNotFound = rendezvous.ErrNotFound
	ErrCodeExpired  = rendezvous.ErrExpired
	ErrMalformed    = rendezvous.ErrMalformed
	ErrThrottled    = rendezvous.ErrThrottled
)

// ClientInfo describes the controller's connection.
type ClientInfo struct {
	Hostname  string `json:"hostname,omitempty"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
	RemoteIP  string `json:"remote_ip"`
	UserAgent string `json:"user_agent,omitempty"`
}

// originOf returns the throttling identity for a caller. An empty origin is
// throttled in a shared bucket by remotekit rather than escaping the limit.
func originOf(info *ClientInfo) string {
	if info == nil {
		return ""
	}
	return info.RemoteIP
}

// Peer is a running Chirp that can be reached: the machine that would be
// controlled. Its ID is its secret -- whoever holds it speaks for the peer --
// so it is returned only to the peer itself.
type Peer struct {
	ID           rendezvous.PeerID `json:"peer_id"`
	Name         string            `json:"name"`
	Code         rendezvous.Code   `json:"code"`
	Link         rendezvous.Token  `json:"link"`
	RegisteredAt time.Time         `json:"registered_at"`
}

type peerEntry struct {
	peer        Peer
	origin      string // where it registered from, for the per-origin cap
	codeExpires time.Time
	active      string // session in progress, if any
}

// Session is one controller's connection to one owner.
type Session struct {
	ID    string            `json:"id"`
	Owner rendezvous.PeerID `json:"-"`
	// OwnerName is the name the owner registered with.
	OwnerName string `json:"owner_name"`
	// ControllerName is whatever the controller typed. Nobody registers, so it
	// is a claim, never an identity, and must be presented as one.
	ControllerName string           `json:"controller_name"`
	Code           rendezvous.Code  `json:"-"`
	State          SessionState     `json:"state"`
	CreatedAt      time.Time        `json:"created_at"`
	ExpiresAt      time.Time        `json:"expires_at"`
	ConnectedAt    *time.Time       `json:"connected_at,omitempty"`
	EndedAt        *time.Time       `json:"ended_at,omitempty"`
	ClientInfo     *ClientInfo      `json:"client_info,omitempty"`
	Consent        *ConsentDecision `json:"consent,omitempty"`

	mu sync.RWMutex
	// settled is closed, under mu, once the consent question has an answer:
	// Consent is set, or the session ended without one. Closing wakes every
	// WaitForConsent at once, where a send would wake only one of them.
	settled chan struct{}
}

// Clone returns a thread-safe snapshot of the session state.
func (s *Session) Clone() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cloneLocked()
}

// cloneLocked is Clone for callers that already hold s.mu, for reading or for
// writing. It must never take the lock itself: sync.RWMutex is not reentrant,
// so a Clone() from under a held lock deadlocks rather than blocking briefly --
// and under a write lock it deadlocks unconditionally.
func (s *Session) cloneLocked() *Session {
	cp := &Session{
		ID:             s.ID,
		Owner:          s.Owner,
		OwnerName:      s.OwnerName,
		ControllerName: s.ControllerName,
		Code:           s.Code,
		State:          s.State,
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
	}
	if s.ConnectedAt != nil {
		t := *s.ConnectedAt
		cp.ConnectedAt = &t
	}
	if s.EndedAt != nil {
		t := *s.EndedAt
		cp.EndedAt = &t
	}
	if s.ClientInfo != nil {
		info := *s.ClientInfo
		cp.ClientInfo = &info
	}
	if s.Consent != nil {
		c := *s.Consent
		cp.Consent = &c
	}
	return cp
}

// settleLocked closes settled unless it already is. Callers hold s.mu for
// writing, so the check and the close cannot race.
func (s *Session) settleLocked() {
	select {
	case <-s.settled:
	default:
		close(s.settled)
	}
}

// overdueLocked reports whether an undecided session has passed its deadline.
// A session with consent granted or already streaming has no deadline.
func (s *Session) overdueLocked(now time.Time) bool {
	switch s.State {
	case StatePending, StateConnected, StateConsentRequested:
		return !now.Before(s.ExpiresAt)
	}
	return false
}

// expireLocked ends an overdue session at its deadline, releases its consent
// waiters, and returns the snapshot to deliver to listeners once s.mu is
// released. The reaper and lazy expiry in transition both end a session here,
// so the two cannot leave it in different shapes.
func (s *Session) expireLocked() *Session {
	s.State = StateExpired
	end := s.ExpiresAt
	s.EndedAt = &end
	s.settleLocked()
	return s.cloneLocked()
}

// RemainingTTL returns how much time remains before the session expires.
func (s *Session) RemainingTTL() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rem := time.Until(s.ExpiresAt)
	if rem < 0 {
		return 0
	}
	return rem
}

// terminal reports whether a state is final.
func terminal(st SessionState) bool {
	return st == StateTerminated || st == StateExpired || st == StateConsentDenied
}

// SessionManager holds peers and sessions in memory.
//
// Lock order, which every path follows: sm.mu before any sess.mu, and
// sm.listenersMu is never held with either.
type SessionManager struct {
	reg         *rendezvous.Registry
	peers       map[rendezvous.PeerID]*peerEntry
	sessions    map[string]*Session
	listeners   map[string][]chan *Session
	sessionTTL  time.Duration
	codeTTL     time.Duration
	maxPerOrig  int
	maxPeers    int
	stopReaper  chan struct{}
	mu          sync.RWMutex
	listenersMu sync.RWMutex
}

// NewSessionManager returns a manager whose sessions must reach a decision
// within sessionTTL, with a background reaper for expired sessions.
func NewSessionManager(sessionTTL time.Duration) *SessionManager {
	if sessionTTL <= 0 {
		sessionTTL = 10 * time.Minute
	}
	reg, err := rendezvous.New(rendezvous.Limits{
		MaxFailures: DefaultMaxRedemptionFailures,
		Window:      DefaultRedemptionWindow,
	})
	if err != nil {
		// Only non-positive limits fail, and these are positive constants.
		panic(err)
	}
	sm := &SessionManager{
		reg:        reg,
		peers:      make(map[rendezvous.PeerID]*peerEntry),
		sessions:   make(map[string]*Session),
		listeners:  make(map[string][]chan *Session),
		sessionTTL: sessionTTL,
		codeTTL:    DefaultCodeTTL,
		maxPerOrig: DefaultMaxSessionsPerOrigin,
		maxPeers:   DefaultMaxPeersPerOrigin,
		stopReaper: make(chan struct{}),
	}
	go sm.startReaper()
	return sm
}

// Close stops the background reaper.
func (sm *SessionManager) Close() {
	select {
	case <-sm.stopReaper:
	default:
		close(sm.stopReaper)
	}
}

// SetRedemptionLimit replaces the throttle on failed redemptions and link
// uses, discarding the counts so far. Self-hosting is a commitment, so the
// limit is tunable: an operator behind a large NAT needs a looser bound.
func (sm *SessionManager) SetRedemptionLimit(maxFailures int, window time.Duration) error {
	return sm.reg.SetLimits(rendezvous.Limits{MaxFailures: maxFailures, Window: window})
}

// SetMaxSessionsPerOrigin replaces the bound on open sessions per origin.
func (sm *SessionManager) SetMaxSessionsPerOrigin(n int) error {
	if n <= 0 {
		return errors.New("chirp: the session limit must be positive")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxPerOrig = n
	return nil
}

// openFromLocked counts the sessions origin holds open as controller. A session
// counts until it ends, is denied, or passes an undecided deadline. Called with
// sm.mu held.
func (sm *SessionManager) openFromLocked(origin string, now time.Time) int {
	n := 0
	for _, s := range sm.sessions {
		s.mu.RLock()
		if originOf(s.ClientInfo) == origin && !terminal(s.State) &&
			(s.State == StateActive || s.State == StateConsentGranted || now.Before(s.ExpiresAt)) {
			n++
		}
		s.mu.RUnlock()
	}
	return n
}

// SetMaxPeersPerOrigin changes how many peers registered from one origin may
// hold a live code at once.
func (sm *SessionManager) SetMaxPeersPerOrigin(n int) error {
	if n <= 0 {
		return errors.New("chirp: the peer limit must be positive")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxPeers = n
	return nil
}

// livePeersFromLocked counts the peers registered from origin whose code is
// still live. Called with sm.mu held.
func (sm *SessionManager) livePeersFromLocked(origin string, now time.Time) int {
	n := 0
	for _, pe := range sm.peers {
		if pe.origin == origin && now.Before(pe.codeExpires) {
			n++
		}
	}
	return n
}

// RegisterPeer makes a running Chirp reachable. It returns the peer's secret ID,
// a single-use code to show, and a link that mints a fresh code per use and
// lives until the peer is released. origin is where the request came from: one
// origin may hold at most DefaultMaxPeersPerOrigin live codes (tunable with
// SetMaxPeersPerOrigin), and an empty origin is one shared origin rather than
// an exemption.
func (sm *SessionManager) RegisterPeer(name, origin string) (*Peer, error) {
	id := rendezvous.PeerID(uuid.New().String())

	// The count and the allocation happen under one lock, so concurrent
	// registrations from one origin cannot all pass the check at once.
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now().UTC()
	if sm.livePeersFromLocked(origin, now) >= sm.maxPeers {
		return nil, ErrTooManyPeers
	}
	code, err := sm.reg.Allocate(id, sm.codeTTL)
	if err != nil {
		return nil, err
	}
	link, err := sm.reg.IssueLink(id, 0)
	if err != nil {
		sm.reg.Release(id)
		return nil, err
	}
	p := Peer{ID: id, Name: name, Code: code, Link: link, RegisteredAt: now}
	sm.peers[id] = &peerEntry{peer: p, origin: origin, codeExpires: now.Add(sm.codeTTL)}

	cp := p
	return &cp, nil
}

// Peer returns a peer by its ID, with the code it should be showing now: a
// code that has been used or has expired is replaced first, so the peer never
// displays a code that opens nothing.
//
// Replacing an expired code takes a place under the per-origin peer cap again,
// since the expired one had given its place up: otherwise an origin could
// register up to the cap, let the codes lapse, register more, and then revive
// them all. Over the cap, Peer returns ErrTooManyPeers; the peer still exists.
func (sm *SessionManager) Peer(id rendezvous.PeerID) (*Peer, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	pe, ok := sm.peers[id]
	if !ok {
		return nil, ErrPeerNotFound
	}
	if now := time.Now().UTC(); !now.Before(pe.codeExpires) {
		if sm.livePeersFromLocked(pe.origin, now) >= sm.maxPeers {
			return nil, ErrTooManyPeers
		}
		if err := sm.renewCodeLocked(pe); err != nil {
			return nil, err
		}
	}
	cp := pe.peer
	return &cp, nil
}

// renewCodeLocked gives a peer a fresh start code. Called with sm.mu held.
func (sm *SessionManager) renewCodeLocked(pe *peerEntry) error {
	code, err := sm.reg.Allocate(pe.peer.ID, sm.codeTTL)
	if err != nil {
		return err
	}
	pe.peer.Code = code
	pe.codeExpires = time.Now().UTC().Add(sm.codeTTL)
	return nil
}

// ReleasePeer forgets a peer when its Chirp exits: its codes and link stop
// working, and a session in progress is terminated.
func (sm *SessionManager) ReleasePeer(id rendezvous.PeerID) error {
	sm.mu.Lock()
	pe, ok := sm.peers[id]
	if !ok {
		sm.mu.Unlock()
		return ErrPeerNotFound
	}
	delete(sm.peers, id)
	active := pe.active
	sm.mu.Unlock()

	sm.reg.Release(id)
	if active != "" {
		_, _ = sm.EndSession(active)
	}
	return nil
}

// Redeem consumes a code on behalf of a controller and opens a session with
// the code's owner, in StatePending. The code is metered against the
// controller's origin by remotekit: malformed, unknown and expired codes all
// count, and an origin out of attempts is refused even for a live code.
//
// The code is spent even if the owner turns out to be busy -- codes are
// single-use -- and if it was the owner's start code, the owner gets a new one.
func (sm *SessionManager) Redeem(code, controllerName string, info *ClientInfo) (*Session, error) {
	origin := originOf(info)
	// Checked before the code is consumed, so an origin at its limit does not
	// burn the owner's code, and again below under the same lock that creates
	// the session, so concurrent redemptions cannot overshoot it.
	sm.mu.RLock()
	full := sm.openFromLocked(origin, time.Now().UTC()) >= sm.maxPerOrig
	sm.mu.RUnlock()
	if full {
		return nil, ErrTooManySessions
	}

	m, err := sm.reg.Redeem(code, origin)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.openFromLocked(origin, time.Now().UTC()) >= sm.maxPerOrig {
		return nil, ErrTooManySessions
	}
	pe, ok := sm.peers[m.Owner]
	if !ok {
		return nil, ErrCodeNotFound
	}
	if pe.peer.Code == m.Code {
		if err := sm.renewCodeLocked(pe); err != nil {
			return nil, err
		}
	}
	if pe.active != "" {
		return nil, ErrPeerBusy
	}

	now := time.Now().UTC()
	sess := &Session{
		ID:             uuid.New().String(),
		Owner:          m.Owner,
		OwnerName:      pe.peer.Name,
		ControllerName: controllerName,
		Code:           m.Code,
		State:          StatePending,
		CreatedAt:      now,
		ExpiresAt:      now.Add(sm.sessionTTL),
		settled:        make(chan struct{}),
	}
	if info != nil {
		ci := *info
		sess.ClientInfo = &ci
	}
	sm.sessions[sess.ID] = sess
	pe.active = sess.ID
	return sess.Clone(), nil
}

// MintFromLink creates a fresh single-use code from a peer's link, metered
// against origin from the same budget as Redeem.
func (sm *SessionManager) MintFromLink(t rendezvous.Token, origin string) (rendezvous.Code, error) {
	return sm.reg.MintFromLink(t, origin, sm.codeTTL)
}

// ActiveSession returns the session a peer is in, if any.
func (sm *SessionManager) ActiveSession(id rendezvous.PeerID) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	pe, ok := sm.peers[id]
	if !ok {
		return nil, ErrPeerNotFound
	}
	sess, ok := sm.sessions[pe.active]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess.Clone(), nil
}

// GetSession retrieves a session snapshot by its ID.
func (sm *SessionManager) GetSession(id string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess.Clone(), nil
}

// get returns the live session, for the transitions below.
func (sm *SessionManager) get(id string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// transition applies fn to a session under its lock and notifies listeners.
// fn returns an error to refuse the transition, leaving the session unchanged.
func (sm *SessionManager) transition(id string, fn func(s *Session, now time.Time) error) (*Session, error) {
	sess, err := sm.get(id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sess.mu.Lock()
	if sess.overdueLocked(now) {
		snap := sess.expireLocked()
		sess.mu.Unlock()
		sm.finished(sess)
		sm.notifyListeners(snap)
		return nil, ErrSessionExpired
	}
	if err := fn(sess, now); err != nil {
		sess.mu.Unlock()
		return nil, err
	}
	snap := sess.cloneLocked()
	sess.mu.Unlock()

	if terminal(snap.State) {
		sm.finished(sess)
	}
	sm.notifyListeners(snap)
	return snap, nil
}

// finished frees the owner for its next session.
func (sm *SessionManager) finished(sess *Session) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if pe, ok := sm.peers[sess.Owner]; ok && pe.active == sess.ID {
		pe.active = ""
	}
}

// Connect records that both ends of a pending session are present.
func (sm *SessionManager) Connect(id string) (*Session, error) {
	return sm.transition(id, func(s *Session, now time.Time) error {
		if s.State != StatePending {
			return ErrSessionNotPending
		}
		s.State = StateConnected
		s.ConnectedAt = &now
		return nil
	})
}

// RequestConsent asks the owner for consent: the prompt is being shown on the
// owner's machine.
func (sm *SessionManager) RequestConsent(id string) (*Session, error) {
	return sm.transition(id, func(s *Session, _ time.Time) error {
		if s.State != StateConnected {
			return ErrInvalidState
		}
		s.State = StateConsentRequested
		return nil
	})
}

// SubmitConsent records the owner's decision. It is refused unless consent was
// requested: accepting a decision in any other state would let one be recorded
// before a prompt was shown, which is the one thing Chirp's consent promises
// cannot happen.
func (sm *SessionManager) SubmitConsent(id string, granted bool, reason string) (*Session, error) {
	return sm.transition(id, func(s *Session, now time.Time) error {
		if s.Consent != nil {
			return ErrConsentAlreadySet
		}
		if s.State != StateConsentRequested {
			return ErrInvalidState
		}
		s.Consent = &ConsentDecision{Granted: granted, Reason: reason, DecidedAt: now}
		if granted {
			s.State = StateConsentGranted
		} else {
			s.State = StateConsentDenied
			s.EndedAt = &now
		}
		s.settleLocked()
		return nil
	})
}

// WaitForConsent blocks until the owner decides, the session ends without a
// decision, or ctx ends. Any number of callers may wait on the same session;
// all of them see the outcome. A session that ends undecided returns
// ErrSessionExpired if its deadline passed and ErrSessionEnded otherwise, at
// once for a caller that arrives after it ended.
func (sm *SessionManager) WaitForConsent(ctx context.Context, id string) (bool, error) {
	sess, err := sm.get(id)
	if err != nil {
		return false, err
	}
	sess.mu.RLock()
	settled := sess.settled
	sess.mu.RUnlock()

	// An outcome already known wins over a context that has also ended.
	select {
	case <-settled:
		return sess.consentOutcome()
	default:
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-settled:
		return sess.consentOutcome()
	}
}

// consentOutcome reports what a settled session settled on.
func (s *Session) consentOutcome() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch {
	case s.Consent != nil:
		return s.Consent.Granted, nil
	case s.State == StateExpired:
		return false, ErrSessionExpired
	default:
		return false, ErrSessionEnded
	}
}

// ActivateSession moves a consented session into streaming.
func (sm *SessionManager) ActivateSession(id string) (*Session, error) {
	return sm.transition(id, func(s *Session, _ time.Time) error {
		if s.State != StateConsentGranted {
			return ErrInvalidState
		}
		s.State = StateActive
		return nil
	})
}

// EndSession terminates a session. Ending one that already ended is not an
// error.
func (sm *SessionManager) EndSession(id string) (*Session, error) {
	sess, err := sm.get(id)
	if err != nil {
		return nil, err
	}
	sess.mu.Lock()
	if terminal(sess.State) {
		snap := sess.cloneLocked()
		sess.mu.Unlock()
		sm.finished(sess)
		return snap, nil
	}
	now := time.Now().UTC()
	sess.State = StateTerminated
	sess.EndedAt = &now
	sess.settleLocked()
	snap := sess.cloneLocked()
	sess.mu.Unlock()

	sm.finished(sess)
	sm.notifyListeners(snap)
	return snap, nil
}

// ListActiveSessions returns every session not in a final state.
func (sm *SessionManager) ListActiveSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	now := time.Now().UTC()
	var list []*Session
	for _, sess := range sm.sessions {
		sess.mu.RLock()
		if !terminal(sess.State) && !sess.overdueLocked(now) {
			list = append(list, sess.cloneLocked())
		}
		sess.mu.RUnlock()
	}
	return list
}

// Subscribe returns a channel of snapshots for a session, and a function that
// stops them.
func (sm *SessionManager) Subscribe(id string) (chan *Session, func()) {
	ch := make(chan *Session, 16)

	sm.listenersMu.Lock()
	sm.listeners[id] = append(sm.listeners[id], ch)
	sm.listenersMu.Unlock()

	if sess, err := sm.GetSession(id); err == nil {
		select {
		case ch <- sess:
		default:
		}
	}

	unsubscribe := func() {
		sm.listenersMu.Lock()
		defer sm.listenersMu.Unlock()
		list := sm.listeners[id]
		for i, c := range list {
			if c == ch {
				sm.listeners[id] = append(list[:i], list[i+1:]...)
				close(ch)
				break
			}
		}
		if len(sm.listeners[id]) == 0 {
			delete(sm.listeners, id)
		}
	}
	return ch, unsubscribe
}

func (sm *SessionManager) notifyListeners(s *Session) {
	sm.listenersMu.RLock()
	defer sm.listenersMu.RUnlock()
	for _, ch := range sm.listeners[s.ID] {
		select {
		case ch <- s:
		default:
		}
	}
}

func (sm *SessionManager) startReaper() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sm.stopReaper:
			return
		case <-ticker.C:
			sm.reapExpired()
		}
	}
}

// reapExpired expires undecided sessions past their deadline and lets the
// rendezvous registry reclaim expired codes and throttle windows.
func (sm *SessionManager) reapExpired() {
	now := time.Now().UTC()

	// Snapshots are collected under the lock and delivered after it is
	// released: a listener must never be fed from inside sm.mu.
	var expired []*Session

	sm.mu.Lock()
	for _, sess := range sm.sessions {
		sess.mu.Lock()
		if sess.overdueLocked(now) {
			expired = append(expired, sess.expireLocked())
			if pe, ok := sm.peers[sess.Owner]; ok && pe.active == sess.ID {
				pe.active = ""
			}
		}
		sess.mu.Unlock()
	}
	sm.mu.Unlock()

	sm.reg.Sweep()

	for _, snap := range expired {
		sm.notifyListeners(snap)
	}
}

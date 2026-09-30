// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionState represents the lifecycle phase of an on-demand Chirp session.
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

var (
	ErrSessionNotFound     = errors.New("chirp: session not found")
	ErrSessionExpired      = errors.New("chirp: session has expired")
	ErrSessionNotPending   = errors.New("chirp: session is no longer pending")
	ErrSessionNotConnected = errors.New("chirp: session is not connected")
	ErrConsentAlreadySet   = errors.New("chirp: consent decision has already been submitted")
	ErrInvalidState        = errors.New("chirp: invalid state transition")
)

// ClientInfo contains metadata about the client connecting to the on-demand session.
type ClientInfo struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	RemoteIP  string `json:"remote_ip"`
	UserAgent string `json:"user_agent,omitempty"`
}

// Session represents an on-demand remote support session.
type Session struct {
	ID             string           `json:"id"`
	Code           string           `json:"code"`
	FormattedCode  string           `json:"formatted_code"`
	TechnicianID   string           `json:"technician_id"`
	TechnicianName string           `json:"technician_name"`
	State          SessionState     `json:"state"`
	CreatedAt      time.Time        `json:"created_at"`
	ExpiresAt      time.Time        `json:"expires_at"`
	ConnectedAt    *time.Time       `json:"connected_at,omitempty"`
	EndedAt        *time.Time       `json:"ended_at,omitempty"`
	ClientInfo     *ClientInfo      `json:"client_info,omitempty"`
	Consent        *ConsentDecision `json:"consent,omitempty"`

	mu        sync.RWMutex `json:"-"`
	consentCh chan bool    `json:"-"`
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
		Code:           s.Code,
		FormattedCode:  s.FormattedCode,
		TechnicianID:   s.TechnicianID,
		TechnicianName: s.TechnicianName,
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

// RemainingTTL returns how much time remains before the session code expires.
func (s *Session) RemainingTTL() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rem := time.Until(s.ExpiresAt)
	if rem < 0 {
		return 0
	}
	return rem
}

// SessionManager coordinates in-memory on-demand sessions, PIN code allocation, and lifecycle transitions.
type SessionManager struct {
	sessions    map[string]*Session
	codeIndex   map[string]*Session
	listeners   map[string][]chan *Session
	defaultTTL  time.Duration
	limiter     *redemptionLimiter
	stopReaper  chan struct{}
	mu          sync.RWMutex
	listenersMu sync.RWMutex
}

// NewSessionManager initializes a new SessionManager with a background expiration reaper.
func NewSessionManager(defaultTTL time.Duration) *SessionManager {
	if defaultTTL <= 0 {
		defaultTTL = 10 * time.Minute
	}

	sm := &SessionManager{
		sessions:   make(map[string]*Session),
		codeIndex:  make(map[string]*Session),
		listeners:  make(map[string][]chan *Session),
		defaultTTL: defaultTTL,
		limiter:    newRedemptionLimiter(DefaultMaxRedemptionFailures, DefaultRedemptionWindow),
		stopReaper: make(chan struct{}),
	}

	go sm.startReaper()
	return sm
}

// Close terminates the session manager and stops the background reaper.
func (sm *SessionManager) Close() {
	select {
	case <-sm.stopReaper:
	default:
		close(sm.stopReaper)
	}
}

// SetRedemptionLimit replaces the throttle applied to failed code redemptions.
// A non-positive value falls back to the corresponding default.
//
// Self-hosting is a design commitment, so the limit has to be tunable: an
// operator behind a NAT that presents thousands of users as one address needs a
// looser bound than one handing out PINs over the open internet. Call it during
// setup, before the manager starts serving; it discards the counts recorded so
// far.
func (sm *SessionManager) SetRedemptionLimit(maxFailures int, window time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.limiter = newRedemptionLimiter(maxFailures, window)
}

// CreateSession generates a new on-demand session with a unique 6-digit PIN code.
func (sm *SessionManager) CreateSession(techID, techName string, ttl time.Duration) (*Session, error) {
	if ttl <= 0 {
		ttl = sm.defaultTTL
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	var code string
	// Attempt code generation with collision prevention (up to 10 attempts)
	for i := 0; i < 10; i++ {
		c, err := GenerateCode()
		if err != nil {
			return nil, err
		}
		if _, exists := sm.codeIndex[c]; !exists {
			code = c
			break
		}
	}
	if code == "" {
		return nil, errors.New("chirp: failed to generate unique session code after multiple attempts")
	}

	now := time.Now().UTC()
	sess := &Session{
		ID:             uuid.New().String(),
		Code:           code,
		FormattedCode:  FormatCode(code),
		TechnicianID:   techID,
		TechnicianName: techName,
		State:          StatePending,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
		consentCh:      make(chan bool, 1),
	}

	sm.sessions[sess.ID] = sess
	sm.codeIndex[sess.Code] = sess

	return sess.Clone(), nil
}

// GetSession retrieves a session snapshot by its UUID.
func (sm *SessionManager) GetSession(id string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sess, ok := sm.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess.Clone(), nil
}

// GetSessionByCode retrieves a session snapshot by its 6-digit code.
func (sm *SessionManager) GetSessionByCode(code string) (*Session, error) {
	norm, err := NormalizeCode(code)
	if err != nil {
		return nil, err
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sess, ok := sm.codeIndex[norm]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess.Clone(), nil
}

// LookupCode resolves a code presented by an untrusted caller, counting the
// attempt against origin's redemption budget exactly as ConnectClient does.
//
// Every entry point that takes a code from the network must come through here
// or through ConnectClient. GetSessionByCode is unmetered, which is right for
// callers that already hold the code legitimately and wrong for anyone else:
// an unmetered lookup is an oracle, and with it the keyspace can be swept
// however tight the limit on ConnectClient is.
//
// An expired session is reported as ErrSessionExpired and counted as a
// failure, so that a caller cannot tell a code that has lapsed from one that
// never existed by watching its budget.
func (sm *SessionManager) LookupCode(code, origin string) (*Session, error) {
	limiter := sm.redemptionLimiter()

	now := time.Now().UTC()
	if !limiter.allow(origin, now) {
		return nil, ErrTooManyAttempts
	}

	norm, err := NormalizeCode(code)
	if err != nil {
		limiter.recordFailure(origin, now)
		return nil, err
	}

	sm.mu.RLock()
	sess, ok := sm.codeIndex[norm]
	sm.mu.RUnlock()

	if !ok {
		limiter.recordFailure(origin, now)
		return nil, ErrSessionNotFound
	}

	sess.mu.RLock()
	expired := sess.State == StateExpired || now.After(sess.ExpiresAt)
	snap := sess.cloneLocked()
	sess.mu.RUnlock()

	if expired {
		limiter.recordFailure(origin, now)
		return nil, ErrSessionExpired
	}

	limiter.reset(origin)
	return snap, nil
}

// redemptionLimiter returns the limiter in force, which SetRedemptionLimit
// may replace.
func (sm *SessionManager) redemptionLimiter() *redemptionLimiter {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.limiter
}

// ConnectClient records the connection of an ephemeral client.
//
// Redemption is throttled per origin (ClientInfo.RemoteIP): the PIN is the only
// credential, so unlimited guessing walks a 900,000-code keyspace in minutes.
// Every attempt that does not end in a connection counts as a failure, and a
// successful one clears the origin's record. See redemption.go.
func (sm *SessionManager) ConnectClient(code string, info *ClientInfo) (*Session, error) {
	origin := originOf(info)

	limiter := sm.redemptionLimiter()

	now := time.Now().UTC()
	if !limiter.allow(origin, now) {
		return nil, ErrTooManyAttempts
	}

	norm, err := NormalizeCode(code)
	if err != nil {
		// A malformed code is still a guess, and refusing to count it would
		// leave an attacker an unmetered channel for probing.
		limiter.recordFailure(origin, now)
		return nil, err
	}

	sm.mu.Lock()
	sess, ok := sm.codeIndex[norm]
	sm.mu.Unlock()

	if !ok {
		limiter.recordFailure(origin, now)
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	if sess.State == StateExpired || now.After(sess.ExpiresAt) {
		sess.State = StateExpired
		sess.mu.Unlock()
		limiter.recordFailure(origin, now)
		return nil, ErrSessionExpired
	}
	if sess.State != StatePending {
		sess.mu.Unlock()
		limiter.recordFailure(origin, now)
		return nil, ErrSessionNotPending
	}

	sess.State = StateConnected
	sess.ConnectedAt = &now
	if info != nil {
		cp := *info
		sess.ClientInfo = &cp
	}
	sess.mu.Unlock()

	limiter.reset(origin)

	snap := sess.Clone()
	sm.notifyListeners(snap)
	return snap, nil
}

// RequestConsent transitions the session to StateConsentRequested.
func (sm *SessionManager) RequestConsent(code string) (*Session, error) {
	norm, err := NormalizeCode(code)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	sess, ok := sm.codeIndex[norm]
	sm.mu.Unlock()

	if !ok {
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	if sess.State == StateExpired || time.Now().UTC().After(sess.ExpiresAt) {
		sess.State = StateExpired
		sess.mu.Unlock()
		return nil, ErrSessionExpired
	}
	if sess.State != StateConnected && sess.State != StatePending {
		sess.mu.Unlock()
		return nil, ErrInvalidState
	}

	sess.State = StateConsentRequested
	sess.mu.Unlock()

	snap := sess.Clone()
	sm.notifyListeners(snap)
	return snap, nil
}

// SubmitConsent records the client authorization response (allow/deny).
func (sm *SessionManager) SubmitConsent(code string, granted bool, reason string) (*Session, error) {
	norm, err := NormalizeCode(code)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	sess, ok := sm.codeIndex[norm]
	sm.mu.Unlock()

	if !ok {
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	if sess.State == StateExpired || time.Now().UTC().After(sess.ExpiresAt) {
		sess.State = StateExpired
		sess.mu.Unlock()
		return nil, ErrSessionExpired
	}
	if sess.Consent != nil {
		sess.mu.Unlock()
		return nil, ErrConsentAlreadySet
	}
	// Consent answers a request. Accepting it in any other state lets a
	// decision be recorded before a machine has connected or a prompt has been
	// shown -- by whoever holds the code, rather than on the helped person's
	// own machine, which is the one thing Chirp's consent promises.
	if sess.State != StateConsentRequested {
		sess.mu.Unlock()
		return nil, ErrInvalidState
	}

	now := time.Now().UTC()
	sess.Consent = &ConsentDecision{
		Granted:   granted,
		Reason:    reason,
		DecidedAt: now,
	}

	if granted {
		sess.State = StateConsentGranted
	} else {
		sess.State = StateConsentDenied
		sess.EndedAt = &now
	}

	// Wake up any blocking waiter
	select {
	case sess.consentCh <- granted:
	default:
	}

	sess.mu.Unlock()

	snap := sess.Clone()
	sm.notifyListeners(snap)
	return snap, nil
}

// WaitForConsent blocks until the end-user submits a consent decision or the context expires.
func (sm *SessionManager) WaitForConsent(ctx context.Context, code string) (bool, error) {
	norm, err := NormalizeCode(code)
	if err != nil {
		return false, err
	}

	sm.mu.RLock()
	sess, ok := sm.codeIndex[norm]
	sm.mu.RUnlock()

	if !ok {
		return false, ErrSessionNotFound
	}

	sess.mu.RLock()
	if sess.Consent != nil {
		granted := sess.Consent.Granted
		sess.mu.RUnlock()
		return granted, nil
	}
	ch := sess.consentCh
	sess.mu.RUnlock()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case granted := <-ch:
		return granted, nil
	}
}

// ActivateSession transitions a consent-granted session into active streaming state.
func (sm *SessionManager) ActivateSession(code string) (*Session, error) {
	norm, err := NormalizeCode(code)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	sess, ok := sm.codeIndex[norm]
	sm.mu.Unlock()

	if !ok {
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	if sess.State != StateConsentGranted {
		sess.mu.Unlock()
		return nil, ErrInvalidState
	}

	sess.State = StateActive
	sess.mu.Unlock()

	snap := sess.Clone()
	sm.notifyListeners(snap)
	return snap, nil
}

// EndSession terminates an active or pending session.
func (sm *SessionManager) EndSession(idOrCode string) (*Session, error) {
	sm.mu.Lock()
	sess, ok := sm.sessions[idOrCode]
	if !ok {
		if norm, err := NormalizeCode(idOrCode); err == nil {
			sess = sm.codeIndex[norm]
		}
	}
	sm.mu.Unlock()

	if sess == nil {
		return nil, ErrSessionNotFound
	}

	sess.mu.Lock()
	if sess.State == StateTerminated || sess.State == StateExpired {
		sess.mu.Unlock()
		return sess.Clone(), nil
	}

	now := time.Now().UTC()
	sess.State = StateTerminated
	sess.EndedAt = &now
	sess.mu.Unlock()

	snap := sess.Clone()
	sm.notifyListeners(snap)
	return snap, nil
}

// ListActiveSessions returns all currently non-terminated, non-expired sessions.
func (sm *SessionManager) ListActiveSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	now := time.Now().UTC()
	var list []*Session
	for _, sess := range sm.sessions {
		sess.mu.RLock()
		if sess.State != StateTerminated && sess.State != StateExpired && sess.State != StateConsentDenied && now.Before(sess.ExpiresAt) {
			list = append(list, sess.cloneLocked())
		}
		sess.mu.RUnlock()
	}
	return list
}

// Subscribe provides a channel receiving live session updates for the given 6-digit code.
func (sm *SessionManager) Subscribe(code string) (chan *Session, func()) {
	norm, err := NormalizeCode(code)
	if err != nil {
		norm = code
	}

	ch := make(chan *Session, 16)

	sm.listenersMu.Lock()
	sm.listeners[norm] = append(sm.listeners[norm], ch)
	sm.listenersMu.Unlock()

	// Initial emission if exists
	if sess, err := sm.GetSessionByCode(norm); err == nil {
		select {
		case ch <- sess:
		default:
		}
	}

	unsubscribe := func() {
		sm.listenersMu.Lock()
		defer sm.listenersMu.Unlock()
		list := sm.listeners[norm]
		for i, c := range list {
			if c == ch {
				sm.listeners[norm] = append(list[:i], list[i+1:]...)
				close(ch)
				break
			}
		}
		if len(sm.listeners[norm]) == 0 {
			delete(sm.listeners, norm)
		}
	}

	return ch, unsubscribe
}

func (sm *SessionManager) notifyListeners(s *Session) {
	sm.listenersMu.RLock()
	defer sm.listenersMu.RUnlock()

	if listeners, ok := sm.listeners[s.Code]; ok {
		for _, ch := range listeners {
			select {
			case ch <- s:
			default:
			}
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

func (sm *SessionManager) reapExpired() {
	now := time.Now().UTC()

	// Snapshots are collected under the lock and delivered after it is
	// released: a listener must never be fed from inside sm.mu, and the
	// delivery is synchronous so that an expiry event is observable the
	// moment reapExpired returns.
	var expired []*Session

	sm.mu.Lock()
	limiter := sm.limiter
	for _, sess := range sm.sessions {
		sess.mu.Lock()
		if (sess.State == StatePending || sess.State == StateConnected || sess.State == StateConsentRequested) && now.After(sess.ExpiresAt) {
			sess.State = StateExpired
			end := sess.ExpiresAt
			sess.EndedAt = &end
			expired = append(expired, sess.cloneLocked())
		}
		sess.mu.Unlock()
	}
	sm.mu.Unlock()

	limiter.reap(now)

	for _, snap := range expired {
		sm.notifyListeners(snap)
	}
}

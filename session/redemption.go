// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session

import (
	"errors"
	"sync"
	"time"
)

// ErrTooManyAttempts is returned when a caller has failed to redeem a code too
// many times inside the limiter's window and is being made to wait.
var ErrTooManyAttempts = errors.New("chirp: too many failed redemption attempts, try again later")

// Defaults for redemption throttling.
//
// Why this is throttled per caller rather than per PIN. In Chirp the PIN *is*
// the credential: there is no second secret to get wrong, so a "failed
// redemption" is not a wrong password against a known session -- it is a guess
// that hit no live session at all. Capping failures per session would therefore
// defend nothing, because the guesses an attacker makes while scanning the
// keyspace belong to no session and would never be counted.
//
// The keyspace is 900,000 codes. An attacker who may guess without limit finds
// a live session in a few hundred thousand requests, which is minutes over a
// fast link. Counting failures per caller is what removes that: it bounds how
// much of the keyspace any one origin may sweep per window, and it costs a
// legitimate user -- who types one PIN, possibly fumbling it once -- nothing.
const (
	DefaultMaxRedemptionFailures = 5
	DefaultRedemptionWindow      = 10 * time.Minute
)

// unknownOrigin is the bucket used when a caller supplies no origin.
//
// It is shared, so callers that omit an origin are throttled collectively. That
// is deliberate: the alternative -- skipping the limit when the origin is empty
// -- turns the whole defence into an opt-out that an attacker simply takes. A
// server that forgets to populate ClientInfo.RemoteIP will throttle its own
// users, which is a loud failure rather than a silent hole.
const unknownOrigin = "\x00unknown"

// attemptRecord counts a single origin's recent redemption failures.
type attemptRecord struct {
	failures    int
	windowStart time.Time
}

// redemptionLimiter bounds how many failed redemptions one origin may make
// inside a window. It is a fixed window rather than a sliding one: an attacker
// gains at most one extra burst at a boundary, which does not change the order
// of magnitude of the keyspace they can sweep, and the cheaper bookkeeping
// keeps this off the hot path of an honest redemption.
type redemptionLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptRecord
	max      int
	window   time.Duration
}

func newRedemptionLimiter(maxFailures int, window time.Duration) *redemptionLimiter {
	if maxFailures <= 0 {
		maxFailures = DefaultMaxRedemptionFailures
	}
	if window <= 0 {
		window = DefaultRedemptionWindow
	}
	return &redemptionLimiter{
		attempts: make(map[string]*attemptRecord),
		max:      maxFailures,
		window:   window,
	}
}

// allow reports whether origin may attempt a redemption now.
func (l *redemptionLimiter) allow(origin string, now time.Time) bool {
	if origin == "" {
		origin = unknownOrigin
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	rec, ok := l.attempts[origin]
	if !ok {
		return true
	}
	if now.Sub(rec.windowStart) >= l.window {
		delete(l.attempts, origin)
		return true
	}
	return rec.failures < l.max
}

// recordFailure counts a failed redemption against origin.
func (l *redemptionLimiter) recordFailure(origin string, now time.Time) {
	if origin == "" {
		origin = unknownOrigin
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	rec, ok := l.attempts[origin]
	if !ok || now.Sub(rec.windowStart) >= l.window {
		l.attempts[origin] = &attemptRecord{failures: 1, windowStart: now}
		return
	}
	rec.failures++
}

// reset clears an origin's failures after it redeems a code successfully, so a
// user who mistypes a PIN before getting it right starts clean.
func (l *redemptionLimiter) reset(origin string) {
	if origin == "" {
		origin = unknownOrigin
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, origin)
}

// reap drops records whose window has closed, so an attacker cycling origins
// cannot grow the map without bound.
func (l *redemptionLimiter) reap(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for origin, rec := range l.attempts {
		if now.Sub(rec.windowStart) >= l.window {
			delete(l.attempts, origin)
		}
	}
}

// originOf returns the throttling identity for a connecting client.
func originOf(info *ClientInfo) string {
	if info == nil {
		return ""
	}
	return info.RemoteIP
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

// An origin that keeps guessing wrong codes must be cut off, because the PIN is
// the only credential standing between an attacker and a live session.
func TestRedemption_WrongCodesAreThrottled(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(3, time.Minute)

	attacker := &session.ClientInfo{RemoteIP: "203.0.113.7"}

	for i := 0; i < 3; i++ {
		_, err := sm.ConnectClient("000000", attacker)
		if !errors.Is(err, session.ErrSessionNotFound) {
			t.Fatalf("attempt %d: expected ErrSessionNotFound, got %v", i+1, err)
		}
	}

	if _, err := sm.ConnectClient("000000", attacker); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatalf("expected ErrTooManyAttempts once the cap is reached, got %v", err)
	}

	// The cap must hold even when the next guess happens to be a live code:
	// otherwise a throttled attacker still wins the moment they guess right.
	live, err := sm.CreateSession("tech", "Tech", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if _, err := sm.ConnectClient(live.Code, attacker); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatalf("a throttled origin redeemed a live code: %v", err)
	}
}

// Throttling one origin must not affect another, or one abusive client would
// deny service to everyone.
func TestRedemption_ThrottleIsPerOrigin(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(2, time.Minute)

	for i := 0; i < 2; i++ {
		if _, err := sm.ConnectClient("000000", &session.ClientInfo{RemoteIP: "198.51.100.1"}); err == nil {
			t.Fatal("expected a failed redemption")
		}
	}
	if _, err := sm.ConnectClient("000000", &session.ClientInfo{RemoteIP: "198.51.100.1"}); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatalf("expected the first origin to be throttled, got %v", err)
	}

	sess, err := sm.CreateSession("tech", "Tech", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	got, err := sm.ConnectClient(sess.Code, &session.ClientInfo{RemoteIP: "198.51.100.2"})
	if err != nil {
		t.Fatalf("an unrelated origin was throttled: %v", err)
	}
	if got.State != session.StateConnected {
		t.Fatalf("expected %s, got %s", session.StateConnected, got.State)
	}
}

// Someone who fumbles a PIN and then gets it right must not carry the failures
// forward into their next session.
func TestRedemption_SuccessClearsFailures(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(3, time.Minute)

	user := &session.ClientInfo{RemoteIP: "192.0.2.50"}

	for i := 0; i < 2; i++ {
		if _, err := sm.ConnectClient("000000", user); err == nil {
			t.Fatal("expected a failed redemption")
		}
	}

	first, err := sm.CreateSession("tech", "Tech", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if _, err := sm.ConnectClient(first.Code, user); err != nil {
		t.Fatalf("redemption after two fumbles failed: %v", err)
	}

	// The record is clear, so the full budget is available again.
	for i := 0; i < 3; i++ {
		_, err := sm.ConnectClient("000000", user)
		if !errors.Is(err, session.ErrSessionNotFound) {
			t.Fatalf("attempt %d after reset: expected ErrSessionNotFound, got %v", i+1, err)
		}
	}
}

// A malformed code is a guess too; leaving it uncounted would hand an attacker
// an unmetered probing channel.
func TestRedemption_MalformedCodesCount(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(2, time.Minute)

	prober := &session.ClientInfo{RemoteIP: "203.0.113.99"}

	for i := 0; i < 2; i++ {
		if _, err := sm.ConnectClient("abc", prober); err == nil {
			t.Fatal("expected a malformed code to be rejected")
		}
	}
	if _, err := sm.ConnectClient("abc", prober); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatalf("expected malformed guesses to be counted, got %v", err)
	}
}

// Callers that supply no origin share one bucket rather than escaping the limit:
// an opt-out defence is one an attacker simply takes.
func TestRedemption_MissingOriginIsNotAnEscapeHatch(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(2, time.Minute)

	for i := 0; i < 2; i++ {
		if _, err := sm.ConnectClient("000000", nil); err == nil {
			t.Fatal("expected a failed redemption")
		}
	}
	if _, err := sm.ConnectClient("000000", nil); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatalf("an origin-less caller escaped the throttle: %v", err)
	}
}

// The window has to reopen, or a shared NAT address would be locked out for good.
func TestRedemption_WindowReopens(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(1, 80*time.Millisecond)

	office := &session.ClientInfo{RemoteIP: "192.0.2.77"}

	if _, err := sm.ConnectClient("000000", office); err == nil {
		t.Fatal("expected a failed redemption")
	}
	if _, err := sm.ConnectClient("000000", office); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatal("expected the origin to be throttled")
	}

	time.Sleep(120 * time.Millisecond)

	sess, err := sm.CreateSession("tech", "Tech", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if _, err := sm.ConnectClient(sess.Code, office); err != nil {
		t.Fatalf("the window did not reopen: %v", err)
	}
}

// The default budget must be small enough to matter and large enough to let a
// real person mistype a PIN.
func TestRedemption_DefaultsAreSane(t *testing.T) {
	if session.DefaultMaxRedemptionFailures < 3 || session.DefaultMaxRedemptionFailures > 10 {
		t.Fatalf("DefaultMaxRedemptionFailures = %d, want a small budget a fumbling user survives",
			session.DefaultMaxRedemptionFailures)
	}

	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()

	client := &session.ClientInfo{RemoteIP: "192.0.2.123"}
	for i := 0; i < session.DefaultMaxRedemptionFailures; i++ {
		if _, err := sm.ConnectClient("000000", client); errors.Is(err, session.ErrTooManyAttempts) {
			t.Fatalf("throttled after %d attempts, before the default budget of %d was spent",
				i, session.DefaultMaxRedemptionFailures)
		}
	}
	if _, err := sm.ConnectClient("000000", client); !errors.Is(err, session.ErrTooManyAttempts) {
		t.Fatalf("the default budget did not apply: %v", err)
	}
}

// Distinct origins must each get their own budget under concurrency, and the
// bookkeeping must be race-free.
func TestRedemption_ConcurrentOrigins(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(4, time.Minute)

	done := make(chan struct{})
	for i := 0; i < 25; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			ip := fmt.Sprintf("198.51.100.%d", n)
			for j := 0; j < 4; j++ {
				if _, err := sm.ConnectClient("000000", &session.ClientInfo{RemoteIP: ip}); errors.Is(err, session.ErrTooManyAttempts) {
					t.Errorf("%s throttled at attempt %d, inside its own budget", ip, j+1)
					return
				}
			}
		}(i)
	}
	for i := 0; i < 25; i++ {
		<-done
	}
}

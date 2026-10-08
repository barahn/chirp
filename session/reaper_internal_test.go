// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// These tests live in package session rather than session_test because the
// defects they pin down are in unexported locking: the reaper is driven
// directly so a 10-second ticker does not have to elapse.

package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A session that reaches its expiry must be reaped without the reaper blocking
// on a lock it already holds. reapExpired used to call Clone() -- which takes
// sess.mu.RLock -- while holding sess.mu.Lock, and sync.RWMutex is not
// reentrant, so the first PIN allowed to expire froze the reaper while it held
// sm.mu, deadlocking every later call on the manager.
func TestReapExpired_DoesNotDeadlock(t *testing.T) {
	sm := NewSessionManager(30 * time.Millisecond)
	defer sm.Close()

	sess, err := openSession(sm)
	if err != nil {
		t.Fatalf("openSession failed: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sm.reapExpired()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reapExpired deadlocked")
	}

	// The manager has to remain usable, which is what the deadlock destroyed.
	got, err := sm.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession after reap failed: %v", err)
	}
	if got.State != StateExpired {
		t.Fatalf("expected %s after reaping, got %s", StateExpired, got.State)
	}
	if got.EndedAt == nil {
		t.Fatal("expected EndedAt to be set on an expired session")
	}
}

// ListActiveSessions had the same shape -- RLock then Clone, which re-enters the
// read lock. That is legal only while no writer is queued, so it deadlocks only
// under contention; this drives a writer alongside it.
func TestListActiveSessions_DoesNotDeadlockUnderWriters(t *testing.T) {
	sm := NewSessionManager(time.Minute)
	defer sm.Close()

	for i := 0; i < 20; i++ {
		if _, err := openSession(sm); err != nil {
			t.Fatalf("openSession failed: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					sm.ListActiveSessions()
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					// A writer queued behind the readers is what makes a
					// recursive RLock deadlock rather than merely work.
					if _, err := openSession(sm); err != nil {
						t.Errorf("openSession failed: %v", err)
						return
					}
				}
			}()
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("ListActiveSessions deadlocked against a queued writer")
	}
}

// The reaper must reach subscribers of an expiring session.
func TestReapExpired_NotifiesSubscribers(t *testing.T) {
	sm := NewSessionManager(30 * time.Millisecond)
	defer sm.Close()

	sess, err := openSession(sm)
	if err != nil {
		t.Fatalf("openSession failed: %v", err)
	}

	ch, unsub := sm.Subscribe(sess.ID)
	defer unsub()
	<-ch // the initial snapshot emitted on subscribe

	time.Sleep(60 * time.Millisecond)
	sm.reapExpired()

	select {
	case got := <-ch:
		if got.State != StateExpired {
			t.Fatalf("expected %s, got %s", StateExpired, got.State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no expiry event reached the subscriber")
	}
}

// openSession registers an owner and redeems its code, which is how every
// session starts.
var registrations atomic.Int64

func openSession(sm *SessionManager) (*Session, error) {
	p, err := sm.RegisterPeer("Owner", fmt.Sprint("owner ", registrations.Add(1)))
	if err != nil {
		return nil, err
	}
	// Each session from its own origin, so the per-origin session cap is not
	// what these tests measure.
	return sm.Redeem(string(p.Code), "Controller", &ClientInfo{RemoteIP: string(p.ID)})
}

// The reaper is the only thing that expires a session nobody touches, such as
// an owner who never answers the prompt. Its waiters must be released too.
func TestReapExpired_ReleasesConsentWaiters(t *testing.T) {
	sm := NewSessionManager(30 * time.Millisecond)
	defer sm.Close()

	sess, err := openSession(sm)
	if err != nil {
		t.Fatalf("openSession failed: %v", err)
	}
	type result struct {
		granted bool
		err     error
	}
	res := make(chan result, 1)
	go func() {
		granted, err := sm.WaitForConsent(context.Background(), sess.ID)
		res <- result{granted, err}
	}()
	time.Sleep(60 * time.Millisecond)
	sm.reapExpired()

	select {
	case r := <-res:
		if r.granted || !errors.Is(r.err, ErrSessionExpired) {
			t.Fatalf("WaitForConsent = %v, %v; want false, %v", r.granted, r.err, ErrSessionExpired)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForConsent still blocked after the reaper expired its session")
	}
}

// A Chirp that exits without releasing itself must not hold its place under
// the peer cap forever: the place comes back when its code expires. And a
// lapsed code cannot be revived past the cap, or an origin could register up
// to the cap, let the codes lapse, register more, and revive them all.
func TestPeerCapCountsLiveCodes(t *testing.T) {
	sm := NewSessionManager(time.Minute)
	defer sm.Close()
	sm.codeTTL = 30 * time.Millisecond
	if err := sm.SetMaxPeersPerOrigin(2); err != nil {
		t.Fatal(err)
	}

	const from = "198.51.100.20"
	lapsed, err := sm.RegisterPeer("a", from)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sm.RegisterPeer("b", from); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.RegisterPeer("c", from); !errors.Is(err, ErrTooManyPeers) {
		t.Fatalf("third registration: got %v, want ErrTooManyPeers", err)
	}

	time.Sleep(50 * time.Millisecond) // a's and b's codes lapse

	sm.codeTTL = time.Minute
	c, err := sm.RegisterPeer("c", from)
	if err != nil {
		t.Fatalf("lapsed codes still held their places: %v", err)
	}
	if _, err := sm.RegisterPeer("d", from); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Peer(lapsed.ID); !errors.Is(err, ErrTooManyPeers) {
		t.Fatalf("reviving a lapsed code past the cap: got %v, want ErrTooManyPeers", err)
	}
	if err := sm.ReleasePeer(c.ID); err != nil {
		t.Fatal(err)
	}
	p, err := sm.Peer(lapsed.ID)
	if err != nil {
		t.Fatalf("reviving with room: %v", err)
	}
	if p.Code == lapsed.Code {
		t.Fatal("the lapsed code was shown again instead of a fresh one")
	}
}

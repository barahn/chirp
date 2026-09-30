// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// These tests live in package session rather than session_test because the
// defects they pin down are in unexported locking: the reaper is driven
// directly so a 10-second ticker does not have to elapse.

package session

import (
	"sync"
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

	sess, err := sm.CreateSession("tech-reap", "Reaper", 30*time.Millisecond)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
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
		if _, err := sm.CreateSession("tech", "Tech", time.Minute); err != nil {
			t.Fatalf("CreateSession failed: %v", err)
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
					if _, err := sm.CreateSession("tech", "Tech", time.Minute); err != nil {
						t.Errorf("CreateSession failed: %v", err)
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

	sess, err := sm.CreateSession("tech-sub", "Subscriber", 30*time.Millisecond)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	ch, unsub := sm.Subscribe(sess.Code)
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

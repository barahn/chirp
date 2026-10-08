// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

const origin = "192.0.2.10"

var owners atomic.Int64

// ownerOrigin gives each registration its own origin, so the per-origin peer
// cap is not what a test measures unless it says so.
func ownerOrigin() string {
	return fmt.Sprint("owner ", owners.Add(1))
}

func info() *session.ClientInfo { return &session.ClientInfo{RemoteIP: origin} }

// open registers an owner and redeems its start code.
func open(t *testing.T, sm *session.SessionManager) (*session.Peer, *session.Session) {
	t.Helper()
	p, err := sm.RegisterPeer("Owner", ownerOrigin())
	if err != nil {
		t.Fatalf("RegisterPeer: %v", err)
	}
	sess, err := sm.Redeem(string(p.Code), "Controller", info())
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	return p, sess
}

// askConsent moves a fresh session to consent_requested.
func askConsent(t *testing.T, sm *session.SessionManager, id string) {
	t.Helper()
	if _, err := sm.Connect(id); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if _, err := sm.RequestConsent(id); err != nil {
		t.Fatalf("RequestConsent: %v", err)
	}
}

func TestRegisterPeerShowsACodeAndALink(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, err := sm.RegisterPeer("Ana", ownerOrigin())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Code) != 6 || p.Link == "" || p.ID == "" {
		t.Fatalf("incomplete peer: %+v", p)
	}
	if f := p.Code.Format(); len(f) != 7 || f[3] != '-' {
		t.Fatalf("formatted code %q, want XXX-XXX", f)
	}
}

// The redeemer controls the code's owner: the session belongs to the peer
// whose code it was, and the controller's name is carried as given.
func TestRedeemOpensASessionWithTheOwner(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, err := sm.RegisterPeer("Ana", ownerOrigin())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sm.Redeem(p.Code.Format(), "Bruno", info())
	if err != nil {
		t.Fatalf("Redeem of the formatted code: %v", err)
	}
	if sess.Owner != p.ID || sess.OwnerName != "Ana" || sess.ControllerName != "Bruno" {
		t.Fatalf("session does not pair Bruno with Ana: %+v", sess)
	}
	if sess.State != session.StatePending {
		t.Fatalf("state %s, want pending", sess.State)
	}
	active, err := sm.ActiveSession(p.ID)
	if err != nil || active.ID != sess.ID {
		t.Fatalf("ActiveSession = %v, %v", active, err)
	}
}

// The start code is single-use and replaced once used (product decision 2).
func TestStartCodeIsSingleUseAndReplaced(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, sess := open(t, sm)
	if _, err := sm.Redeem(string(p.Code), "Again", info()); !errors.Is(err, session.ErrCodeNotFound) {
		t.Fatalf("second redemption: got %v, want ErrCodeNotFound", err)
	}
	now, err := sm.Peer(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if now.Code == p.Code {
		t.Fatal("the used start code is still the one shown")
	}
	// The owner is busy, so the new code is spent but opens nothing.
	if _, err := sm.Redeem(string(now.Code), "Third", info()); !errors.Is(err, session.ErrPeerBusy) {
		t.Fatalf("redeeming while busy: got %v, want ErrPeerBusy", err)
	}
	if _, err := sm.EndSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := sm.Peer(p.ID)
	if _, err := sm.Redeem(string(fresh.Code), "Fourth", info()); err != nil {
		t.Fatalf("redeeming after the session ended: %v", err)
	}
}

// Each use of a link mints a different single-use code.
func TestLinkMintsAFreshCodePerUse(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Ana", ownerOrigin())
	a, err := sm.MintFromLink(p.Link, origin)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sm.MintFromLink(p.Link, origin)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || a == p.Code || b == p.Code {
		t.Fatalf("codes not distinct: start %s, minted %s and %s", p.Code, a, b)
	}
	sess, err := sm.Redeem(string(a), "Bruno", info())
	if err != nil || sess.Owner != p.ID {
		t.Fatalf("minted code did not reach the owner: %v", err)
	}
	// Minting from a link does not spend the owner's start code.
	if cur, _ := sm.Peer(p.ID); cur.Code != p.Code {
		t.Fatal("a minted code replaced the start code")
	}
}

// Releasing a peer -- its Chirp exiting -- kills its code and link and ends its
// session.
func TestReleasePeerRevokesEverything(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, sess := open(t, sm)
	cur, _ := sm.Peer(p.ID)
	if err := sm.ReleasePeer(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Redeem(string(cur.Code), "X", info()); !errors.Is(err, session.ErrCodeNotFound) {
		t.Fatalf("code after release: %v", err)
	}
	if _, err := sm.MintFromLink(p.Link, origin); !errors.Is(err, session.ErrCodeNotFound) {
		t.Fatalf("link after release: %v", err)
	}
	if got, _ := sm.GetSession(sess.ID); got.State != session.StateTerminated {
		t.Fatalf("session after release: %s", got.State)
	}
	if _, err := sm.Peer(p.ID); !errors.Is(err, session.ErrPeerNotFound) {
		t.Fatalf("peer after release: %v", err)
	}
}

// Chirp's values are what the registry enforces: five failures per origin, and
// the sixth attempt is refused even for a live code. Link minting draws on the
// same budget.
func TestRedemptionIsThrottledWithChirpsValues(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Ana", ownerOrigin())
	bad := &session.ClientInfo{RemoteIP: "198.51.100.7"}
	for i := 0; i < session.DefaultMaxRedemptionFailures-1; i++ {
		if _, err := sm.Redeem("000000", "", bad); !errors.Is(err, session.ErrCodeNotFound) &&
			!errors.Is(err, session.ErrMalformed) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := sm.MintFromLink("not-a-link", bad.RemoteIP); !errors.Is(err, session.ErrCodeNotFound) {
		t.Fatalf("bad link: %v", err)
	}
	if _, err := sm.Redeem(string(p.Code), "", bad); !errors.Is(err, session.ErrThrottled) {
		t.Fatalf("live code after the budget: got %v, want ErrThrottled", err)
	}
	// Another origin is unaffected.
	if _, err := sm.Redeem(string(p.Code), "", info()); err != nil {
		t.Fatalf("other origin: %v", err)
	}
}

func TestSetRedemptionLimit(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	if err := sm.SetRedemptionLimit(0, time.Minute); err == nil {
		t.Fatal("a zero limit was accepted")
	}
	if err := sm.SetRedemptionLimit(1, time.Minute); err != nil {
		t.Fatal(err)
	}
	_, _ = sm.Redeem("111111", "", info())
	if _, err := sm.Redeem("111111", "", info()); !errors.Is(err, session.ErrThrottled) {
		t.Fatalf("got %v, want ErrThrottled after one failure", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, sess := open(t, sm)
	askConsent(t, sm, sess.ID)

	granted, err := sm.SubmitConsent(sess.ID, true, "")
	if err != nil || granted.State != session.StateConsentGranted || !granted.Consent.Granted {
		t.Fatalf("SubmitConsent: %+v, %v", granted, err)
	}
	active, err := sm.ActivateSession(sess.ID)
	if err != nil || active.State != session.StateActive {
		t.Fatalf("ActivateSession: %v", err)
	}
	if n := len(sm.ListActiveSessions()); n != 1 {
		t.Fatalf("%d active sessions, want 1", n)
	}
	ended, err := sm.EndSession(sess.ID)
	if err != nil || ended.State != session.StateTerminated {
		t.Fatalf("EndSession: %v", err)
	}
	if n := len(sm.ListActiveSessions()); n != 0 {
		t.Fatalf("%d active sessions after ending, want 0", n)
	}
	if _, err := sm.ActiveSession(p.ID); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("owner still busy after the session ended: %v", err)
	}
}

func TestConsentDeniedEndsTheSessionAndFreesTheOwner(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, sess := open(t, sm)
	askConsent(t, sm, sess.ID)
	denied, err := sm.SubmitConsent(sess.ID, false, "not expecting anyone")
	if err != nil || denied.State != session.StateConsentDenied || denied.Consent.Granted {
		t.Fatalf("deny: %+v, %v", denied, err)
	}
	if _, err := sm.ActivateSession(sess.ID); err == nil {
		t.Fatal("a denied session was activated")
	}
	if _, err := sm.ActiveSession(p.ID); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("owner still busy after denying: %v", err)
	}
}

func TestWaitForConsent(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	_, sess := open(t, sm)
	askConsent(t, sm, sess.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = sm.SubmitConsent(sess.ID, true, "")
	}()
	granted, err := sm.WaitForConsent(ctx, sess.ID)
	if err != nil || !granted {
		t.Fatalf("WaitForConsent = %v, %v", granted, err)
	}
}

// Every waiter must see the decision, not only the first one woken.
func TestWaitForConsentWakesEveryWaiter(t *testing.T) {
	for _, grant := range []bool{true, false} {
		t.Run(fmt.Sprintf("granted=%v", grant), func(t *testing.T) {
			sm := session.NewSessionManager(10 * time.Minute)
			defer sm.Close()

			_, sess := open(t, sm)
			askConsent(t, sm, sess.ID)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			const waiters = 3
			type result struct {
				granted bool
				err     error
			}
			results := make(chan result, waiters)
			for range waiters {
				go func() {
					granted, err := sm.WaitForConsent(ctx, sess.ID)
					results <- result{granted, err}
				}()
			}
			time.Sleep(50 * time.Millisecond)
			if _, err := sm.SubmitConsent(sess.ID, grant, ""); err != nil {
				t.Fatalf("SubmitConsent: %v", err)
			}
			for range waiters {
				r := <-results
				if r.err != nil || r.granted != grant {
					t.Fatalf("WaitForConsent = %v, %v; want %v", r.granted, r.err, grant)
				}
			}
		})
	}
}

type waitResult struct {
	granted bool
	err     error
}

// waitAsync starts a WaitForConsent and returns where its result arrives.
func waitAsync(ctx context.Context, sm *session.SessionManager, id string) <-chan waitResult {
	ch := make(chan waitResult, 1)
	go func() {
		granted, err := sm.WaitForConsent(ctx, id)
		ch <- waitResult{granted, err}
	}()
	return ch
}

// A session that ends without a decision must release its waiters with the
// reason, not leave them blocked until their context gives up.
func TestWaitForConsentReturnsWhenSessionEndsUndecided(t *testing.T) {
	cases := []struct {
		name string
		ttl  time.Duration
		end  func(sm *session.SessionManager, p *session.Peer, id string)
		want error
	}{
		{"EndSession", 10 * time.Minute, func(sm *session.SessionManager, _ *session.Peer, id string) {
			if _, err := sm.EndSession(id); err != nil {
				t.Errorf("EndSession: %v", err)
			}
		}, session.ErrSessionEnded},
		{"ReleasePeer", 10 * time.Minute, func(sm *session.SessionManager, p *session.Peer, _ string) {
			if err := sm.ReleasePeer(p.ID); err != nil {
				t.Errorf("ReleasePeer: %v", err)
			}
		}, session.ErrSessionEnded},
		{"expiry on the next transition", 100 * time.Millisecond, func(sm *session.SessionManager, _ *session.Peer, id string) {
			time.Sleep(150 * time.Millisecond)
			if _, err := sm.SubmitConsent(id, true, ""); !errors.Is(err, session.ErrSessionExpired) {
				t.Errorf("SubmitConsent after the deadline: %v", err)
			}
		}, session.ErrSessionExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := session.NewSessionManager(tc.ttl)
			defer sm.Close()

			p, sess := open(t, sm)
			askConsent(t, sm, sess.ID)

			// No deadline: before the fix, this waiter never returned.
			res := waitAsync(context.Background(), sm, sess.ID)
			time.Sleep(20 * time.Millisecond)
			tc.end(sm, p, sess.ID)

			select {
			case r := <-res:
				if r.granted || !errors.Is(r.err, tc.want) {
					t.Fatalf("WaitForConsent = %v, %v; want false, %v", r.granted, r.err, tc.want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("WaitForConsent still blocked after the session ended")
			}

			// A caller arriving afterwards gets the same answer at once, even
			// with a context that has already ended.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if granted, err := sm.WaitForConsent(ctx, sess.ID); granted || !errors.Is(err, tc.want) {
				t.Fatalf("late WaitForConsent = %v, %v; want false, %v", granted, err, tc.want)
			}
		})
	}
}

// Ending a session after consent was granted does not rewrite the decision.
func TestWaitForConsentKeepsADecisionAfterTheSessionEnds(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	_, sess := open(t, sm)
	askConsent(t, sm, sess.ID)
	if _, err := sm.SubmitConsent(sess.ID, true, ""); err != nil {
		t.Fatalf("SubmitConsent: %v", err)
	}
	if _, err := sm.EndSession(sess.ID); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if granted, err := sm.WaitForConsent(context.Background(), sess.ID); !granted || err != nil {
		t.Fatalf("WaitForConsent = %v, %v; want true, nil", granted, err)
	}
}

func TestUndecidedSessionExpires(t *testing.T) {
	sm := session.NewSessionManager(50 * time.Millisecond)
	defer sm.Close()

	_, sess := open(t, sm)
	ch, unsub := sm.Subscribe(sess.ID)
	defer unsub()
	if s := <-ch; s.State != session.StatePending {
		t.Fatalf("first event %s, want %s", s.State, session.StatePending)
	}

	time.Sleep(80 * time.Millisecond)
	if _, err := sm.Connect(sess.ID); !errors.Is(err, session.ErrSessionExpired) {
		t.Fatalf("Connect on an expired session: %v", err)
	}

	// Expiry found by a transition must leave the session exactly as the
	// reaper would: ended at its deadline, and announced to subscribers.
	select {
	case s := <-ch:
		if s.State != session.StateExpired {
			t.Fatalf("event %s, want %s", s.State, session.StateExpired)
		}
	case <-time.After(time.Second):
		t.Fatal("no expired event after lazy expiry")
	}
	got, err := sm.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.State != session.StateExpired {
		t.Fatalf("state %s, want %s", got.State, session.StateExpired)
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(got.ExpiresAt) {
		t.Fatalf("EndedAt = %v, want the deadline %v", got.EndedAt, got.ExpiresAt)
	}
}

func TestSubscriptionEvents(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	_, sess := open(t, sm)
	ch, unsub := sm.Subscribe(sess.ID)
	defer unsub()

	for _, want := range []session.SessionState{session.StatePending, session.StateConnected} {
		if want == session.StateConnected {
			go func() { _, _ = sm.Connect(sess.ID) }()
		}
		select {
		case s := <-ch:
			if s.State != want {
				t.Fatalf("got %s, want %s", s.State, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("no %s event", want)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := sm.RegisterPeer(fmt.Sprint("peer ", i), ownerOrigin())
			if err != nil {
				t.Errorf("RegisterPeer: %v", err)
				return
			}
			sess, err := sm.Redeem(string(p.Code), "c", &session.ClientInfo{RemoteIP: fmt.Sprint("origin ", i)})
			if err != nil {
				t.Errorf("Redeem: %v", err)
				return
			}
			_, _ = sm.GetSession(sess.ID)
			_, _ = sm.Connect(sess.ID)
			_, _ = sm.Peer(p.ID)
			_, _ = sm.EndSession(sess.ID)
			_ = sm.ReleasePeer(p.ID)
		}(i)
	}
	wg.Wait()
}

// Consent answers a request. Recording it earlier lets a decision exist before
// any prompt was shown on the owner's machine.
func TestConsentRequiresARequest(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	_, pending := open(t, sm)
	if _, err := sm.SubmitConsent(pending.ID, true, ""); !errors.Is(err, session.ErrInvalidState) {
		t.Fatalf("consent on a pending session: %v", err)
	}
	_, connected := open(t, sm)
	if _, err := sm.Connect(connected.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.SubmitConsent(connected.ID, true, ""); !errors.Is(err, session.ErrInvalidState) {
		t.Fatalf("consent on a connected session never asked: %v", err)
	}
	for _, id := range []string{pending.ID, connected.ID} {
		s, _ := sm.GetSession(id)
		if s.Consent != nil || s.State == session.StateConsentGranted {
			t.Fatalf("a refused consent was recorded: state %s", s.State)
		}
	}
}

// One origin may hold at most DefaultMaxSessionsPerOrigin open sessions; a
// refused redemption leaves the owner's code usable, and ending a session makes
// room again.
// One origin may hold at most DefaultMaxPeersPerOrigin live codes: every
// registration holds one, and without a cap one origin could fill the space.
func TestPeersPerOriginAreCapped(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	const from = "198.51.100.7"
	var first *session.Peer
	for i := 0; i < session.DefaultMaxPeersPerOrigin; i++ {
		p, err := sm.RegisterPeer("Owner", from)
		if err != nil {
			t.Fatalf("registration %d: %v", i+1, err)
		}
		if first == nil {
			first = p
		}
	}
	if _, err := sm.RegisterPeer("One too many", from); !errors.Is(err, session.ErrTooManyPeers) {
		t.Fatalf("registration %d: got %v, want ErrTooManyPeers", session.DefaultMaxPeersPerOrigin+1, err)
	}
	if _, err := sm.RegisterPeer("Elsewhere", "198.51.100.8"); err != nil {
		t.Fatalf("another origin was capped: %v", err)
	}
	if err := sm.ReleasePeer(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.RegisterPeer("After a release", from); err != nil {
		t.Fatalf("releasing a peer did not free its place: %v", err)
	}
}

// The check and the allocation are one step: concurrent registrations from
// one origin cannot all pass the check before any of them is counted.
func TestPeerCapHoldsUnderConcurrency(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	const from = "198.51.100.9"
	var ok atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 2*session.DefaultMaxPeersPerOrigin; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := sm.RegisterPeer("Owner", from)
			switch {
			case err == nil:
				ok.Add(1)
			case !errors.Is(err, session.ErrTooManyPeers):
				t.Errorf("RegisterPeer: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := ok.Load(); got != session.DefaultMaxPeersPerOrigin {
		t.Fatalf("%d registrations succeeded, want exactly %d", got, session.DefaultMaxPeersPerOrigin)
	}
}

func TestSetMaxPeersPerOriginRejectsNonPositive(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()
	for _, n := range []int{0, -1} {
		if err := sm.SetMaxPeersPerOrigin(n); err == nil {
			t.Fatalf("SetMaxPeersPerOrigin(%d) accepted", n)
		}
	}
}

func TestSessionsPerOriginAreCapped(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	var first *session.Session
	for i := 0; i < session.DefaultMaxSessionsPerOrigin; i++ {
		_, s := open(t, sm)
		if first == nil {
			first = s
		}
	}
	p, _ := sm.RegisterPeer("One too many", ownerOrigin())
	if _, err := sm.Redeem(string(p.Code), "c", info()); !errors.Is(err, session.ErrTooManySessions) {
		t.Fatalf("session %d: got %v, want ErrTooManySessions", session.DefaultMaxSessionsPerOrigin+1, err)
	}
	if _, err := sm.Redeem(string(p.Code), "c", &session.ClientInfo{RemoteIP: "203.0.113.9"}); err != nil {
		t.Fatalf("the refused code was burnt, or another origin was capped: %v", err)
	}
	if _, err := sm.EndSession(first.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = open(t, sm) // fails the test if there is still no room
}

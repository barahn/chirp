// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session_test

import (
	"errors"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

// An IPv6 client is handed a whole /64, so each address in it is not a
// separate caller: two addresses in one /64 are one origin.
const (
	sameNetA = "2001:db8:1:2::1"
	sameNetB = "2001:db8:1:2:ffff:ffff:ffff:fffe"
	otherNet = "2001:db8:1:3::1"
	v4       = "198.51.100.7"
	v4Next   = "198.51.100.8"
	v4Mapped = "::ffff:198.51.100.7"
)

func spendRedemptionBudget(t *testing.T, sm *session.SessionManager, from string) {
	t.Helper()
	for i := 0; i < session.DefaultMaxRedemptionFailures; i++ {
		_, _ = sm.Redeem("000000", "", &session.ClientInfo{RemoteIP: from})
	}
}

func TestRedemptionBudgetIsPerOrigin(t *testing.T) {
	cases := []struct {
		name, spent, then string
		shared            bool
	}{
		{"one IPv6 /64", sameNetA, sameNetB, true},
		{"two IPv6 /64s", sameNetA, otherNet, false},
		{"two IPv4 addresses", v4, v4Next, false},
		{"an IPv4-mapped address and its IPv4 address", v4Mapped, v4, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sm := session.NewSessionManager(10 * time.Minute)
			defer sm.Close()

			p, _ := sm.RegisterPeer("Owner", ownerOrigin())
			spendRedemptionBudget(t, sm, c.spent)
			_, err := sm.Redeem(string(p.Code), "", &session.ClientInfo{RemoteIP: c.then})
			if throttled := errors.Is(err, session.ErrThrottled); throttled != c.shared {
				t.Fatalf("redeem from %s after %s spent its budget: %v, shared budget = %v",
					c.then, c.spent, err, c.shared)
			}
		})
	}
}

func TestLinksAndLookupsShareTheIPv6PrefixBudget(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Owner", ownerOrigin())
	if err := sm.SetContact(p.ID, contact()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < session.DefaultMaxRedemptionFailures; i++ {
		_, _ = sm.MintFromLink("not-a-link", sameNetA)
	}
	if _, err := sm.Lookup(string(p.Code), sameNetB); !errors.Is(err, session.ErrThrottled) {
		t.Fatalf("lookup from the same /64 after failed links: %v, want ErrThrottled", err)
	}
	if _, err := sm.LookupLink(p.Link, sameNetB); !errors.Is(err, session.ErrThrottled) {
		t.Fatalf("lookup link from the same /64 after failed links: %v, want ErrThrottled", err)
	}
	if _, err := sm.Lookup(string(p.Code), otherNet); err != nil {
		t.Fatalf("lookup from another /64: %v", err)
	}
}

func TestPeerCapCountsAnIPv6PrefixAsOneOrigin(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	for i := 0; i < session.DefaultMaxPeersPerOrigin; i++ {
		if _, err := sm.RegisterPeer("Owner", sameNetA); err != nil {
			t.Fatalf("registration %d: %v", i+1, err)
		}
	}
	if _, err := sm.RegisterPeer("Same /64", sameNetB); !errors.Is(err, session.ErrTooManyPeers) {
		t.Fatalf("registration from the same /64: got %v, want ErrTooManyPeers", err)
	}
	if _, err := sm.RegisterPeer("Other /64", otherNet); err != nil {
		t.Fatalf("another /64 was capped: %v", err)
	}
}

func TestSessionCapCountsAnIPv6PrefixAsOneOrigin(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	redeem := func(from string) (*session.Session, error) {
		p, err := sm.RegisterPeer("Owner", ownerOrigin())
		if err != nil {
			t.Fatal(err)
		}
		return sm.Redeem(string(p.Code), "c", &session.ClientInfo{RemoteIP: from})
	}
	for i := 0; i < session.DefaultMaxSessionsPerOrigin; i++ {
		if _, err := redeem(sameNetA); err != nil {
			t.Fatalf("session %d: %v", i+1, err)
		}
	}
	if _, err := redeem(sameNetB); !errors.Is(err, session.ErrTooManySessions) {
		t.Fatalf("session from the same /64: got %v, want ErrTooManySessions", err)
	}
	if _, err := redeem(otherNet); err != nil {
		t.Fatalf("another /64 was capped: %v", err)
	}
}

func TestSessionKeepsTheFullRemoteIP(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Owner", ownerOrigin())
	sess, err := sm.Redeem(string(p.Code), "c", &session.ClientInfo{RemoteIP: sameNetB})
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.ClientInfo.RemoteIP; got != sameNetB {
		t.Fatalf("RemoteIP = %q, want the full address %q", got, sameNetB)
	}
}

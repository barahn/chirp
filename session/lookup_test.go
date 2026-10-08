// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

// key is shaped like a Nostr public key: 64 lowercase hex digits.
var key = strings.Repeat("7e", 32)

func contact() session.Contact {
	return session.Contact{Key: key, Relays: []string{"wss://relay.example.org", "ws://192.0.2.7:7777/nostr"}}
}

func TestValidateContact(t *testing.T) {
	many := make([]string, session.MaxContactRelays+1)
	for i := range many {
		many[i] = "wss://relay.example.org"
	}
	bad := map[string]session.Contact{
		"short key":       {Key: key[:62], Relays: []string{"wss://r.example"}},
		"non-hex key":     {Key: strings.Repeat("g", 64), Relays: []string{"wss://r.example"}},
		"uppercase key":   {Key: strings.ToUpper(key), Relays: []string{"wss://r.example"}},
		"no relays":       {Key: key},
		"too many relays": {Key: key, Relays: many},
		"https relay":     {Key: key, Relays: []string{"https://r.example"}},
		"no host":         {Key: key, Relays: []string{"wss://"}},
		"credentials":     {Key: key, Relays: []string{"wss://user:pass@r.example"}},
		"long relay":      {Key: key, Relays: []string{"wss://" + strings.Repeat("a", session.MaxContactRelayLen)}},
	}
	for name, c := range bad {
		if err := session.ValidateContact(c); !errors.Is(err, session.ErrBadContact) {
			t.Errorf("%s: ValidateContact = %v, want ErrBadContact", name, err)
		}
	}
	if err := session.ValidateContact(contact()); err != nil {
		t.Fatalf("a good contact was refused: %v", err)
	}
}

// A lookup hands back the contact and nothing else: the code is spent as a
// redemption spends it, but no session is opened and the owner is not busy.
func TestLookupConsumesTheCodeAndOpensNoSession(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Owner", ownerOrigin())
	if err := sm.SetContact(p.ID, contact()); err != nil {
		t.Fatal(err)
	}
	got, err := sm.Lookup(string(p.Code), origin)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Key != key || len(got.Relays) != 2 || got.Relays[0] != "wss://relay.example.org" {
		t.Fatalf("Lookup = %+v", got)
	}
	got.Relays[0] = "wss://evil.example" // the caller's copy, not the stored one

	if _, err := sm.Lookup(string(p.Code), origin); !errors.Is(err, session.ErrCodeNotFound) {
		t.Fatalf("second lookup of a single-use code: %v, want ErrCodeNotFound", err)
	}
	now, err := sm.Peer(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if now.Code == p.Code {
		t.Fatal("the start code was not replaced after it was used")
	}
	if _, err := sm.ActiveSession(p.ID); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("a lookup opened a session: %v", err)
	}
	again, err := sm.Lookup(string(now.Code), origin)
	if err != nil {
		t.Fatal(err)
	}
	if again.Relays[0] != "wss://relay.example.org" {
		t.Fatal("a caller changed the stored contact through its copy")
	}
}

func TestLookupLinkMintsAFreshCodePerUse(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Owner", ownerOrigin())
	if err := sm.SetContact(p.ID, contact()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := sm.LookupLink(p.Link, origin); err != nil {
			t.Fatalf("use %d of the link: %v", i+1, err)
		}
	}
	if err := sm.ReleasePeer(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.LookupLink(p.Link, origin); err == nil {
		t.Fatal("a released peer's link still looks up")
	}
}

func TestLookupWithoutAContact(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Owner", ownerOrigin())
	if _, err := sm.Lookup(string(p.Code), origin); !errors.Is(err, session.ErrNoContact) {
		t.Fatalf("Lookup = %v, want ErrNoContact", err)
	}
	if err := sm.SetContact("no such peer", contact()); !errors.Is(err, session.ErrPeerNotFound) {
		t.Fatalf("SetContact on an unknown peer = %v, want ErrPeerNotFound", err)
	}
	if err := sm.SetContact(p.ID, session.Contact{Key: "x"}); !errors.Is(err, session.ErrBadContact) {
		t.Fatalf("SetContact with a bad contact = %v, want ErrBadContact", err)
	}
}

// Lookups and redemptions draw on one budget per origin: the lookup must not
// be a second, cheaper way to sweep the code space.
func TestLookupSharesTheRedemptionBudget(t *testing.T) {
	sm := session.NewSessionManager(10 * time.Minute)
	defer sm.Close()

	p, _ := sm.RegisterPeer("Owner", ownerOrigin())
	if err := sm.SetContact(p.ID, contact()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < session.DefaultMaxRedemptionFailures; i++ {
		_, _ = sm.Lookup("000000", origin)
	}
	if _, err := sm.Lookup(string(p.Code), origin); !errors.Is(err, session.ErrThrottled) {
		t.Fatalf("lookup of a live code after the budget: %v, want ErrThrottled", err)
	}
	if _, err := sm.Redeem(string(p.Code), "c", info()); !errors.Is(err, session.ErrThrottled) {
		t.Fatalf("redeem after failed lookups: %v, want ErrThrottled", err)
	}
}

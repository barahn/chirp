// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session

import (
	"encoding/hex"
	"errors"
	"net/url"

	"github.com/barahn/remotekit/rendezvous"
)

// The lookup is the blind server's half of decision 5 (docs/product-model.md):
// an owner registers how it can be reached over Nostr -- its session key and
// the relays it listens on -- and a controller who has the owner's code gets
// that back, and nothing else. No session is opened, no consent is asked
// here, and no signalling passes through: the two Chirps do all of that over
// Nostr, end to end. What the lookup keeps is everything that bounds abuse:
// codes stay single-use, and every redemption is metered per origin from the
// same budget as Redeem.

// Contact is how a peer can be reached over Nostr relays.
type Contact struct {
	// Key is the peer's per-session Nostr public key: 32 bytes, as the 64
	// lowercase hex characters Nostr uses.
	Key string `json:"key"`
	// Relays are the relays the peer listens on, as ws:// or wss:// URLs.
	Relays []string `json:"relays"`
}

// Bounds on a Contact, so that registering one cannot store more than a
// controller needs to dial.
const (
	MaxContactRelays   = 8
	MaxContactRelayLen = 256
)

var (
	// ErrBadContact reports a Contact that is not a key and a short list of
	// relay URLs.
	ErrBadContact = errors.New("chirp: contact must be a 64-hex-digit key and 1 to 8 ws:// or wss:// relays")
	// ErrNoContact reports that the owner of a valid code registered no
	// contact, so it cannot be reached over Nostr.
	ErrNoContact = errors.New("chirp: that peer has no contact")
)

// ValidateContact reports whether c can be stored and handed out. The lookup
// never connects to the relays it stores, but whatever it hands a controller
// is dialled by the controller's Chirp, so only well-formed websocket URLs
// pass.
func ValidateContact(c Contact) error {
	if len(c.Key) != 64 {
		return ErrBadContact
	}
	if _, err := hex.DecodeString(c.Key); err != nil {
		return ErrBadContact
	}
	for _, ch := range c.Key {
		if ch >= 'A' && ch <= 'F' {
			return ErrBadContact // Nostr keys are lowercase; one spelling per key
		}
	}
	if len(c.Relays) == 0 || len(c.Relays) > MaxContactRelays {
		return ErrBadContact
	}
	for _, r := range c.Relays {
		if len(r) > MaxContactRelayLen {
			return ErrBadContact
		}
		u, err := url.Parse(r)
		if err != nil || (u.Scheme != "wss" && u.Scheme != "ws") || u.Host == "" || u.User != nil {
			return ErrBadContact
		}
	}
	return nil
}

// SetContact records, or replaces, how a peer can be reached over Nostr. A
// peer replaces it when it makes a new session key.
func (sm *SessionManager) SetContact(id rendezvous.PeerID, c Contact) error {
	if err := ValidateContact(c); err != nil {
		return err
	}
	cp := Contact{Key: c.Key, Relays: append([]string(nil), c.Relays...)}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	pe, ok := sm.peers[id]
	if !ok {
		return ErrPeerNotFound
	}
	pe.contact = &cp
	return nil
}

// Lookup redeems a code for its owner's Contact. The code is consumed exactly
// as Redeem consumes it -- a start code is replaced, a minted one is gone --
// and failures count against origin from the same budget, so the lookup is no
// cheaper an oracle than Redeem. It opens no session and does not mark the
// owner busy: whatever follows happens between the two Chirps.
//
// An owner that registered no contact still loses the code it was reached
// by, since which owner a code belongs to is known only once it is redeemed.
func (sm *SessionManager) Lookup(code, origin string) (*Contact, error) {
	m, err := sm.reg.Redeem(code, origin)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	pe, ok := sm.peers[m.Owner]
	if !ok {
		return nil, ErrCodeNotFound
	}
	if pe.peer.Code == m.Code {
		if err := sm.renewCodeLocked(pe); err != nil {
			return nil, err
		}
	}
	if pe.contact == nil {
		return nil, ErrNoContact
	}
	cp := Contact{Key: pe.contact.Key, Relays: append([]string(nil), pe.contact.Relays...)}
	return &cp, nil
}

// LookupLink mints a single-use code from a link and looks it up in one step,
// metered against origin like MintFromLink.
func (sm *SessionManager) LookupLink(t rendezvous.Token, origin string) (*Contact, error) {
	code, err := sm.reg.MintFromLink(t, origin, sm.codeTTL)
	if err != nil {
		return nil, err
	}
	return sm.Lookup(string(code), origin)
}

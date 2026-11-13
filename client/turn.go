// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// TURNCredential is one end's short-lived credential for the relay's TURN
// server (docs/stun-turn.md, Credentials). It can open an allocation for a
// few minutes; an allocation opened with it lasts the whole session.
type TURNCredential struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
	ExpiresIn  int      `json:"expires_in"`
}

// TypeTURN is the message an owner sends its controller over the relay's
// sockets, before the session's first signal, with the controller's TURN
// credential, or none when the relay has no TURN. It is Chirp's, not
// remotekit's: the controller's side keeps it from the viewer. Over Nostr the
// credential rides on the consent answer instead.
const TypeTURN = "chirp_turn"

type turnWord struct {
	Type string          `json:"type"`
	TURN *TURNCredential `json:"turn,omitempty"`
}

// defaultTURNWait is how long a controller let in through the relay waits for
// its owner's TypeTURN before it starts viewing without TURN. Only an owner
// from before TURN never sends one.
const defaultTURNWait = 3 * time.Second

func (c *Client) waitForTURN() time.Duration {
	if c.turnWait > 0 {
		return c.turnWait
	}
	return defaultTURNWait
}

// TURNCredentials asks the relay, as the owner peerID, for two credentials:
// its own, and the one it hands the controller it has let in. A relay without
// TURN answers ErrNotFound.
func (c *Client) TURNCredentials(ctx context.Context, peerID string) (owner, controller TURNCredential, err error) {
	var creds struct {
		Owner      TURNCredential `json:"owner"`
		Controller TURNCredential `json:"controller"`
	}
	err = c.do(ctx, http.MethodPost, c.endpoint("api", "peers", peerID, "turn"), nil, &creds)
	return creds.Owner, creds.Controller, err
}

// sessionTURN is what the owner got for the session it allowed last: both
// credentials, or neither when the relay gave none.
type sessionTURN struct {
	sessionID  string
	owner      *TURNCredential
	controller *TURNCredential
}

// fetchTURN gets the credentials for a session the owner is allowing. A relay
// without TURN, or one that does not answer in time, leaves the session on
// STUN rather than hold up the answer.
func (o *Owner) fetchTURN(ctx context.Context, peerID, sessionID string) {
	got := &sessionTURN{sessionID: sessionID}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if owner, controller, err := o.client.TURNCredentials(ctx, peerID); err == nil {
		got.owner, got.controller = &owner, &controller
	}
	o.mu.Lock()
	o.turn = got
	o.mu.Unlock()
}

// turnFor returns the credentials fetched for sessionID, if those were the
// last fetched.
func (o *Owner) turnFor(sessionID string) (owner, controller *TURNCredential) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.turnForLocked(sessionID)
}

func (o *Owner) turnForLocked(sessionID string) (owner, controller *TURNCredential) {
	if o.turn == nil || o.turn.sessionID != sessionID {
		return nil, nil
	}
	return o.turn.owner, o.turn.controller
}

// readTURNWord reports whether raw is an owner's TypeTURN, and the credential
// it carries, if any.
func readTURNWord(raw []byte) (*TURNCredential, bool) {
	if !bytes.Contains(raw, []byte(TypeTURN)) {
		return nil, false
	}
	var w turnWord
	if json.Unmarshal(raw, &w) != nil || w.Type != TypeTURN {
		return nil, false
	}
	return w.TURN, true
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/barahn/remotekit/bark"
	"github.com/gorilla/websocket"
)

// The relay's own messages (signalling/relay.go). Everything else on a socket
// is a signal from the other end, which the relay forwards unread.
const (
	TypeRelayError     = "chirp.error"
	TypePeerJoined     = "chirp.peer_joined"
	TypePeerLeft       = "chirp.peer_left"
	TypeConsentRequest = "chirp.consent_request"
)

// Message is one message read off a socket. The relay's own messages are
// decoded; a signal from the other end is left in Raw for remotekit to read.
//
// Relay messages are hints, not authority: they come from the relay by
// definition, and nothing security-relevant may rest on them alone (HANDOFF
// §5.5).
type Message struct {
	Type    string
	Role    string          // which end joined or left
	Error   string          // what the relay refused
	Consent *ConsentRequest // what a controller is asking for
	Raw     []byte
}

// FromRelay reports whether the relay sent m itself.
func (m Message) FromRelay() bool { return strings.HasPrefix(m.Type, "chirp.") }

// ConsentRequest is a controller asking to be let in. Name is whatever the
// controller typed: a claim, never an identity, and NameVerified says so.
type ConsentRequest struct {
	SessionID    string
	Name         string
	Permissions  []string
	NameVerified bool
	// ExpiresIn is how many seconds the request has left before the relay
	// declines it for the owner. The request itself does not carry it, so
	// Owner asks the relay; zero means it could not.
	ExpiresIn int
}

// Socket is one end's WebSocket to the relay. Next is for one reader at a
// time; Close may be called from anywhere, and ends a Next in progress.
type Socket struct {
	conn *websocket.Conn
}

// DialOwner opens this machine's owner socket. Holding it open is how the
// relay knows the machine is still running (product-model decision 1), and it
// is where consent requests arrive.
func (c *Client) DialOwner(ctx context.Context, peerID string) (*Socket, error) {
	return c.dial(ctx, url.Values{"role": {"owner"}, "peer": {peerID}})
}

// DialController joins a session this machine opened by redeeming a code.
func (c *Client) DialController(ctx context.Context, sessionID string) (*Socket, error) {
	return c.dial(ctx, url.Values{"role": {"controller"}, "session": {sessionID}})
}

func (c *Client) dial(ctx context.Context, q url.Values) (*Socket, error) {
	u := c.base.JoinPath("ws")
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.RawQuery = q.Encode()
	conn, resp, err := c.dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			// The relay refuses a socket before upgrading it, with a status
			// code: an unknown peer, a session that is not admissible.
			return nil, relayError(resp)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	conn.SetReadLimit(1 << 20)
	return &Socket{conn: conn}, nil
}

// Next blocks until a message arrives, or the socket ends.
func (s *Socket) Next() (Message, error) {
	_, data, err := s.conn.ReadMessage()
	if err != nil {
		return Message{}, err
	}
	return decode(data), nil
}

// Close ends the socket.
func (s *Socket) Close() error { return s.conn.Close() }

// decode reads a relay message, and leaves anything else raw.
func decode(data []byte) Message {
	m := Message{Raw: data}
	var head struct {
		Type         string                      `json:"type"`
		Role         string                      `json:"role"`
		Error        string                      `json:"error"`
		Request      *bark.ConsentRequestPayload `json:"request"`
		NameVerified bool                        `json:"name_verified"`
	}
	if json.Unmarshal(data, &head) != nil {
		return m
	}
	m.Type, m.Role, m.Error = head.Type, head.Role, head.Error
	if head.Type == TypeConsentRequest && head.Request != nil {
		m.Consent = &ConsentRequest{
			SessionID:    head.Request.SessionID,
			Name:         head.Request.TechnicianName,
			Permissions:  head.Request.RequestedPermissions,
			NameVerified: head.NameVerified,
		}
	}
	return m
}

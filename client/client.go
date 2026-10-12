// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package client is the relay side of the one Chirp binary: registering this
// machine, following the code it shows, redeeming someone else's code or link,
// and the two sockets a session needs. It draws nothing; cmd/chirp puts a
// window on it.
//
// It speaks the protocol signalling.Server serves (HANDOFF §4.5). It imports
// neither signalling, which is the server, nor Fyne, so it builds without cgo
// for every platform Chirp ships on.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/barahn/remotekit/bark"
	"github.com/gorilla/websocket"
)

// Errors a caller can act on. Each is the relay's answer already mapped from
// its status code, so a window can say what happened without knowing HTTP.
var (
	// ErrNotFound: no such code, link, session or peer. The relay does not
	// say whether a code was wrong, used or expired, on purpose
	// (signalling.writeSessionError), and neither can this.
	ErrNotFound = errors.New("client: not found")
	// ErrBusy: the code's owner is already in a session, or the request does
	// not fit the state the session is in.
	ErrBusy = errors.New("client: busy")
	// ErrTooMany: this network has made too many attempts, or holds too many
	// sessions or registered Chirps.
	ErrTooMany = errors.New("client: too many attempts from this network")
	// ErrUnavailable: the relay could not do it now, or could not be reached.
	ErrUnavailable = errors.New("client: relay unavailable")
)

// RelayError is a refusal from the relay: its status code and its message.
// It unwraps to one of the errors above where the status maps to one.
type RelayError struct {
	Status  int
	Message string
}

func (e *RelayError) Error() string {
	return fmt.Sprintf("client: relay answered %d: %s", e.Status, e.Message)
}

func (e *RelayError) Unwrap() error {
	switch e.Status {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrBusy
	case http.StatusTooManyRequests:
		return ErrTooMany
	case http.StatusServiceUnavailable:
		return ErrUnavailable
	}
	return nil
}

// Peer is this machine as the relay knows it. ID is its credential: it goes
// to the relay and nowhere else, and is never shown.
type Peer struct {
	ID        string `json:"peer_id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Formatted string `json:"formatted_code"`
	Link      string `json:"link"`
}

// Session is what either end of a session may know about it.
type Session struct {
	ID        string `json:"session_id"`
	State     string `json:"state"`
	OwnerName string `json:"owner_name"`
	ExpiresIn int    `json:"expires_in_seconds"`
}

// Session states, spelled as session.SessionState spells them.
const (
	StatePending          = "pending"
	StateConnected        = "connected"
	StateConsentRequested = "consent_requested"
	StateConsentGranted   = "consent_granted"
	StateActive           = "active"
	StateTerminated       = "terminated"
	StateConsentDenied    = "consent_denied"
	StateExpired          = "expired"
)

// Client talks to one relay.
type Client struct {
	base   *url.URL
	http   *http.Client
	dialer *websocket.Dialer

	// pollEvery is how often Help asks the relay how a session stands: the
	// controller is not told on its socket when the owner decides.
	pollEvery time.Duration

	// View, when set, shows each session Help is let into until its
	// SessionConn ends; cmd/chirp runs remotekit's stream.Viewer in it. It
	// is called on a goroutine of its own, with Help's context, and the
	// SessionConn is closed when the session ends or Help returns. Set it
	// before the first Help.
	View func(ctx context.Context, conn *SessionConn)
}

// New returns a client for the relay at addr, an http or https address.
// Behind a TLS-terminating proxy that address is the proxy's https one.
func New(addr string) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(addr))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("client: %q is not an http or https address", addr)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return &Client{
		base: u,
		http: &http.Client{Timeout: 15 * time.Second},
		dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
			Proxy:            http.ProxyFromEnvironment,
		},
		pollEvery: time.Second,
	}, nil
}

// Register makes this machine reachable: the relay gives it a code, a link and
// the peer ID that is its credential from then on. name is optional; nobody
// registers, so the relay cannot check it either.
func (c *Client) Register(ctx context.Context, name string) (Peer, error) {
	var p Peer
	err := c.do(ctx, http.MethodPost, c.endpoint("api", "peers"), map[string]string{"name": name}, &p)
	return p, err
}

// Peer returns the code this machine should be showing now. The relay
// replaces a start code once it is used or expires, and says so only when
// asked.
func (c *Client) Peer(ctx context.Context, id string) (Peer, error) {
	var p Peer
	err := c.do(ctx, http.MethodGet, c.endpoint("api", "peers", id), nil, &p)
	return p, err
}

// Release makes this machine unreachable at once: its codes and link stop
// working, and a session in progress ends.
func (c *Client) Release(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, c.endpoint("api", "peers", id), nil, nil)
}

// Redeem spends someone else's code and opens a session to help them. name is
// what their consent prompt will show, as a claim.
func (c *Client) Redeem(ctx context.Context, code, name string) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodPost, c.endpoint("api", "redeem"), map[string]string{"code": code, "name": name}, &s)
	return s, err
}

// RedeemLink mints a single-use code from someone's link and redeems it, in
// one step.
func (c *Client) RedeemLink(ctx context.Context, token, name string) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodPost, c.endpoint("api", "links", token, "sessions"), map[string]string{"name": name}, &s)
	return s, err
}

// Consent answers a consent request, as the owner peerID. The relay applies
// it only to that peer's current session, only if sessionID is that session,
// and only while the session is asking; a declined session is over for good.
func (c *Client) Consent(ctx context.Context, peerID, sessionID string, granted bool, reason string) (Session, error) {
	var s Session
	body := bark.ConsentResponsePayload{
		SessionID: sessionID,
		Granted:   granted,
		Reason:    reason,
		Timestamp: time.Now().UTC(),
	}
	err := c.do(ctx, http.MethodPost, c.endpoint("api", "peers", peerID, "consent"), body, &s)
	return s, err
}

// Session returns how a session stands. Only its two ends know its ID.
func (c *Client) Session(ctx context.Context, id string) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodGet, c.endpoint("api", "sessions", id), nil, &s)
	return s, err
}

// endpoint joins path segments onto the relay's address, escaping each.
func (c *Client) endpoint(segments ...string) string {
	return c.base.JoinPath(segments...).String()
}

// do sends one request and decodes the answer into out, which may be nil.
func (c *Client) do(ctx context.Context, method, endpoint string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("client: encoding the request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("client: building the request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return relayError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(out); err != nil {
		return fmt.Errorf("client: reading the relay's answer: %w", err)
	}
	return nil
}

// relayError reads the relay's {"error": "..."} body into a RelayError.
func relayError(resp *http.Response) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
	return &RelayError{Status: resp.StatusCode, Message: e.Error}
}

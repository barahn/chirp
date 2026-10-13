// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"errors"
	"sync"
	"time"
)

// HelperPhase is where an attempt to help someone stands, as the window shows
// it (docs/ux.md, The bird's states).
type HelperPhase int

const (
	// HelperAsking: the code or link worked, and the other machine is
	// asking its user whether to let this one in.
	HelperAsking HelperPhase = iota + 1
	// HelperAllowed: they let this machine in.
	HelperAllowed
	// HelperDeclined: they said no. Nothing was shared.
	HelperDeclined
	// HelperExpired: they did not answer before the session's deadline.
	HelperExpired
	// HelperEnded: the session ended, or the other end went away.
	HelperEnded
	// HelperFailed: the attempt never reached them. Err says why.
	HelperFailed
)

// HelperEvent is one change in an attempt to help someone.
type HelperEvent struct {
	Phase   HelperPhase
	Session Session
	Err     error
}

// ErrNoName means Help was called without a name. Their prompt shows the
// name, as a claim; a blank one would leave them nothing to judge.
var ErrNoName = errors.New("client: no name to show them")

// Help redeems what the person typed -- a code or a link -- under name, joins
// the session as its controller, and follows it until it is decided or over,
// or until ctx is cancelled because the person gave up. It reports each change
// to emit, from the calling goroutine, and returns when the attempt is over.
func (c *Client) Help(ctx context.Context, input, name string, emit func(HelperEvent)) {
	fail := func(err error) {
		if ctx.Err() == nil {
			emit(HelperEvent{Phase: HelperFailed, Err: err})
		}
	}
	if name == "" {
		fail(ErrNoName)
		return
	}
	t, err := ParseTarget(input)
	if err != nil {
		fail(err)
		return
	}
	if c.Nostr != nil {
		c.helpOverNostr(ctx, t, name, emit, fail)
		return
	}
	var sess Session
	if t.Code != "" {
		sess, err = c.Redeem(ctx, t.Code, name)
	} else {
		sess, err = c.RedeemLink(ctx, t.Token, name)
	}
	if err != nil {
		fail(err)
		return
	}
	sock, err := c.DialController(ctx, sess.ID)
	if err != nil {
		fail(err)
		return
	}
	c.follow(ctx, sock, sess, emit)
}

// follow watches a session until it is decided or over. The socket says when
// the other end comes or goes; the session's state, which the relay does not
// push to the controller, comes from asking.
func (c *Client) follow(ctx context.Context, sock *Socket, sess Session, emit func(HelperEvent)) {
	msgs := make(chan Message)
	dropped := make(chan struct{})
	stop := make(chan struct{})
	defer func() {
		close(stop)
		_ = sock.Close()
	}()
	go func() {
		defer close(dropped)
		for {
			m, err := sock.Next()
			if err != nil {
				return
			}
			select {
			case msgs <- m:
			case <-stop:
				return
			}
		}
	}()

	poll := time.NewTicker(c.pollEvery)
	defer poll.Stop()
	allowed := false

	// conn carries the session's signals once the owner lets this machine
	// in. The owner's runner starts the moment it allows, a poll before this
	// end learns of it, so what arrives meanwhile is kept, up to a queue's
	// worth, and handed over in order. Whatever shows the session is waited
	// for before Help returns, and its end -- the user ending it, or the
	// viewer failing -- ends Help too: connDone is conn's Done once there is
	// one, and nil (never ready) before.
	var conn *SessionConn
	var connDone <-chan struct{}
	var early [][]byte
	var earlyBytes int
	var viewing sync.WaitGroup
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
		viewing.Wait()
	}()
	letIn := func(s Session) {
		emit(HelperEvent{Phase: HelperAllowed, Session: s})
		allowed = true
		if c.View == nil {
			return
		}
		cn := newSessionConn(s.ID, nil, sock.Send)
		for _, m := range early {
			cn.deliver(m)
		}
		early = nil
		conn, connDone = cn, cn.Done()
		viewing.Add(1)
		go func() {
			defer viewing.Done()
			defer func() { _ = cn.Close() }()
			c.View(ctx, cn)
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-dropped:
			if ctx.Err() == nil {
				emit(HelperEvent{Phase: HelperFailed, Session: sess, Err: ErrUnavailable})
			}
			return
		case <-connDone:
			if ctx.Err() == nil {
				emit(HelperEvent{Phase: HelperEnded, Session: sess})
			}
			return
		case m := <-msgs:
			if !m.FromRelay() {
				switch {
				case conn != nil:
					conn.deliver(m.Raw)
				case len(early) < signalQueue && earlyBytes+len(m.Raw) <= signalBudget:
					early = append(early, m.Raw)
					earlyBytes += len(m.Raw)
				}
				continue
			}
			switch m.Type {
			case TypePeerJoined:
				// Both ends are in, so the relay is asking the owner now
				// (signalling.maybeAskConsent). Say so without waiting for
				// the next poll; the polls then follow the decision.
				if !allowed {
					emit(HelperEvent{Phase: HelperAsking, Session: sess})
				}
			case TypePeerLeft:
				emit(HelperEvent{Phase: HelperEnded, Session: sess})
				return
			}
		case <-poll.C:
			s, err := c.Session(ctx, sess.ID)
			if errors.Is(err, ErrNotFound) {
				emit(HelperEvent{Phase: HelperExpired, Session: sess})
				return
			}
			if err != nil {
				continue // A missed poll is retried on the next tick.
			}
			sess = s
			switch s.State {
			case StateConsentRequested:
				emit(HelperEvent{Phase: HelperAsking, Session: s})
			case StateConsentGranted, StateActive:
				if !allowed {
					letIn(s)
				}
			case StateConsentDenied:
				emit(HelperEvent{Phase: HelperDeclined, Session: s})
				return
			case StateExpired:
				emit(HelperEvent{Phase: HelperExpired, Session: s})
				return
			case StateTerminated:
				emit(HelperEvent{Phase: HelperEnded, Session: s})
				return
			}
		}
	}
}

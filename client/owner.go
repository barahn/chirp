// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"errors"
	"sync"
	"time"
)

// OwnerPhase is where this machine stands with the relay, as the main window
// shows it (docs/ux.md, The bird's states).
type OwnerPhase int

const (
	// OwnerStarting: not registered yet, so there is no code to show.
	OwnerStarting OwnerPhase = iota
	// OwnerReady: registered and holding its socket. The code and link work.
	OwnerReady
	// OwnerReconnecting: the socket dropped. Within the relay's grace the
	// code and link keep working (product-model decision 1).
	OwnerReconnecting
	// OwnerOffline: the relay cannot be reached, or has released this
	// machine. A code shown before no longer works.
	OwnerOffline
)

// OwnerEvent is one change for the window to show.
type OwnerEvent struct {
	Phase OwnerPhase
	// Peer holds the code and link to show. It is zero until the first
	// registration, and keeps the last ones while reconnecting.
	Peer Peer
	// Replaced is set when the code differs from the one shown before: it
	// was used, it expired, or the person asked for a new one.
	Replaced bool
	// Lost is set with OwnerOffline when a code was shown and stopped working.
	Lost bool
	// Err is why the relay could not be reached, while offline or
	// reconnecting.
	Err error
	// Consent is set when a controller has typed this machine's code and is
	// asking to be let in.
	Consent *ConsentRequest
}

// Owner keeps this machine registered and reachable, reports what to show,
// and gives its code and link up when it stops.
type Owner struct {
	client *Client
	name   string
	emit   func(OwnerEvent)
	renew  chan struct{}

	// peerID is the registration Answer speaks for. Run sets it; Answer,
	// called from the window, reads it.
	mu     sync.Mutex
	peerID string

	// Poll is how often the code shown is checked against the relay's.
	Poll time.Duration
	// Grace is how long to keep showing a code while reconnecting. It
	// matches the relay's default peer grace, after which the relay will
	// have released the peer anyway.
	Grace time.Duration
	// Retry is the pause between attempts to reach the relay.
	Retry time.Duration
}

// NewOwner returns an Owner that registers through c under name and reports
// every change to emit. emit is called from Run's goroutine.
func NewOwner(c *Client, name string, emit func(OwnerEvent)) *Owner {
	return &Owner{
		client: c,
		name:   name,
		emit:   emit,
		renew:  make(chan struct{}, 1),
		Poll:   15 * time.Second,
		Grace:  2 * time.Minute,
		Retry:  3 * time.Second,
	}
}

// NewCode asks for a fresh code and link, revoking the ones shown
// (product-model decision 1). It returns at once; Run does the work.
func (o *Owner) NewCode() {
	select {
	case o.renew <- struct{}{}:
	default:
	}
}

var (
	errRenew = errors.New("client: a new code was asked for")
	errLost  = errors.New("client: the relay has released this peer")
)

// Run registers this machine and keeps it reachable until ctx ends. Then it
// releases the registration, so the code and link stop working at once rather
// than after the relay's grace. It returns ctx's error.
func (o *Owner) Run(ctx context.Context) error {
	o.emit(OwnerEvent{Phase: OwnerStarting})
	var shown Peer
	for {
		peer, err := o.register(ctx, shown.Code != "")
		if err != nil {
			return err
		}
		replaced := shown.Code != "" && peer.Code != shown.Code
		shown = peer
		o.setPeerID(peer.ID)
		o.emit(OwnerEvent{Phase: OwnerReady, Peer: shown, Replaced: replaced})

		err = o.serve(ctx, &shown)
		o.setPeerID("")
		switch {
		case errors.Is(err, errRenew):
			o.release(shown.ID)
		case errors.Is(err, errLost):
			// Nothing to release: the relay already has.
		default:
			o.release(shown.ID)
			return err
		}
	}
}

// register retries until the relay gives this machine a code, or ctx ends.
// lost says whether a code was being shown, and so has stopped working.
func (o *Owner) register(ctx context.Context, lost bool) (Peer, error) {
	reported := false
	for {
		p, err := o.client.Register(ctx, o.name)
		if err == nil {
			return p, nil
		}
		if ctx.Err() != nil {
			return Peer{}, ctx.Err()
		}
		if !reported {
			o.emit(OwnerEvent{Phase: OwnerOffline, Lost: lost, Err: err})
			reported = true
		}
		if err := sleep(ctx, o.Retry); err != nil {
			return Peer{}, err
		}
	}
}

// serve holds the owner socket for shown, redialling it within the grace. It
// returns errRenew, errLost, or ctx's error.
func (o *Owner) serve(ctx context.Context, shown *Peer) error {
	down := false
	since := time.Now()
	for {
		sock, err := o.client.DialOwner(ctx, shown.ID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, ErrNotFound) || time.Since(since) > o.Grace {
				return errLost
			}
			o.emit(OwnerEvent{Phase: OwnerReconnecting, Peer: *shown, Err: err})
			down = true
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-o.renew:
				return errRenew
			case <-time.After(o.Retry):
			}
			continue
		}
		if down {
			o.emit(OwnerEvent{Phase: OwnerReady, Peer: *shown})
		}
		err = o.listen(ctx, sock, shown)
		_ = sock.Close()
		if err != nil {
			return err
		}
		// The socket dropped. The grace starts now, on the relay as here.
		since = time.Now()
		down = true
		o.emit(OwnerEvent{Phase: OwnerReconnecting, Peer: *shown})
	}
}

// listen reads the owner socket until it drops, and returns nil then. It
// returns errRenew, errLost or ctx's error when serving shown must stop.
func (o *Owner) listen(ctx context.Context, sock *Socket, shown *Peer) error {
	msgs := make(chan Message)
	dropped := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)
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

	poll := time.NewTicker(o.Poll)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-o.renew:
			return errRenew
		case <-dropped:
			return nil
		case m := <-msgs:
			switch m.Type {
			case TypeConsentRequest:
				if m.Consent != nil {
					// The prompt counts down to the session's deadline,
					// which only the session knows.
					if s, err := o.client.Session(ctx, m.Consent.SessionID); err == nil {
						m.Consent.ExpiresIn = s.ExpiresIn
					}
					o.emit(OwnerEvent{Phase: OwnerReady, Peer: *shown, Consent: m.Consent})
				}
				// The session that asked spent the start code.
				if err := o.refresh(ctx, shown); err != nil {
					return err
				}
			case TypePeerLeft:
				if err := o.refresh(ctx, shown); err != nil {
					return err
				}
			}
		case <-poll.C:
			if err := o.refresh(ctx, shown); err != nil {
				return err
			}
		}
	}
}

// refresh asks the relay which code to show, and reports a new one. A failed
// poll changes nothing: the socket is what says whether the relay is gone.
func (o *Owner) refresh(ctx context.Context, shown *Peer) error {
	p, err := o.client.Peer(ctx, shown.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		return errLost
	case err != nil:
		return ctx.Err()
	}
	if p.Code != shown.Code || p.Link != shown.Link {
		*shown = p
		o.emit(OwnerEvent{Phase: OwnerReady, Peer: p, Replaced: true})
	}
	return nil
}

// ErrNotRegistered means Answer was called while this machine had no
// registration to answer for: before the first, or between losing one and
// getting the next. The request it answered is gone with it.
var ErrNotRegistered = errors.New("client: not registered")

// Answer gives this machine's user's decision on a consent request. It is
// the one way consent is given, and only the window calls it, when its user
// has pressed Allow or Decline in Chirp's own prompt.
func (o *Owner) Answer(ctx context.Context, sessionID string, granted bool) (Session, error) {
	o.mu.Lock()
	id := o.peerID
	o.mu.Unlock()
	if id == "" {
		return Session{}, ErrNotRegistered
	}
	reason := "declined by the owner"
	if granted {
		reason = "allowed by the owner"
	}
	return o.client.Consent(ctx, id, sessionID, granted, reason)
}

func (o *Owner) setPeerID(id string) {
	o.mu.Lock()
	o.peerID = id
	o.mu.Unlock()
}

// release gives the registration up. It runs as the owner stops, when ctx may
// already be done, so it has a short deadline of its own.
func (o *Owner) release(id string) {
	if id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = o.client.Release(ctx, id)
}

// sleep waits for d, or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

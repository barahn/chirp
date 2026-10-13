// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"errors"
	"sync"
)

// ErrSessionOver is what a SessionConn returns once its session has ended.
var ErrSessionOver = errors.New("client: the session is over")

// SessionConn carries one allowed session's signals between this machine's
// socket and whatever runs its end of the session. It satisfies remotekit's
// stream.SignalConn, so cmd/chirp hands it to a stream.Runner on the owner's
// side and to a stream.Viewer on the controller's; this package does not
// import stream, and so does not decide how a session is served or shown.
//
// On the owner's side it outlives the socket under it: while the owner socket
// reconnects, within the relay's grace, reads wait and writes fail, and the
// session resumes on the new socket. It ends when the session does -- either
// end leaves, the owner ends it, or Chirp quits -- or when Close is called.
type SessionConn struct {
	// ID is the session the owner allowed.
	ID string
	// Permissions are what the owner was asked for and allowed, spelled as
	// remotekit spells them (session.DefaultPermissions). The controller's
	// side is not told them, and leaves this empty.
	Permissions []string
	// Nostr says the session's signals travel over Nostr relays rather
	// than the relay's sockets (nostr.go). Each message must then fit a
	// gift wrap, so the runner keeps everything but negotiation on the
	// data channel.
	Nostr bool

	send func([]byte) error
	in   chan []byte
	done chan struct{}
	once sync.Once

	mu     sync.Mutex
	queued int // bytes waiting in in
}

func newSessionConn(id string, permissions []string, send func([]byte) error) *SessionConn {
	return &SessionConn{
		ID:          id,
		Permissions: append([]string(nil), permissions...),
		send:        send,
		in:          make(chan []byte, signalQueue),
		done:        make(chan struct{}),
	}
}

// ReadMessage blocks until the other end sends a signal, or the session ends.
//
// Once the session has ended it reports so, even with signals still queued:
// a select between a full queue and a closed done picks either at random.
func (s *SessionConn) ReadMessage() ([]byte, error) {
	select {
	case <-s.done:
		return nil, ErrSessionOver
	default:
	}
	select {
	case m := <-s.in:
		s.mu.Lock()
		s.queued -= len(m)
		s.mu.Unlock()
		return m, nil
	case <-s.done:
		return nil, ErrSessionOver
	}
}

// WriteMessage sends one signal to the other end. It fails while the socket
// is down.
func (s *SessionConn) WriteMessage(data []byte) error {
	select {
	case <-s.done:
		return ErrSessionOver
	default:
	}
	return s.send(data)
}

// Close ends the session's signals and unblocks ReadMessage. It may be called
// more than once.
func (s *SessionConn) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}

// Done is closed when the session's signals have ended.
func (s *SessionConn) Done() <-chan struct{} { return s.done }

// signalQueue is how many signals wait for a busy reader. Negotiation is an
// offer or answer and a few dozen candidates; input and files go over the data
// channel, not here.
const signalQueue = 256

// signalBudget is how many bytes of signals wait for a busy reader, however
// many signals that is. The other end may send messages up to the socket's
// read limit, and signalQueue of those would hold 256 MiB of this machine's
// memory; negotiation needs a few kilobytes.
const signalBudget = 1 << 20

// deliver hands a signal from the other end to the reader, and drops it if
// the reader is that far behind, in signals or in bytes. It never waits: the
// socket loop calling it is also what notices the session ending, a new code
// being asked for, or the socket dropping, and the other end -- untrusted --
// must not be able to stall that by sending faster than the reader reads.
func (s *SessionConn) deliver(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queued+len(data) > signalBudget {
		return
	}
	select {
	case s.in <- data:
		s.queued += len(data)
	default:
	}
}

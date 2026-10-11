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

	send func([]byte) error
	in   chan []byte
	done chan struct{}
	once sync.Once
}

func newSessionConn(id string, permissions []string, send func([]byte) error) *SessionConn {
	return &SessionConn{
		ID:          id,
		Permissions: append([]string(nil), permissions...),
		send:        send,
		in:          make(chan []byte, 64),
		done:        make(chan struct{}),
	}
}

// ReadMessage blocks until the other end sends a signal, or the session ends.
func (s *SessionConn) ReadMessage() ([]byte, error) {
	select {
	case m := <-s.in:
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

// deliver hands a signal from the other end to the reader, waiting while the
// reader is busy. It gives up when the session or stop ends.
func (s *SessionConn) deliver(data []byte, stop <-chan struct{}) {
	select {
	case s.in <- data:
	case <-s.done:
	case <-stop:
	}
}

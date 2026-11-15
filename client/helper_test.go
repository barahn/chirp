// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import "testing"

func TestSignalsBeforeTheSessionAreKeptUpToAQueuesWorth(t *testing.T) {
	var e earlySignals
	for range signalQueue + 1 {
		e.pass([]byte("s"), nil)
	}
	if len(e.queue) != signalQueue {
		t.Errorf("kept %d signals, want %d", len(e.queue), signalQueue)
	}

	e = earlySignals{}
	e.pass(make([]byte, signalBudget), nil)
	e.pass([]byte("s"), nil)
	if len(e.queue) != 1 || e.bytes != signalBudget {
		t.Errorf("kept %d signals of %d bytes past a full budget", len(e.queue), e.bytes)
	}
}

func TestKeptSignalsReachTheSessionInOrder(t *testing.T) {
	var e earlySignals
	e.pass([]byte("1"), nil)
	e.pass([]byte("2"), nil)
	conn := newSessionConn("s", nil, func([]byte) error { return nil }, nil)
	e.handTo(conn)
	e.pass([]byte("3"), conn)
	for _, want := range []string{"1", "2", "3"} {
		if got := string(<-conn.in); got != want {
			t.Fatalf("the session read %q, want %q", got, want)
		}
	}
	if len(e.queue) != 0 || e.bytes != 0 {
		t.Errorf("still kept %d signals of %d bytes after handing them over", len(e.queue), e.bytes)
	}
}

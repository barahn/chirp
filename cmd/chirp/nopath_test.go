// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestASessionWithNoPictureIsToldSo(t *testing.T) {
	var told atomic.Int32
	w := watchForPicture(20*time.Millisecond, func() { told.Add(1) })
	defer w.stop()
	time.Sleep(100 * time.Millisecond)
	if told.Load() != 1 {
		t.Fatalf("told %d times, want once after the wait", told.Load())
	}
}

func TestAPictureOrANoticeBeforeTheWaitSaysNothing(t *testing.T) {
	var told atomic.Int32
	w := watchForPicture(50*time.Millisecond, func() { told.Add(1) })
	defer w.stop()
	w.settled()
	time.Sleep(120 * time.Millisecond)
	if told.Load() != 0 {
		t.Fatalf("told %d times after the session settled", told.Load())
	}
}

func TestAnEndedSessionSaysNothing(t *testing.T) {
	var told atomic.Int32
	w := watchForPicture(50*time.Millisecond, func() { told.Add(1) })
	w.stop()
	time.Sleep(120 * time.Millisecond)
	if told.Load() != 0 {
		t.Fatalf("told %d times after the session ended", told.Load())
	}
}

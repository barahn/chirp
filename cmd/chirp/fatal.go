// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"io"
	"strings"
	"sync"
	"time"
)

// windowFailure watches what Fyne logs, and calls fatal once, with the cause,
// when Fyne reports that it could not create a window (barahn/chirp#108).
//
// Fyne neither shows anything nor exits when that happens before its event
// loop starts: it logs the error and calls Quit, which stops only a loop that
// is already running, and the loop it then starts never ends. That is what
// happens on a machine whose driver offers no OpenGL 2.1 (barahn/chirp#107),
// and it left a process running with nothing on screen. Fyne's only report of
// it is its log, so this is where Chirp learns of it.
//
// Fyne writes the reason and the cause in two log lines. fatal runs on a
// goroutine of its own, never under the logger's lock, since it may log too.
type windowFailure struct {
	out   io.Writer
	fatal func(cause string)

	mu      sync.Mutex
	pending *time.Timer // set between the reason and its cause
	fired   bool        // the first failure is the one reported
}

// causeWait is how long the cause may lag the reason before fatal runs
// without it.
const causeWait = 200 * time.Millisecond

func (f *windowFailure) Write(p []byte) (int, error) {
	n, err := f.out.Write(p)
	line := string(p)
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.fired:
	case strings.Contains(line, "Fyne error:") && strings.Contains(line, "window creation error"):
		if f.pending == nil {
			f.pending = time.AfterFunc(causeWait, func() { f.fire("") })
		}
	case f.pending != nil && strings.Contains(line, "Cause:"):
		if f.pending.Stop() {
			f.fired = true
			cause := strings.TrimSpace(line[strings.Index(line, "Cause:")+len("Cause:"):])
			go f.fatal(cause)
		}
		f.pending = nil
	}
	return n, err
}

// fire reports a failure whose cause never came.
func (f *windowFailure) fire(cause string) {
	f.mu.Lock()
	first := !f.fired
	f.fired = true
	f.pending = nil
	f.mu.Unlock()
	if first {
		f.fatal(cause)
	}
}

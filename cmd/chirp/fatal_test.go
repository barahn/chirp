// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"bytes"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// failures collects what windowFailure reports, through a real logger that
// Fyne's LogError writes to.
func failures(t *testing.T) (*bytes.Buffer, <-chan string) {
	t.Helper()
	var out bytes.Buffer
	got := make(chan string, 4)
	prev, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&windowFailure{out: &syncWriter{w: &out}, fatal: func(c string) { got <- c }})
	t.Cleanup(func() {
		log.SetOutput(prev)
		log.SetFlags(prevFlags)
	})
	return &out, got
}

type syncWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// Fyne's own report of a window it could not create, as its glfw driver makes
// it (initFailed), reaches fatal once, with the cause, and still reaches the
// log.
func TestAWindowCreationErrorIsFatal(t *testing.T) {
	out, got := failures(t)
	fyne.LogError("window creation error", errors.New("APIUnavailable: WGL: The driver does not appear to support OpenGL"))
	fyne.LogError("window creation error", errors.New("again"))
	select {
	case c := <-got:
		if c != "APIUnavailable: WGL: The driver does not appear to support OpenGL" {
			t.Errorf("cause %q", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a window creation error was not reported")
	}
	select {
	case c := <-got:
		t.Errorf("reported twice; second cause %q", c)
	case <-time.After(3 * causeWait):
	}
	if !bytes.Contains(out.Bytes(), []byte("window creation error")) {
		t.Error("the error no longer reaches the log")
	}
}

func TestAWindowCreationErrorWithoutCauseIsFatal(t *testing.T) {
	_, got := failures(t)
	fyne.LogError("window creation error", nil)
	select {
	case c := <-got:
		if c != "" {
			t.Errorf("cause %q, want none", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a window creation error without a cause was not reported")
	}
}

// Every other Fyne error is only logged.
func TestOtherFyneErrorsAreNotFatal(t *testing.T) {
	_, got := failures(t)
	fyne.LogError("Chirp's translation could not be read; using English", errors.New("Cause: anything"))
	log.Println("  Cause: a line on its own")
	select {
	case c := <-got:
		t.Fatalf("an ordinary error was fatal (cause %q)", c)
	case <-time.After(3 * causeWait):
	}
}

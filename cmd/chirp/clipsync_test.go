// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/barahn/remotekit/clipboard"
)

// sentTexts records what a clipSync sends to the other computer.
type sentTexts struct {
	mu    sync.Mutex
	texts []string
}

func (s *sentTexts) send(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, text)
	return nil
}

func (s *sentTexts) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

func runSync(t *testing.T, before string) (*clipSync, *clipboard.MemoryClipboard, *sentTexts) {
	t.Helper()
	clip := clipboard.NewMemoryClipboard()
	_ = clip.SetText(context.Background(), before)
	sent := &sentTexts{}
	c := newClipSync(clip, 10*time.Millisecond, sent.send)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go c.run(ctx)
	time.Sleep(30 * time.Millisecond)
	return c, clip, sent
}

// The helper's copies reach the other computer only while its owner allows
// the clipboard, and only what was copied after the session began.
func TestTheHelpersCopiesGoOnlyWhileAllowed(t *testing.T) {
	c, clip, sent := runSync(t, "copied before the session")
	ctx := context.Background()

	_ = clip.SetText(ctx, "copied before the grant")
	time.Sleep(40 * time.Millisecond)
	if got := sent.got(); len(got) != 0 {
		t.Fatalf("sent %q before the clipboard was allowed", got)
	}

	c.granted([]string{"screen_view", "clipboard"})
	_ = clip.SetText(ctx, "copied while allowed")
	time.Sleep(40 * time.Millisecond)
	if got := sent.got(); len(got) != 1 || got[0] != "copied while allowed" {
		t.Fatalf("sent %q, want the copy made while allowed", got)
	}

	c.granted([]string{"screen_view"})
	_ = clip.SetText(ctx, "copied after it was taken back")
	time.Sleep(40 * time.Millisecond)
	if got := sent.got(); len(got) != 1 {
		t.Fatalf("sent %q after the clipboard was taken back", got)
	}
}

// What a password manager copies on the helper's side stays there too.
func TestTheHelpersPasswordsStay(t *testing.T) {
	c, clip, sent := runSync(t, "")
	c.granted([]string{"clipboard"})
	_ = clip.SetConcealedText(context.Background(), "hunter2")
	time.Sleep(40 * time.Millisecond)
	if got := sent.got(); len(got) != 0 {
		t.Fatalf("sent %q, a concealed copy", got)
	}
}

// Text from the other computer lands on this clipboard while allowed, and
// is not sent back.
func TestTheirCopiesLandHereAndDoNotEcho(t *testing.T) {
	c, clip, sent := runSync(t, "")
	c.received("ignored: not allowed yet")
	if got, _ := clip.GetText(context.Background()); got != "" {
		t.Fatalf("applied %q before the clipboard was allowed", got)
	}
	c.granted([]string{"clipboard"})
	c.received("from the owner")
	if got, _ := clip.GetText(context.Background()); got != "from the owner" {
		t.Fatalf("clipboard holds %q", got)
	}
	time.Sleep(40 * time.Millisecond)
	if got := sent.got(); len(got) != 0 {
		t.Fatalf("echoed %q back", got)
	}
}

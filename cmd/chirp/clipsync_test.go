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
	files []clipboard.FileList
}

func (s *sentTexts) sendFiles(list clipboard.FileList) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, list)
	return nil
}

func (s *sentTexts) gotFiles() []clipboard.FileList {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]clipboard.FileList(nil), s.files...)
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
	c := newClipSync(clip, 10*time.Millisecond, sent.send, sent.sendFiles)
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

// The helper's copied files reach the other computer only while its owner
// allows both the clipboard and files.
func TestTheHelpersCopiedFilesGoOnlyWhileFilesAreAllowed(t *testing.T) {
	c, clip, sent := runSync(t, "")
	ctx := context.Background()

	c.granted([]string{"clipboard"})
	_ = clip.SetFiles(ctx, clipboard.FileList{Files: []string{"/home/kali/a.txt"}})
	time.Sleep(40 * time.Millisecond)
	if got := sent.gotFiles(); len(got) != 0 {
		t.Fatalf("sent %v without files allowed", got)
	}

	c.granted([]string{"clipboard", "file_transfer"})
	_ = clip.SetFiles(ctx, clipboard.FileList{Files: []string{"/home/kali/b.txt"}})
	time.Sleep(40 * time.Millisecond)
	if got := sent.gotFiles(); len(got) != 1 || len(got[0].Files) != 1 || got[0].Files[0] != "/home/kali/b.txt" {
		t.Fatalf("sent %v, want the file copied while allowed", got)
	}

	c.granted([]string{"file_transfer"})
	_ = clip.SetFiles(ctx, clipboard.FileList{Files: []string{"/home/kali/c.txt"}})
	time.Sleep(40 * time.Millisecond)
	if got := sent.gotFiles(); len(got) != 1 {
		t.Fatalf("sent %v after the clipboard was taken back", got)
	}
}

// Files the owner copied land on this clipboard while allowed, and are not
// sent back.
func TestTheirCopiedFilesLandHereAndDoNotEcho(t *testing.T) {
	c, clip, sent := runSync(t, "")
	ctx := context.Background()
	theirs := clipboard.FileList{Files: []string{"/tmp/remotekit-clip-staging-viewer-1/report.pdf"}}

	c.granted([]string{"clipboard"})
	c.receivedFiles(theirs)
	if got, _ := clip.GetFiles(ctx); len(got.Files) != 0 {
		t.Fatalf("applied %v without files allowed", got)
	}

	c.granted([]string{"clipboard", "file_transfer"})
	c.receivedFiles(theirs)
	if got, _ := clip.GetFiles(ctx); len(got.Files) != 1 || got.Files[0] != theirs.Files[0] {
		t.Fatalf("clipboard holds %v, want their files", got)
	}
	time.Sleep(40 * time.Millisecond)
	if got := sent.gotFiles(); len(got) != 0 {
		t.Fatalf("echoed %v back", got)
	}
}

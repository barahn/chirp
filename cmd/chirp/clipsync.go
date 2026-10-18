// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"github.com/barahn/remotekit/clipboard"
)

// clipSync is the helper's half of the clipboard sync (barahn/chirp#134): on
// the owner's side remotekit's runner syncs it; on the helper's, this does,
// for as long as the owner allows the clipboard. What the helper copies goes
// to the other computer, what the owner copies lands here, and what a
// password manager marks as secret stays where it was copied
// (barahn/remotekit#130).
type clipSync struct {
	mgr     clipboard.Manager
	watcher *clipboard.Watcher
	send    func(text string) error
	allowed atomic.Bool
}

func newClipSync(mgr clipboard.Manager, interval time.Duration, send func(string) error) *clipSync {
	return &clipSync{mgr: mgr, watcher: clipboard.NewWatcher(mgr, interval), send: send}
}

// run watches this machine's clipboard until ctx ends. What was on it when
// run began is never sent.
func (c *clipSync) run(ctx context.Context) {
	c.watcher.Start(ctx, func(text string) {
		if c.allowed.Load() {
			_ = c.send(text)
		}
	})
}

// granted takes what the owner allows now.
func (c *clipSync) granted(permissions []string) {
	c.allowed.Store(slices.Contains(permissions, "clipboard"))
}

// received puts the owner's copied text on this machine's clipboard, without
// sending it back.
func (c *clipSync) received(text string) {
	if !c.allowed.Load() {
		return
	}
	c.watcher.UpdateLastText(text)
	_ = c.mgr.SetText(context.Background(), text)
}

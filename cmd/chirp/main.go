// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command chirp is the one program both sides of a Chirp session run
// (docs/product-model.md). It shows this machine's code and link, and lets its
// user help someone else by typing theirs.
//
// It asks for consent in its own prompt and sends the answer (barahn/chirp#11).
// The screen itself is barahn/chirp#12: until then, a session that is let in
// shows nothing.
//
// Usage:
//
//	chirp -relay https://chirp.example.org
//
// There is no public relay yet (docs/product-model.md, decision 5), so the
// address of one its users run or trust must be given, with -relay or in
// CHIRP_RELAY. Nothing is written to disk to remember it (barahn/chirp#49).
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/barahn/chirp/client"
	"github.com/barahn/chirp/cmd/chirp/internal/ui"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

func main() {
	relay := flag.String("relay", os.Getenv("CHIRP_RELAY"), "address of the Chirp server, http or https (default $CHIRP_RELAY)")
	flag.Parse()

	a := app.New()
	a.Settings().SetTheme(warm.New())
	a.SetIcon(warm.AppIcon)
	w := ui.New(a)

	c, err := client.New(*relay)
	if err != nil {
		w.ShowNoServer(*relay)
		w.Window().ShowAndRun()
		return
	}

	ctx, stop := context.WithCancel(context.Background())
	w.Animate(ctx)
	owner := client.NewOwner(c, "", func(ev client.OwnerEvent) {
		fyne.Do(func() { w.ApplyOwner(ev) })
	})
	ownerDone := make(chan struct{})
	go func() {
		defer close(ownerDone)
		_ = owner.Run(ctx)
	}()
	w.OnNewCode = owner.NewCode
	// Consent is given here and nowhere else: the user's own answer, from
	// Chirp's own prompt, sent as this machine (barahn/chirp#11).
	w.OnConsent = func(sessionID string, granted bool) {
		go func() {
			if _, err := owner.Answer(ctx, sessionID, granted); err != nil {
				fyne.Do(func() { w.AnswerFailed(granted) })
			}
		}()
	}
	// The relay ends a session only when its owner gives the registration
	// up, so ending one replaces the code and link too.
	w.OnEndSession = owner.NewCode

	// One attempt to help someone at a time. The callbacks run on Fyne's
	// goroutine, so cancelHelp needs no lock.
	cancelHelp := func() {}
	w.OnConnect = func(target, name string) {
		cancelHelp()
		var hctx context.Context
		hctx, cancelHelp = context.WithCancel(ctx)
		go c.Help(hctx, target, name, func(ev client.HelperEvent) {
			fyne.Do(func() { w.ApplyHelper(ev) })
		})
	}
	w.OnCancel = func() { cancelHelp() }

	// Closing the window quits, and quitting gives the code and link up at
	// once (barahn/chirp#50): no tray icon keeps a live code behind the
	// user's back.
	w.Window().ShowAndRun()
	cancelHelp()
	stop()
	select {
	case <-ownerDone:
	case <-time.After(6 * time.Second):
	}
}

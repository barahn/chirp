// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command chirp is the one program both sides of a Chirp session run
// (docs/product-model.md). It shows this machine's code and link, and lets its
// user help someone else by typing theirs.
//
// It asks for consent in its own prompt and sends the answer (barahn/chirp#11).
// A session the owner allows is served by remotekit's stream.Runner over the
// owner socket, granted what the owner allowed and nothing more
// (barahn/chirp#12). The controller's side shows that screen in a window of its
// own with remotekit's stream.Viewer, decoded in Go, and sends the mouse and
// keyboard back over the same WebRTC session.
//
// Usage:
//
//	chirp -relay https://chirp.example.org -ice stun:turn.example.org:3478
//
// There is no public relay yet (docs/product-model.md, decision 5), so the
// address of one its users run or trust must be given, with -relay or in
// CHIRP_RELAY. The STUN and TURN servers are given the same way, with -ice or
// in CHIRP_ICE, and a TURN credential only in CHIRP_TURN_USERNAME and
// CHIRP_TURN_PASSWORD, never on the command line other users can read
// (docs/stun-turn.md). Without any, the screen reaches only a controller on
// the same network. Nothing is written to disk to remember them
// (barahn/chirp#49).
package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/barahn/chirp/client"
	"github.com/barahn/chirp/cmd/chirp/internal/ui"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
	"github.com/barahn/remotekit/bark"
	"github.com/barahn/remotekit/stream"
	"github.com/barahn/remotekit/webrtc"
)

func main() {
	relay := flag.String("relay", os.Getenv("CHIRP_RELAY"), "address of the Chirp server, http or https (default $CHIRP_RELAY)")
	iceList := flag.String("ice", os.Getenv("CHIRP_ICE"), "STUN and TURN servers, comma-separated (default $CHIRP_ICE)")
	flag.Parse()
	ice := iceConfig(*iceList, os.Getenv("CHIRP_TURN_USERNAME"), os.Getenv("CHIRP_TURN_PASSWORD"))

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
	owner.Serve = func(ctx context.Context, s *client.SessionConn) {
		serveSession(ctx, s, ice)
	}
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
	// A session this machine is let into opens the other computer's screen
	// in a window of its own; ending it, or closing that window, gives up
	// the session like Cancel does.
	c.View = func(ctx context.Context, conn *client.SessionConn) {
		viewSession(ctx, conn, ice, w)
	}

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

// serveSession serves a session its owner has allowed, until it ends.
func serveSession(ctx context.Context, s *client.SessionConn, ice webrtc.PeerConfig) {
	_ = newRunner(s.ID, s.Permissions, ice).Serve(ctx, s)
}

// newRunner builds the runner for a session its owner has allowed.
//
// remotekit's runner enforces consent, not Chirp (HANDOFF §5.5): it is granted
// exactly what the owner allowed in the prompt, and withholds the screen until
// then. The screen goes over WebRTC only: the JPEG fallback through the relay
// is refused, since Chirp's viewer never uses it and the relay could see every
// frame. Signed viewers stay off for the trusted relay the first runnable Chirp
// ships with (docs/product-model.md, decision 5): they need the controller's
// key from a route the relay does not control, which is the PAKE
// (barahn/chirp#21). A public relay turns them on, with RequireDataChannel.
func newRunner(id string, permissions []string, ice webrtc.PeerConfig) *stream.Runner {
	r := stream.New(stream.Config{ID: id, PeerConfig: &ice})
	r.RequireScreenViewConsent(true)
	r.DisableRelayFallback(true)
	r.Grant(permissions...)
	return r
}

// iceConfig builds the STUN and TURN servers sessions use. It is always
// explicit, and empty when none are given: remotekit's default would tell a
// third party about every session (docs/stun-turn.md).
func iceConfig(list, username, password string) webrtc.PeerConfig {
	servers := []string{}
	for _, s := range strings.Split(list, ",") {
		if s = strings.TrimSpace(s); s != "" {
			servers = append(servers, s)
		}
	}
	return webrtc.PeerConfig{ICEServers: servers, TURNUsername: username, TURNPassword: password}
}

// viewSession shows a session this machine was let into, until it ends: the
// other computer's screen through remotekit's viewer, and the user's mouse and
// keyboard sent back over it. Whether input is applied is the other side's
// runner's decision, from what its user allowed.
func viewSession(ctx context.Context, conn *client.SessionConn, ice webrtc.PeerConfig, w *ui.Window) {
	v := stream.NewViewer(stream.ViewerConfig{ID: "chirp-controller", AgentID: conn.ID, PeerConfig: &ice})
	opened := make(chan *ui.Remote, 1)
	fyne.Do(func() {
		// End closes this session's conn, which ends this Help and no
		// other; the viewer failing does the same.
		opened <- w.OpenRemote(func(ev bark.InputEvent) { _ = v.SendInput(ev) }, func() { _ = conn.Close() })
	})
	remote := <-opened
	defer fyne.Do(remote.Close)
	v.OnFrame(remote.Show)
	v.OnNotice(func(n stream.Notice) { remote.Notice(n.Type) })
	_ = v.Serve(ctx, conn)
}

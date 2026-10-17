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
// With no server given, with -relay or in CHIRP_RELAY, Chirp uses Fabrintek's
// relay, relay.usechirp.app, and its STUN server; anyone may run their own
// instead (docs/product-model.md, decision 5; docs/relay.md). The STUN and
// TURN servers are given with -ice or in CHIRP_ICE, and a TURN credential only
// in CHIRP_TURN_USERNAME and CHIRP_TURN_PASSWORD, never on the command line
// other users can read (docs/stun-turn.md). With a server of one's own and no
// -ice, the screen reaches only a controller on the same network. Nothing is
// written to disk to remember any of it (barahn/chirp#49).
//
// With -nostr, or CHIRP_NOSTR, a comma-separated list of Nostr relays
// (wss://...), sessions are signalled over those relays instead of through
// the Chirp server, which then only looks codes up (docs/product-model.md,
// decision 5; barahn/chirp#43). There is no default list yet.
//
// It speaks the system's language when it has a translation for it, English
// and Brazilian Portuguese so far (barahn/chirp#80). On Linux the language
// comes from LANGUAGE, LC_ALL, LC_MESSAGES or LANG, so LANGUAGE=pt_BR chirp
// shows it in Portuguese whatever the desktop's language.
package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/lang"

	"github.com/barahn/chirp/client"
	"github.com/barahn/chirp/cmd/chirp/internal/alert"
	"github.com/barahn/chirp/cmd/chirp/internal/privilege"
	"github.com/barahn/chirp/cmd/chirp/internal/ui"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
	"github.com/barahn/remotekit/bark"
	"github.com/barahn/remotekit/stream"
	"github.com/barahn/remotekit/webrtc"
)

func main() {
	relay := flag.String("relay", os.Getenv("CHIRP_RELAY"), "address of the Chirp server, http or https (default $CHIRP_RELAY, else "+defaultRelay+")")
	iceList := flag.String("ice", os.Getenv("CHIRP_ICE"), "STUN and TURN servers, comma-separated (default $CHIRP_ICE, else "+defaultICE+" with the default server)")
	nostrList := flag.String("nostr", os.Getenv("CHIRP_NOSTR"), "Nostr relays to signal sessions over, comma-separated (default $CHIRP_NOSTR)")
	allowElevated := flag.Bool("allow-elevated", false, "run as administrator anyway, giving whoever is let in full control of this computer")
	flag.Parse()
	*relay, *iceList = withDefaults(*relay, *iceList)
	ice := iceConfig(*iceList, os.Getenv("CHIRP_TURN_USERNAME"), os.Getenv("CHIRP_TURN_PASSWORD"))

	if err := ui.UseLanguage(chooseLanguage(displayLanguages(), lang.SystemLocale().String())); err != nil {
		fyne.LogError("Chirp's translation could not be read; using English", err)
	}
	// If Fyne cannot open a window it says so only in its log, and then
	// neither shows anything nor exits (barahn/chirp#108). Chirp says why in
	// a message box, gives its code up, and exits.
	var release atomic.Pointer[func()]
	log.SetOutput(&windowFailure{out: os.Stderr, fatal: func(cause string) {
		released := make(chan struct{})
		go func() {
			defer close(released)
			if f := release.Load(); f != nil {
				(*f)()
			}
		}()
		alert.Show(ui.WindowFailure(cause))
		select {
		case <-released:
		case <-time.After(6 * time.Second):
		}
		os.Exit(1)
	}})
	a := app.New()
	a.Settings().SetTheme(warm.New())
	a.SetIcon(warm.AppIcon)
	w := ui.New(a)

	// Whoever is let in acts with this process's rights. As administrator,
	// that is full control of the machine, past every UAC prompt, so Chirp
	// will not show a code unless told to on purpose (barahn/chirp#117,
	// option 1; GHSA-r385-j539-p738); and then the prompt says so.
	if privilege.Elevated() {
		if !*allowElevated {
			w.ShowElevated()
			w.Window().ShowAndRun()
			return
		}
		w.Elevated = true
	}

	c, err := client.New(*relay)
	if err != nil {
		w.ShowNoServer(*relay)
		w.Window().ShowAndRun()
		return
	}
	c.Nostr = nostrConfig(*nostrList)

	ctx, stop := context.WithCancel(context.Background())
	w.Animate(ctx)
	owner := client.NewOwner(c, "", func(ev client.OwnerEvent) {
		fyne.Do(func() { w.ApplyOwner(ev) })
	})
	var serving runners
	owner.Serve = func(ctx context.Context, s *client.SessionConn) {
		r := newRunner(s, ice)
		serving.add(s.ID, r)
		defer serving.remove(s.ID, r)
		_ = r.Serve(ctx, s)
	}
	ownerDone := make(chan struct{})
	go func() {
		defer close(ownerDone)
		_ = owner.Run(ctx)
	}()
	giveUp := func() {
		stop()
		<-ownerDone
	}
	release.Store(&giveUp)
	w.OnNewCode = owner.NewCode
	// Consent is given here and nowhere else: the user's own answer, from
	// Chirp's own prompt, sent as this machine (barahn/chirp#11).
	w.OnConsent = func(sessionID string, grant []string) {
		go func() {
			if _, err := owner.Answer(ctx, sessionID, grant); err != nil {
				fyne.Do(func() { w.AnswerFailed(len(grant) > 0) })
			}
		}()
	}
	// A permission turned off in the session bar stops at once, on this
	// machine, whatever the other end does (barahn/chirp#52).
	w.OnRevoke = serving.revoke
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

// runners are the sessions being served, by ID, so that a permission the
// owner turns off reaches the runner enforcing it.
type runners struct {
	mu sync.Mutex
	m  map[string]*stream.Runner
}

func (rs *runners) add(id string, r *stream.Runner) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.m == nil {
		rs.m = make(map[string]*stream.Runner)
	}
	rs.m[id] = r
}

func (rs *runners) remove(id string, r *stream.Runner) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.m[id] == r {
		delete(rs.m, id)
	}
}

// revoke withdraws permission from the session id, if it is still served.
// The runner stops it at once and tells the controller's Chirp.
func (rs *runners) revoke(id, permission string) {
	rs.mu.Lock()
	r := rs.m[id]
	rs.mu.Unlock()
	if r != nil {
		r.Revoke(permission)
	}
}

// newRunner builds the runner for a session its owner has allowed.
//
// remotekit's runner enforces consent, not Chirp (HANDOFF §5.5): it is granted
// exactly what the owner allowed in the prompt, and withholds the screen until
// then. The screen goes over WebRTC only: the JPEG fallback through the relay
// is refused, since Chirp's viewer never uses it and the relay could see every
// frame.
//
// A session signalled over Nostr keeps input, clipboard and files on the data
// channel: every signal must fit a gift wrap, and the relays have no business
// carrying them even encrypted. It is also signed, with the keys the two
// Chirps proved to each other with the PAKE over the code's secret part
// (barahn/chirp#21): the runner signs its answers, and acts only on offers
// and candidates the controller signed. Neither the server nor the relays
// can then take part in the session. Through the trusted relay there is no
// PAKE, and so no keys: signed viewers stay off there (docs/product-model.md,
// decision 5).
func newRunner(s *client.SessionConn, ice webrtc.PeerConfig) *stream.Runner {
	r := stream.New(stream.Config{ID: s.ID, PeerConfig: &ice, SigningKey: s.SigningKey})
	r.RequireScreenViewConsent(true)
	r.DisableRelayFallback(true)
	r.RequireDataChannel(s.Nostr)
	if peer := s.PeerKey; peer != nil {
		r.RequireSignedViewers(func(k ed25519.PublicKey) bool { return k.Equal(peer) })
	}
	// Grantable again, so no caller can hand the runner a withheld
	// permission it did not get through the prompt.
	r.Grant(client.Grantable(s.Permissions)...)
	return r
}

// nostrConfig turns signalling over Nostr on when relays are given. The
// relays are checked when the contact is registered, not here.
func nostrConfig(list string) *client.NostrConfig {
	relays := splitList(list)
	if len(relays) == 0 {
		return nil
	}
	return &client.NostrConfig{Relays: relays}
}

// iceConfig builds the STUN and TURN servers sessions use. It is always
// explicit, and empty when none are given: remotekit's default would tell a
// third party about every session (docs/stun-turn.md).
// Fabrintek's public network: the relay a downloaded Chirp uses when it is
// given none, and the STUN server that goes with it (network/servers.json,
// docs/relay.md). There is no TURN by default: a TURN credential baked into
// every binary would work for anyone who unpacks one (docs/stun-turn.md,
// Credentials).
const (
	defaultRelay = "https://relay.usechirp.app"
	defaultICE   = "stun:relay.usechirp.app:3478"
)

// withDefaults fills in the public network for what was not given. The STUN
// server comes only with the default relay: someone who runs their own relay
// chose not to depend on Fabrintek, and a STUN server tells its operator
// that a session exists and from which addresses.
func withDefaults(relay, ice string) (string, string) {
	if strings.TrimSpace(relay) != "" {
		return relay, ice
	}
	if strings.TrimSpace(ice) == "" {
		ice = defaultICE
	}
	return defaultRelay, ice
}

func iceConfig(list, username, password string) webrtc.PeerConfig {
	return webrtc.PeerConfig{ICEServers: splitList(list), TURNUsername: username, TURNPassword: password}
}

// splitList reads a comma-separated list, never nil.
func splitList(list string) []string {
	items := []string{}
	for _, s := range strings.Split(list, ",") {
		if s = strings.TrimSpace(s); s != "" {
			items = append(items, s)
		}
	}
	return items
}

// viewerConfig is how this machine views a session it was let into. Over
// Nostr the viewer signs its offers with the key the PAKE proved, and accepts
// answers only from the owner's (barahn/chirp#21); it goes by the name the
// owner's runner addresses it by, which its signed answers carry.
func viewerConfig(conn *client.SessionConn, ice webrtc.PeerConfig) stream.ViewerConfig {
	cfg := stream.ViewerConfig{ID: "chirp-controller", AgentID: conn.ID, PeerConfig: &ice}
	if conn.PeerKey != nil {
		cfg.ID, cfg.SigningKey, cfg.AgentKey = conn.ViewerID, conn.SigningKey, conn.PeerKey
	}
	return cfg
}

// viewSession shows a session this machine was let into, until it ends: the
// other computer's screen through remotekit's viewer, and the user's mouse and
// keyboard sent back over it. Whether input is applied is the other side's
// runner's decision, from what its user allowed.
func viewSession(ctx context.Context, conn *client.SessionConn, ice webrtc.PeerConfig, w *ui.Window) {
	v := stream.NewViewer(viewerConfig(conn, ice))
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
	v.OnPermissions(remote.Granted)
	_ = v.Serve(ctx, conn)
}

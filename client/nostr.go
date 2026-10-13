// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/remotekit/signal/nostr"
)

// Signalling over Nostr relays: decision 5 in docs/product-model.md, and
// barahn/chirp#43. With Client.Nostr set, the server keeps only what it must
// -- registering this machine, its code and link, and looking a code up --
// and the two Chirps carry everything else between themselves over public
// Nostr relays, gift-wrapped and encrypted to the other end's key, through
// remotekit's signal/nostr:
//
//  1. The owner listens on the relays under a key made for its registration,
//     and registers that key and the relays as its contact.
//  2. The controller looks the code up, gets the contact, and dials it. The
//     lookup spends the code exactly as a redemption does.
//  3. The controller asks to be let in (TypeAskConsent, with its name); the
//     owner's user answers in Chirp's own prompt (TypeConsentAnswer).
//  4. An allowed session is served and shown as over the relay, by
//     remotekit's runner and viewer, but its signals go over Nostr.
//
// What this does not do yet: bind the two ends. The controller trusts the
// key the lookup returned, and the owner sees the controller's name as a
// claim, as it always has. The PAKE over the code's secret part
// (barahn/chirp#21) is what removes the trust in the server, and until it
// lands the server's lookup is as trusted as the relay is today.

// NostrConfig turns signalling over Nostr relays on.
type NostrConfig struct {
	// Relays are the Nostr relays this machine listens on when someone
	// helps it, as ws:// or wss:// URLs: 1 to session.MaxContactRelays. To
	// help someone else, Chirp dials the relays their contact names.
	Relays []string
}

// The messages Chirp's two ends exchange over Nostr before remotekit takes
// the session over. They are dotted, so they cannot collide with remotekit's
// bare-word types, and they are not chirp.*, which is the relay's alone.
const (
	// TypeAskConsent: a controller asking to be let in, under a name.
	TypeAskConsent = "consent.request"
	// TypeConsentAnswer: the owner's answer. A session it allows is named
	// in it, and the controller's viewer uses that name.
	TypeConsentAnswer = "consent.answer"
	// TypeSessionEnd: the owner has ended the session, or the request.
	TypeSessionEnd = "session.end"
	// typeClose is remotekit's own "close", which a controller's viewer
	// sends when it stops, and Chirp sends when it gives up asking.
	typeClose = "close"
)

// Why an owner says no.
const (
	reasonDeclined = "declined"
	reasonExpired  = "expired"
	reasonBusy     = "busy"
)

// nostrAskFor is how long an owner keeps a request over Nostr open. The
// prompt counts it down and declines by itself at the end.
const nostrAskFor = 2 * time.Minute

// maxNostrName bounds the name a controller asks under, in bytes.
const maxNostrName = 128

var (
	// ErrNoNostr means the owner of the code registered no contact, so it
	// cannot be reached over Nostr. The code is spent all the same.
	ErrNoNostr = errors.New("client: that Chirp cannot be reached over Nostr")
	// ErrBadContact means the server answered a lookup with a contact that
	// is not a key and a short list of relay URLs. Nothing is dialled.
	ErrBadContact = errors.New("client: the server returned an unusable contact")
)

// knock is every message of the exchange above. viewer_id is how a Listen
// conn labels who sent a message, and addresses who a reply goes to.
type knock struct {
	Type      string `json:"type"`
	ViewerID  string `json:"viewer_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Granted   bool   `json:"granted,omitempty"`
	Reason    string `json:"reason,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	ExpiresIn int    `json:"expires_in_seconds,omitempty"`
}

func (k knock) encode() []byte {
	data, _ := json.Marshal(k)
	return data
}

// SetContact registers how the owner peerID can be reached over Nostr.
func (c *Client) SetContact(ctx context.Context, peerID string, ct session.Contact) error {
	return c.do(ctx, http.MethodPut, c.endpoint("api", "peers", peerID, "contact"), ct, nil)
}

// Lookup spends someone's code for their contact.
func (c *Client) Lookup(ctx context.Context, code string) (session.Contact, error) {
	var ct session.Contact
	err := c.do(ctx, http.MethodPost, c.endpoint("api", "lookup"), map[string]string{"code": code}, &ct)
	return lookedUp(ct, err)
}

// LookupLink mints a single-use code from someone's link and spends it for
// their contact, in one step.
func (c *Client) LookupLink(ctx context.Context, token string) (session.Contact, error) {
	var ct session.Contact
	err := c.do(ctx, http.MethodPost, c.endpoint("api", "links", token, "lookup"), nil, &ct)
	return lookedUp(ct, err)
}

// lookedUp maps a lookup's 409 to ErrNoNostr, and checks a contact before
// anything dials it: the server is not trusted to name only relays.
func lookedUp(ct session.Contact, err error) (session.Contact, error) {
	var re *RelayError
	if errors.As(err, &re) && re.Status == http.StatusConflict {
		return session.Contact{}, ErrNoNostr
	}
	if err != nil {
		return session.Contact{}, err
	}
	if session.ValidateContact(ct) != nil {
		return session.Contact{}, ErrBadContact
	}
	return ct, nil
}

// newSessionID names a session over Nostr. It is the runner's ID and the
// viewer's AgentID, and identifies the request in the owner's prompt.
func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// relabel returns msg with its viewer_id set to viewer, or removed when viewer
// is empty.
func relabel(msg []byte, viewer string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(msg, &fields); err != nil || fields == nil {
		return nil, errors.New("client: a signal is not a JSON object")
	}
	if viewer == "" {
		delete(fields, "viewer_id")
	} else {
		id, _ := json.Marshal(viewer)
		fields["viewer_id"] = id
	}
	return json.Marshal(fields)
}

// The owner's side.

// nostrMsg is one message from a controller, labelled with its key. reset
// says the Listen conn was replaced, so every controller is gone.
type nostrMsg struct {
	viewer string
	head   knock
	raw    []byte
	reset  bool
}

// nostrOwner is the owner's end for one registration: a Listen conn under a
// key made for it, registered as the peer's contact. If every relay drops
// (remotekit does not redial yet, barahn/remotekit#70), it listens again
// under a new key and registers that.
type nostrOwner struct {
	client *Client
	cfg    NostrConfig
	peerID string
	retry  time.Duration
	msgs   chan nostrMsg
	done   chan struct{}
	// listening is closed once the contact is first registered.
	listening chan struct{}
	once      sync.Once

	mu   sync.Mutex
	conn *nostr.Conn
}

func startNostrOwner(ctx context.Context, c *Client, peerID string, retry time.Duration) *nostrOwner {
	n := &nostrOwner{
		client:    c,
		cfg:       *c.Nostr,
		peerID:    peerID,
		retry:     retry,
		msgs:      make(chan nostrMsg, signalQueue),
		done:      make(chan struct{}),
		listening: make(chan struct{}),
	}
	go n.run(ctx)
	return n
}

// run listens, registers the contact and reads, until ctx ends or the server
// has released the peer.
func (n *nostrOwner) run(ctx context.Context) {
	defer close(n.done)
	first := true
	for ctx.Err() == nil {
		if !first {
			select {
			case n.msgs <- nostrMsg{reset: true}:
			case <-ctx.Done():
				return
			}
			if sleep(ctx, n.retry) != nil {
				return
			}
		}
		first = false
		conn, err := nostr.Listen(ctx, nostr.Config{Relays: n.cfg.Relays, Dialer: n.client.dialer, MaxViewers: 4})
		if err != nil {
			continue
		}
		err = n.client.SetContact(ctx, n.peerID, session.Contact{Key: conn.PublicKey(), Relays: n.cfg.Relays})
		if err != nil {
			_ = conn.Close()
			if errors.Is(err, ErrNotFound) {
				return
			}
			continue
		}
		n.mu.Lock()
		n.conn = conn
		n.mu.Unlock()
		n.once.Do(func() { close(n.listening) })
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		n.read(ctx, conn)
		stop()
		n.mu.Lock()
		n.conn = nil
		n.mu.Unlock()
		_ = conn.Close()
	}
}

// read hands every message to the owner's loop until conn ends.
func (n *nostrOwner) read(ctx context.Context, conn *nostr.Conn) {
	for {
		raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var head knock
		if json.Unmarshal(raw, &head) != nil || head.ViewerID == "" {
			continue
		}
		select {
		case n.msgs <- nostrMsg{viewer: head.ViewerID, head: head, raw: raw}:
		case <-ctx.Done():
			return
		}
	}
}

// send writes data to the controller viewer, and to no one else: viewer_id
// is set to it whatever data said, so a broadcast cannot reach a controller
// who was not let in. Writes are one at a time, as a conn needs.
func (n *nostrOwner) send(viewer string, data []byte) error {
	labelled, err := relabel(data, viewer)
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.conn == nil {
		return ErrUnavailable
	}
	return n.conn.WriteMessage(labelled)
}

// nostrListenWait bounds how long an owner holds its first code back while
// it registers its contact. Past it the code is shown anyway: it still works
// through the relay, and over Nostr once the relays answer.
const nostrListenWait = 10 * time.Second

// awaitListening waits until the contact is registered, ctx ends, or
// nostrListenWait passes. A code shown before then would be spent by a
// lookup that finds no contact.
func (n *nostrOwner) awaitListening(ctx context.Context) {
	t := time.NewTimer(nostrListenWait)
	defer t.Stop()
	select {
	case <-n.listening:
	case <-n.done:
	case <-ctx.Done():
	case <-t.C:
	}
}

// wait waits for run to return; its context must be done.
func (n *nostrOwner) wait() {
	if n != nil {
		<-n.done
	}
}

// nostrAsk is a request over Nostr the owner's user was shown: who asked, the
// session it would be, and until when the answer counts.
type nostrAsk struct {
	viewer    string
	sessionID string
	until     time.Time
	allowed   bool
}

// onNostr handles one message from a controller, on Run's goroutine.
func (o *Owner) onNostr(ctx context.Context, m nostrMsg, shown *Peer) error {
	o.mu.Lock()
	ask := o.nostrAsk
	o.mu.Unlock()

	if m.reset {
		if ask != nil {
			o.endSession()
		}
		return nil
	}
	if ask != nil && m.viewer == ask.viewer {
		switch m.head.Type {
		case typeClose:
			// The controller gave up, or ended the session.
			o.endSession()
			return o.refresh(ctx, shown)
		case TypeAskConsent, TypeConsentAnswer:
			return nil // asked already; not for remotekit
		}
		// As over the relay: Answer may have let the session in an instant
		// ago, so that is looked at first.
		select {
		case id := <-o.allowed:
			o.startSession(ctx, id)
		default:
		}
		if o.active != nil {
			o.active.deliver(m.raw)
		}
		return nil
	}
	if m.head.Type != TypeAskConsent {
		return nil
	}
	n := o.nostrOwner()
	if o.active != nil || ask != nil && (ask.allowed || time.Now().Before(ask.until)) {
		// One controller at a time, as over the relay.
		_ = n.send(m.viewer, knock{Type: TypeConsentAnswer, Reason: reasonBusy}.encode())
		return nil
	}
	o.endSession()
	name := m.head.Name
	if len(name) > maxNostrName {
		name = name[:maxNostrName]
	}
	req := &ConsentRequest{
		SessionID:   newSessionID(),
		Name:        name,
		Permissions: append([]string(nil), session.DefaultPermissions...),
		ExpiresIn:   int(nostrAskFor / time.Second),
	}
	o.mu.Lock()
	o.asked = req
	o.nostrAsk = &nostrAsk{viewer: m.viewer, sessionID: req.SessionID, until: time.Now().Add(nostrAskFor)}
	o.mu.Unlock()
	o.emit(OwnerEvent{Phase: OwnerReady, Peer: *shown, Consent: req})
	// The lookup that led here spent the start code.
	return o.refresh(ctx, shown)
}

// answerNostr gives the owner's answer to a request that came over Nostr. It
// reports false when sessionID is not that request, which then is not one.
func (o *Owner) answerNostr(sessionID string, granted bool) (Session, bool, error) {
	o.mu.Lock()
	ask := o.nostrAsk
	n := o.nostr
	if ask == nil || ask.sessionID != sessionID || ask.allowed || n == nil {
		o.mu.Unlock()
		return Session{}, false, nil
	}
	reply := knock{Type: TypeConsentAnswer, SessionID: sessionID, Granted: granted, Reason: reasonDeclined}
	if time.Now().After(ask.until) {
		// Too late: the controller has stopped waiting.
		granted, reply.Granted, reply.Reason = false, false, reasonExpired
	}
	if granted {
		reply.Reason = ""
		ask.allowed = true
	} else {
		o.nostrAsk = nil
	}
	o.mu.Unlock()

	if err := n.send(ask.viewer, reply.encode()); err != nil {
		if granted {
			o.mu.Lock()
			if o.nostrAsk == ask {
				o.nostrAsk = nil
			}
			o.mu.Unlock()
		}
		return Session{}, true, err
	}
	if !granted {
		return Session{ID: sessionID, State: StateConsentDenied}, true, nil
	}
	select {
	case o.allowed <- sessionID:
	default:
	}
	return Session{ID: sessionID, State: StateConsentGranted}, true, nil
}

// endNostr tells the controller of a request or session over Nostr that it is
// over, and forgets it. endSession calls it.
func (o *Owner) endNostr() {
	o.mu.Lock()
	ask := o.nostrAsk
	n := o.nostr
	o.nostrAsk = nil
	o.mu.Unlock()
	if ask != nil && n != nil {
		_ = n.send(ask.viewer, knock{Type: TypeSessionEnd}.encode())
	}
}

func (o *Owner) nostrOwner() *nostrOwner {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.nostr
}

// nostrSender returns how the allowed session sessionID writes to its
// controller over Nostr, or nil if it did not come over Nostr.
func (o *Owner) nostrSender(sessionID string) func([]byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	ask, n := o.nostrAsk, o.nostr
	if ask == nil || ask.sessionID != sessionID || n == nil {
		return nil
	}
	return func(data []byte) error { return n.send(ask.viewer, data) }
}

// The controller's side.

// helpOverNostr is Help for a Client with Nostr set: it looks the target up,
// dials the owner's contact, asks to be let in, and follows the answer.
func (c *Client) helpOverNostr(ctx context.Context, t Target, name string, emit func(HelperEvent), fail func(error)) {
	var ct session.Contact
	var err error
	if t.Code != "" {
		ct, err = c.Lookup(ctx, t.Code)
	} else {
		ct, err = c.LookupLink(ctx, t.Token)
	}
	if err != nil {
		fail(err)
		return
	}
	conn, err := nostr.Dial(ctx, nostr.Config{Relays: ct.Relays, Dialer: c.dialer}, ct.Key)
	if err != nil {
		fail(ErrUnavailable)
		return
	}
	defer func() { _ = conn.Close() }()

	// Writes are one at a time: the viewer and this loop both write.
	var wmu sync.Mutex
	send := func(data []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return conn.WriteMessage(data)
	}
	if len(name) > maxNostrName {
		name = name[:maxNostrName]
	}
	if err := send(knock{Type: TypeAskConsent, Name: name}.encode()); err != nil {
		fail(ErrUnavailable)
		return
	}

	msgs := make(chan []byte)
	dropped := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(dropped)
		for {
			m, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case msgs <- m:
			case <-stop:
				return
			}
		}
	}()

	var sc *SessionConn
	var scDone <-chan struct{}
	var viewing sync.WaitGroup
	defer func() {
		// Whatever ends this attempt tells the owner, so its prompt or
		// session does not wait on a controller that has gone.
		_ = send(knock{Type: typeClose}.encode())
		close(stop)
		if sc != nil {
			_ = sc.Close()
		}
		viewing.Wait()
	}()

	until := time.Now().Add(nostrAskFor + 10*time.Second)
	sess := Session{State: StateConsentRequested}
	asking := func() {
		sess.ExpiresIn = int(time.Until(until).Truncate(time.Second) / time.Second)
		emit(HelperEvent{Phase: HelperAsking, Session: sess})
	}
	asking()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-dropped:
			if ctx.Err() == nil {
				emit(HelperEvent{Phase: HelperFailed, Session: sess, Err: ErrUnavailable})
			}
			return
		case <-scDone:
			if ctx.Err() == nil {
				emit(HelperEvent{Phase: HelperEnded, Session: sess})
			}
			return
		case <-tick.C:
			if sc != nil {
				continue
			}
			if time.Now().After(until) {
				emit(HelperEvent{Phase: HelperExpired, Session: sess})
				return
			}
			asking()
		case raw := <-msgs:
			var head knock
			if json.Unmarshal(raw, &head) != nil {
				continue
			}
			switch {
			case head.Type == TypeSessionEnd:
				emit(HelperEvent{Phase: HelperEnded, Session: sess})
				return
			case head.Type == TypeConsentAnswer && sc == nil:
				switch {
				case head.Granted && head.SessionID != "":
					sess = Session{ID: head.SessionID, State: StateConsentGranted}
					emit(HelperEvent{Phase: HelperAllowed, Session: sess})
					if c.View == nil {
						continue
					}
					cn := newSessionConn(head.SessionID, nil, send)
					cn.Nostr = true
					sc, scDone = cn, cn.Done()
					viewing.Add(1)
					go func() {
						defer viewing.Done()
						defer func() { _ = cn.Close() }()
						c.View(ctx, cn)
					}()
				case head.Reason == reasonBusy:
					emit(HelperEvent{Phase: HelperFailed, Session: sess, Err: ErrBusy})
					return
				case head.Reason == reasonExpired:
					emit(HelperEvent{Phase: HelperExpired, Session: sess})
					return
				default:
					emit(HelperEvent{Phase: HelperDeclined, Session: sess})
					return
				}
			case head.Type == TypeAskConsent || head.Type == TypeConsentAnswer:
				// Not for remotekit.
			case sc != nil:
				// The owner's runner addressed this to the key this end
				// dials from; the viewer knows itself by another name.
				if m, err := relabel(raw, ""); err == nil {
					sc.deliver(m)
				}
			}
		}
	}
}

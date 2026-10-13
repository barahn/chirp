// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/remotekit/bark"
	"github.com/barahn/remotekit/rendezvous"
	"github.com/gorilla/websocket"
)

// Server exposes the hub and the session manager over HTTP.
//
// It is the whole control surface of a Chirp deployment, in the direction of
// docs/product-model.md:
//
//   - A running Chirp registers as a peer and gets its code and share link
//     (POST /api/peers). It then holds its owner socket open
//     (GET /ws?role=owner&peer=ID) to hear about controllers, and answers
//     their consent requests (POST /api/peers/{peer}/consent).
//   - Another Chirp redeems that code (POST /api/redeem), or mints a code from
//     the link and redeems it in one step (POST /api/links/{token}/sessions),
//     and joins as controller (GET /ws?role=controller&session=ID).
//
// There are no accounts. The peer ID is the owner's credential and is returned
// only to the owner; a live code, or a link, is the controller's. Every entry
// point that takes a code or a link from the network is metered per origin by
// remotekit's rendezvous registry, and registration is capped per origin by
// the session manager, since every registration holds a code.
type Server struct {
	hub      *Hub
	sessions *session.SessionManager
	upgrader websocket.Upgrader

	// ClientIP names the origin a request's code guesses, open sessions and
	// registrations are counted against. Nil uses the connection's remote
	// address.
	//
	// Behind a reverse proxy every request arrives from the proxy, so all users
	// share one budget and throttle each other. Set this to read the proxy's
	// forwarded-for header -- but only when that proxy is the sole way in,
	// because a client that can reach this server directly can write the header
	// itself and choose a fresh origin for every guess.
	ClientIP func(*http.Request) string

	// ControllerGrace is how long a session waits for a controller whose
	// socket dropped to dial back before the relay ends it. Until then the
	// owner stays busy, so a dropped controller can resume; after, the owner
	// is free for the next one. Zero means DefaultControllerGrace.
	ControllerGrace time.Duration

	// PingInterval is how often the relay pings each socket. A socket that
	// answers none of the pings for two intervals is closed, so a peer that
	// vanished without closing is noticed; and the pings keep the socket from
	// looking idle to a reverse proxy that closes idle connections. Zero
	// means DefaultPingInterval, and anything over MaxPingInterval is taken
	// as MaxPingInterval: a Chirp gives up on a relay that has not pinged for
	// client.PingWait.
	PingInterval time.Duration

	ownersMu sync.Mutex
	owners   map[rendezvous.PeerID]*ownerConn

	// graces holds, per session, the pending end of a controller that
	// left; see holdSession and scheduleEnd.
	graceMu sync.Mutex
	graces  map[string]*graceEnd
}

// graceEnd is a session's pending end: when it is due, and the timer that
// carries it out.
type graceEnd struct {
	due   time.Time
	timer *time.Timer
}

// ownerConn is an owner's socket and the session it is joined to, if any.
type ownerConn struct {
	conn    *wsConn
	session string
}

func (s *Server) clientIP(r *http.Request) string {
	if s.ClientIP != nil {
		return s.ClientIP(r)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// NewServer returns a Server over sm, routing through its own hub.
func NewServer(sm *session.SessionManager) *Server {
	return &Server{
		hub:      NewHub(sm),
		sessions: sm,
		owners:   make(map[rendezvous.PeerID]*ownerConn),
		graces:   make(map[string]*graceEnd),
		upgrader: websocket.Upgrader{
			HandshakeTimeout: 10 * time.Second,
			// Same-origin checking is left off because Chirp is a binary, not
			// a page: it has no origin, and CheckOrigin's default would reject
			// it. The peer ID or the code is the credential; an origin header
			// is not one.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// Hub exposes the underlying hub, for a caller that needs to inspect or close it.
func (s *Server) Hub() *Hub { return s.hub }

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/peers", s.registerPeer)
	mux.HandleFunc("GET /api/peers/{peer}", s.getPeer)
	mux.HandleFunc("DELETE /api/peers/{peer}", s.releasePeer)
	mux.HandleFunc("POST /api/peers/{peer}/consent", s.submitConsent)
	mux.HandleFunc("POST /api/redeem", s.redeem)
	mux.HandleFunc("POST /api/links/{token}/sessions", s.redeemLink)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("PUT /api/peers/{peer}/contact", s.setContact)
	mux.HandleFunc("POST /api/lookup", s.lookup)
	mux.HandleFunc("POST /api/links/{token}/lookup", s.lookupLink)
	mux.HandleFunc("GET /ws", s.serveWS)
	return mux
}

type peerView struct {
	PeerID    string `json:"peer_id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Formatted string `json:"formatted_code"`
	Link      string `json:"link"`
}

func viewOfPeer(p *session.Peer) peerView {
	return peerView{
		PeerID:    string(p.ID),
		Name:      p.Name,
		Code:      string(p.Code),
		Formatted: p.Code.Format(),
		Link:      string(p.Link),
	}
}

// registerPeer is what a Chirp calls when it starts: it gets its code, its
// link, and the peer ID that is its credential from then on.
func (s *Server) registerPeer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	// An empty body is fine: nobody registers, so a name is optional.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	p, err := s.sessions.RegisterPeer(req.Name, s.clientIP(r))
	if errors.Is(err, session.ErrTooManyPeers) {
		writeSessionError(w, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "could not allocate a code")
		return
	}
	writeJSON(w, http.StatusCreated, viewOfPeer(p))
}

// getPeer returns the code the owner should be showing now: a used or expired
// start code has already been replaced.
func (s *Server) getPeer(w http.ResponseWriter, r *http.Request) {
	p, err := s.sessions.Peer(rendezvous.PeerID(r.PathValue("peer")))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOfPeer(p))
}

// releasePeer is what a Chirp calls when it exits: its codes and link stop
// working, and a session in progress ends.
func (s *Server) releasePeer(w http.ResponseWriter, r *http.Request) {
	id := rendezvous.PeerID(r.PathValue("peer"))
	if err := s.sessions.ReleasePeer(id); err != nil {
		writeSessionError(w, err)
		return
	}
	s.ownersMu.Lock()
	oc := s.owners[id]
	delete(s.owners, id)
	s.ownersMu.Unlock()
	if oc != nil {
		_ = oc.conn.Close()
	}
	w.WriteHeader(http.StatusNoContent)
}

type sessionView struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	OwnerName string `json:"owner_name"`
	ExpiresIn int    `json:"expires_in_seconds"`
}

func viewOf(sess *session.Session) sessionView {
	return sessionView{
		SessionID: sess.ID,
		State:     string(sess.State),
		OwnerName: sess.OwnerName,
		ExpiresIn: int(sess.RemainingTTL().Seconds()),
	}
}

type redeemRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// redeem consumes a code typed by the controller and opens a session with the
// code's owner. It is metered: every failure counts against the caller's
// origin.
func (s *Server) redeem(w http.ResponseWriter, r *http.Request) {
	var req redeemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	s.openSession(w, r, req.Code, req.Name)
}

// redeemLink mints a single-use code from a shared link and redeems it at
// once, so a link opens exactly one session per use. Minting is metered from
// the same budget as redeeming.
func (s *Server) redeemLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	origin := s.clientIP(r)
	code, err := s.sessions.MintFromLink(rendezvous.Token(r.PathValue("token")), origin)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	s.openSession(w, r, string(code), req.Name)
}

func (s *Server) openSession(w http.ResponseWriter, r *http.Request, code, name string) {
	sess, err := s.sessions.Redeem(code, name, &session.ClientInfo{
		RemoteIP:  s.clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeSessionError(w, err)
		return
	}
	// The owner may already be listening; if it is not, it joins the session
	// when its socket arrives.
	s.attachOwner(sess.Owner)
	writeJSON(w, http.StatusCreated, viewOf(sess))
}

// getSession lets either end follow a session by its ID, which only the two
// ends hold. It returns nothing about the controller.
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessions.GetSession(r.PathValue("id"))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(sess))
}

// submitConsent records the owner's answer to the prompt. The body is
// remotekit's bark.ConsentResponsePayload; the peer ID in the path is the
// owner's credential, and the answer applies only to the session that peer is
// in, so a controller cannot consent on the owner's behalf.
func (s *Server) submitConsent(w http.ResponseWriter, r *http.Request) {
	var req bark.ConsentResponsePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	active, err := s.sessions.ActiveSession(rendezvous.PeerID(r.PathValue("peer")))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	if req.SessionID != active.ID {
		// An answer to a prompt that is no longer the current one must not
		// land on the session that replaced it.
		writeError(w, http.StatusConflict, "that session is not this peer's current session")
		return
	}
	sess, err := s.sessions.SubmitConsent(active.ID, req.Granted, req.Reason)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(sess))
}

// wsConn adapts a WebSocket to the hub's Conn.
//
// gorilla permits one concurrent writer, and the hub calls Send from the other
// peer's goroutine, so writes are serialised here rather than left to chance.
// Pings take the same lock: gorilla would allow them alongside a Send, but one
// rule for every write is easier to keep than an exception.
type wsConn struct {
	conn *websocket.Conn
	mu   chan struct{}

	done      chan struct{}
	closeOnce sync.Once
}

func newWSConn(c *websocket.Conn) *wsConn {
	return &wsConn{conn: c, mu: make(chan struct{}, 1), done: make(chan struct{})}
}

// writeWait bounds one write, so a peer that stopped reading cannot hold the
// write lock.
const writeWait = 10 * time.Second

func (w *wsConn) Send(data []byte) error {
	w.mu <- struct{}{}
	defer func() { <-w.mu }()
	if err := w.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, data)
}

func (w *wsConn) Close() error {
	w.closeOnce.Do(func() { close(w.done) })
	return w.conn.Close()
}

// keepAlive pings the socket every interval until it is closed, and closes it
// when the other end stops answering: each pong pushes the read deadline two
// intervals out, so a socket that answers none for that long fails its read,
// and its handler cleans up as for any other departure.
//
// It is called before the socket's first read, from the goroutine that will
// read it, since the pong handler runs inside reads.
func (w *wsConn) keepAlive(interval time.Duration) {
	wait := 2 * interval
	_ = w.conn.SetReadDeadline(time.Now().Add(wait))
	w.conn.SetPongHandler(func(string) error {
		return w.conn.SetReadDeadline(time.Now().Add(wait))
	})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-w.done:
				return
			case <-t.C:
			}
			if w.ping() != nil {
				_ = w.Close()
				return
			}
		}
	}()
}

func (w *wsConn) ping() error {
	select {
	case w.mu <- struct{}{}:
	case <-w.done:
		return net.ErrClosed
	}
	defer func() { <-w.mu }()
	return w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
}

// serveWS upgrades and dispatches by role.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	switch Role(r.URL.Query().Get("role")) {
	case RoleOwner:
		s.serveOwner(w, r)
	case RoleController:
		s.serveController(w, r)
	default:
		writeError(w, http.StatusBadRequest, "role must be controller or owner")
	}
}

// serveOwner holds a peer's socket for as long as its Chirp runs. It is joined
// to each session the peer's code opens, in turn, and what it sends is
// forwarded to the controller of the current one.
func (s *Server) serveOwner(w http.ResponseWriter, r *http.Request) {
	id := rendezvous.PeerID(r.URL.Query().Get("peer"))
	// Over the peer cap the peer has no code to show, but it still exists and
	// its link still works, so it may hold its socket.
	if _, err := s.sessions.Peer(id); err != nil && !errors.Is(err, session.ErrTooManyPeers) {
		writeSessionError(w, err)
		return
	}
	s.ownersMu.Lock()
	_, taken := s.owners[id]
	s.ownersMu.Unlock()
	if taken {
		writeError(w, http.StatusConflict, ErrRoleTaken.Error())
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade has already written a response.
	}
	conn.SetReadLimit(MaxMessageSize)
	peer := newWSConn(conn)
	peer.keepAlive(s.pingInterval())

	s.ownersMu.Lock()
	if _, taken := s.owners[id]; taken {
		s.ownersMu.Unlock()
		_ = peer.Send(encodeError(ErrRoleTaken))
		_ = peer.Close()
		return
	}
	s.owners[id] = &ownerConn{conn: peer}
	s.ownersMu.Unlock()

	defer func() {
		s.ownersMu.Lock()
		oc := s.owners[id]
		var sessionID string
		ours := oc != nil && oc.conn == peer
		if ours {
			sessionID = oc.session
			delete(s.owners, id)
		}
		s.ownersMu.Unlock()
		if ours {
			// The grace period starts now: a Chirp that does not come back
			// within it is released, and its link stops working.
			s.sessions.OwnerDetached(id)
		}
		if sessionID != "" {
			s.hub.Leave(sessionID, RoleOwner)
		}
		_ = peer.Close()
	}()

	// The peer may have been released between the check above and now, by
	// the reaper or by DELETE; a socket for a peer that is gone is closed.
	if s.sessions.OwnerAttached(id) != nil {
		return
	}

	// A controller may have redeemed the code before this socket arrived.
	s.attachOwner(id)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		s.ownersMu.Lock()
		var sessionID string
		if oc := s.owners[id]; oc != nil {
			sessionID = oc.session
		}
		s.ownersMu.Unlock()
		if sessionID == "" {
			if peer.Send(encodeError(ErrNoPeer)) != nil {
				return
			}
			continue
		}
		if err := s.hub.Forward(sessionID, RoleOwner, data); err != nil {
			if peer.Send(encodeError(err)) != nil {
				return
			}
		}
	}
}

// attachOwner joins the owner's socket, if it is connected, to the session its
// code opened, leaving the one before. It is called from both sides -- when a
// session opens and when the socket arrives -- because either can come first.
func (s *Server) attachOwner(id rendezvous.PeerID) {
	sess, err := s.sessions.ActiveSession(id)
	if err != nil {
		return
	}
	s.ownersMu.Lock()
	oc := s.owners[id]
	if oc == nil || oc.session == sess.ID {
		s.ownersMu.Unlock()
		return
	}
	previous := oc.session
	oc.session = sess.ID
	conn := oc.conn
	s.ownersMu.Unlock()

	if previous != "" {
		s.hub.Leave(previous, RoleOwner)
	}
	if err := s.hub.Join(sess.ID, RoleOwner, conn); err != nil {
		_ = conn.Send(encodeError(err))
		return
	}
	s.maybeAskConsent(sess.ID)
}

// serveController joins the Chirp that redeemed a code to its session.
func (s *Server) serveController(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	if _, err := s.sessions.GetSession(id); err != nil {
		writeSessionError(w, err)
		return
	}
	// A controller dialling back holds the end its leaving scheduled, before
	// admission is decided; the session is read again after, since an end
	// that won the race has already made it inadmissible. If this attempt
	// never joins -- refused, or not a WebSocket at all -- the end goes back
	// as it was, due when it was due: a failed attempt must not buy the
	// session more time. Only a controller that joined schedules a new one,
	// when it leaves.
	held := s.holdSession(id)
	joined := false
	defer func() {
		switch {
		case joined:
			s.scheduleEnd(id, time.Now().Add(s.controllerGrace()))
		case held != nil:
			s.scheduleEnd(id, held.due)
		}
	}()
	sess, err := s.sessions.GetSession(id)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	// Admission is decided before the upgrade. Rejecting after it would send
	// the reason down a socket the client has to interpret, instead of as a
	// status code it already knows how to read.
	if err := Admissible(sess); err != nil {
		writeSessionError(w, err)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(MaxMessageSize)
	peer := newWSConn(conn)
	peer.keepAlive(s.pingInterval())
	if err := s.hub.Join(sess.ID, RoleController, peer); err != nil {
		_ = peer.Send(encodeError(err))
		_ = peer.Close()
		return
	}
	joined = true
	defer func() {
		s.hub.Leave(sess.ID, RoleController)
		_ = peer.Close()
	}()

	s.maybeAskConsent(sess.ID)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := s.hub.Forward(sess.ID, RoleController, data); err != nil {
			if sendErr := peer.Send(encodeError(err)); sendErr != nil {
				return
			}
		}
	}
}

// MaxMessageSize bounds one message from either end, in bytes. A larger one
// closes the socket that sent it, before the relay reads it into memory or
// parses it (Hub.Forward reads every message's type). It matches what a Chirp
// client accepts (client.Socket), so the relay never forwards a message the
// other end would refuse anyway; signalling itself is a few kilobytes.
const MaxMessageSize = 1 << 20

// DefaultControllerGrace is ControllerGrace's default.
const DefaultControllerGrace = 30 * time.Second

// DefaultPingInterval is PingInterval's default. It is well under the idle
// timeouts proxies are known for -- Cloudflare's is about 100 s -- and a
// third of what a Chirp waits between pings before it gives up.
const DefaultPingInterval = 30 * time.Second

// MaxPingInterval caps PingInterval. A Chirp waits client.PingWait for each
// ping before it gives the relay up, so a relay pinging less often would drop
// healthy clients; the cap leaves that wait room for two pings to go missing.
const MaxPingInterval = 40 * time.Second

// pingInterval is PingInterval, or its default, capped at MaxPingInterval.
func (s *Server) pingInterval() time.Duration {
	switch {
	case s.PingInterval <= 0:
		return DefaultPingInterval
	case s.PingInterval > MaxPingInterval:
		return MaxPingInterval
	}
	return s.PingInterval
}

// controllerGrace is ControllerGrace, or its default.
func (s *Server) controllerGrace() time.Duration {
	if s.ControllerGrace > 0 {
		return s.ControllerGrace
	}
	return DefaultControllerGrace
}

// holdSession takes sessionID's pending end, if a controller that left
// scheduled one, and stops it. The caller puts it back with scheduleEnd if it
// turns out not to be needed.
func (s *Server) holdSession(sessionID string) *graceEnd {
	s.graceMu.Lock()
	defer s.graceMu.Unlock()
	e, ok := s.graces[sessionID]
	if !ok {
		return nil
	}
	e.timer.Stop()
	delete(s.graces, sessionID)
	return e
}

// scheduleEnd ends sessionID at due unless a controller is connected then.
// Of two ends, the later is kept: the grace runs from the latest departure,
// so an earlier one is always stale -- an attempt that never joined putting
// back the end it held, after a controller came and went meanwhile, must not
// cut that controller's grace short. Nothing else ends a session its owner
// has not given up: without this, a controller that left an allowed session
// kept its owner busy until the session expired, and every other controller
// was told the owner was already being helped.
//
// The end runs under graceMu and only if it is still the pending one, so a
// controller dialling back -- whose holdSession takes the same lock before
// admission is decided -- either takes it first or finds the session over.
func (s *Server) scheduleEnd(sessionID string, due time.Time) {
	s.graceMu.Lock()
	defer s.graceMu.Unlock()
	if old, ok := s.graces[sessionID]; ok {
		if !due.After(old.due) {
			return
		}
		old.timer.Stop()
	}
	e := &graceEnd{due: due}
	e.timer = time.AfterFunc(time.Until(due), func() {
		s.graceMu.Lock()
		defer s.graceMu.Unlock()
		if s.graces[sessionID] != e {
			return // replaced or taken
		}
		delete(s.graces, sessionID)
		if controller, _ := s.hub.Connected(sessionID); controller {
			return // a controller is connected
		}
		_, _ = s.sessions.EndSession(sessionID)
	})
	s.graces[sessionID] = e
}

// maybeAskConsent asks the owner for consent once both ends are in the hub,
// and not before: a prompt with nobody behind it is a prompt the owner cannot
// judge. Whichever end arrives second triggers it; a session already past
// pending -- a reconnection -- is left alone.
func (s *Server) maybeAskConsent(sessionID string) {
	controller, owner := s.hub.Connected(sessionID)
	if !controller || !owner {
		return
	}
	if _, err := s.sessions.Connect(sessionID); err != nil {
		return // Another arrival moved it on, or it has ended.
	}
	sess, err := s.sessions.RequestConsent(sessionID)
	if err != nil {
		return
	}
	req := bark.ConsentRequestPayload{
		SessionID:            sess.ID,
		Code:                 string(sess.Code),
		TechnicianName:       sess.ControllerName,
		RequestedPermissions: append([]string(nil), session.DefaultPermissions...),
		Timestamp:            time.Now().UTC(),
	}
	_ = s.hub.Notify(sessionID, RoleOwner, encodeConsentRequest(req))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("chirp/signalling: writing response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeSessionError maps the session layer's errors onto status codes.
//
// A wrong code and an expired one are both 404 on purpose. rendezvous tells
// them apart, but passing that on would confirm to someone guessing that a
// code once existed, and six digits is a small space to guess in.
func writeSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, session.ErrTooManySessions):
		writeError(w, http.StatusTooManyRequests, "too many open sessions, end one first")
	case errors.Is(err, session.ErrTooManyPeers):
		writeError(w, http.StatusTooManyRequests, "too many Chirps registered from this address, try again later")
	case errors.Is(err, session.ErrThrottled):
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
	case errors.Is(err, session.ErrCodeNotFound), errors.Is(err, session.ErrCodeExpired),
		errors.Is(err, session.ErrMalformed):
		writeError(w, http.StatusNotFound, "no such code")
	case errors.Is(err, session.ErrSessionNotFound), errors.Is(err, session.ErrSessionExpired):
		writeError(w, http.StatusNotFound, "no such session")
	case errors.Is(err, session.ErrPeerNotFound):
		writeError(w, http.StatusNotFound, "no such peer")
	case errors.Is(err, ErrNotPermitted), errors.Is(err, session.ErrInvalidState),
		errors.Is(err, session.ErrConsentAlreadySet), errors.Is(err, session.ErrSessionNotPending),
		errors.Is(err, session.ErrPeerBusy), errors.Is(err, ErrRoleTaken):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/gorilla/websocket"
)

// Server exposes the hub and the session manager over HTTP.
//
// It is the whole control surface of a Chirp deployment: create a session,
// read its state, answer the consent prompt, and signal. There is no tenancy
// and no authentication on the session endpoints, which is deliberate --
// possession of an unexpired six-digit code is the credential, and that is
// what the product promises. Anything more would mean accounts, which Chirp
// does not have.
type Server struct {
	hub      *Hub
	sessions *session.SessionManager
	upgrader websocket.Upgrader

	// DefaultTTL is how long a newly created session lives. Zero uses the
	// session manager's own default.
	DefaultTTL time.Duration
}

// NewServer returns a Server over sm, routing through its own hub.
func NewServer(sm *session.SessionManager) *Server {
	return &Server{
		hub:      NewHub(sm),
		sessions: sm,
		upgrader: websocket.Upgrader{
			HandshakeTimeout: 10 * time.Second,
			// Same-origin checking is left off because the technician's viewer
			// and the agent binary are not served from this origin: the agent
			// has no origin at all, and CheckOrigin's default would reject it.
			// The code is the credential; an origin header is not one.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// Hub exposes the underlying hub, for a caller that needs to inspect or close it.
func (s *Server) Hub() *Hub { return s.hub }

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions", s.createSession)
	mux.HandleFunc("GET /api/sessions/{code}", s.getSession)
	mux.HandleFunc("POST /api/sessions/{code}/consent", s.submitConsent)
	mux.HandleFunc("GET /ws", s.serveWS)
	return mux
}

type createResponse struct {
	SessionID string `json:"session_id"`
	Code      string `json:"code"`
	Formatted string `json:"formatted_code"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TechnicianID   string `json:"technician_id"`
		TechnicianName string `json:"technician_name"`
	}
	// An empty body is fine: a technician who has no identity system still
	// needs to open a session.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	sess, err := s.sessions.CreateSession(req.TechnicianID, req.TechnicianName, s.DefaultTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	writeJSON(w, http.StatusCreated, createResponse{
		SessionID: sess.ID,
		Code:      sess.Code,
		Formatted: session.FormatCode(sess.Code),
		ExpiresAt: sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

type sessionView struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	ExpiresIn int    `json:"expires_in_seconds"`
}

// getSession lets the join page follow a session without holding a socket
// open: the page polls this between the code being accepted and the agent
// reporting in.
//
// It answers by code, not by session id, because the code is what the person
// on that page has. It deliberately returns no technician identity, no client
// hostname and no consent reason -- everything it does return, the caller
// already knew or is waiting on.
func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.lookup(r.PathValue("code"))
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionView{
		SessionID: sess.ID,
		State:     string(sess.State),
		ExpiresIn: int(sess.RemainingTTL().Seconds()),
	})
}

func (s *Server) submitConsent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Granted bool   `json:"granted"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	code, err := session.NormalizeCode(r.PathValue("code"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "malformed code")
		return
	}
	sess, err := s.sessions.SubmitConsent(code, req.Granted, req.Reason)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionView{
		SessionID: sess.ID,
		State:     string(sess.State),
		ExpiresIn: int(sess.RemainingTTL().Seconds()),
	})
}

// wsConn adapts a WebSocket to the hub's Conn.
//
// gorilla permits one concurrent writer, and the hub calls Send from the other
// peer's goroutine, so writes are serialised here rather than left to chance.
type wsConn struct {
	conn *websocket.Conn
	mu   chan struct{}
}

func newWSConn(c *websocket.Conn) *wsConn {
	return &wsConn{conn: c, mu: make(chan struct{}, 1)}
}

func (w *wsConn) Send(data []byte) error {
	w.mu <- struct{}{}
	defer func() { <-w.mu }()
	if err := w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, data)
}

func (w *wsConn) Close() error { return w.conn.Close() }

// serveWS upgrades and then pumps messages from this peer to the other one
// until the socket closes.
//
// The technician joins with the session id it got when it created the session;
// the agent joins with the code the person typed, because that is all it has.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	role := Role(r.URL.Query().Get("role"))
	if role != RoleTechnician && role != RoleAgent {
		writeError(w, http.StatusBadRequest, "role must be technician or agent")
		return
	}

	var sess *session.Session
	var err error
	if code := r.URL.Query().Get("code"); code != "" {
		sess, err = s.lookup(code)
	} else if id := r.URL.Query().Get("session"); id != "" {
		sess, err = s.sessions.GetSession(id)
	} else {
		writeError(w, http.StatusBadRequest, "code or session is required")
		return
	}
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
		return // Upgrade has already written a response.
	}
	peer := newWSConn(conn)
	if err := s.hub.Join(sess.ID, role, peer); err != nil {
		_ = peer.Send(encodeError(err))
		_ = peer.Close()
		return
	}
	defer func() {
		s.hub.Leave(sess.ID, role)
		_ = peer.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return // Peer went away; Leave runs in the defer.
		}
		if err := s.hub.Forward(sess.ID, role, data); err != nil {
			// The other end not being connected yet is the ordinary case
			// while one side waits, so tell this peer and keep the socket.
			if sendErr := peer.Send(encodeError(err)); sendErr != nil {
				return
			}
		}
	}
}

// lookup resolves a typed code to its session, normalising the formatting a
// person may have copied along with it.
func (s *Server) lookup(raw string) (*session.Session, error) {
	code, err := session.NormalizeCode(raw)
	if err != nil {
		return nil, err
	}
	return s.sessions.GetSessionByCode(code)
}

func encodeError(err error) []byte {
	b, _ := json.Marshal(map[string]string{"type": "error", "error": err.Error()})
	return b
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
// A wrong code and an expired one are both 404 on purpose. Distinguishing them
// would confirm that a code exists to someone guessing, and six digits is a
// small space to guess in.
func writeSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, session.ErrSessionNotFound), errors.Is(err, session.ErrSessionExpired):
		writeError(w, http.StatusNotFound, "no such session")
	case errors.Is(err, ErrNotPermitted), errors.Is(err, session.ErrInvalidState),
		errors.Is(err, session.ErrConsentAlreadySet), errors.Is(err, session.ErrSessionNotConnected),
		errors.Is(err, session.ErrSessionNotPending):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrRoleTaken):
		writeError(w, http.StatusConflict, err.Error())
	case strings.Contains(err.Error(), "code"):
		writeError(w, http.StatusBadRequest, "malformed code")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

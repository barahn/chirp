// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/gorilla/websocket"
)

// newServer returns a running httptest server over a real SessionManager, so
// the tests exercise the same admission rules production does.
func newServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	sm := session.NewSessionManager(10 * time.Minute)
	srv := NewServer(sm)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = srv.Hub().Close()
		sm.Close()
	})
	return srv, ts
}

func create(t *testing.T, ts *httptest.Server) createResponse {
	t.Helper()
	resp, err := ts.Client().Post(ts.URL+"/api/sessions", "application/json",
		strings.NewReader(`{"technician_id":"t-1","technician_name":"Ana"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var out createResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// dial opens a signalling socket. q is the query that identifies the peer:
// the technician has a session id, the agent has only the typed code.
func dial(t *testing.T, ts *httptest.Server, q url.Values) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?" + q.Encode()
	c, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if c != nil {
		t.Cleanup(func() { _ = c.Close() })
	}
	return c, resp, err
}

// waitJoined blocks until the hub has registered both ends of sessionID.
//
// A client's Dial returns when the server writes the upgrade response, and the
// server joins the peer to the hub only after that, so there is a window in
// which a peer considers itself connected and the hub does not yet know it. A
// message forwarded to it in that window is refused with ErrNoPeer -- the
// documented behaviour, see TestSendingBeforeTheOtherEndArrivesKeepsTheSocket.
// A test that means to exercise two present peers has to wait out the window,
// or it fails whenever the scheduler widens it.
func waitJoined(t *testing.T, srv *Server, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tech, agent := srv.Hub().Connected(sessionID); tech && agent {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("both peers dialled, but the hub never registered them")
}

func TestCreateSessionReturnsAUsableCode(t *testing.T) {
	_, ts := newServer(t)
	got := create(t, ts)

	if len(got.Code) != 6 {
		t.Errorf("code = %q, want six digits", got.Code)
	}
	if got.SessionID == "" {
		t.Error("no session id returned")
	}
	// The formatted form is what a technician reads aloud; it has to
	// normalise back to the code the agent will send.
	back, err := session.NormalizeCode(got.Formatted)
	if err != nil || back != got.Code {
		t.Errorf("formatted %q does not normalise back to %q (err %v)", got.Formatted, got.Code, err)
	}
}

func TestSignallingCrossesBetweenPeers(t *testing.T) {
	srv, ts := newServer(t)
	sess := create(t, ts)

	tech, _, err := dial(t, ts, url.Values{"role": {"technician"}, "session": {sess.SessionID}})
	if err != nil {
		t.Fatalf("technician dial: %v", err)
	}
	// The agent joins with the code, because that is all the person typed.
	agent, _, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {sess.Formatted}})
	if err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	waitJoined(t, srv, sess.SessionID)

	offer := `{"type":"offer","sdp":"v=0"}`
	if err := tech.WriteMessage(websocket.TextMessage, []byte(offer)); err != nil {
		t.Fatalf("write offer: %v", err)
	}
	_ = agent.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, got, err := agent.ReadMessage()
	if err != nil {
		t.Fatalf("agent read: %v", err)
	}
	if string(got) != offer {
		t.Errorf("agent got %q, want the offer unchanged", got)
	}

	answer := `{"type":"answer","sdp":"v=0"}`
	if err := agent.WriteMessage(websocket.TextMessage, []byte(answer)); err != nil {
		t.Fatalf("write answer: %v", err)
	}
	_ = tech.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, got, err = tech.ReadMessage()
	if err != nil {
		t.Fatalf("technician read: %v", err)
	}
	if string(got) != answer {
		t.Errorf("technician got %q, want the answer unchanged", got)
	}
}

func TestSendingBeforeTheOtherEndArrivesKeepsTheSocket(t *testing.T) {
	_, ts := newServer(t)
	sess := create(t, ts)

	tech, _, err := dial(t, ts, url.Values{"role": {"technician"}, "session": {sess.SessionID}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := tech.WriteMessage(websocket.TextMessage, []byte(`{"type":"offer"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// A technician who opens the viewer before the person has run the binary
	// must be told, and must not be disconnected for it.
	_ = tech.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := tech.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg map[string]string
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg["type"] != RelayErrorType || !strings.Contains(msg["error"], "other end") {
		t.Errorf("got %v, want an error naming the missing peer", msg)
	}
	if err := tech.WriteMessage(websocket.TextMessage, []byte(`{"type":"offer"}`)); err != nil {
		t.Errorf("socket did not survive the error: %v", err)
	}
}

func TestUnknownAndExpiredCodesAreIndistinguishable(t *testing.T) {
	_, ts := newServer(t)

	resp, err := ts.Client().Get(ts.URL + "/api/sessions/000000")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Six digits is a small space to guess in, so a wrong code must not be
	// distinguishable from one that existed and ran out.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if strings.Contains(strings.ToLower(body["error"]), "expire") {
		t.Errorf("error %q reveals whether the code existed", body["error"])
	}
}

func TestConsentDenialIsRecorded(t *testing.T) {
	srv, ts := newServer(t)
	sess := create(t, ts)

	if _, err := srv.sessions.ConnectClient(sess.Code, &session.ClientInfo{Hostname: "kitchen-pc"}); err != nil {
		t.Fatalf("ConnectClient: %v", err)
	}
	if _, err := srv.sessions.RequestConsent(sess.Code); err != nil {
		t.Fatalf("RequestConsent: %v", err)
	}

	resp, err := ts.Client().Post(ts.URL+"/api/sessions/"+sess.Code+"/consent",
		"application/json", bytes.NewBufferString(`{"granted":false,"reason":"not now"}`))
	if err != nil {
		t.Fatalf("consent: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var view sessionView
	_ = json.NewDecoder(resp.Body).Decode(&view)
	if view.State != string(session.StateConsentDenied) {
		t.Errorf("state = %q, want consent_denied", view.State)
	}

	// Refusal has to be final: a denied session must not accept peers
	// afterwards, or "no" becomes "no, until you retry".
	if _, _, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {sess.Code}}); err == nil {
		t.Error("agent joined a session whose consent was denied")
	}
}

func TestRoleIsRequired(t *testing.T) {
	_, ts := newServer(t)
	sess := create(t, ts)

	for _, role := range []string{"", "admin", "Technician"} {
		q := url.Values{"session": {sess.SessionID}}
		if role != "" {
			q.Set("role", role)
		}
		_, resp, err := dial(t, ts, q)
		if err == nil {
			t.Errorf("role %q was accepted", role)
			continue
		}
		if resp != nil && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("role %q gave status %d, want 400", role, resp.StatusCode)
		}
	}
}

// get issues a GET for a code, as if from addr.
func getFrom(h http.Handler, code, addr string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+code, nil)
	req.RemoteAddr = addr
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

// The HTTP surface must not be an unmetered oracle for which codes are live.
// It was, when the session layer's throttle lived only in ConnectClient: fifty
// wrong guesses from one address all came back 404, and the fifty-first, a
// live code, came back 200.
func TestCodeLookupsAreThrottled(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(3, time.Minute)
	h := NewServer(sm).Handler()

	for i := 0; i < 3; i++ {
		if got := getFrom(h, "000000", "203.0.113.7:4000"); got != http.StatusNotFound {
			t.Fatalf("guess %d: status %d, want 404", i+1, got)
		}
	}
	live, err := sm.CreateSession("tech", "Tech", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if got := getFrom(h, live.Code, "203.0.113.7:4001"); got != http.StatusTooManyRequests {
		t.Fatalf("a throttled address resolved a live code: status %d, want 429", got)
	}
	// The port is not part of the origin; the address is.
	if got := getFrom(h, live.Code, "198.51.100.9:4000"); got != http.StatusOK {
		t.Fatalf("an unrelated address was throttled: status %d", got)
	}
}

// Consent submitted against a guessed code is the worst thing a guess could
// buy, so the consent endpoint is metered too -- and it must refuse a session
// that never asked.
func TestConsentEndpointIsMeteredAndNeedsARequest(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(2, time.Minute)
	h := NewServer(sm).Handler()

	post := func(code, addr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+code+"/consent",
			strings.NewReader(`{"granted":true}`))
		req.RemoteAddr = addr
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	pending, err := sm.CreateSession("tech", "Tech", time.Minute)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if got := post(pending.Code, "192.0.2.10:1"); got != http.StatusConflict {
		t.Fatalf("consent on a pending session: status %d, want 409", got)
	}
	if s, _ := sm.GetSession(pending.ID); s.State != session.StatePending {
		t.Fatalf("consent was recorded on a pending session: state %s", s.State)
	}

	for i := 0; i < 2; i++ {
		_ = post("000000", "192.0.2.20:1")
	}
	if got := post(pending.Code, "192.0.2.20:1"); got != http.StatusTooManyRequests {
		t.Fatalf("consent guesses were not metered: status %d, want 429", got)
	}
}

// Behind a reverse proxy the operator names the origin. The hook must be
// what the throttle counts against.
func TestClientIPHookSetsTheOrigin(t *testing.T) {
	sm := session.NewSessionManager(time.Minute)
	defer sm.Close()
	sm.SetRedemptionLimit(1, time.Minute)
	srv := NewServer(sm)
	srv.ClientIP = func(r *http.Request) string { return r.Header.Get("X-Test-Origin") }
	h := srv.Handler()

	req := func(origin string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/sessions/000000", nil)
		r.RemoteAddr = "10.0.0.1:1" // the proxy: identical for everyone
		r.Header.Set("X-Test-Origin", origin)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Code
	}

	_ = req("user-a")
	if got := req("user-a"); got != http.StatusTooManyRequests {
		t.Fatalf("origin from the hook was not throttled: status %d", got)
	}
	if got := req("user-b"); got != http.StatusNotFound {
		t.Fatalf("users behind one proxy throttled each other: status %d", got)
	}
}

// waitFor polls until cond holds, failing the test after five seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The agent taking its place is the moment the join page waits for: the
// session records where the agent is and moves on to asking for consent.
func TestAgentArrivalConnectsAndRequestsConsent(t *testing.T) {
	srv, ts := newServer(t)
	sess := create(t, ts)

	if _, _, err := dial(t, ts, url.Values{"role": {"technician"}, "session": {sess.SessionID}}); err != nil {
		t.Fatalf("technician dial: %v", err)
	}
	waitFor(t, "technician to join", func() bool { tech, _ := srv.Hub().Connected(sess.SessionID); return tech })
	if got, _ := srv.sessions.GetSession(sess.SessionID); got.State != session.StatePending {
		t.Fatalf("the technician alone moved the session to %s", got.State)
	}

	if _, _, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {sess.Code}}); err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	waitFor(t, "session to request consent", func() bool {
		got, _ := srv.sessions.GetSession(sess.SessionID)
		return got.State == session.StateConsentRequested
	})

	got, _ := srv.sessions.GetSession(sess.SessionID)
	if got.ClientInfo == nil || got.ClientInfo.RemoteIP != "127.0.0.1" {
		t.Errorf("client info = %+v, want the agent's address recorded", got.ClientInfo)
	}
	if got.ConnectedAt == nil {
		t.Error("ConnectedAt was not recorded")
	}
}

// An agent that drops and dials back is reconnecting, not guessing. It must be
// let back in, the session must stay connected, and it must not be charged
// against the redemption budget -- with a budget of one, a charge would lock
// it out.
func TestAgentReconnectionIsNotAnError(t *testing.T) {
	srv, ts := newServer(t)
	srv.sessions.SetRedemptionLimit(1, time.Minute)
	sess := create(t, ts)

	first, _, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {sess.Code}})
	if err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	waitFor(t, "session to request consent", func() bool {
		got, _ := srv.sessions.GetSession(sess.SessionID)
		return got.State == session.StateConsentRequested
	})

	_ = first.Close()
	waitFor(t, "agent to leave the hub", func() bool { _, agent := srv.Hub().Connected(sess.SessionID); return !agent })

	for i := 0; i < 3; i++ {
		again, resp, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {sess.Code}})
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("reconnection %d refused: %v (status %d)", i+1, err, status)
		}
		waitFor(t, "agent to rejoin", func() bool { _, agent := srv.Hub().Connected(sess.SessionID); return agent })
		_ = again.Close()
		waitFor(t, "agent to leave again", func() bool { _, agent := srv.Hub().Connected(sess.SessionID); return !agent })
	}

	if got, _ := srv.sessions.GetSession(sess.SessionID); got.State != session.StateConsentRequested {
		t.Fatalf("reconnections moved the session to %s", got.State)
	}
}

// getView reads the join page's view of a session.
func getView(t *testing.T, ts *httptest.Server, code string) sessionView {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + "/api/sessions/" + code)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: status %d", resp.StatusCode)
	}
	var v sessionView
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// Consent given without knowing who asks, and for what, is not informed. The
// join page learns both once consent is requested -- and not before, when
// the caller is not yet waiting on them.
func TestJoinPageSeesTheConsentRequestOnlyWhenAsked(t *testing.T) {
	srv, ts := newServer(t)
	resp, err := ts.Client().Post(ts.URL+"/api/sessions", "application/json",
		bytes.NewBufferString(`{"technician_id":"t-1","technician_name":"Alice Technician"}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created createResponse
	_ = json.NewDecoder(resp.Body).Decode(&created)
	_ = resp.Body.Close()

	if v := getView(t, ts, created.Code); v.Consent != nil {
		t.Fatalf("a pending session disclosed its consent request: %+v", v.Consent)
	}

	if _, _, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {created.Code}}); err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	waitFor(t, "session to request consent", func() bool {
		got, _ := srv.sessions.GetSession(created.SessionID)
		return got.State == session.StateConsentRequested
	})

	v := getView(t, ts, created.Code)
	if v.Consent == nil {
		t.Fatal("the join page cannot see who is asking")
	}
	if v.Consent.TechnicianName != "Alice Technician" {
		t.Errorf("technician = %q", v.Consent.TechnicianName)
	}
	if len(v.Consent.RequestedPermissions) != len(session.DefaultPermissions) {
		t.Errorf("permissions = %v, want %v", v.Consent.RequestedPermissions, session.DefaultPermissions)
	}
}

// The whole state machine, driven only through the server: create, the agent
// arrives, the join page answers. Nothing here calls the session manager
// directly -- until this flow existed, no session could reach consent_granted
// over HTTP at all.
func TestConsentFlowEndToEnd(t *testing.T) {
	srv, ts := newServer(t)
	sess := create(t, ts)

	if _, _, err := dial(t, ts, url.Values{"role": {"agent"}, "code": {sess.Code}}); err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	waitFor(t, "the join page to see the request", func() bool {
		return getView(t, ts, sess.Code).State == string(session.StateConsentRequested)
	})

	resp, err := ts.Client().Post(ts.URL+"/api/sessions/"+sess.Code+"/consent",
		"application/json", bytes.NewBufferString(`{"granted":true}`))
	if err != nil {
		t.Fatalf("consent: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("consent: status %d, want 200", resp.StatusCode)
	}
	got, _ := srv.sessions.GetSession(sess.SessionID)
	if got.State != session.StateConsentGranted || got.Consent == nil || !got.Consent.Granted {
		t.Fatalf("state %s, consent %+v; want granted", got.State, got.Consent)
	}
}

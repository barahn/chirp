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
	_, ts := newServer(t)
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
	if msg["type"] != "error" || !strings.Contains(msg["error"], "other end") {
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

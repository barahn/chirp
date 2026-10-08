// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/remotekit/bark"
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

// call sends a JSON request and decodes a JSON response into out, if given.
func call(t *testing.T, ts *httptest.Server, method, path, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return resp.StatusCode
}

func register(t *testing.T, ts *httptest.Server, name string) peerView {
	t.Helper()
	var p peerView
	if st := call(t, ts, "POST", "/api/peers", fmt.Sprintf(`{"name":%q}`, name), &p); st != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", st)
	}
	return p
}

func redeem(t *testing.T, ts *httptest.Server, code, name string) (sessionView, int) {
	t.Helper()
	var v sessionView
	st := call(t, ts, "POST", "/api/redeem", fmt.Sprintf(`{"code":%q,"name":%q}`, code, name), &v)
	return v, st
}

func consent(t *testing.T, ts *httptest.Server, peer, sessionID string, granted bool) int {
	t.Helper()
	b, _ := json.Marshal(bark.ConsentResponsePayload{SessionID: sessionID, Granted: granted})
	return call(t, ts, "POST", "/api/peers/"+peer+"/consent", string(b), nil)
}

func dial(t *testing.T, ts *httptest.Server, q url.Values) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?" + q.Encode()
	c, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if c != nil {
		t.Cleanup(func() { _ = c.Close() })
	}
	return c, resp, err
}

func dialOwner(t *testing.T, ts *httptest.Server, peer string) *websocket.Conn {
	t.Helper()
	c, _, err := dial(t, ts, url.Values{"role": {"owner"}, "peer": {peer}})
	if err != nil {
		t.Fatalf("owner dial: %v", err)
	}
	return c
}

func dialController(t *testing.T, ts *httptest.Server, sessionID string) *websocket.Conn {
	t.Helper()
	c, _, err := dial(t, ts, url.Values{"role": {"controller"}, "session": {sessionID}})
	if err != nil {
		t.Fatalf("controller dial: %v", err)
	}
	return c
}

// read returns the next message on c as raw bytes and as a decoded object.
func read(t *testing.T, c *websocket.Conn) ([]byte, map[string]any) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return data, m
}

// awaitPeer reads until the relay announces the other end, which is what a
// real client does before sending. It fails on any other message first.
func awaitPeer(t *testing.T, c *websocket.Conn, want Role) {
	t.Helper()
	data, m := read(t, c)
	if m["type"] != PeerJoinedType || m["role"] != string(want) {
		t.Fatalf("got %s, want %s naming %s", data, PeerJoinedType, want)
	}
}

// awaitConsentRequest reads the consent request the owner's prompt is built
// from.
func awaitConsentRequest(t *testing.T, c *websocket.Conn) (bark.ConsentRequestPayload, bool) {
	t.Helper()
	data, _ := read(t, c)
	var msg struct {
		Type         string                     `json:"type"`
		Request      bark.ConsentRequestPayload `json:"request"`
		NameVerified bool                       `json:"name_verified"`
	}
	if err := json.Unmarshal(data, &msg); err != nil || msg.Type != ConsentRequestType {
		t.Fatalf("got %s, want %s", data, ConsentRequestType)
	}
	return msg.Request, msg.NameVerified
}

func state(t *testing.T, ts *httptest.Server, id string) string {
	t.Helper()
	var v sessionView
	if st := call(t, ts, "GET", "/api/sessions/"+id, "", &v); st != http.StatusOK {
		t.Fatalf("get session status = %d", st)
	}
	return v.State
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRegisterReturnsACodeALinkAndACredential(t *testing.T) {
	_, ts := newServer(t)
	p := register(t, ts, "Ana")
	if len(p.Code) != 6 || p.Link == "" || p.PeerID == "" {
		t.Fatalf("incomplete registration: %+v", p)
	}
	if p.Formatted != p.Code[:3]+"-"+p.Code[3:] {
		t.Fatalf("formatted %q does not match code %q", p.Formatted, p.Code)
	}
}

// The whole product flow, over HTTP and WebSocket only: the owner shows a
// code, the controller types it, the owner is asked and grants, and signals
// cross in both directions.
func TestConsentFlowEndToEnd(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	ownerWS := dialOwner(t, ts, owner.PeerID)

	sess, st := redeem(t, ts, owner.Formatted, "Bruno")
	if st != http.StatusCreated {
		t.Fatalf("redeem status = %d", st)
	}
	if sess.OwnerName != "Ana" {
		t.Fatalf("owner name %q", sess.OwnerName)
	}
	ctrlWS := dialController(t, ts, sess.SessionID)

	awaitPeer(t, ctrlWS, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
	req, verified := awaitConsentRequest(t, ownerWS)
	if req.SessionID != sess.SessionID || req.TechnicianName != "Bruno" || verified {
		t.Fatalf("consent request %+v (verified %v)", req, verified)
	}
	if len(req.RequestedPermissions) != len(session.DefaultPermissions) {
		t.Fatalf("permissions %v", req.RequestedPermissions)
	}
	if got := state(t, ts, sess.SessionID); got != string(session.StateConsentRequested) {
		t.Fatalf("state %s before the answer", got)
	}

	if st := consent(t, ts, owner.PeerID, sess.SessionID, true); st != http.StatusOK {
		t.Fatalf("consent status = %d", st)
	}
	if got := state(t, ts, sess.SessionID); got != string(session.StateConsentGranted) {
		t.Fatalf("state %s after granting", got)
	}

	offer := `{"type":"offer","sdp":"v=0"}`
	if err := ctrlWS.WriteMessage(websocket.TextMessage, []byte(offer)); err != nil {
		t.Fatal(err)
	}
	if data, _ := read(t, ownerWS); string(data) != offer {
		t.Fatalf("owner received %s, want the offer", data)
	}
	answer := `{"type":"answer","sdp":"v=0"}`
	if err := ownerWS.WriteMessage(websocket.TextMessage, []byte(answer)); err != nil {
		t.Fatal(err)
	}
	if data, _ := read(t, ctrlWS); string(data) != answer {
		t.Fatalf("controller received %s, want the answer", data)
	}
}

// The owner's socket may arrive after the code was redeemed; the session and
// the prompt still reach it.
func TestOwnerArrivingLateStillGetsThePrompt(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	sess, _ := redeem(t, ts, owner.Code, "Bruno")
	ctrlWS := dialController(t, ts, sess.SessionID)
	ownerWS := dialOwner(t, ts, owner.PeerID)

	awaitPeer(t, ctrlWS, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
	if req, _ := awaitConsentRequest(t, ownerWS); req.SessionID != sess.SessionID {
		t.Fatalf("prompt for %s, want %s", req.SessionID, sess.SessionID)
	}
}

func TestACodeOpensOneSessionOnly(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	if _, st := redeem(t, ts, owner.Code, "Bruno"); st != http.StatusCreated {
		t.Fatalf("first redeem = %d", st)
	}
	if _, st := redeem(t, ts, owner.Code, "Carla"); st != http.StatusNotFound {
		t.Fatalf("second redeem = %d, want 404", st)
	}
	var now peerView
	call(t, ts, "GET", "/api/peers/"+owner.PeerID, "", &now)
	if now.Code == owner.Code {
		t.Fatal("the used code is still the one shown")
	}
	// The owner is busy with Bruno, so its next code does not open a second
	// session over the first.
	if _, st := redeem(t, ts, now.Code, "Carla"); st != http.StatusConflict {
		t.Fatalf("redeem while busy = %d, want 409", st)
	}
}

func TestLinkOpensASession(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	var v sessionView
	if st := call(t, ts, "POST", "/api/links/"+owner.Link+"/sessions", `{"name":"Bruno"}`, &v); st != http.StatusCreated {
		t.Fatalf("link status = %d", st)
	}
	if v.OwnerName != "Ana" {
		t.Fatalf("link reached %q", v.OwnerName)
	}
	if st := call(t, ts, "POST", "/api/links/not-a-link/sessions", `{}`, nil); st != http.StatusNotFound {
		t.Fatalf("bad link = %d, want 404", st)
	}
}

// Unknown, expired and malformed codes are all 404: telling them apart would
// tell a guesser which codes once existed.
func TestBadCodesAreIndistinguishable(t *testing.T) {
	_, ts := newServer(t)
	for _, code := range []string{"000000", "12ab56", ""} {
		if _, st := redeem(t, ts, code, ""); st != http.StatusNotFound {
			t.Fatalf("redeem %q = %d, want 404", code, st)
		}
	}
}

// Registration is metered per origin: past the cap the relay answers 429, and
// releasing a peer gives its place back.
func TestRegistrationIsCapped(t *testing.T) {
	_, ts := newServer(t)
	var first peerView
	for i := 0; i < session.DefaultMaxPeersPerOrigin; i++ {
		p := register(t, ts, fmt.Sprint("peer ", i))
		if i == 0 {
			first = p
		}
	}
	if st := call(t, ts, "POST", "/api/peers", `{"name":"one too many"}`, nil); st != http.StatusTooManyRequests {
		t.Fatalf("registration %d = %d, want 429", session.DefaultMaxPeersPerOrigin+1, st)
	}
	if st := call(t, ts, "DELETE", "/api/peers/"+first.PeerID, "", nil); st != http.StatusNoContent {
		t.Fatalf("release = %d, want 204", st)
	}
	register(t, ts, "after a release") // fails the test if there is still no room
}

// Redemption and link minting share one budget per origin, and a throttled
// origin is refused even for a live code.
func TestRedemptionIsThrottled(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	for i := 0; i < session.DefaultMaxRedemptionFailures-1; i++ {
		redeem(t, ts, "000000", "")
	}
	call(t, ts, "POST", "/api/links/nope/sessions", `{}`, nil)
	if _, st := redeem(t, ts, owner.Code, ""); st != http.StatusTooManyRequests {
		t.Fatalf("live code after the budget = %d, want 429", st)
	}
}

func TestClientIPHookSetsTheOrigin(t *testing.T) {
	srv, _ := newServer(t)
	srv.ClientIP = func(r *http.Request) string { return r.Header.Get("X-Test-Origin") }
	h := srv.Handler()

	do := func(origin string) int {
		r := httptest.NewRequest("POST", "/api/redeem", bytes.NewBufferString(`{"code":"000000"}`))
		r.Header.Set("X-Test-Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < session.DefaultMaxRedemptionFailures; i++ {
		do("a")
	}
	if st := do("a"); st != http.StatusTooManyRequests {
		t.Fatalf("origin a = %d, want 429", st)
	}
	if st := do("b"); st != http.StatusNotFound {
		t.Fatalf("origin b = %d, want 404: the hook did not separate origins", st)
	}
}

// Consent is the owner's, answers a prompt that was shown, and applies to the
// owner's current session only.
func TestConsentNeedsTheOwnerAndARequest(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	sess, _ := redeem(t, ts, owner.Code, "Bruno")

	if st := consent(t, ts, owner.PeerID, sess.SessionID, true); st != http.StatusConflict {
		t.Fatalf("consent before any prompt = %d, want 409", st)
	}
	if st := consent(t, ts, "someone-else", sess.SessionID, true); st != http.StatusNotFound {
		t.Fatalf("consent from a stranger = %d, want 404", st)
	}

	ownerWS := dialOwner(t, ts, owner.PeerID)
	ctrlWS := dialController(t, ts, sess.SessionID)
	awaitPeer(t, ctrlWS, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
	awaitConsentRequest(t, ownerWS)

	if st := consent(t, ts, owner.PeerID, "another-session", true); st != http.StatusConflict {
		t.Fatalf("consent for another session = %d, want 409", st)
	}
	if got := state(t, ts, sess.SessionID); got != string(session.StateConsentRequested) {
		t.Fatalf("a refused answer changed the state to %s", got)
	}
}

func TestDenialEndsTheSession(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	ownerWS := dialOwner(t, ts, owner.PeerID)
	sess, _ := redeem(t, ts, owner.Code, "Bruno")
	ctrlWS := dialController(t, ts, sess.SessionID)
	awaitPeer(t, ctrlWS, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
	awaitConsentRequest(t, ownerWS)

	if st := consent(t, ts, owner.PeerID, sess.SessionID, false); st != http.StatusOK {
		t.Fatalf("deny = %d", st)
	}
	if got := state(t, ts, sess.SessionID); got != string(session.StateConsentDenied) {
		t.Fatalf("state %s after denying", got)
	}
	_ = ctrlWS.Close()
	if _, resp, err := dial(t, ts, url.Values{"role": {"controller"}, "session": {sess.SessionID}}); err == nil ||
		resp == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("controller rejoining a denied session: err %v", err)
	}
	// Denying frees the owner for the next person.
	var now peerView
	call(t, ts, "GET", "/api/peers/"+owner.PeerID, "", &now)
	if _, st := redeem(t, ts, now.Code, "Carla"); st != http.StatusCreated {
		t.Fatalf("redeem after a denial = %d", st)
	}
}

func TestRoleIsRequired(t *testing.T) {
	_, ts := newServer(t)
	for _, role := range []string{"", "technician", "agent"} {
		_, resp, err := dial(t, ts, url.Values{"role": {role}})
		if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("role %q: err %v", role, err)
		}
	}
}

func TestOwnerSocketNeedsItsCredential(t *testing.T) {
	_, ts := newServer(t)
	if _, resp, err := dial(t, ts, url.Values{"role": {"owner"}, "peer": {"nobody"}}); err == nil ||
		resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown peer: err %v", err)
	}
	owner := register(t, ts, "Ana")
	first := dialOwner(t, ts, owner.PeerID)
	// A second socket for the same peer is refused -- before the upgrade, or
	// on it if the first was still registering -- and the first stays.
	c, _, err := dial(t, ts, url.Values{"role": {"owner"}, "peer": {owner.PeerID}})
	if err == nil {
		if _, m := read(t, c); m["type"] != RelayErrorType {
			t.Fatalf("second owner socket got %v, want a relay error", m)
		}
	}
	sess, _ := redeem(t, ts, owner.Code, "Bruno")
	ctrl := dialController(t, ts, sess.SessionID)
	awaitPeer(t, ctrl, RoleOwner)
	awaitPeer(t, first, RoleController)
}

func TestReleaseRevokesTheCodeAndClosesTheSocket(t *testing.T) {
	_, ts := newServer(t)
	owner := register(t, ts, "Ana")
	ownerWS := dialOwner(t, ts, owner.PeerID)
	if st := call(t, ts, "DELETE", "/api/peers/"+owner.PeerID, "", nil); st != http.StatusNoContent {
		t.Fatalf("release = %d", st)
	}
	if _, st := redeem(t, ts, owner.Code, "Bruno"); st != http.StatusNotFound {
		t.Fatalf("code after release = %d, want 404", st)
	}
	if st := call(t, ts, "POST", "/api/links/"+owner.Link+"/sessions", `{}`, nil); st != http.StatusNotFound {
		t.Fatalf("link after release = %d, want 404", st)
	}
	_ = ownerWS.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := ownerWS.ReadMessage(); err == nil {
		t.Fatal("owner socket still open after release")
	}
}

// Sending before the other end is present is refused with a relay error, and
// the socket stays open for when it arrives.
func TestSendingBeforeTheOtherEndArrivesKeepsTheSocket(t *testing.T) {
	srv, ts := newServer(t)
	owner := register(t, ts, "Ana")
	sess, _ := redeem(t, ts, owner.Code, "Bruno")
	ctrl := dialController(t, ts, sess.SessionID)
	waitFor(t, "controller joined", func() bool { c, _ := srv.Hub().Connected(sess.SessionID); return c })

	if err := ctrl.WriteMessage(websocket.TextMessage, []byte(`{"type":"offer"}`)); err != nil {
		t.Fatal(err)
	}
	if _, m := read(t, ctrl); m["type"] != RelayErrorType {
		t.Fatalf("got %v, want a relay error", m)
	}
	ownerWS := dialOwner(t, ts, owner.PeerID)
	awaitPeer(t, ctrl, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
}

// A controller that drops and dials back is a reconnection: the owner is not
// asked a second time.
func TestControllerReconnectionDoesNotAskAgain(t *testing.T) {
	srv, ts := newServer(t)
	owner := register(t, ts, "Ana")
	ownerWS := dialOwner(t, ts, owner.PeerID)
	sess, _ := redeem(t, ts, owner.Code, "Bruno")
	ctrl := dialController(t, ts, sess.SessionID)
	awaitPeer(t, ctrl, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
	awaitConsentRequest(t, ownerWS)

	_ = ctrl.Close()
	if _, m := read(t, ownerWS); m["type"] != PeerLeftType {
		t.Fatalf("got %v, want %s", m, PeerLeftType)
	}
	waitFor(t, "controller left", func() bool { c, _ := srv.Hub().Connected(sess.SessionID); return !c })

	again := dialController(t, ts, sess.SessionID)
	awaitPeer(t, again, RoleOwner)
	awaitPeer(t, ownerWS, RoleController)
	if got := state(t, ts, sess.SessionID); got != string(session.StateConsentRequested) {
		t.Fatalf("state %s after reconnecting", got)
	}
	// Nothing else is waiting for the owner: a second prompt would be here.
	_ = ownerWS.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, data, err := ownerWS.ReadMessage(); err == nil {
		t.Fatalf("owner got %s after a reconnection", data)
	}
}

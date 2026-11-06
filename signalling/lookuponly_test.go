// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

func newLookupOnlyServer(t *testing.T) *httptest.Server {
	t.Helper()
	sm := session.NewSessionManager(10 * time.Minute)
	srv := NewServer(sm)
	srv.LookupOnly = true
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = srv.Hub().Close()
		sm.Close()
	})
	return ts
}

func TestLookupOnlyServerRefusesTheTrustedPath(t *testing.T) {
	ts := newLookupOnlyServer(t)
	owner := register(t, ts, "Ana")

	for _, r := range []struct{ method, path, body string }{
		{"POST", "/api/redeem", fmt.Sprintf(`{"code":%q,"name":"Bia"}`, owner.Code)},
		{"POST", "/api/links/" + owner.Link + "/sessions", `{"name":"Bia"}`},
		{"GET", "/api/sessions/00000000-0000-0000-0000-000000000000", ""},
		{"POST", "/api/peers/" + owner.PeerID + "/consent", `{"session":"x","allow":true}`},
	} {
		if st := call(t, ts, r.method, r.path, r.body, nil); st != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", r.method, r.path, st)
		}
	}

	_, resp, err := dial(t, ts, url.Values{"role": {"controller"}, "session": {"x"}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("controller socket: err %v, response %v; want 404", err, resp)
	}
}

func TestLookupOnlyServerStillLooksUpAndKeepsTheOwnerSocket(t *testing.T) {
	ts := newLookupOnlyServer(t)
	owner := register(t, ts, "Ana")
	dialOwner(t, ts, owner.PeerID)
	body := `{"key":"` + nostrKey + `","relays":["wss://relay.example.org"]}`
	if st := call(t, ts, "PUT", "/api/peers/"+owner.PeerID+"/contact", body, nil); st != http.StatusNoContent {
		t.Fatalf("set contact = %d, want 204", st)
	}

	var got session.Contact
	if st := call(t, ts, "POST", "/api/lookup", fmt.Sprintf(`{"code":%q}`, owner.Code), &got); st != http.StatusOK {
		t.Fatalf("lookup = %d, want 200", st)
	}
	if got.Key != nostrKey {
		t.Fatalf("lookup returned %+v", got)
	}
	if st := call(t, ts, "POST", "/api/links/"+owner.Link+"/lookup", "", nil); st != http.StatusOK {
		t.Fatalf("link lookup = %d, want 200", st)
	}
	if st := call(t, ts, "GET", "/api/peers/"+owner.PeerID, "", nil); st != http.StatusOK {
		t.Fatalf("get peer = %d, want 200", st)
	}
	if st := call(t, ts, "DELETE", "/api/peers/"+owner.PeerID, "", nil); st != http.StatusNoContent {
		t.Fatalf("release peer = %d, want 204", st)
	}
}

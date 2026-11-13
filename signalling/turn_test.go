// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- the TURN REST credential format coturn checks is HMAC-SHA1
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/barahn/chirp/session"
)

const testTURNSecret = "a shared secret only the relay and coturn know"

var testTURNURLs = []string{"turn:relay.example:3478?transport=udp", "turn:relay.example:3478?transport=tcp"}

func newTURNServer(t *testing.T, lookupOnly bool) (*TURN, *httptest.Server) {
	t.Helper()
	sm := session.NewSessionManager(10 * time.Minute)
	srv := NewServer(sm)
	srv.LookupOnly = lookupOnly
	srv.TURN = &TURN{Secret: testTURNSecret, URLs: testTURNURLs}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = srv.Hub().Close()
		sm.Close()
	})
	return srv.TURN, ts
}

func askTURN(t *testing.T, ts *httptest.Server, peer string) (turnCredentials, int) {
	t.Helper()
	var creds turnCredentials
	st := call(t, ts, "POST", "/api/peers/"+peer+"/turn", "", &creds)
	return creds, st
}

// checkTURNCredential holds a credential to the contract coturn's
// use-auth-secret checks: a username of "<expiry>:<id>" and a password that is
// the base64 HMAC-SHA1 of the username under the shared secret.
func checkTURNCredential(t *testing.T, who string, c turnCredential, peerID string) {
	t.Helper()
	if strings.Join(c.URLs, ",") != strings.Join(testTURNURLs, ",") {
		t.Errorf("%s: urls = %v, want %v", who, c.URLs, testTURNURLs)
	}
	expiry, id, ok := strings.Cut(c.Username, ":")
	if !ok || id == "" {
		t.Fatalf("%s: username %q is not <expiry>:<id>", who, c.Username)
	}
	if strings.Contains(c.Username, peerID) {
		t.Errorf("%s: username %q carries the peer ID, which would tie coturn's records to a registration", who, c.Username)
	}
	at, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		t.Fatalf("%s: expiry %q: %v", who, expiry, err)
	}
	if left := time.Until(time.Unix(at, 0)); left < DefaultTURNLifetime-time.Minute || left > DefaultTURNLifetime+time.Second {
		t.Errorf("%s: expires in %v, want about %v", who, left, DefaultTURNLifetime)
	}
	if c.ExpiresIn != int(DefaultTURNLifetime.Seconds()) {
		t.Errorf("%s: expires_in = %d, want %d", who, c.ExpiresIn, int(DefaultTURNLifetime.Seconds()))
	}
	mac := hmac.New(sha1.New, []byte(testTURNSecret))
	_, _ = mac.Write([]byte(c.Username))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); c.Credential != want {
		t.Errorf("%s: credential does not verify under the shared secret", who)
	}
}

func TestARelayWithoutTURNHasNoCredentialRoute(t *testing.T) {
	_, ts := newServer(t)
	p := register(t, ts, "Ana")
	if _, st := askTURN(t, ts, p.PeerID); st != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 from a relay with no TURN secret", st)
	}
}

func TestTheOwnerGetsACredentialForItselfAndOneForTheController(t *testing.T) {
	for _, lookupOnly := range []bool{false, true} {
		_, ts := newTURNServer(t, lookupOnly)
		p := register(t, ts, "Ana")
		creds, st := askTURN(t, ts, p.PeerID)
		if st != http.StatusOK {
			t.Fatalf("lookupOnly=%v: status = %d, want 200", lookupOnly, st)
		}
		checkTURNCredential(t, "owner", creds.Owner, p.PeerID)
		checkTURNCredential(t, "controller", creds.Controller, p.PeerID)
		if creds.Owner.Username == creds.Controller.Username {
			t.Errorf("lookupOnly=%v: both ends got the same username %q", lookupOnly, creds.Owner.Username)
		}
	}
}

func TestOnlyARegisteredOwnerGetsTURNCredentials(t *testing.T) {
	_, ts := newTURNServer(t, false)
	if _, st := askTURN(t, ts, "not-a-peer"); st != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unknown peer", st)
	}
}

func TestTURNCredentialsAreCappedPerRegistration(t *testing.T) {
	turn, ts := newTURNServer(t, false)
	now := time.Now()
	turn.now = func() time.Time { return now }

	ana, bea := register(t, ts, "Ana"), register(t, ts, "Bea")
	for i := 0; i < DefaultTURNPerHour/2; i++ {
		if _, st := askTURN(t, ts, ana.PeerID); st != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 within the allowance", i+1, st)
		}
	}
	if _, st := askTURN(t, ts, ana.PeerID); st != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 past %d credentials an hour", st, DefaultTURNPerHour)
	}
	if _, st := askTURN(t, ts, bea.PeerID); st != http.StatusOK {
		t.Fatalf("another registration: status = %d, want 200", st)
	}

	now = now.Add(time.Hour + time.Second)
	if _, st := askTURN(t, ts, ana.PeerID); st != http.StatusOK {
		t.Fatalf("an hour later: status = %d, want 200", st)
	}
}

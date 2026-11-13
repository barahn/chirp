// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- coturn's use-auth-secret checks HMAC-SHA1
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/barahn/remotekit/rendezvous"
)

const (
	// DefaultTURNLifetime is how long a credential can open an allocation.
	// coturn checks the expiry only on an allocation's first authenticated
	// request, so a session outlives its credential
	// (docs/network-paths-proposal.md, part 1).
	DefaultTURNLifetime = 5 * time.Minute

	// DefaultTURNPerHour is how many credentials one registration gets in an
	// hour: two per session, one for each end.
	DefaultTURNPerHour = 8
)

// TURN hands out short-lived credentials for a TURN server that checks them
// with coturn's use-auth-secret, under a secret only it and the relay hold.
// Only an owner asks, for itself and for the controller it lets in, so a
// lookup -- or a guess -- yields nothing to relay with.
type TURN struct {
	Secret string
	URLs   []string

	// Lifetime and PerHour default to DefaultTURNLifetime and
	// DefaultTURNPerHour when zero.
	Lifetime time.Duration
	PerHour  int

	now func() time.Time

	mu     sync.Mutex
	issued map[rendezvous.PeerID][]time.Time
}

type turnCredential struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
	ExpiresIn  int      `json:"expires_in"`
}

type turnCredentials struct {
	Owner      turnCredential `json:"owner"`
	Controller turnCredential `json:"controller"`
}

func (t *TURN) lifetime() time.Duration {
	if t.Lifetime > 0 {
		return t.Lifetime
	}
	return DefaultTURNLifetime
}

func (t *TURN) perHour() int {
	if t.PerHour > 0 {
		return t.PerHour
	}
	return DefaultTURNPerHour
}

func (t *TURN) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// allow records n credentials for peer if its last hour leaves room for them.
func (t *TURN) allow(peer rendezvous.PeerID, n int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clock()
	hourAgo := now.Add(-time.Hour)
	if t.issued == nil {
		t.issued = make(map[rendezvous.PeerID][]time.Time)
	}
	for id, times := range t.issued {
		t.issued[id] = within(times, hourAgo)
		if len(t.issued[id]) == 0 {
			delete(t.issued, id)
		}
	}
	if len(t.issued[peer])+n > t.perHour() {
		return false
	}
	for range n {
		t.issued[peer] = append(t.issued[peer], now)
	}
	return true
}

func within(times []time.Time, since time.Time) []time.Time {
	kept := times[:0]
	for _, at := range times {
		if at.After(since) {
			kept = append(kept, at)
		}
	}
	return kept
}

// credential is in the format coturn's use-auth-secret checks: a username of
// "<expiry>:<id>", and a password that is the base64 HMAC-SHA1 of the
// username under the shared secret. The relay writes it itself rather than
// take remotekit's webrtc.GenerateEphemeralTURNCredentials, which would bring
// all of pion into the relay. The ID is random rather than the peer ID, so the
// TURN server's records cannot tie an allocation to a registration.
func (t *TURN) credential() (turnCredential, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return turnCredential{}, err
	}
	user := strconv.FormatInt(t.clock().Add(t.lifetime()).Unix(), 10) + ":" + hex.EncodeToString(id)
	mac := hmac.New(sha1.New, []byte(t.Secret))
	_, _ = mac.Write([]byte(user))
	return turnCredential{
		URLs:       t.URLs,
		Username:   user,
		Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		ExpiresIn:  int(t.lifetime().Seconds()),
	}, nil
}

// issueTURN gives a registered owner two credentials: its own, and the one it
// passes to the controller over the session's signalling. The peer ID in the
// path is the owner's credential, as on every owner endpoint.
func (s *Server) issueTURN(w http.ResponseWriter, r *http.Request) {
	id := rendezvous.PeerID(r.PathValue("peer"))
	if _, err := s.sessions.Peer(id); err != nil {
		writeSessionError(w, err)
		return
	}
	if !s.TURN.allow(id, 2) {
		writeError(w, http.StatusTooManyRequests, "too many TURN credentials for this Chirp, try again later")
		return
	}
	owner, err := s.TURN.credential()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	controller, err := s.TURN.credential()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, turnCredentials{Owner: owner, Controller: controller})
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/barahn/chirp/session"
	"github.com/barahn/remotekit/rendezvous"
)

// The lookup endpoints are the blind server of decision 5
// (docs/product-model.md). An owner says how it can be reached over Nostr; a
// controller with the owner's code, or link, gets that back. Nothing else
// crosses this server: no session, no consent, no signalling. The owner keeps
// its owner socket open only so that the server knows it is still running
// (decision 1 and the peer grace).
//
//	PUT  /api/peers/{peer}/contact     {"key": "...", "relays": ["wss://..."]}
//	POST /api/lookup                   {"code": "123456"}     -> contact
//	POST /api/links/{token}/lookup                             -> contact

// maxLookupBody bounds what a lookup request may send; a contact with the
// most relays allowed fits several times over.
const maxLookupBody = 8 << 10

// setContact records how the peer can be reached over Nostr. The peer ID in
// the path is the credential, as for every other owner endpoint.
func (s *Server) setContact(w http.ResponseWriter, r *http.Request) {
	var c session.Contact
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLookupBody)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	err := s.sessions.SetContact(rendezvous.PeerID(r.PathValue("peer")), c)
	if errors.Is(err, session.ErrBadContact) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeSessionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// lookup redeems a code for its owner's contact. It is metered like redeem:
// every failure counts against the caller's origin, from the same budget.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLookupBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body")
		return
	}
	c, err := s.sessions.Lookup(req.Code, s.clientIP(r))
	writeLookup(w, c, err)
}

// lookupLink mints a single-use code from a link and redeems it for the
// owner's contact in one step.
func (s *Server) lookupLink(w http.ResponseWriter, r *http.Request) {
	c, err := s.sessions.LookupLink(rendezvous.Token(r.PathValue("token")), s.clientIP(r))
	writeLookup(w, c, err)
}

func writeLookup(w http.ResponseWriter, c *session.Contact, err error) {
	if errors.Is(err, session.ErrNoContact) {
		writeError(w, http.StatusConflict, "that Chirp cannot be reached over Nostr")
		return
	}
	if err != nil {
		writeSessionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

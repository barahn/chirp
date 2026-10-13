// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/barahn/remotekit/pake"
)

// Binding the two Chirps: decision 5 in docs/product-model.md, and
// barahn/chirp#21. Over Nostr, before a controller may ask to be let in, the
// two Chirps run a PAKE (remotekit's pake: CPace from mendsec/cpace) over the
// code's secret part, the four digits the server never sees:
//
//  1. The controller sends TypePakeStart, naming the routing part it used.
//  2. The owner answers with TypePakeReply, under the secret it showed with
//     that code -- each code has its own, used once -- or with its link's.
//  3. The controller checks the owner's proof and sends TypePakeConfirm,
//     then asks to be let in. The owner checks the controller's proof
//     before it shows any prompt.
//
// Each end comes out with the other's ed25519 key, proved to come from
// whoever holds the same code. The session that follows is signed with
// those keys: the owner's runner trusts only the controller's offers
// (RequireSignedViewers), and the controller's viewer only the owner's
// answers (AgentKey). The server, and the relays, can carry the session but
// can no longer take part in it.

// The PAKE's messages over Nostr. Dotted, like the consent messages.
const (
	TypePakeStart   = "pake.start"
	TypePakeReply   = "pake.reply"
	TypePakeConfirm = "pake.confirm"
)

// reasonAuth is the owner's answer to a controller that did not prove it
// holds the code.
const reasonAuth = "auth"

// How a controller reached the owner, which says which secret applies.
const (
	viaCode = "code"
	viaLink = "link"
)

var (
	// ErrWrongCode means the PAKE failed: the four secret digits were
	// mistyped, or someone in the middle tried to take part. The two
	// cannot be told apart, on purpose.
	ErrWrongCode = errors.New("client: the code's last four digits do not match")
	// ErrIncompleteCode means a code or link without its secret part was
	// given to a Chirp that signals over Nostr, which needs it. Nothing was
	// spent.
	ErrIncompleteCode = errors.New("client: the code is missing its last four digits")
)

// maxLinkFailures is how many failed PAKEs through the link a registration
// takes before it gives the link up, with its code: a link mints a code per
// use, so without a bound its secret could be guessed one attempt at a time.
const maxLinkFailures = 3

// maxCodeSecrets is how many codes' secrets an owner remembers. A lookup
// replaces the code at once, so the controller's PAKE always names a code
// that is no longer shown; a few are kept for that.
const maxCodeSecrets = 8

// pakeContext is the PAKE's channel identifier: what the exchange is for,
// and which code or link it is for. Both ends know all of it.
func pakeContext(via, routing string) []byte {
	return fmt.Appendf(nil, "chirp/pake/v1\x00%s\x00%s", via, routing)
}

// pakeSessionID is the PAKE's session identifier: the two Nostr keys of this
// session, owner first. The controller's is made for each attempt, so it is
// unique, and both ends know both.
func pakeSessionID(ownerKey, controllerKey string) []byte {
	return []byte(ownerKey + controllerKey)
}

// newSecret returns four random digits.
func newSecret() string {
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		panic("client: no randomness: " + err.Error())
	}
	return fmt.Sprintf("%04d", n.Int64())
}

func newSigningKey() ed25519.PrivateKey {
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic("client: no randomness: " + err.Error())
	}
	return k
}

// secretBook is an owner's secrets for one registration: one per code shown,
// each given up once a PAKE has used it, and one for the link.
type secretBook struct {
	codes        []codeSecret
	link         string
	linkFailures int
}

type codeSecret struct{ code, secret string }

func newSecretBook() *secretBook { return &secretBook{link: newSecret()} }

// forCode returns the secret shown with code, making one if it is new.
func (b *secretBook) forCode(code string) string {
	for _, c := range b.codes {
		if c.code == code {
			return c.secret
		}
	}
	s := newSecret()
	b.codes = append(b.codes, codeSecret{code, s})
	if len(b.codes) > maxCodeSecrets {
		b.codes = b.codes[1:]
	}
	return s
}

// take returns code's secret and forgets it: one PAKE per code.
func (b *secretBook) take(code string) (string, bool) {
	for i, c := range b.codes {
		if c.code == code {
			b.codes = append(b.codes[:i], b.codes[i+1:]...)
			return c.secret, true
		}
	}
	return "", false
}

// pending is a PAKE an owner has answered and waits to see confirmed. One
// abandoned -- the controller found the owner's proof wrong and left, or
// never came back -- counts as a failure, like a wrong confirmation.
type pending struct {
	resp  *pake.Responder
	via   string
	until time.Time
}

// maxPending bounds the PAKEs an owner keeps open at once.
const maxPending = 4

func encodePake(msg []byte) string { return base64.StdEncoding.EncodeToString(msg) }

func decodePake(s string) ([]byte, bool) {
	b, err := base64.StdEncoding.DecodeString(s)
	return b, err == nil && len(b) > 0 && len(b) <= 4096
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package node is a public relay's identity in the network
// (docs/status-page-proposal.md, phase 4): an ed25519 key the relay keeps
// across restarts, and /.well-known/chirp-node, where it proves it holds that
// key for the host it is reached at by signing a nonce the asker chose.
package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

// Path is where a node publishes its identity.
const Path = "/.well-known/chirp-node"

// The bounds of a nonce, in bytes before encoding.
const (
	MinNonce = 16
	MaxNonce = 64
)

// The signature is bound to this protocol, the host and the nonce, so that it
// proves nothing anywhere else.
const signingContext = "chirp-node/v1"

// Identity is what Path answers. Key, Nonce and Signature are base64url
// without padding; Nonce and Signature are there only when a nonce was asked.
type Identity struct {
	Key       string `json:"key"`
	Nonce     string `json:"nonce,omitempty"`
	Signature string `json:"signature,omitempty"`
}

var b64 = base64.RawURLEncoding

// LoadOrCreateKey reads the node's key from path, or creates one there if
// there is no file. A key anyone but its owner can read is refused, not used.
func LoadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	seed, err := readSeed(path)
	if errors.Is(err, os.ErrNotExist) {
		return createKey(path)
	}
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func readSeed(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s can be read by others (mode %o): make it 600", path, fi.Mode().Perm())
	}
	seed := make([]byte, ed25519.SeedSize+1)
	n, err := io.ReadFull(f, seed)
	if n != ed25519.SeedSize || !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("%s is not a node key: it must hold exactly %d bytes", path, ed25519.SeedSize)
	}
	return seed[:n], nil
}

func createKey(path string) (ed25519.PrivateKey, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	_, err = f.Write(seed)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func message(host, nonce string) []byte {
	return []byte(signingContext + "\x00" + host + "\x00" + nonce)
}

// canonicalHost is the host a request was addressed to, lowercased and
// without a port.
func canonicalHost(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	return strings.ToLower(host)
}

func validNonce(nonce string) bool {
	raw, err := b64.DecodeString(nonce)
	return err == nil && len(raw) >= MinNonce && len(raw) <= MaxNonce
}

// Handler answers Path for the node holding key.
func Handler(key ed25519.PrivateKey) http.Handler {
	pub := b64.EncodeToString(key.Public().(ed25519.PublicKey))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := Identity{Key: pub}
		if r.URL.Query().Has("nonce") {
			nonce := r.URL.Query().Get("nonce")
			if !validNonce(nonce) {
				http.Error(w, fmt.Sprintf("nonce must be %d to %d bytes, base64url without padding", MinNonce, MaxNonce), http.StatusBadRequest)
				return
			}
			id.Nonce = nonce
			id.Signature = b64.EncodeToString(ed25519.Sign(key, message(canonicalHost(r.Host), nonce)))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(id)
	})
}

// Verify checks that id proves its key was held, for host, when asked nonce.
func Verify(id Identity, host, nonce string) error {
	pub, err := b64.DecodeString(id.Key)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("node: the key is not an ed25519 public key")
	}
	if id.Nonce != nonce {
		return errors.New("node: the answer is for another nonce")
	}
	sig, err := b64.DecodeString(id.Signature)
	if err != nil || !ed25519.Verify(pub, message(canonicalHost(host), nonce), sig) {
		return errors.New("node: the signature does not prove the key for this host and nonce")
	}
	return nil
}

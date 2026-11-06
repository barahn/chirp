// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package node

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrCreateKeyCreatesAKeyOnlyItsOwnerCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.key")
	first, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode = %o, want 600", perm)
	}

	again, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, again) {
		t.Fatal("the key changed between two starts")
	}
}

func TestLoadOrCreateKeyRefusesAKeyOthersCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.key")
	if _, err := LoadOrCreateKey(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Fatal("a key readable by its group was accepted")
	}
}

func TestLoadOrCreateKeyRefusesAFileThatIsNotAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.key")
	if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Fatal("a file of the wrong size was accepted as a key")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Fatal("an empty file was accepted as a key")
	}
}

func TestLoadOrCreateKeyRefusesADirectory(t *testing.T) {
	if _, err := LoadOrCreateKey(t.TempDir()); err == nil {
		t.Fatal("a directory was accepted as a key")
	}
}

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	k, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "node.key"))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

const nonce = "AAECAwQFBgcICQoLDA0ODw"

func ask(t *testing.T, h http.Handler, host, query string) (*httptest.ResponseRecorder, Identity) {
	t.Helper()
	req := httptest.NewRequest("GET", "/.well-known/chirp-node"+query, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var id Identity
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &id); err != nil {
			t.Fatalf("body %q: %v", rec.Body, err)
		}
	}
	return rec, id
}

func TestIdentityProvesTheNodeHoldsItsKeyForThisHost(t *testing.T) {
	key := newKey(t)
	rec, id := ask(t, Handler(key), "Relay.Example.org", "?nonce="+nonce)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	pub := key.Public().(ed25519.PublicKey)
	if id.Key != base64.RawURLEncoding.EncodeToString(pub) {
		t.Fatalf("key = %q, not the node's", id.Key)
	}
	if id.Nonce != nonce {
		t.Fatalf("nonce = %q, want %q", id.Nonce, nonce)
	}
	if err := Verify(id, "relay.example.org", nonce); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := Verify(id, "other.example.org", nonce); err == nil {
		t.Fatal("the proof verified for a host it was not made for")
	}
	if err := Verify(id, "relay.example.org", "BAECAwQFBgcICQoLDA0ODw"); err == nil {
		t.Fatal("the proof verified for a nonce it was not made for")
	}
}

func TestIdentityIgnoresThePortTheProxyPassesOn(t *testing.T) {
	key := newKey(t)
	_, id := ask(t, Handler(key), "relay.example.org:443", "?nonce="+nonce)
	if err := Verify(id, "relay.example.org", nonce); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestIdentityWithoutANonceGivesTheKeyAndNoProof(t *testing.T) {
	rec, id := ask(t, Handler(newKey(t)), "relay.example.org", "")
	if rec.Code != http.StatusOK || id.Key == "" || id.Signature != "" || id.Nonce != "" {
		t.Fatalf("status %d, identity %+v", rec.Code, id)
	}
}

func TestIdentityRefusesANonceOutOfBounds(t *testing.T) {
	h := Handler(newKey(t))
	for _, n := range []string{
		"short",
		"not+base64url/at=all",
		base64.RawURLEncoding.EncodeToString(make([]byte, MinNonce-1)),
		base64.RawURLEncoding.EncodeToString(make([]byte, MaxNonce+1)),
	} {
		if rec, _ := ask(t, h, "relay.example.org", "?nonce="+n); rec.Code != http.StatusBadRequest {
			t.Errorf("nonce %q: status %d, want 400", n, rec.Code)
		}
	}
}

func TestVerifyRefusesAMalformedIdentity(t *testing.T) {
	for _, id := range []Identity{
		{},
		{Key: "AAAA", Nonce: nonce, Signature: "AAAA"},
		{Key: strings.Repeat("A", 43), Nonce: nonce, Signature: "!"},
	} {
		if err := Verify(id, "relay.example.org", nonce); err == nil {
			t.Errorf("%+v verified", id)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/barahn/chirp/node"
)

func TestPrivateModeHasNoNodeIdentity(t *testing.T) {
	m, err := relayMode("private", "")
	if err != nil {
		t.Fatal(err)
	}
	if m.public || m.identity != nil {
		t.Fatalf("private mode = %+v", m)
	}
	ts := httptest.NewServer(routes(http.NotFoundHandler(), "", m.identity))
	defer ts.Close()
	resp, err := http.Get(ts.URL + node.Path)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("%s in private mode = %s, want 404", node.Path, resp.Status)
	}
}

func TestPublicModeNeedsAKeyFile(t *testing.T) {
	if _, err := relayMode("public", ""); err == nil {
		t.Fatal("public mode started without -node-key")
	}
}

func TestPublicModeServesTheNodeIdentityFromItsKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.key")
	m, err := relayMode("public", path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.public || m.identity == nil {
		t.Fatalf("public mode = %+v", m)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no key was created: %v", err)
	}

	ts := httptest.NewServer(routes(http.NotFoundHandler(), "", m.identity))
	defer ts.Close()
	resp, err := http.Get(ts.URL + node.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var id node.Identity
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&id) != nil || id.Key == "" {
		t.Fatalf("%s = %s, identity %+v", node.Path, resp.Status, id)
	}
}

func TestUnknownModeIsRefused(t *testing.T) {
	if _, err := relayMode("cluster", ""); err == nil {
		t.Fatal("an unknown mode was accepted")
	}
}

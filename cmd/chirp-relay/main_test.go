// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthzAndProbe(t *testing.T) {
	relay := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ts := httptest.NewServer(routes(relay, ""))
	defer ts.Close()

	if err := probe(strings.TrimPrefix(ts.URL, "http://")); err != nil {
		t.Fatalf("probe: %v", err)
	}
	resp, err := http.Get(ts.URL + "/api/peers")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("other paths should reach the relay, got %s", resp.Status)
	}
}

func TestProbeFailsWithNothingListening(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	addr := strings.TrimPrefix(ts.URL, "http://")
	ts.Close()
	if err := probe(addr); err == nil {
		t.Fatal("probe succeeded against a closed port")
	}
}

func TestUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.sock")
	addr := "unix:" + path

	// A socket left behind by an earlier relay is replaced.
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = stale.Close()

	ln, err := listen(addr, 0o660)
	if err != nil {
		t.Fatalf("listen over a stale socket: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o660 {
		t.Fatalf("socket mode %v, want 0660", fi.Mode().Perm())
	}

	srv := &http.Server{Handler: routes(http.NotFoundHandler(), "")}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	if err := probe(addr); err != nil {
		t.Fatalf("probe over the socket: %v", err)
	}
}

func TestListenRefusesToReplaceAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.sock")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listen("unix:"+path, 0o660); err == nil {
		t.Fatal("listen replaced a regular file")
	}
	if b, _ := os.ReadFile(path); string(b) != "keep me" {
		t.Fatal("the file was touched")
	}
}

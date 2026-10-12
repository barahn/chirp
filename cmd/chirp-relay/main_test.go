// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzAndProbe(t *testing.T) {
	relay := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ts := httptest.NewServer(routes(relay))
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

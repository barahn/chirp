// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	up := httptest.NewTLSServer(mux)
	defer up.Close()
	broken := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()
	moved := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, up.URL+"/healthz", http.StatusFound)
	}))
	defer moved.Close()

	// The test servers' certificate is trusted only by their own client.
	trusting := newClient()
	trusting.Transport.(*http.Transport).TLSClientConfig = up.Client().Transport.(*http.Transport).TLSClientConfig
	p := &Prober{Client: trusting}
	ctx := context.Background()

	if s := p.Probe(ctx, up.URL+"/"); !s.OK || s.Err != "" || s.Latency <= 0 {
		t.Errorf("up: %+v", s)
	}
	if s := p.Probe(ctx, broken.URL); s.OK || s.Err != ErrStatus {
		t.Errorf("500: %+v, want %s", s, ErrStatus)
	}
	if s := p.Probe(ctx, moved.URL); s.OK || s.Err != ErrStatus {
		t.Errorf("a redirect to a healthy server: %+v, want %s: redirects are not followed", s, ErrStatus)
	}
	if s := (&Prober{}).Probe(ctx, up.URL); s.OK || s.Err != ErrTLS {
		t.Errorf("an untrusted certificate: %+v, want %s", s, ErrTLS)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "https://" + ln.Addr().String()
	_ = ln.Close()
	if s := p.Probe(ctx, closed); s.OK || s.Err != ErrConnect {
		t.Errorf("nothing listening: %+v, want %s", s, ErrConnect)
	}

	hang, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hang.Close() }()
	go func() {
		for {
			c, err := hang.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }()
		}
	}()
	short, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if s := p.Probe(short, "https://"+hang.Addr().String()); s.OK || s.Err != ErrTimeout {
		t.Errorf("a server that never answers: %+v, want %s", s, ErrTimeout)
	}
}

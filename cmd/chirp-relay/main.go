// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command chirp-relay serves a Chirp relay: registration, codes and links,
// consent, and the signalling between the two ends of a session
// (signalling.Server).
//
// It is the relay its users run or trust that the first runnable Chirp ships
// with (docs/product-model.md, decision 5), not a public one. It speaks plain
// HTTP: put a TLS-terminating proxy in front of it for anything beyond one
// machine, name that proxy in -trusted-proxies, and see docs/relay.md for
// running it and docs/stun-turn.md for the STUN and TURN servers a session
// also needs.
//
// Usage:
//
//	chirp-relay -addr localhost:8080
//	chirp-relay -addr :8080 -trusted-proxies 127.0.0.1,::1
//	chirp-relay -healthcheck
//
// Each flag can also be set in the environment: CHIRP_RELAY_ADDR and
// CHIRP_RELAY_TRUSTED_PROXIES. A flag given on the command line wins.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/chirp/signalling"
)

func main() {
	addr := flag.String("addr", envOr("CHIRP_RELAY_ADDR", "localhost:8080"), "address to listen on")
	proxies := flag.String("trusted-proxies", os.Getenv("CHIRP_RELAY_TRUSTED_PROXIES"),
		"comma-separated addresses or CIDRs of the reverse proxies in front of the relay, whose X-Forwarded-For is believed")
	healthcheck := flag.Bool("healthcheck", false, "ask the relay listening on -addr whether it is up, and exit 0 if it is")
	flag.Parse()

	if *healthcheck {
		if err := probe(*addr); err != nil {
			fmt.Fprintf(os.Stderr, "chirp-relay: %v\n", err)
			os.Exit(1)
		}
		return
	}

	trusted, err := signalling.ParsePrefixes(*proxies)
	if err != nil {
		log.Fatalf("chirp-relay: -trusted-proxies: %v", err)
	}

	sm := session.NewSessionManager(0)
	defer sm.Close()
	relay := signalling.NewServer(sm)
	if len(trusted) > 0 {
		relay.ClientIP = signalling.ForwardedFor(trusted)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           routes(relay.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	if len(trusted) > 0 {
		log.Printf("chirp-relay: listening on %s, trusting X-Forwarded-For from %s", *addr, *proxies)
	} else {
		log.Printf("chirp-relay: listening on %s", *addr)
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("chirp-relay: %v", err)
		stop()
		sm.Close()
		os.Exit(1)
	}
}

// routes adds the relay's own health check in front of the signalling routes.
// It says only that the process is serving; it reads no state.
func routes(relay http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("/", relay)
	return mux
}

// probe asks the relay listening on addr for /healthz. It is what a container
// health check runs, since the relay's image has no shell or curl.
func probe(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

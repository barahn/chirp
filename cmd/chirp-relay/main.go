// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command chirp-relay serves a Chirp relay: registration, codes and links,
// consent, and the signalling between the two ends of a session
// (signalling.Server).
//
// It is the relay its users run or trust that the first runnable Chirp ships
// with (docs/product-model.md, decision 5), not a public one. It speaks plain
// HTTP: put a TLS-terminating proxy in front of it for anything beyond one
// machine, and see docs/stun-turn.md for the STUN and TURN servers a session
// also needs.
//
// Usage:
//
//	chirp-relay -addr localhost:8080
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/chirp/signalling"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "address to listen on")
	flag.Parse()

	sm := session.NewSessionManager(0)
	defer sm.Close()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           signalling.NewServer(sm).Handler(),
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

	log.Printf("chirp-relay: listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("chirp-relay: %v", err)
		stop()
		sm.Close()
		os.Exit(1)
	}
}

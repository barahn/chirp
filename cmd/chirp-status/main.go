// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command chirp-status measures the servers of Chirp's public network and
// serves the figures the status page shows (docs/status-page-proposal.md).
//
// It probes every server listed in the registry each 30 s, keeps the probes
// in SQLite for 90 days and each day's totals for good, and answers:
//
//	GET /v1/servers                    every server's status now
//	GET /v1/servers/{id}/history?days= one server's days, oldest first
//	GET /healthz                       200 while serving
//
// Run it on a different host and network from every server it watches, or
// it goes down with them. Like chirp-relay it speaks plain HTTP: put a TLS
// proxy in front of it.
//
// Usage:
//
//	chirp-status -servers network/servers.json -db /var/lib/chirp-status/status.db
//
// Each flag can also be set in the environment: CHIRP_STATUS_ADDR,
// CHIRP_STATUS_SERVERS, CHIRP_STATUS_DB, CHIRP_STATUS_VANTAGE and
// CHIRP_STATUS_ALLOW_ORIGIN. A flag given on the command line wins.
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

	"github.com/barahn/chirp/status"
)

func main() {
	addr := flag.String("addr", envOr("CHIRP_STATUS_ADDR", "localhost:8081"), "address to listen on, host:port")
	servers := flag.String("servers", envOr("CHIRP_STATUS_SERVERS", "network/servers.json"), "the registry of the network's servers")
	db := flag.String("db", envOr("CHIRP_STATUS_DB", "chirp-status.db"), "the SQLite database the probes are kept in")
	vantage := flag.String("vantage", envOr("CHIRP_STATUS_VANTAGE", "primary"), "a name for the place this service probes from")
	origin := flag.String("allow-origin", envOr("CHIRP_STATUS_ALLOW_ORIGIN", "https://usechirp.app"),
		"the origin whose pages may read the API; empty for none")
	flag.Parse()

	list, err := status.LoadServers(*servers)
	if err != nil {
		log.Fatalf("chirp-status: %v", err)
	}
	store, err := status.OpenStore(*db)
	if err != nil {
		log.Fatalf("chirp-status: %s: %v", *db, err)
	}
	defer func() { _ = store.Close() }()

	svc := &status.Service{
		Servers:     list,
		Store:       store,
		Prober:      &status.Prober{},
		Vantage:     *vantage,
		AllowOrigin: *origin,
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("chirp-status: listening on %s, probing %d servers from %q", *addr, len(list), *vantage)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("chirp-status: %v", err)
		stop()
	}
	<-done
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

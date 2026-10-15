// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command chirp-relay serves a Chirp relay: registration, codes and links,
// consent, and the signalling between the two ends of a session
// (signalling.Server).
//
// It is the relay its users run or trust that the first runnable Chirp ships
// with (docs/product-model.md, decision 5), not a public one. It speaks plain
// HTTP: put a TLS-terminating proxy in front of it for anything beyond one
// machine, and see docs/relay.md for running it and docs/stun-turn.md for the
// STUN and TURN servers a session also needs.
//
// The proxy reaches the relay best over a Unix socket (-addr unix:PATH),
// which only the processes its permissions admit can connect to: every
// request on it comes from the proxy, so its X-Forwarded-For is believed.
// Over TCP, name the proxy in -trusted-proxies instead, and keep everything
// else from reaching the port.
//
// Usage:
//
//	chirp-relay -addr localhost:8080
//	chirp-relay -addr unix:/run/chirp-relay/relay.sock
//	chirp-relay -addr 10.0.0.5:8080 -trusted-proxies 10.0.0.1
//	chirp-relay -healthcheck
//
// A browser at the relay's address gets a page saying what it is
// (pages.go), robots.txt, and with -security-contact a security.txt.
//
// Each flag can also be set in the environment: CHIRP_RELAY_ADDR,
// CHIRP_RELAY_SOCKET_MODE, CHIRP_RELAY_TRUSTED_PROXIES and
// CHIRP_RELAY_SECURITY_CONTACT. A flag given on the command line wins.
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/barahn/chirp/session"
	"github.com/barahn/chirp/signalling"
)

func main() {
	addr := flag.String("addr", envOr("CHIRP_RELAY_ADDR", "localhost:8080"), "address to listen on: host:port, or unix:PATH for a Unix socket")
	socketMode := flag.String("socket-mode", envOr("CHIRP_RELAY_SOCKET_MODE", "0660"), "permissions of the Unix socket, in octal")
	proxies := flag.String("trusted-proxies", os.Getenv("CHIRP_RELAY_TRUSTED_PROXIES"),
		"comma-separated addresses or CIDRs of the reverse proxies in front of the relay, whose X-Forwarded-For is believed")
	healthcheck := flag.Bool("healthcheck", false, "ask the relay listening on -addr whether it is up, and exit 0 if it is")
	contactFlag := flag.String("security-contact", os.Getenv("CHIRP_RELAY_SECURITY_CONTACT"),
		"where to report a security problem with this relay: an email address or an https: URL, served as /.well-known/security.txt")
	flag.Parse()

	if *healthcheck {
		if err := probe(*addr); err != nil {
			fmt.Fprintf(os.Stderr, "chirp-relay: %v\n", err)
			os.Exit(1)
		}
		return
	}

	contact, err := securityContact(*contactFlag)
	if err != nil {
		log.Fatalf("chirp-relay: -security-contact: %v", err)
	}
	trusted, err := signalling.ParsePrefixes(*proxies)
	if err != nil {
		log.Fatalf("chirp-relay: -trusted-proxies: %v", err)
	}
	mode, err := strconv.ParseUint(*socketMode, 8, 32)
	if err != nil || mode > 0o777 {
		log.Fatalf("chirp-relay: -socket-mode: %q is not an octal permission", *socketMode)
	}
	ln, err := listen(*addr, os.FileMode(mode))
	if err != nil {
		log.Fatalf("chirp-relay: %v", err)
	}
	_, unix := socketPath(*addr)

	sm := session.NewSessionManager(0)
	defer sm.Close()
	relay := signalling.NewServer(sm)
	if unix || len(trusted) > 0 {
		relay.ClientIP = signalling.ForwardedFor(trusted)
	}

	srv := &http.Server{
		Handler:           routes(relay.Handler(), contact),
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

	switch {
	case unix:
		log.Printf("chirp-relay: listening on %s, trusting X-Forwarded-For from whoever can open it", *addr)
	case len(trusted) > 0:
		log.Printf("chirp-relay: listening on %s, trusting X-Forwarded-For from %s", *addr, *proxies)
	default:
		log.Printf("chirp-relay: listening on %s", *addr)
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("chirp-relay: %v", err)
		stop()
		sm.Close()
		os.Exit(1)
	}
}

// routes adds the relay's own health check, and the pages a browser finds,
// in front of the signalling routes. None of them reads any state.
func routes(relay http.Handler, contact string) http.Handler {
	mux := http.NewServeMux()
	pages(mux, contact, time.Now)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("/", relay)
	return mux
}

// socketPath reports whether addr names a Unix socket, and its path.
func socketPath(addr string) (string, bool) {
	return strings.CutPrefix(addr, "unix:")
}

// listen opens addr. A Unix socket left behind by a relay that did not exit
// cleanly is replaced; any other file at that path is an error, not something
// to delete. The socket gets mode, whatever the umask.
func listen(addr string, mode os.FileMode) (net.Listener, error) {
	path, unix := socketPath(addr)
	if !unix {
		return net.Listen("tcp", addr)
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode().Type() != os.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, mode); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// probe asks the relay listening on addr for /healthz. It is what a container
// health check runs, since the relay's image has no shell or curl.
func probe(addr string) error {
	transport := &http.Transport{}
	target := ""
	if path, unix := socketPath(addr); unix {
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}
		target = "relay"
	} else {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return err
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "localhost"
		}
		target = net.JoinHostPort(host, port)
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}
	resp, err := client.Get("http://" + target + "/healthz")
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

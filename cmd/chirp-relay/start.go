// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/barahn/chirp/signalling"
)

func logStart(m mode, turn *signalling.TURN) {
	if m.public {
		log.Printf("chirp-relay: public mode, lookup only, node key in %s", m.nodeKey)
	}
	if turn != nil {
		log.Printf("chirp-relay: handing out TURN credentials for %s", strings.Join(turn.URLs, ", "))
	}
}

func logListening(ln net.Listener, trusted []netip.Prefix) {
	addr := ln.Addr()
	switch {
	case addr.Network() == "unix":
		log.Printf("chirp-relay: listening on %s, trusting X-Forwarded-For from whoever can open it", addr)
	case len(trusted) > 0:
		log.Printf("chirp-relay: listening on %s, trusting X-Forwarded-For from %v", addr, trusted)
	default:
		log.Printf("chirp-relay: listening on %s", addr)
	}
}

// exitWithHealth asks the relay at addr whether it is up, and exits 0 if it
// is, 1 if not.
func exitWithHealth(addr string) {
	if err := probe(addr); err != nil {
		fmt.Fprintf(os.Stderr, "chirp-relay: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

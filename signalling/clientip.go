// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ForwardedFor returns a Server.ClientIP that reads X-Forwarded-For, believing
// only the hops that trusted vouches for.
//
// A request from an address outside trusted is its own origin, whatever its
// headers say: anyone who can reach the relay directly can write the header,
// and choosing a fresh origin per request would escape the throttle. A request
// from a trusted proxy is attributed to the rightmost forwarded address that is
// not itself trusted, which is the last hop a trusted proxy saw. Addresses
// further left were written by the client and are ignored.
//
// A connection with no IP address is one over a Unix socket, which only the
// processes its permissions admit can open: it is trusted like a listed
// proxy. That is what makes a socket the tightest way to put a proxy in front
// of the relay -- no other process on the machine can pose as it.
//
// An entry that does not parse stops the walk at the hop that wrote it, so a
// misbehaving proxy costs its users a shared budget rather than giving anyone
// a free one. With trusted empty, a TCP connection's origin is its own
// address, as with a nil ClientIP.
func ForwardedFor(trusted []netip.Prefix) func(*http.Request) string {
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	return func(r *http.Request) string {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		origin := host
		if peer, err := netip.ParseAddr(host); err == nil {
			peer = peer.Unmap()
			if !isTrusted(peer) {
				return peer.String()
			}
			origin = peer.String()
		}

		// Several X-Forwarded-For headers are one list, in order.
		var hops []string
		for _, h := range r.Header.Values("X-Forwarded-For") {
			hops = append(hops, strings.Split(h, ",")...)
		}
		for i := len(hops) - 1; i >= 0; i-- {
			a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				break
			}
			a = a.Unmap()
			if !isTrusted(a) {
				return a.String()
			}
			origin = a.String()
		}
		return origin
	}
}

// ParsePrefixes parses a comma-separated list of CIDR prefixes or single
// addresses, as an operator writes trusted proxies on a command line. A bare
// address is the prefix holding just that address.
func ParsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if strings.Contains(f, "/") {
			p, err := netip.ParsePrefix(f)
			if err != nil {
				return nil, err
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, err
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package signalling

import (
	"net/http/httptest"
	"testing"
)

func TestForwardedForBelievesOnlyTrustedHops(t *testing.T) {
	trusted, err := ParsePrefixes("10.0.0.0/8, 127.0.0.1, ::1")
	if err != nil {
		t.Fatal(err)
	}
	clientIP := ForwardedFor(trusted)

	cases := []struct {
		name   string
		remote string
		xff    []string
		want   string
	}{
		{"direct client ignores its own header", "203.0.113.7:5000", []string{"198.51.100.1"}, "203.0.113.7"},
		{"proxy forwards the client", "127.0.0.1:4000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"forged hops left of the real one are ignored", "127.0.0.1:4000", []string{"1.2.3.4, 5.6.7.8, 203.0.113.7"}, "203.0.113.7"},
		{"a chain of trusted proxies is walked", "127.0.0.1:4000", []string{"203.0.113.7, 10.1.2.3"}, "203.0.113.7"},
		{"several headers are one list", "127.0.0.1:4000", []string{"1.2.3.4", "203.0.113.7"}, "203.0.113.7"},
		{"proxy without the header is the origin", "127.0.0.1:4000", nil, "127.0.0.1"},
		{"garbage stops at the hop that wrote it", "127.0.0.1:4000", []string{"203.0.113.7, nonsense"}, "127.0.0.1"},
		{"all hops trusted gives the leftmost trusted", "127.0.0.1:4000", []string{"10.9.9.9"}, "10.9.9.9"},
		{"IPv6 proxy", "[::1]:4000", []string{"2001:db8::5"}, "2001:db8::5"},
		{"IPv4-mapped addresses are unmapped", "[::ffff:127.0.0.1]:4000", []string{"::ffff:203.0.113.7"}, "203.0.113.7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			for _, h := range c.xff {
				r.Header.Add("X-Forwarded-For", h)
			}
			if got := clientIP(r); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestForwardedForWithNothingTrustedIsTheConnection(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:4000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := ForwardedFor(nil)(r); got != "127.0.0.1" {
		t.Fatalf("got %q, want the connection's address", got)
	}
}

func TestParsePrefixesRejectsGarbage(t *testing.T) {
	for _, s := range []string{"10.0.0.0/33", "not-an-ip", "10.0.0.0/8,,x"} {
		if _, err := ParsePrefixes(s); err == nil {
			t.Errorf("ParsePrefixes(%q) accepted it", s)
		}
	}
	got, err := ParsePrefixes(" ")
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty list: got %v, %v", got, err)
	}
}

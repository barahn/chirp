// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// What a browser finds at the relay's address. People open it: it is in
// CHIRP_RELAY, in the README, in a report about the server. They get a page
// saying what the relay is and that Chirp never asks for a code in a web
// page, which is what a scammer would send them there for; search engines
// get told to stay out, since every other path carries credentials; and a
// security researcher, if the operator gives a contact, gets security.txt.
// None of it says which version runs, or reads any state.

//go:embed landing.html
var landingPage string

// landingCSP allows the page's one inline style, by its hash, and nothing
// else: no script, no request to anywhere, no framing.
var landingCSP = func() string {
	style := landingPage[strings.Index(landingPage, "<style>")+len("<style>") : strings.Index(landingPage, "</style>")]
	sum := sha256.Sum256([]byte(style))
	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) +
		"'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}()

const robotsTxt = "User-agent: *\nDisallow: /\n"

// securityTxtFor is how long a security.txt the relay serves stays valid.
// RFC 9116 asks for less than a year; the file is made on each request, so
// it never expires while the relay runs.
const securityTxtFor = 180 * 24 * time.Hour

var emailPattern = regexp.MustCompile(`^[^@\s:/]+@[^@\s:/]+\.[^@\s:/]+$`)

// securityContact turns the operator's contact into a security.txt Contact
// URI: an email address becomes mailto:, a mailto: or https: URI is kept,
// and anything else is refused. Empty means no security.txt.
func securityContact(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if emailPattern.MatchString(s) {
		return "mailto:" + s, nil
	}
	u, err := url.Parse(s)
	if err != nil || strings.ContainsAny(s, " \t\r\n") {
		return "", errors.New("not an email address or a URI")
	}
	switch {
	case u.Scheme == "mailto" && emailPattern.MatchString(u.Opaque):
		return s, nil
	case u.Scheme == "https" && u.Host != "":
		return s, nil
	}
	return "", fmt.Errorf("%q is neither an email address, a mailto: URI nor an https: URL", s)
}

// pages adds the landing page, robots.txt and, with a contact, security.txt.
func pages(mux *http.ServeMux, contact string, now func() time.Time) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", landingCSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "public, max-age=3600")
		_, _ = io.WriteString(w, landingPage)
	})
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = io.WriteString(w, robotsTxt)
	})
	if contact == "" {
		return
	}
	mux.HandleFunc("GET /.well-known/security.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		expires := now().UTC().Add(securityTxtFor).Truncate(24 * time.Hour)
		_, _ = fmt.Fprintf(w, "# This relay's operator. For flaws in Chirp itself:\n"+
			"# https://github.com/barahn/chirp/security/policy\n"+
			"Contact: %s\nExpires: %s\n", contact, expires.Format(time.RFC3339))
	})
}

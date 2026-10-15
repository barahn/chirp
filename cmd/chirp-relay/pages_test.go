// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, h http.Handler, method, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Result()
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func teapot() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
}

func TestLandingPage(t *testing.T) {
	h := routes(teapot(), "")
	r := get(t, h, http.MethodGet, "/")
	if r.StatusCode != http.StatusOK || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET /: %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	page := body(t, r)
	for _, want := range []string{"This is a Chirp relay", "never asks you to type a code", "Este é um relay do Chirp", "https://usechirp.app/"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
	// Nothing on it can run, be filled in, or fetch anything.
	for _, never := range []string{"<script", "<form", "<input", "<iframe", "<img", "src="} {
		if strings.Contains(strings.ToLower(page), never) {
			t.Errorf("the page has %s", never)
		}
	}

	// The policy admits exactly the page's one inline style.
	styles := regexp.MustCompile(`(?s)<style>(.*?)</style>`).FindAllStringSubmatch(page, -1)
	if len(styles) != 1 {
		t.Fatalf("%d style blocks, want 1", len(styles))
	}
	sum := sha256.Sum256([]byte(styles[0][1]))
	csp := r.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
		t.Errorf("CSP %q does not hash the page's style", csp)
	}
	for _, want := range []string{"default-src 'none'", "form-action 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %s", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe") || strings.Contains(csp, "script-src") {
		t.Errorf("CSP %q admits more than the style", csp)
	}

	if r := get(t, h, http.MethodHead, "/"); r.StatusCode != http.StatusOK {
		t.Errorf("HEAD /: %d", r.StatusCode)
	}
	// Only the root is the page: everything else, and anything but a read of
	// the root, is the relay's.
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/peers/x"}, {http.MethodPost, "/"}, {http.MethodGet, "/index.html"},
	} {
		if r := get(t, h, c.method, c.path); r.StatusCode != http.StatusTeapot {
			t.Errorf("%s %s: %d, want the relay's", c.method, c.path, r.StatusCode)
		}
	}
}

func TestRobotsTxtKeepsCrawlersOut(t *testing.T) {
	r := get(t, routes(teapot(), ""), http.MethodGet, "/robots.txt")
	if r.StatusCode != http.StatusOK || body(t, r) != "User-agent: *\nDisallow: /\n" {
		t.Fatalf("robots.txt: %d", r.StatusCode)
	}
}

func TestSecurityTxt(t *testing.T) {
	if r := get(t, routes(http.NotFoundHandler(), ""), http.MethodGet, "/.well-known/security.txt"); r.StatusCode != http.StatusNotFound {
		t.Errorf("without a contact: %d, want 404", r.StatusCode)
	}

	mux := http.NewServeMux()
	now := time.Date(2026, 10, 15, 13, 45, 0, 0, time.UTC)
	pages(mux, "mailto:security@example.org", func() time.Time { return now })
	r := get(t, mux, http.MethodGet, "/.well-known/security.txt")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("with a contact: %d", r.StatusCode)
	}
	txt := body(t, r)
	if !strings.Contains(txt, "\nContact: mailto:security@example.org\n") {
		t.Errorf("no Contact line:\n%s", txt)
	}
	m := regexp.MustCompile(`\nExpires: (\S+)\n`).FindStringSubmatch(txt)
	if m == nil {
		t.Fatalf("no Expires line:\n%s", txt)
	}
	expires, err := time.Parse(time.RFC3339, m[1])
	if err != nil {
		t.Fatal(err)
	}
	// RFC 9116: in the future, and less than a year away.
	if d := expires.Sub(now); d <= 0 || d >= 365*24*time.Hour {
		t.Errorf("Expires %s is %v from now", m[1], d)
	}
}

func TestSecurityContact(t *testing.T) {
	good := map[string]string{
		"":                             "",
		"security@example.org":         "mailto:security@example.org",
		" security@example.org ":       "mailto:security@example.org",
		"mailto:security@example.org":  "mailto:security@example.org",
		"https://example.org/security": "https://example.org/security",
	}
	for in, want := range good {
		got, err := securityContact(in)
		if err != nil || got != want {
			t.Errorf("securityContact(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"security", "http://example.org/security", "javascript:alert(1)", "mailto:nobody",
		"https:///nohost", "a b@example.org", "security@example.org\nExpires: never",
	} {
		if got, err := securityContact(in); err == nil {
			t.Errorf("securityContact(%q) = %q, want an error", in, got)
		}
	}
}

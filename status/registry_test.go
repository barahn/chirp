// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func node() Server {
	return Server{ID: "node-sp-01", Kind: KindNode, URL: "https://relay.example.org", Operator: "Example", GitHub: "example-ops", Added: "2026-10-01"}
}

func TestCheckServersAcceptsAGoodEntry(t *testing.T) {
	ss := []Server{node()}
	if err := CheckServers(ss); err != nil {
		t.Fatal(err)
	}
	if got := ss[0].AddedAt(); !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AddedAt = %v", got)
	}
	if ss[0].Listing() != ListingActive {
		t.Errorf("an entry without a state is %q, want active", ss[0].Listing())
	}
}

func TestCheckServersRefuses(t *testing.T) {
	cases := map[string]func(*Server){
		"id with capitals":      func(s *Server) { s.ID = "Node" },
		"id with double hyphen": func(s *Server) { s.ID = "a--b" },
		"unknown kind":          func(s *Server) { s.Kind = "mirror" },
		"kind not measured yet": func(s *Server) { s.Kind = KindNostr },
		"http":                  func(s *Server) { s.URL = "http://relay.example.org" },
		"ip address":            func(s *Server) { s.URL = "https://203.0.113.7" },
		"ipv6 address":          func(s *Server) { s.URL = "https://[2001:db8::1]" },
		"path":                  func(s *Server) { s.URL = "https://example.org/relay" },
		"query":                 func(s *Server) { s.URL = "https://example.org/?x=1" },
		"credentials":           func(s *Server) { s.URL = "https://u:p@example.org" },
		"no operator":           func(s *Server) { s.Operator = "" },
		"no github":             func(s *Server) { s.GitHub = "" },
		"bad github":            func(s *Server) { s.GitHub = "-ops" },
		"bad region":            func(s *Server) { s.Region = "bra" },
		"bad date":              func(s *Server) { s.Added = "15/10/2026" },
		"bad state":             func(s *Server) { s.State = "paused" },
	}
	for name, spoil := range cases {
		s := node()
		spoil(&s)
		if err := CheckServers([]Server{s}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := CheckServers([]Server{node(), node()}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a duplicate id: %v", err)
	}
}

func TestLoadServersRefusesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	doc := `{"servers": [{"id": "a", "kind": "node", "url": "https://a.example", "operator": "A", "github": "a", "added": "2026-10-01", "behind_cnd": true}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServers(path); err == nil || !strings.Contains(err.Error(), "behind_cnd") {
		t.Fatalf("a misspelt key: %v", err)
	}
}

func TestTheShippedRegistryLoads(t *testing.T) {
	if _, err := LoadServers("../network/servers.json"); err != nil {
		t.Fatal(err)
	}
}

func TestIsNew(t *testing.T) {
	ss := []Server{node()}
	if err := CheckServers(ss); err != nil {
		t.Fatal(err)
	}
	added := ss[0].AddedAt()
	if !ss[0].IsNew(added.Add(NewFor - time.Second)) {
		t.Error("not new a second before NewFor")
	}
	if ss[0].IsNew(added.Add(NewFor)) {
		t.Error("still new at NewFor")
	}
}

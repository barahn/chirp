// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package status measures the servers of Chirp's public network and serves
// what it measured: whether each is up now, since when it has been off-line,
// and how much of each day it answered (docs/status-page-proposal.md).
//
// Every figure comes from probes made from outside. Nothing a server says
// about itself is counted, and a minute nobody probed is "no data", never up
// and never down.
package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"time"
)

// Kind is what a member of the network is.
type Kind string

const (
	// KindNode is a chirp-relay in public mode: a dedicated server.
	KindNode Kind = "node"
	// KindNostr is a Nostr relay the network recommends for signalling.
	KindNostr Kind = "nostr"
	// KindTURN is a TURN server.
	KindTURN Kind = "turn"
)

// Listing is whether a member is in the network's list or suspended from it.
type Listing string

const (
	ListingActive    Listing = "active"
	ListingSuspended Listing = "suspended"
)

// NewFor is how long a member counts as new after it was added.
const NewFor = 30 * 24 * time.Hour

// Server is one member of the network, as network/servers.json lists it.
type Server struct {
	ID        string  `json:"id"`
	Kind      Kind    `json:"kind"`
	URL       string  `json:"url"`
	Operator  string  `json:"operator"`
	GitHub    string  `json:"github"`
	Contact   string  `json:"contact,omitempty"`
	Region    string  `json:"region,omitempty"`
	BehindCDN bool    `json:"behind_cdn"`
	Added     string  `json:"added"`
	State     Listing `json:"state,omitempty"`

	added time.Time
}

// AddedAt is the day the member was added, at 00:00 UTC.
func (s Server) AddedAt() time.Time { return s.added }

// IsNew reports whether the member was added less than NewFor before now.
func (s Server) IsNew(now time.Time) bool { return now.Sub(s.added) < NewFor }

// Listing is the member's state in the list; an entry without one is active.
func (s Server) Listing() Listing {
	if s.State == "" {
		return ListingActive
	}
	return s.State
}

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	githubPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	regionPattern = regexp.MustCompile(`^[A-Z]{2}$`)
)

// LoadServers reads and checks a registry file. A field the file does not
// know is an error, so a misspelt key is caught rather than ignored.
func LoadServers(path string) ([]Server, error) {
	f, err := os.Open(path) // #nosec G304 -- the operator names the registry
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return decodeServers(f)
}

func decodeServers(f *os.File) ([]Server, error) {
	var doc struct {
		Servers []Server `json:"servers"`
	}
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name(), err)
	}
	if err := CheckServers(doc.Servers); err != nil {
		return nil, fmt.Errorf("%s: %w", f.Name(), err)
	}
	return doc.Servers, nil
}

// CheckServers validates every entry and fills in the parsed fields. It
// reports every problem it finds, not just the first.
func CheckServers(servers []Server) error {
	var errs []error
	seen := make(map[string]bool)
	for i := range servers {
		s := &servers[i]
		if err := s.check(); err != nil {
			errs = append(errs, fmt.Errorf("server %d (%q): %w", i, s.ID, err))
		}
		if seen[s.ID] {
			errs = append(errs, fmt.Errorf("server %d: id %q is listed twice", i, s.ID))
		}
		seen[s.ID] = true
	}
	return errors.Join(errs...)
}

func (s *Server) check() error {
	var errs []error
	if len(s.ID) > 40 || !idPattern.MatchString(s.ID) {
		errs = append(errs, errors.New("id must be lowercase letters and digits joined by single hyphens, at most 40"))
	}
	switch s.Kind {
	case KindNode:
	case KindNostr, KindTURN:
		errs = append(errs, fmt.Errorf("kind %q is not measured yet: only %q is", s.Kind, KindNode))
	default:
		errs = append(errs, fmt.Errorf("kind %q is not one of node, nostr, turn", s.Kind))
	}
	if err := checkURL(s.URL); err != nil {
		errs = append(errs, err)
	}
	if s.Operator == "" {
		errs = append(errs, errors.New("operator is required"))
	}
	if !githubPattern.MatchString(s.GitHub) {
		errs = append(errs, errors.New("github must be the operator's GitHub handle: alerts reach them through it"))
	}
	if s.Region != "" && !regionPattern.MatchString(s.Region) {
		errs = append(errs, errors.New("region must be an ISO 3166 two-letter code in capitals"))
	}
	added, err := time.Parse(time.DateOnly, s.Added)
	if err != nil {
		errs = append(errs, errors.New("added must be a date, YYYY-MM-DD"))
	}
	s.added = added
	switch s.State {
	case "", ListingActive, ListingSuspended:
	default:
		errs = append(errs, fmt.Errorf("state %q is not active or suspended", s.State))
	}
	return errors.Join(errs...)
}

// checkURL accepts only what the conformance rules allow a node to be
// reached at: HTTPS, by a domain name, at its root.
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	switch {
	case u.Scheme != "https":
		return errors.New("url must be https")
	case u.Hostname() == "":
		return errors.New("url has no host")
	case net.ParseIP(u.Hostname()) != nil:
		return errors.New("url must name a domain, not an IP address")
	case u.User != nil, u.RawQuery != "", u.Fragment != "", u.Path != "" && u.Path != "/":
		return errors.New("url must be the server's root, with nothing after the host")
	}
	return nil
}

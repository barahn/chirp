// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

import (
	"errors"
	"net/url"
	"strings"

	"github.com/barahn/remotekit/rendezvous"
)

// ErrNotATarget means the input is neither a code nor a link. It is decided
// here, before anything reaches the relay, where every attempt is metered
// (product-model decision 4).
var ErrNotATarget = errors.New("client: not a code or a link")

// Target is what someone typed or pasted into "Help someone": exactly one of
// a code or a link's token.
type Target struct {
	Code  string // six digits, normalised
	Token string // a link's token
}

// ParseTarget reads a code as people say it ("482 913", "482-913"), a link as
// the page shows it (its token in the fragment, after "#", where the browser
// never sends it), or a link's bare token.
func ParseTarget(s string) (Target, error) {
	s = strings.TrimSpace(s)
	if c, err := rendezvous.Normalize(s); err == nil {
		return Target{Code: string(c)}, nil
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return Target{}, ErrNotATarget
		}
		s = u.Fragment
	}
	if isToken(s) {
		return Target{Token: s}, nil
	}
	return Target{}, ErrNotATarget
}

// isToken reports whether s has a link token's form: 32 random bytes in
// unpadded base64url, which is 43 characters (rendezvous.IssueLink).
func isToken(s string) bool {
	if len(s) != 43 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

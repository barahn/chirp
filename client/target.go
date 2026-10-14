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
// a code or a link's token, and the secret part that came with it, if any.
type Target struct {
	Code  string // six digits, normalised: the routing part
	Token string // a link's token
	// Secret is the four digits that follow the routing part of a code
	// ("482-913-5071"), or a link's token (".5071"). The server never sees
	// them: they are the password of the PAKE that binds the two Chirps
	// (docs/product-model.md, decision 5).
	Secret string
}

// SecretLength is how many digits the secret part of a code has.
const SecretLength = 4

// ParseTarget reads a code as people say it ("482 913", "482-913"), a link as
// the page shows it (its token in the fragment, after "#", where the browser
// never sends it), or a link's bare token.
//
// A code has six digits, or ten: six that route and four that are secret,
// read as "482-913-5071". A link's token may carry the secret after a dot.
func ParseTarget(s string) (Target, error) {
	s = strings.TrimSpace(s)
	if c, err := rendezvous.Normalize(s); err == nil {
		return Target{Code: string(c)}, nil
	}
	if digits, ok := codeDigits(s); ok && len(digits) == rendezvous.CodeLength+SecretLength {
		return Target{Code: digits[:rendezvous.CodeLength], Secret: digits[rendezvous.CodeLength:]}, nil
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return Target{}, ErrNotATarget
		}
		s = u.Fragment
	}
	token, secret, withSecret := strings.Cut(s, ".")
	if !isToken(token) {
		return Target{}, ErrNotATarget
	}
	if withSecret && !isSecret(secret) {
		return Target{}, ErrNotATarget
	}
	return Target{Token: token, Secret: secret}, nil
}

// codeDigits reads s as rendezvous.Normalize does -- digits, with spaces,
// dashes and dots between them -- whatever its length.
func codeDigits(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '-' || r == '.':
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	return b.String(), true
}

func isSecret(s string) bool {
	if len(s) != SecretLength {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

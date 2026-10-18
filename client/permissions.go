// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

// withheld are the permissions this Chirp never grants, whatever a request
// asks for. On the trusted-relay path the relay writes the request, so the
// owner's side has to decide this itself: a permission dropped here is never
// shown in the consent prompt, and so never granted.
//
// None is withheld now. The clipboard was from v0.3.1, because it went to the
// controller without the owner being asked or told (GHSA-8jh9-jw95-3v27). It
// is now a switch in the prompt, off until the owner turns it on, every sync
// is told to the owner, and passwords never leave (barahn/chirp#134).
var withheld = map[string]bool{}

// Grantable returns the permissions in requested that this Chirp may grant,
// in their order: everything but the withheld ones. A name Chirp does not
// know is kept, so the prompt shows it rather than granting anything unseen.
func Grantable(requested []string) []string {
	out := make([]string, 0, len(requested))
	for _, p := range requested {
		if !withheld[p] {
			out = append(out, p)
		}
	}
	return out
}

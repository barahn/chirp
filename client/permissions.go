// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package client

// withheld are the permissions this Chirp never grants, whatever a request
// asks for. On the trusted-relay path the relay writes the request, so the
// owner's side has to decide this itself: a permission dropped here is never
// shown in the consent prompt, and so never granted.
//
// clipboard: once remotekit's runner is granted it, the runner polls the
// system clipboard and sends every change to the controller -- through the
// relay, too, while a viewer has no data channel. That reads the owner's
// clipboard without the owner pasting, which decision 4 in
// docs/product-model.md rules out. It stays withheld until the clipboard
// moves only on an explicit action at either end (barahn/chirp#79).
var withheld = map[string]bool{
	"clipboard": true,
}

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

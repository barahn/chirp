// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"sync"

	"github.com/barahn/chirp/cmd/chirp/internal/chirpsound"
)

// The chirps (barahn/chirp#81) are synthesized once each and kept: Windows
// plays them straight from memory, and needs the bytes to stay put while it
// does.
var (
	wavsMu sync.Mutex
	wavs   = map[chirpsound.Sound][]byte{}
)

// wav is s's WAV file, made on first use.
func wav(s chirpsound.Sound) []byte {
	wavsMu.Lock()
	defer wavsMu.Unlock()
	b, ok := wavs[s]
	if !ok {
		b = chirpsound.WAV(s)
		wavs[s] = b
	}
	return b
}

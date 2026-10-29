// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/chirpsound"
)

// newSoundPlayer plays a chirp with afplay, which every Mac has. It takes a
// file, so the WAV is written to a temporary one and removed after.
func newSoundPlayer() func(chirpsound.Sound) {
	if _, err := exec.LookPath("afplay"); err != nil {
		return nil
	}
	return func(s chirpsound.Sound) {
		go func() {
			f, err := os.CreateTemp("", "chirp-*.wav")
			if err != nil {
				return
			}
			defer func() { _ = os.Remove(f.Name()) }()
			_, err = f.Write(wav(s))
			if cerr := f.Close(); err != nil || cerr != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, "afplay", f.Name()).Run() // #nosec G204 -- a file this function made
		}()
	}
}

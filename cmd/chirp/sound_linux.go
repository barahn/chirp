// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/chirpsound"
)

// newSoundPlayer plays a chirp through the first of PipeWire's pw-play,
// PulseAudio's paplay or ALSA's aplay it finds, feeding it the WAV on its
// standard input. With none of them Chirp is silent: Linux has no one
// sound API every desktop has, and these need no library linked in.
func newSoundPlayer() func(chirpsound.Sound) {
	var command func(ctx context.Context) *exec.Cmd
	switch {
	case found("pw-play"):
		command = func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "pw-play", "-") }
	case found("paplay"):
		command = func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "paplay") }
	case found("aplay"):
		command = func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "aplay", "-q", "-") }
	default:
		return nil
	}
	return func(s chirpsound.Sound) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := command(ctx)
			cmd.Stdin = bytes.NewReader(wav(s))
			_ = cmd.Run()
		}()
	}
}

// found reports whether name is on the PATH.
func found(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !linux && !windows && !darwin

package main

import "github.com/barahn/chirp/cmd/chirp/internal/chirpsound"

// newSoundPlayer has no way to play a chirp here: Chirp is silent.
func newSoundPlayer() func(chirpsound.Sound) { return nil }

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/barahn/chirp/cmd/chirp/internal/chirpsound"
)

var procPlaySound = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

// newSoundPlayer plays a chirp with PlaySound, from memory and without
// waiting, as Windows plays its own sounds. A new chirp cuts the one
// before short, as system sounds do.
func newSoundPlayer() func(chirpsound.Sound) {
	if procPlaySound.Find() != nil {
		return nil
	}
	return func(s chirpsound.Sound) {
		const (
			sndAsync     = 0x0001
			sndNoDefault = 0x0002
			sndMemory    = 0x0004
		)
		b := wav(s)
		_, _, _ = procPlaySound.Call(uintptr(unsafe.Pointer(&b[0])), 0, sndMemory|sndAsync|sndNoDefault)
	}
}

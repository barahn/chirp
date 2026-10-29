// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package chirpsound

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

var all = []Sound{LetIn, Landed, Alert, Leaving}

// samples reads a WAV this package made: its header, and its samples.
func samples(t *testing.T, s Sound) []int16 {
	t.Helper()
	b := WAV(s)
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || string(b[36:40]) != "data" {
		t.Fatalf("%v: not a WAV file", s)
	}
	if got := binary.LittleEndian.Uint32(b[4:8]); int(got) != len(b)-8 {
		t.Fatalf("%v: RIFF size %d, want %d", s, got, len(b)-8)
	}
	if ch, rate, bits := binary.LittleEndian.Uint16(b[22:24]), binary.LittleEndian.Uint32(b[24:28]), binary.LittleEndian.Uint16(b[34:36]); ch != 1 || rate != Rate || bits != 16 {
		t.Fatalf("%v: %d channels at %d Hz, %d bits; want mono %d Hz 16-bit", s, ch, rate, bits, Rate)
	}
	out := make([]int16, (len(b)-44)/2)
	if err := binary.Read(bytes.NewReader(b[44:]), binary.LittleEndian, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func peak(xs []int16) float64 {
	m := 0.0
	for _, x := range xs {
		m = math.Max(m, math.Abs(float64(x)))
	}
	return m / math.MaxInt16
}

// Each sound is a short chirp: under half a second, never clipped, and
// starting and ending in silence, so it does not click.
func TestEachSoundIsAShortCleanChirp(t *testing.T) {
	for _, s := range all {
		xs := samples(t, s)
		if d := float64(len(xs)) / Rate; d < 0.05 || d > 0.5 {
			t.Errorf("%v lasts %.2f s, want a short chirp", s, d)
		}
		if p := peak(xs); p > 0.9 || p < 0.1 {
			t.Errorf("%v peaks at %.2f of full scale, want audible and unclipped", s, p)
		}
		if p := peak(xs[:20]); p > 0.01 {
			t.Errorf("%v starts at %.3f, want silence", s, p)
		}
		if p := peak(xs[len(xs)-20:]); p > 0.01 {
			t.Errorf("%v ends at %.3f, want silence", s, p)
		}
	}
}

// An alert is quieter than the landing: it comes often, and is a nudge.
func TestAnAlertIsSofterThanTheLanding(t *testing.T) {
	if a, l := peak(samples(t, Alert)), peak(samples(t, Landed)); a >= l*0.8 {
		t.Fatalf("alert peaks at %.2f, landing at %.2f: want the alert softer", a, l)
	}
}

// The sounds can be told apart, and each is the same every time.
func TestTheSoundsDifferAndRepeat(t *testing.T) {
	seen := map[string]Sound{}
	for _, s := range all {
		b := string(WAV(s))
		if o, ok := seen[b]; ok {
			t.Fatalf("%v sounds the same as %v", s, o)
		}
		seen[b] = s
		if string(WAV(s)) != b {
			t.Fatalf("%v is not the same twice", s)
		}
	}
}

// It sounds like a bird: high, in the range of a small bird's call.
func TestItIsABirdsPitch(t *testing.T) {
	xs := samples(t, Landed)
	crossings := 0
	for i := 1; i < len(xs); i++ {
		if (xs[i-1] < 0) != (xs[i] < 0) {
			crossings++
		}
	}
	hz := float64(crossings) / 2 / (float64(len(xs)) / Rate)
	// Silences between notes pull the average down; the notes sing above it.
	if hz < 1200 || hz > 6000 {
		t.Fatalf("about %.0f Hz on average, want a bird's 1.2 to 6 kHz", hz)
	}
}

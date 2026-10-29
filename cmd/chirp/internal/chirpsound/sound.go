// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package chirpsound synthesizes Chirp's namesake sound (barahn/chirp#81):
// short rising or falling chirps, made in code rather than recorded, so
// there is no audio file to ship and no licence to clear. Each Sound is a
// tiny mono WAV, the same every time, for the system to play.
package chirpsound

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Rate is the sample rate, in samples a second.
const Rate = 44100

// Sound is one of Chirp's chirps.
type Sound int

const (
	// LetIn is heard by the person helping when they are let in: three
	// quick rising notes, the good news for both people.
	LetIn Sound = iota + 1
	// Landed is the bird by the taskbar arriving: two rising notes.
	Landed
	// Alert is one short, soft note: something the person helping did.
	Alert
	// Leaving is the bird flying off: three quick rising notes, each
	// starting lower and softer, as a call fades into the distance. A
	// single falling sweep sounded like a robot, not a bird.
	Leaving
)

func (s Sound) String() string {
	switch s {
	case LetIn:
		return "let in"
	case Landed:
		return "landed"
	case Alert:
		return "alert"
	case Leaving:
		return "leaving"
	}
	return "unknown"
}

// note is one chirp: a pitch sweep from Hz to To over Length seconds, at
// Level of full scale, after Gap seconds of silence.
type note struct {
	Gap, Length, Hz, To, Level float64
}

var tunes = map[Sound][]note{
	LetIn:   {{0, .07, 2600, 3600, .55}, {.03, .07, 3000, 4000, .6}, {.03, .10, 3400, 4600, .65}},
	Landed:  {{0, .08, 2400, 3800, .6}, {.04, .10, 2800, 4300, .6}},
	Alert:   {{0, .07, 3000, 3900, .35}},
	Leaving: {{0, .05, 3900, 4600, .5}, {.035, .05, 3400, 4100, .42}, {.035, .06, 2900, 3600, .34}},
}

// WAV is s as a 16-bit mono PCM WAV file at Rate.
func WAV(s Sound) []byte {
	pcm := synth(tunes[s])
	var b bytes.Buffer
	b.WriteString("RIFF")
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	le(uint32(36 + 2*len(pcm))) // #nosec G115 -- under a second of samples
	b.WriteString("WAVEfmt ")
	le(uint32(16))
	le(uint16(1)) // PCM
	le(uint16(1)) // mono
	le(uint32(Rate))
	le(uint32(Rate * 2)) // bytes a second
	le(uint16(2))        // bytes a frame
	le(uint16(16))       // bits a sample
	b.WriteString("data")
	le(uint32(2 * len(pcm))) // #nosec G115 -- as above
	le(pcm)
	return b.Bytes()
}

// synth renders notes in turn, each a sine whose pitch sweeps along a
// curve that rises fast and settles, as a small bird's call does, with a
// quick attack and a smooth fade so nothing clicks.
func synth(notes []note) []int16 {
	// A moment of silence first: some outputs wake up on the first sample.
	out := make([]int16, Rate/200)
	for _, n := range notes {
		out = append(out, make([]int16, int(n.Gap*Rate))...)
		count := int(n.Length * Rate)
		phase := 0.0
		for i := range count {
			t := float64(i) / float64(count)
			sweep := 1 - (1-t)*(1-t)*(1-t)
			hz := n.Hz + (n.To-n.Hz)*sweep
			phase += 2 * math.Pi * hz / Rate
			// Rises over 12 ms and fades over the last 60% of the note,
			// both along a squared sine, which starts and ends flat.
			attack := math.Pow(math.Sin(math.Pi/2*math.Min(1, t*n.Length/0.012)), 2)
			fade := math.Pow(math.Sin(math.Pi/2*math.Min(1, (1-t)/0.6)), 2)
			env := attack * fade
			// A touch of the second harmonic makes it a chirp, not a beep.
			v := math.Sin(phase) + 0.25*math.Sin(2*phase)
			out = append(out, int16(math.Round(n.Level/1.25*env*v*math.MaxInt16)))
		}
	}
	// A short tail of silence, so the last note's fade is not cut.
	return append(out, make([]int16, Rate/100)...)
}

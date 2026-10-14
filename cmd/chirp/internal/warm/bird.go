// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package warm

import (
	"embed"

	"fyne.io/fyne/v2"
)

// Pose is one of the bird's states (docs/ux.md, The bird's states). Each is
// one SVG in bird/, drawn by Fyne itself; the art keeps to what Fyne's
// rasteriser draws, so it has no text, filters or masks.
type Pose string

const (
	Perched  Pose = "perched"  // ready: the code and link work
	Blink    Pose = "blink"    // ready, now and then
	Landing  Pose = "landing"  // starting
	Looking  Pose = "looking"  // reconnecting, within the grace
	Asleep   Pose = "asleep"   // offline
	Hop      Pose = "hop"      // the code was replaced
	Chirping Pose = "chirping" // helping: waiting to be let in
	Flying   Pose = "flying"   // helping: let in
)

//go:embed bird/*.svg
var birdFiles embed.FS

var birds = func() map[Pose]fyne.Resource {
	m := map[Pose]fyne.Resource{}
	for _, p := range []Pose{Perched, Blink, Landing, Looking, Asleep, Hop, Chirping, Flying} {
		m[p] = load(string(p) + ".svg")
	}
	return m
}()

// AppIcon is the perched bird, close up, on a near-round teal tile, for the
// window and the operating system. The tile is a path with arcs rather than a
// rect with rx: Fyne's SVG renderer ignores rx, and drew the corners square.
var AppIcon = load("icon.svg")

func load(name string) fyne.Resource {
	b, err := birdFiles.ReadFile("bird/" + name)
	if err != nil {
		// The files are embedded at build time; a missing one is a bug in
		// this package, caught by its tests.
		panic("warm: no bird/" + name)
	}
	return fyne.NewStaticResource(name, b)
}

// Bird returns the art for p, or the perched bird for a pose it does not
// know.
func Bird(p Pose) fyne.Resource {
	if r, ok := birds[p]; ok {
		return r
	}
	return birds[Perched]
}

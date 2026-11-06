// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package perch

import (
	"image"
	"math"
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// The bird walks by the taskbar as the site's bird walks on the app window
// (barahn/chirp#187, site/assets/hop.js): small hops along its stretch, a
// rest between them, now and then two short hops in a row, and away when
// the pointer passes over it three times in a row -- to the far end, or to
// the top of a window, and walks there. The rules
// are the site's, in the site's pixels, where the bird is 74 pixels wide;
// Scale turns them into the screen's.

// Rules are how the bird walks, in site pixels.
type Rules struct {
	Shortest, Longest           float64
	ShortestRest, LongestRest   time.Duration
	TurnChance, PairChance      float64
	PairShortest, PairLongest   float64
	PairBeat                    time.Duration
	ChirpChance                 float64
	Chirping                    time.Duration // how long a chirp shows
	AfterFlee                   time.Duration // the rest after a shoo
	WindowChance                float64       // of a shoo from the taskbar going to a window
	FlightSpeed                 float64       // to and from windows, site pixels a second
	LongestFlight               time.Duration
	Passes                      int
	Within                      time.Duration
	HopTime, PairTime, FleeTime time.Duration
	HopLift, PairLift, FleeLift float64
	Flapping                    time.Duration // of FleeTime
}

// Site is the site's walk, from hop.js and site.js.
var Site = Rules{
	Shortest: 18, Longest: 30,
	ShortestRest: 800 * time.Millisecond, LongestRest: 3000 * time.Millisecond,
	TurnChance: 0.12, PairChance: 0.25,
	PairShortest: 10, PairLongest: 15, PairBeat: 140 * time.Millisecond,
	ChirpChance: 0.3, Chirping: 700 * time.Millisecond, AfterFlee: 1500 * time.Millisecond,
	WindowChance: 0.5, FlightSpeed: 1200, LongestFlight: 2 * time.Second,
	Passes: 3, Within: 4 * time.Second,
	HopTime: 320 * time.Millisecond, PairTime: 200 * time.Millisecond, FleeTime: 700 * time.Millisecond,
	HopLift: 10, PairLift: 6, FleeLift: 60,
	Flapping: 640 * time.Millisecond,
}

// sitePixel is a site pixel in the art's units: the site draws the art's
// 120 units 74 pixels wide.
const sitePixel = 120.0 / 74

// stretchLength is how far the bird walks from its spot, in site pixels.
const stretchLength = 300

// Scale is how many screen pixels a site pixel is, for a bird window width
// pixels wide.
func Scale(width int) float64 {
	return float64(width) * 120 / canvasW / 74
}

// Bounds are how far the bird may stand from its spot, in screen pixels:
// Min to the left, up to Max, which is the spot itself.
type Bounds struct{ Min, Max int }

// Stretch is where a bird standing at spot walks: to its left, as far as
// the work area allows.
func Stretch(spot, work image.Rectangle, scale float64) Bounds {
	return Bounds{Min: max(-int(math.Round(stretchLength*scale)), work.Min.X-spot.Min.X)}
}

// At is where the bird stands: X pixels from its spot, Facing 1 for right
// and -1 for left. Pair is 1 after the first of two short hops, and 2 after
// the second.
type At struct {
	X, Facing, Pair int
}

// Hop is where a hop lands, and the rest after it.
type Hop struct {
	At
	Rest time.Duration
}

// NextHop is the bird's next hop from at, as hop.js's nextHop: random
// returns numbers from 0 to 1.
func NextHop(at At, b Bounds, r Rules, scale float64, random func() float64) Hop {
	between := func(lo, hi float64) float64 { return (lo + (hi-lo)*random()) * scale }
	room := func(x, facing int) int {
		if facing > 0 {
			return b.Max - x
		}
		return x - b.Min
	}
	rest := func() time.Duration {
		return r.ShortestRest + time.Duration(random()*float64(r.LongestRest-r.ShortestRest))
	}
	x := min(b.Max, max(b.Min, at.X))
	if at.Pair == 1 {
		length := min(between(r.PairShortest, r.PairLongest), float64(room(x, at.Facing)))
		return Hop{At{X: x + at.Facing*round(length), Facing: at.Facing, Pair: 2}, rest()}
	}
	facing := at.Facing
	if random() < r.TurnChance {
		facing = -facing
	}
	if random() < r.PairChance && float64(room(x, facing)) >= 2*r.PairLongest*scale {
		return Hop{At{X: x + facing*round(between(r.PairShortest, r.PairLongest)), Facing: facing, Pair: 1}, r.PairBeat}
	}
	length := between(r.Shortest, r.Longest)
	if float64(room(x, facing)) < length {
		facing = -facing
		length = min(length, float64(room(x, facing)))
	}
	return Hop{At{X: x + facing*round(length), Facing: facing}, rest()}
}

// Shooer counts the pointer's passes over the bird; each call is one pass,
// at now, and says whether it was the one that shoos.
func Shooer(r Rules) func(now time.Time) bool {
	var passes []time.Time
	return func(now time.Time) bool {
		kept := passes[:0]
		for _, t := range passes {
			if now.Sub(t) < r.Within {
				kept = append(kept, t)
			}
		}
		passes = append(kept, now)
		if len(passes) < r.Passes {
			return false
		}
		passes = nil
		return true
	}
}

// Flee is where a shooed bird goes: the far end of its stretch, flying the
// way it flies, then facing back, toward the pointer.
func Flee(at At, b Bounds) (to At, flies int) {
	if at.X-b.Min > b.Max-at.X {
		return At{X: b.Min, Facing: 1}, -1
	}
	return At{X: b.Max, Facing: -1}, 1
}

// Ledge is somewhere the bird walks: standing at Spot, in screen pixels,
// and Bounds from there.
type Ledge struct {
	Spot   image.Rectangle
	Bounds Bounds
}

// Ledges are the tops of windows, given top first, where a bird window w by
// h pixels can stand and walk: the widest part of each top edge that shows,
// with the space above it, where the bird stands, clear of the windows
// above and on the work area.
func Ledges(windows []image.Rectangle, work image.Rectangle, w, h int) []Ledge {
	reach := image.Rectangle{Min: Hit(w, false).Min, Max: Hit(w, true).Max}
	var ledges []Ledge
	for i, win := range windows {
		top := win.Min.Y
		head := HeadY(image.Rect(0, top-h, w, top))
		if head < work.Min.Y || top >= work.Max.Y {
			continue
		}
		band := image.Rect(win.Min.X, head, win.Max.X, top+1).Intersect(work)
		if band.Empty() {
			continue
		}
		spans := [][2]int{{band.Min.X, band.Max.X}}
		for _, above := range windows[:i] {
			if above.Overlaps(band) {
				spans = cut(spans, above.Min.X, above.Max.X)
			}
		}
		var widest [2]int
		for _, sp := range spans {
			if sp[1]-sp[0] > widest[1]-widest[0] {
				widest = sp
			}
		}
		rightmost, leftmost := widest[1]-reach.Max.X, widest[0]-reach.Min.X
		if rightmost-leftmost < w {
			continue
		}
		ledges = append(ledges, Ledge{Spot: image.Rect(rightmost, top-h, rightmost+w, top), Bounds: Bounds{Min: leftmost - rightmost}})
	}
	return ledges
}

// cut takes lo to hi out of spans.
func cut(spans [][2]int, lo, hi int) [][2]int {
	var kept [][2]int
	for _, sp := range spans {
		if sp[0] < lo {
			kept = append(kept, [2]int{sp[0], min(sp[1], lo)})
		}
		if sp[1] > hi {
			kept = append(kept, [2]int{max(sp[0], hi), sp[1]})
		}
	}
	return kept
}

// Moment is one frame of the walk: the bird window's top left At, on the
// screen, and the bird in it moved by Motion, in Pose.
type Moment struct {
	At     image.Point
	Motion Motion
	Pose   warm.Pose
}

// Walker walks the bird along its ledge, a frame at a time: its stretch of
// the taskbar, or a window's top.
type Walker struct {
	home, ledge Ledge
	scale       float64
	fps         int
	rules       Rules
	random      func() float64
	shoo        func(time.Time) bool
	at          At
}

// NewWalker starts a bird on its spot by the tray, facing right, as it
// lands.
func NewWalker(home Ledge, scale float64, fps int, r Rules, random func() float64) *Walker {
	return &Walker{home: home, ledge: home, scale: scale, fps: fps, rules: r, random: random, shoo: Shooer(r), at: At{Facing: 1}}
}

// At is where the bird stands now, on its ledge.
func (w *Walker) At() At { return w.at }

// Away reports the window's ledge the bird is on, if it is on one.
func (w *Walker) Away() (Ledge, bool) { return w.ledge, w.ledge != w.home }

// Still is the bird standing where it is, in pose.
func (w *Walker) Still(pose warm.Pose) Moment {
	return Moment{At: w.point(w.at.X), Motion: Motion{Alpha: 1, Left: w.at.Facing < 0}, Pose: pose}
}

func (w *Walker) point(x int) image.Point { return w.ledge.Spot.Min.Add(image.Pt(x, 0)) }

// Hop is the bird's next hop: its frames, the rest after it, and how long
// it chirps when it lands, if it does.
func (w *Walker) Hop() (moments []Moment, rest, chirp time.Duration) {
	hop := NextHop(w.at, w.ledge.Bounds, w.rules, w.scale, w.random)
	duration, lift := w.rules.HopTime, w.rules.HopLift
	if hop.Pair != 0 {
		duration, lift = w.rules.PairTime, w.rules.PairLift
	}
	moments = w.arc(w.at.X, hop.At, duration, lift, 0)
	w.at = hop.At
	if hop.Pair != 1 && w.random() < w.rules.ChirpChance {
		chirp = w.rules.Chirping
	}
	return moments, hop.Rest, chirp
}

// Pass is the pointer passing over the bird at now, with ledges the windows'
// tops it may fly to. When it is shooed: off a window, back to its spot by
// the tray; from the taskbar, now and then to one of ledges, otherwise to
// the far end of its stretch. It returns the flight's frames, with its turn
// to face back at the end, and the rest after them; nil when not shooed.
func (w *Walker) Pass(now time.Time, ledges []Ledge) ([]Moment, time.Duration) {
	if !w.shoo(now) {
		return nil, 0
	}
	if _, away := w.Away(); away {
		return w.flyTo(w.home), w.rules.AfterFlee
	}
	if len(ledges) > 0 && w.random() < w.rules.WindowChance {
		return w.flyTo(ledges[int(w.random()*float64(len(ledges)))%len(ledges)]), w.rules.AfterFlee
	}
	to, flies := Flee(w.at, w.ledge.Bounds)
	flight := w.arc(w.at.X, At{X: to.X, Facing: flies}, w.rules.FleeTime, w.rules.FleeLift, w.rules.Flapping)
	turn := w.arc(to.X, to, w.rules.PairTime, w.rules.PairLift, 0)
	w.at = to
	return append(flight, turn[1:]...), w.rules.AfterFlee
}

// Home flies the bird back to its spot by the tray from a window's top, as
// when the window goes; nil when it is on the taskbar.
func (w *Walker) Home() []Moment {
	if _, away := w.Away(); !away {
		return nil
	}
	return w.flyTo(w.home)
}

// flyTo flies the bird from where it stands to l's right end, flapping, in
// an arc over both, then turns it to face back the way it came.
func (w *Walker) flyTo(l Ledge) []Moment {
	from, to := w.point(w.at.X), l.Spot.Min
	flies := -1
	if to.X > from.X {
		flies = 1
	}
	d := to.Sub(from)
	distance := math.Hypot(float64(d.X), float64(d.Y))
	duration := min(w.rules.FleeTime+time.Duration(distance/(w.rules.FlightSpeed*w.scale)*float64(time.Second)), w.rules.LongestFlight)
	lift := w.rules.FleeLift * w.scale
	ease := cubicBezier(.42, 0, .58, 1)
	n := max(2, round(float64(w.fps)*duration.Seconds()))
	moments := make([]Moment, n)
	for i := range moments {
		t := float64(i) / float64(n-1)
		p := ease(t)
		pose := warm.Flying
		if t == 1 {
			pose = warm.Perched
		}
		moments[i] = Moment{
			At:     from.Add(image.Pt(round(float64(d.X)*p), round(float64(d.Y)*p-lift*4*p*(1-p)))),
			Motion: Motion{Alpha: 1, Left: flies < 0},
			Pose:   pose,
		}
	}
	w.ledge, w.at = l, At{Facing: -flies}
	turn := w.arc(0, w.at, w.rules.PairTime, w.rules.PairLift, 0)
	return append(moments, turn[1:]...)
}

// arc is a hop of duration along the ledge from x to to, lift site pixels
// high at its middle, eased in and out, facing to's way; the bird flaps for
// the first flapping of it.
func (w *Walker) arc(x int, to At, duration time.Duration, lift float64, flapping time.Duration) []Moment {
	ease := cubicBezier(.42, 0, .58, 1)
	n := max(2, int(math.Round(float64(w.fps)*duration.Seconds())))
	moments := make([]Moment, n)
	for i := range moments {
		t := float64(i) / float64(n-1)
		p := ease(t)
		pose := warm.Perched
		if time.Duration(t*float64(duration)) < flapping {
			pose = warm.Flying
		}
		moments[i] = Moment{
			At:     w.point(x + round(float64(to.X-x)*p)),
			Motion: Motion{DY: -lift * sitePixel * 4 * p * (1 - p), Alpha: 1, Left: to.Facing < 0},
			Pose:   pose,
		}
	}
	return moments
}

func round(f float64) int { return int(math.Round(f)) }

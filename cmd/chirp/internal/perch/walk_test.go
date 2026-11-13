// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package perch

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// seeded is a small deterministic random source, so a walk can be replayed;
// the same one site/assets/hop.test.js uses.
func seeded(seed uint32) func() float64 {
	s := seed
	return func() float64 {
		s = s*1664525 + 1013904223
		return float64(s) / 4294967296
	}
}

type step struct{ from, to Hop }

func walkFrom(at At, n int, b Bounds, scale float64, random func() float64) []step {
	steps := make([]step, 0, n)
	from := Hop{At: at}
	for range n {
		to := NextHop(from.At, b, Site, scale, random)
		steps = append(steps, step{from, to})
		from = to
	}
	return steps
}

func walk(n int, b Bounds, scale float64, random func() float64) []step {
	return walkFrom(At{Facing: 1}, n, b, scale, random)
}

// The stretch the bird walks: 300 site pixels to the left of its spot by the
// tray, at 96 DPI.
var (
	stretch = Bounds{Min: -300, Max: 0}
	home    = Ledge{Spot: image.Rect(1582, 922, 1692, 1032), Bounds: stretch}
)

// x is how far a moment's bird stands from its spot by the tray.
func x(m Moment) int { return m.At.X - home.Spot.Min.X }

// It walks only to the left of its spot, so never over the tray, and never
// past the end of its stretch.
func TestTheBirdHopsWithinItsStretch(t *testing.T) {
	for _, s := range walk(2000, stretch, 1, seeded(1)) {
		if s.to.X < stretch.Min || s.to.X > stretch.Max {
			t.Fatalf("hopped to %d, outside %v", s.to.X, stretch)
		}
	}
}

// Each hop is a small one, the way it faces, and scales with the screen's
// DPI.
func TestHopsAreSmallAndGoTheWayItFaces(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		b := Bounds{Min: int(-300 * scale)}
		for _, s := range walk(2000, b, scale, seeded(2)) {
			d := s.to.X - s.from.X
			if math.Abs(float64(d)) > Site.Longest*scale+1 {
				t.Fatalf("at scale %g, a hop of %d pixels", scale, d)
			}
			if d != 0 && sign(d) != s.to.Facing {
				t.Fatalf("hopped %d facing %d", d, s.to.Facing)
			}
		}
	}
}

// It rests between hops, for a moment.
func TestTheBirdRestsBetweenHops(t *testing.T) {
	for _, s := range walk(500, stretch, 1, seeded(4)) {
		if s.to.Pair == 1 {
			continue
		}
		if s.to.Rest < Site.ShortestRest || s.to.Rest > Site.LongestRest {
			t.Fatalf("rested %v", s.to.Rest)
		}
	}
}

// It reaches both ends of its stretch, and comes back.
func TestTheBirdGoesFromOneEndToTheOther(t *testing.T) {
	steps := walk(400, stretch, 1, seeded(5))
	reached := func(x int) bool {
		for _, s := range steps {
			if math.Abs(float64(s.to.X-x)) < Site.Longest {
				return true
			}
		}
		return false
	}
	if !reached(stretch.Min) || !reached(stretch.Max) {
		t.Fatal("did not walk from one end of its stretch to the other")
	}
}

// At an end it turns round instead of hopping past it.
func TestTheBirdTurnsAtAnEnd(t *testing.T) {
	hop := NextHop(At{X: stretch.Min, Facing: -1}, stretch, Site, 1, func() float64 { return 0.5 })
	if hop.Facing != 1 || hop.X <= stretch.Min {
		t.Fatalf("at the left end it went to %+v, want turned round and back in", hop.At)
	}
	hop = NextHop(At{X: 0, Facing: 1}, stretch, Site, 1, func() float64 { return 0.5 })
	if hop.Facing != -1 || hop.X >= 0 {
		t.Fatalf("by the tray it went to %+v, want turned round and away from it", hop.At)
	}
}

// A stretch made shorter, as when the screen changes, brings it back in; one
// with no room keeps it on its spot.
func TestAShorterStretchBringsTheBirdBack(t *testing.T) {
	half := func() float64 { return 0.5 }
	if hop := NextHop(At{X: -500, Facing: -1}, stretch, Site, 1, half); hop.X < stretch.Min {
		t.Fatalf("stayed out at %d", hop.X)
	}
	if hop := NextHop(At{Facing: -1}, Bounds{}, Site, 1, half); hop.X != 0 {
		t.Fatalf("with no room it hopped to %d", hop.X)
	}
}

// Now and then a hop is two short ones in a row, quickly, as birds do on
// the ground.
func TestNowAndThenTwoShortHops(t *testing.T) {
	pairs := 0
	for _, s := range walk(2000, stretch, 1, seeded(6)) {
		if s.to.Pair != 1 {
			continue
		}
		pairs++
		if s.to.Rest != Site.PairBeat {
			t.Fatalf("the first of a pair rested %v, want %v", s.to.Rest, Site.PairBeat)
		}
		if d := math.Abs(float64(s.to.X - s.from.X)); d > Site.PairLongest+1 {
			t.Fatalf("the first of a pair was %g pixels", d)
		}
	}
	if pairs < 200 || pairs > 600 {
		t.Fatalf("%d pairs in 2000 hops, want about a quarter", pairs)
	}
	for _, s := range walk(2000, stretch, 1, seeded(7)) {
		if s.from.Pair == 1 {
			if s.to.Pair != 2 || s.to.Facing != s.from.Facing {
				t.Fatalf("after the first of a pair came %+v", s.to)
			}
		}
	}
}

// The stretch is 300 site pixels to the left of its spot, scaled, and stays
// on the work area.
func TestTheStretchStaysOnTheWorkArea(t *testing.T) {
	spot := image.Rect(1582, 922, 1692, 1032)
	if b := Stretch(spot, work, 1); b != (Bounds{Min: -300}) {
		t.Fatalf("stretch %v, want 300 pixels to the left", b)
	}
	if b := Stretch(spot, work, 2); b != (Bounds{Min: -600}) {
		t.Fatalf("stretch at scale 2 %v, want 600 pixels", b)
	}
	near := image.Rect(100, 922, 210, 1032)
	if b := Stretch(near, work, 1); b.Min != -100 {
		t.Fatalf("stretch %v by the left edge, want it to stop there", b)
	}
}

// Three passes of the pointer within four seconds shoo it.
func TestThreePassesShooTheBird(t *testing.T) {
	pass := Shooer(Site)
	t0 := time.Unix(0, 0)
	if pass(t0) || pass(t0.Add(800*time.Millisecond)) {
		t.Fatal("shooed before the third pass")
	}
	if !pass(t0.Add(1600 * time.Millisecond)) {
		t.Fatal("three passes did not shoo it")
	}
	if pass(t0.Add(1700*time.Millisecond)) || pass(t0.Add(1800*time.Millisecond)) {
		t.Fatal("once shooed, it took fewer than three new passes")
	}
	if !pass(t0.Add(1900 * time.Millisecond)) {
		t.Fatal("three new passes did not shoo it again")
	}
}

// Passes spread out do not.
func TestSpreadOutPassesDoNotShooTheBird(t *testing.T) {
	pass := Shooer(Site)
	t0 := time.Unix(0, 0)
	for i := range 3 {
		if pass(t0.Add(time.Duration(i) * Site.Within)) {
			t.Fatalf("pass %d, %v apart, shooed it", i+1, Site.Within)
		}
	}
}

// Shooed, it flies to the far end facing the way it flies, then lands
// facing back, toward the pointer.
func TestAShooedBirdFliesToTheFarEnd(t *testing.T) {
	to, flies := Flee(At{X: -20, Facing: -1}, stretch)
	if to.X != stretch.Min || flies != -1 || to.Facing != 1 {
		t.Fatalf("from near the tray: to %+v flying %d", to, flies)
	}
	to, flies = Flee(At{X: -280, Facing: -1}, stretch)
	if to.X != stretch.Max || flies != 1 || to.Facing != -1 {
		t.Fatalf("from the far end: to %+v flying %d", to, flies)
	}
}

// A hop's frames go from where it is to where it lands, up and down again,
// facing the way it hops, perched at both ends.
func TestAHopsFramesArcFromWhereItIsToWhereItLands(t *testing.T) {
	w := NewWalker(home, 1, 30, Site, seeded(8))
	for range 50 {
		from := w.At()
		moments, _, _ := w.Hop()
		to := w.At()
		first, last := moments[0], moments[len(moments)-1]
		if x(first) != from.X || x(last) != to.X {
			t.Fatalf("frames from %d to %d, want %d to %d", x(first), x(last), from.X, to.X)
		}
		if first.At.Y != home.Spot.Min.Y || last.At.Y != home.Spot.Min.Y {
			t.Fatal("a hop left the taskbar")
		}
		if first.Motion.DY != 0 || last.Motion.DY != 0 {
			t.Fatal("a hop does not start and end on the perch")
		}
		up := false
		for _, m := range moments {
			up = up || m.Motion.DY < 0
			if m.Motion.Left != (to.Facing < 0) || m.Pose != warm.Perched || m.Motion.Alpha != 1 {
				t.Fatalf("a hop's frame %+v facing %d", m, to.Facing)
			}
		}
		if to.X != from.X && !up {
			t.Fatal("a hop never left the ground")
		}
	}
}

// Now and then, after a hop, it chirps; never between the hops of a pair.
func TestTheBirdChirpsNowAndThen(t *testing.T) {
	w := NewWalker(home, 1, 30, Site, seeded(9))
	chirps := 0
	for range 1000 {
		_, _, chirp := w.Hop()
		if chirp > 0 {
			chirps++
			if w.At().Pair == 1 {
				t.Fatal("chirped between the hops of a pair")
			}
		}
	}
	if chirps < 150 || chirps > 400 {
		t.Fatalf("%d chirps in 1000 hops, want a few in ten", chirps)
	}
}

// Shooed, it flaps off to the far end, then turns round with a little hop,
// not a flip.
func TestAShooFliesThenTurnsWithALittleHop(t *testing.T) {
	w := NewWalker(home, 1, 30, Site, seeded(10))
	t0 := time.Unix(0, 0)
	if m, _ := w.Pass(t0, nil); m != nil {
		t.Fatal("flew off on the first pass")
	}
	if m, _ := w.Pass(t0.Add(time.Second), nil); m != nil {
		t.Fatal("flew off on the second pass")
	}
	moments, rest := w.Pass(t0.Add(2*time.Second), nil)
	if moments == nil || rest != Site.AfterFlee {
		t.Fatalf("three passes: %d frames, then %v, want a flight and its rest", len(moments), rest)
	}
	if w.At().X != stretch.Min || w.At().Facing != 1 {
		t.Fatalf("shooed to %+v, want the far end facing back", w.At())
	}
	if moments[0].Pose != warm.Flying || !moments[0].Motion.Left {
		t.Fatalf("it set off as %+v, want flying the way it flies", moments[0])
	}
	last := moments[len(moments)-1]
	if x(last) != stretch.Min || last.Pose != warm.Perched || last.Motion.Left || last.Motion.DY != 0 {
		t.Fatalf("it ended as %+v, want perched at the far end, facing back", last)
	}
	turned := false
	for i, m := range moments[1:] {
		if m.Motion.Left != moments[i].Motion.Left {
			if turned || x(m) != stretch.Min {
				t.Fatalf("turned at %d, or more than once", x(m))
			}
			turned = true
		}
	}
}

// A left-facing bird is the mirror image of the right-facing one, and its
// box of clicks turns with it.
func TestALeftFacingBirdIsTheMirrorImage(t *testing.T) {
	const width = 110
	right, err := Frame(warm.Perched, Perched, width)
	if err != nil {
		t.Fatal(err)
	}
	left, err := Frame(warm.Perched, Motion{Alpha: 1, Left: true}, width)
	if err != nil {
		t.Fatal(err)
	}
	r, l := opaque(right), opaque(left)
	axis := 60 * float64(width) / canvasW
	if math.Abs(float64(r.Min.X+l.Max.X)-2*axis) > 2 || math.Abs(float64(r.Max.X+l.Min.X)-2*axis) > 2 || r.Min.Y != l.Min.Y {
		t.Fatalf("left-facing bird at %v, want the mirror of %v about x %.1f", l, r, axis)
	}
	if !l.In(Hit(width, true).Inset(-2)) || !r.In(Hit(width, false).Inset(-2)) {
		t.Fatalf("birds %v and %v outside their boxes %v and %v", l, r, Hit(width, true), Hit(width, false))
	}
}

// opaque is the box of img's pixels that show.
func opaque(img *image.RGBA) image.Rectangle {
	var box image.Rectangle
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			if img.RGBAAt(x, y).A > 128 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return box
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// Two windows on a 1920x1032 work area, the top one first: an editor, and a
// browser behind it whose top edge shows only to the editor's right.
var (
	editor  = image.Rect(200, 300, 1000, 900)
	browser = image.Rect(600, 400, 1700, 950)
)

// A window's top edge is a ledge where it shows and the bird has room: the
// uncovered part of it, with the bird's head on the work area.
func TestLedgesAreTheTopsOfWindowsThatShow(t *testing.T) {
	const w = 110
	h := Height(w)
	ledges := Ledges([]image.Rectangle{editor, browser}, work, w, h)
	if len(ledges) != 2 {
		t.Fatalf("%d ledges, want the editor's and the browser's", len(ledges))
	}
	for i, win := range []image.Rectangle{editor, browser} {
		l := ledges[i]
		if l.Spot.Max.Y != win.Min.Y || l.Spot.Dx() != w || l.Spot.Dy() != h {
			t.Fatalf("ledge %d stands at %v, want on %v's top edge", i, l.Spot, win)
		}
		if l.Bounds.Max != 0 || l.Bounds.Min > -w {
			t.Fatalf("ledge %d walks %v, want at least a bird's length to the left", i, l.Bounds)
		}
		for _, end := range []image.Rectangle{l.Spot.Add(image.Pt(l.Bounds.Min, 0)), l.Spot} {
			for _, left := range []bool{false, true} {
				if hit := Hit(w, left).Add(end.Min); hit.Min.X < win.Min.X || hit.Max.X > win.Max.X {
					t.Fatalf("ledge %d, %v walking %v, lets the bird hang off %v", i, l.Spot, l.Bounds, win)
				}
			}
		}
	}
	under := ledges[1].Spot.Add(image.Pt(ledges[1].Bounds.Min, 0))
	if Hit(w, true).Add(under.Min).Min.X < editor.Max.X {
		t.Fatalf("the browser's ledge %v reaches under the editor", ledges[1])
	}
}

// No ledge where the bird would not show or fit: a window covered along its
// top, one too narrow, a maximized one with no room above it, one off the
// screen.
func TestNoLedgeWhereTheBirdDoesNotFit(t *testing.T) {
	const w = 110
	target := image.Rect(300, 400, 900, 800)
	for name, wins := range map[string][]image.Rectangle{
		"covered":   {image.Rect(250, 330, 950, 700), target},
		"narrow":    {image.Rect(1300, 500, 1400, 800)},
		"maximized": {work},
		"offscreen": {image.Rect(-900, 500, -100, 800)},
	} {
		last := wins[len(wins)-1]
		for _, l := range Ledges(wins, work, w, Height(w)) {
			if l.Spot.Max.Y == last.Min.Y {
				t.Fatalf("%s: a ledge %+v on %v", name, l, last)
			}
		}
	}
}

// Shooed from the taskbar, now and then it flies to a window's top, and
// otherwise to the far end of its stretch; with no window it stays on the
// taskbar.
func TestAShooedBirdSometimesFliesToAWindow(t *testing.T) {
	ledges := Ledges([]image.Rectangle{editor}, work, 110, Height(110))
	onWindow, onTaskbar := 0, 0
	for i := range 100 {
		w := NewWalker(home, 1, 30, Site, seeded(uint32(i)*2654435761))
		moments := shoo(w, ledges)
		last := moments[len(moments)-1]
		switch last.At.Y {
		case ledges[0].Spot.Min.Y:
			onWindow++
			if last.At.X != ledges[0].Spot.Min.X {
				t.Fatalf("landed on the window at x %d, want its right end %d", last.At.X, ledges[0].Spot.Min.X)
			}
		case home.Spot.Min.Y:
			onTaskbar++
		default:
			t.Fatalf("landed at %v, neither on the taskbar nor on the window", last.At)
		}
	}
	if onWindow < 25 || onTaskbar < 25 {
		t.Fatalf("%d to the window, %d along the taskbar: want both, about half each", onWindow, onTaskbar)
	}
	for i := range 50 {
		w := NewWalker(home, 1, 30, Site, seeded(uint32(i)*2246822519))
		if last := shoo(w, nil); last[len(last)-1].At.Y != home.Spot.Min.Y {
			t.Fatal("left the taskbar with no window to go to")
		}
	}
}

// The flight to a window is one smooth path, flapping, from where it was to
// the window's top, and on the window it walks along its top.
func TestOnAWindowTheBirdWalksAlongItsTop(t *testing.T) {
	ledge := Ledges([]image.Rectangle{editor}, work, 110, Height(110))[0]
	w := toWindow(t, ledge)
	for range 300 {
		moments, _, _ := w.Hop()
		for _, m := range moments {
			if m.At.Y != ledge.Spot.Min.Y {
				t.Fatalf("hopped off the window's top, to %v", m.At)
			}
			if dx := m.At.X - ledge.Spot.Min.X; dx < ledge.Bounds.Min || dx > ledge.Bounds.Max {
				t.Fatalf("walked to %d along a ledge that goes %v", dx, ledge.Bounds)
			}
		}
	}
}

// Shooed off a window, or when its window moves, shuts or is covered, it
// flies back to its spot by the tray.
func TestOffAWindowTheBirdFliesBackToTheTaskbar(t *testing.T) {
	ledge := Ledges([]image.Rectangle{editor}, work, 110, Height(110))[0]
	w := toWindow(t, ledge)
	if away, on := w.Away(); !on || away != ledge {
		t.Fatalf("away on %v %v, want on the editor's ledge", away, on)
	}
	back := shoo(w, []Ledge{ledge})
	if last := back[len(back)-1]; last.At != home.Spot.Min || last.Pose != warm.Perched {
		t.Fatalf("shooed off the window to %+v, want perched on its spot by the tray", last)
	}
	if _, on := w.Away(); on {
		t.Fatal("still away once back on the taskbar")
	}

	w = toWindow(t, ledge)
	back = w.Home()
	smooth(t, back)
	if last := back[len(back)-1]; last.At != home.Spot.Min {
		t.Fatalf("flew back to %v, want its spot by the tray", last.At)
	}
	if moments := w.Home(); moments != nil {
		t.Fatal("flew home when already there")
	}
}

// shoo passes the pointer over the bird three times, quickly.
func shoo(w *Walker, ledges []Ledge) []Moment {
	t0 := time.Unix(0, 0)
	w.Pass(t0, ledges)
	w.Pass(t0.Add(time.Second), ledges)
	moments, _ := w.Pass(t0.Add(2*time.Second), ledges)
	return moments
}

// toWindow is a walker that was shooed onto ledge.
func toWindow(t *testing.T, ledge Ledge) *Walker {
	t.Helper()
	for i := range 100 {
		w := NewWalker(home, 1, 30, Site, seeded(uint32(i)*3266489917))
		moments := shoo(w, []Ledge{ledge})
		if last := moments[len(moments)-1]; last.At.Y == ledge.Spot.Min.Y {
			smooth(t, moments)
			flapped := 0
			for _, m := range moments {
				if m.Pose == warm.Flying {
					flapped++
				}
			}
			if flapped < len(moments)/2 {
				t.Fatalf("flapped %d of %d frames of the flight to a window", flapped, len(moments))
			}
			return w
		}
	}
	t.Fatal("never flew to the window")
	return nil
}

// smooth fails unless moments are one path, no frame jumping far from the
// one before.
func smooth(t *testing.T, moments []Moment) {
	t.Helper()
	for i := 1; i < len(moments); i++ {
		d := moments[i].At.Sub(moments[i-1].At)
		if d.X*d.X+d.Y*d.Y > 120*120 {
			t.Fatalf("frame %d jumped %v", i, d)
		}
	}
}

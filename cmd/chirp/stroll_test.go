// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"image"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/perch"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// quick is the site's walk, sped up so a test sees many hops.
var quick = func() perch.Rules {
	r := perch.Site
	r.ShortestRest, r.LongestRest, r.PairBeat = time.Millisecond, 2*time.Millisecond, time.Millisecond
	r.HopTime, r.PairTime, r.FleeTime, r.Flapping = 4*time.Millisecond, 4*time.Millisecond, 8*time.Millisecond, 6*time.Millisecond
	r.Chirping, r.AfterFlee = 2*time.Millisecond, 2*time.Millisecond
	r.FlightSpeed, r.LongestFlight = 100000, 20*time.Millisecond
	return r
}()

// stage records what a stroll shows.
type stage struct {
	mu    sync.Mutex
	shown []perch.Moment
}

func (s *stage) show(m perch.Moment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shown = append(s.shown, m)
}

func (s *stage) moments() []perch.Moment {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]perch.Moment(nil), s.shown...)
}

func (s *stage) count() int { return len(s.moments()) }

var home = perch.Ledge{Spot: image.Rect(1582, 922, 1692, 1032), Bounds: perch.Bounds{Min: -300}}

func strollFor(t *testing.T, reduced bool) (*stroll, *stage) {
	t.Helper()
	return strollOver(t, reduced, func() []perch.Ledge { return nil })
}

func strollOver(t *testing.T, reduced bool, ledges func() []perch.Ledge) (*stroll, *stage) {
	t.Helper()
	w := perch.NewWalker(home, 1, 1000, quick, rand.New(rand.NewPCG(1, 2)).Float64)
	st := &stage{}
	s := startStroll(w, time.Millisecond, time.Millisecond, reduced, ledges, st.show)
	t.Cleanup(s.stop)
	return s, st
}

func walked(st *stage) bool {
	for _, m := range st.moments() {
		if m.At != home.Spot.Min {
			return true
		}
	}
	return false
}

// Perched, the bird hops along its stretch by itself.
func TestTheBirdWalksByItself(t *testing.T) {
	_, st := strollFor(t, false)
	time.Sleep(100 * time.Millisecond)
	if !walked(st) {
		t.Fatal("the bird never left its spot")
	}
}

// While its balloon is open it holds still, standing, and walks on once
// the balloon folds.
func TestTheBirdHoldsStillWhileItsBalloonIsOpen(t *testing.T) {
	s, st := strollFor(t, false)
	time.Sleep(30 * time.Millisecond)
	s.hold(true)
	time.Sleep(30 * time.Millisecond) // a hop under way lands
	held := st.count()
	time.Sleep(60 * time.Millisecond)
	shown := st.moments()
	for _, m := range shown[held:] {
		if m.At != shown[held-1].At || m.Motion != shown[held-1].Motion {
			t.Fatalf("moved from %+v to %+v while held", shown[held-1], m)
		}
	}
	if last := shown[held-1]; last.Motion.DY != 0 {
		t.Fatalf("held in mid-air: %+v", last)
	}
	s.hold(false)
	time.Sleep(60 * time.Millisecond)
	if st.count() == len(shown) {
		t.Fatal("did not walk on once let go")
	}
}

// A chirp under way when the balloon opens still ends: the bird does not
// stay chirping under its balloon.
func TestAChirpEndsWhileTheBirdIsHeld(t *testing.T) {
	r := quick
	r.ChirpChance, r.Chirping = 1, 20*time.Millisecond
	w := perch.NewWalker(home, 1, 1000, r, rand.New(rand.NewPCG(3, 4)).Float64)
	st := &stage{}
	s := startStroll(w, time.Millisecond, time.Millisecond, false, func() []perch.Ledge { return nil }, st.show)
	t.Cleanup(s.stop)
	for range 200 {
		if shown := st.moments(); len(shown) > 0 && shown[len(shown)-1].Pose == warm.Chirping {
			break
		}
		time.Sleep(time.Millisecond)
	}
	s.hold(true)
	time.Sleep(60 * time.Millisecond)
	shown := st.moments()
	if last := shown[len(shown)-1]; last.Pose != warm.Perched {
		t.Fatalf("held as %+v, want perched once the chirp ended", last)
	}
}

// Three quick passes of the pointer shoo it; while held, they do not.
func TestPassesShooTheBirdUnlessHeld(t *testing.T) {
	flew := func(st *stage) bool {
		for _, m := range st.moments() {
			if m.Pose == warm.Flying {
				return true
			}
		}
		return false
	}
	pass := func(s *stroll) {
		for range 3 {
			s.pass(time.Now())
			time.Sleep(20 * time.Millisecond) // past any hop under way
		}
	}

	s, st := strollFor(t, false)
	s.hold(true)
	time.Sleep(20 * time.Millisecond)
	pass(s)
	time.Sleep(30 * time.Millisecond)
	if flew(st) {
		t.Fatal("shooed while its balloon was open")
	}

	s, st = strollFor(t, false)
	pass(s)
	time.Sleep(30 * time.Millisecond)
	if !flew(st) {
		t.Fatal("three passes did not shoo it")
	}
}

// Its pose, chirping for an alert, shows where it stands.
func TestTheBirdsPoseShowsWhereItStands(t *testing.T) {
	s, st := strollFor(t, false)
	time.Sleep(30 * time.Millisecond)
	s.hold(true)
	time.Sleep(30 * time.Millisecond)
	s.pose(warm.Chirping)
	time.Sleep(10 * time.Millisecond)
	shown := st.moments()
	last, before := shown[len(shown)-1], shown[len(shown)-2]
	if last.Pose != warm.Chirping || last.At != before.At || last.Motion.Left != before.Motion.Left {
		t.Fatalf("chirping as %+v after %+v, want where it stood", last, before)
	}
}

// Where the system asks for less motion, the bird only perches.
func TestWithReducedMotionTheBirdOnlyPerches(t *testing.T) {
	s, st := strollFor(t, true)
	time.Sleep(60 * time.Millisecond)
	for range 3 {
		s.pass(time.Now())
	}
	time.Sleep(30 * time.Millisecond)
	if walked(st) {
		t.Fatal("walked with reduced motion")
	}
	for _, m := range st.moments() {
		if m.Pose == warm.Flying {
			t.Fatal("was shooed with reduced motion")
		}
	}
}

// Once it flies off, the stroll shows nothing more.
func TestAStoppedStrollShowsNothing(t *testing.T) {
	s, st := strollFor(t, false)
	time.Sleep(20 * time.Millisecond)
	s.stop()
	time.Sleep(10 * time.Millisecond)
	n := st.count()
	time.Sleep(40 * time.Millisecond)
	if st.count() != n {
		t.Fatal("kept walking after it stopped")
	}
	s.stop()
}

// Shooed onto a window's top, the bird flies back to its spot by the tray
// once that window moves, shuts or is covered.
func TestWhenItsWindowGoesTheBirdFliesBack(t *testing.T) {
	window := perch.Ledge{Spot: image.Rect(890, 190, 1000, 300), Bounds: perch.Bounds{Min: -500}}
	var gone atomic.Bool
	s, st := strollOver(t, false, func() []perch.Ledge {
		if gone.Load() {
			return nil
		}
		return []perch.Ledge{window}
	})
	onWindow := func() bool {
		shown := st.moments()
		return len(shown) > 0 && shown[len(shown)-1].At.Y == window.Spot.Min.Y
	}
	for range 20 {
		if onWindow() {
			break
		}
		for range 3 {
			s.pass(time.Now())
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !onWindow() {
		t.Fatal("never flew to the window")
	}
	gone.Store(true)
	time.Sleep(500 * time.Millisecond)
	shown := st.moments()
	if last := shown[len(shown)-1]; last.At != home.Spot.Min && last.At.Y != home.Spot.Min.Y {
		t.Fatalf("stayed at %v once its window went, want back on the taskbar", last.At)
	}
}

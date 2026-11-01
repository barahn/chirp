// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"slices"
	"sync"
	"time"

	"github.com/barahn/chirp/cmd/chirp/internal/perch"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// stroll walks the perched bird along its stretch on a goroutine of its own
// (barahn/chirp#187), the same on every OS: show draws each moment, where
// the OS's window is; ledges are the windows' tops it may fly to, read when
// it is shooed and, while it is on one, to see that it is still there. It
// stands still while held, under its open balloon, and only perches where
// the system asks for less motion.
type stroll struct {
	walker  *perch.Walker
	frame   time.Duration
	reduced bool
	ledges  func() []perch.Ledge
	show    func(perch.Moment)

	holds  chan bool
	passes chan time.Time
	poses  chan warm.Pose
	done   chan struct{}
	once   sync.Once

	// The stroll's goroutine only.
	held bool
	rest warm.Pose
}

// startStroll sets the bird walking after settle, a frame every frame.
func startStroll(w *perch.Walker, frame, settle time.Duration, reduced bool, ledges func() []perch.Ledge, show func(perch.Moment)) *stroll {
	s := &stroll{
		walker: w, frame: frame, reduced: reduced, ledges: ledges, show: show,
		holds: make(chan bool), passes: make(chan time.Time, 8), poses: make(chan warm.Pose), done: make(chan struct{}),
		rest: warm.Perched,
	}
	go s.run(settle)
	return s
}

func (s *stroll) hold(held bool) {
	select {
	case s.holds <- held:
	case <-s.done:
	}
}

// pass is the pointer passing over the bird.
func (s *stroll) pass(at time.Time) {
	select {
	case s.passes <- at:
	default:
	}
}

// pose is how the bird stands between hops.
func (s *stroll) pose(p warm.Pose) {
	select {
	case s.poses <- p:
	case <-s.done:
	}
}

func (s *stroll) stop() { s.once.Do(func() { close(s.done) }) }

func (s *stroll) run(settle time.Duration) {
	next := time.NewTimer(settle)
	defer next.Stop()
	look := time.NewTicker(lookEvery)
	defer look.Stop()
	var chirped <-chan time.Time
	for {
		select {
		case <-s.done:
			return
		case s.held = <-s.holds:
			if !s.held {
				next.Reset(settle)
			}
		case s.rest = <-s.poses:
			s.show(s.walker.Still(s.rest))
		case at := <-s.passes:
			if s.held || s.reduced {
				continue
			}
			if moments, rest := s.walker.Pass(at, s.ledges()); moments != nil {
				if !s.play(moments) {
					return
				}
				next.Reset(rest)
			}
		case <-next.C:
			if s.held || s.reduced {
				continue
			}
			moments, rest, chirp := s.walker.Hop()
			if !s.play(moments) {
				return
			}
			if chirp > 0 && !s.held && s.rest == warm.Perched {
				s.show(s.walker.Still(warm.Chirping))
				chirped = time.After(chirp)
			}
			next.Reset(rest)
		case <-look.C:
			ledge, away := s.walker.Away()
			if !away || s.held || slices.Contains(s.ledges(), ledge) {
				continue
			}
			if !s.play(s.walker.Home()) {
				return
			}
			next.Reset(settle)
		case <-chirped:
			chirped = nil
			s.show(s.walker.Still(s.rest))
		}
	}
}

// play shows moments a frame apart, and the pose asked for meanwhile at
// the end; it reports false when the stroll stopped first.
func (s *stroll) play(moments []perch.Moment) bool {
	tick := time.NewTicker(s.frame)
	defer tick.Stop()
	for i, m := range moments {
		s.show(m)
		for waiting := i < len(moments)-1; waiting; {
			select {
			case <-s.done:
				return false
			case s.held = <-s.holds:
			case s.rest = <-s.poses:
			case <-tick.C:
				waiting = false
			}
		}
	}
	if s.rest != warm.Perched {
		s.show(s.walker.Still(s.rest))
	}
	return true
}

// settleAfterLanding is the bird's rest on landing, before its first hop, as
// on the site.
const settleAfterLanding = 1200 * time.Millisecond

// lookEvery is how often the bird on a window's top looks that the window
// is still there.
const lookEvery = 300 * time.Millisecond

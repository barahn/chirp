// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"slices"
	"time"
)

// OfflineAfter is how many failed probes in a row make a server off-line:
// about 90 s at the 30 s interval, the rule the relay's own Podman health
// check uses (docs/relay.md).
const OfflineAfter = 3

// Sample is one probe of one server from one vantage point.
type Sample struct {
	Vantage string
	At      time.Time
	OK      bool
	Latency time.Duration
	Err     string
}

// Uptime is a count of minutes. A minute nobody probed is in neither count:
// it is a gap in the record, not a minute up or down.
type Uptime struct {
	Up       int `json:"up_minutes"`
	Observed int `json:"observed_minutes"`
}

// Add returns the sum of two counts.
func (u Uptime) Add(v Uptime) Uptime {
	return Uptime{Up: u.Up + v.Up, Observed: u.Observed + v.Observed}
}

// Percent is the share of observed minutes that were up, or nil when no
// minute was observed. It is cut, never rounded, at three decimals, so it
// reads 100 only when every observed minute was up.
func (u Uptime) Percent() *float64 {
	if u.Observed == 0 {
		return nil
	}
	p := float64(u.Up*100000/u.Observed) / 1000
	return &p
}

// Tally counts the minutes samples cover. A vantage point succeeded in a
// minute when more than half its probes in that minute succeeded, and the
// minute was up when more than half the vantage points that probed in it
// succeeded. A tie is down: "most" means more than half, at both levels.
func Tally(samples []Sample) Uptime {
	type count struct{ ok, failed int }
	minutes := make(map[int64]map[string]*count)
	for _, s := range samples {
		m := s.At.Unix() / 60
		byVantage := minutes[m]
		if byVantage == nil {
			byVantage = make(map[string]*count)
			minutes[m] = byVantage
		}
		c := byVantage[s.Vantage]
		if c == nil {
			c = &count{}
			byVantage[s.Vantage] = c
		}
		if s.OK {
			c.ok++
		} else {
			c.failed++
		}
	}
	var u Uptime
	for _, byVantage := range minutes {
		up := 0
		for _, c := range byVantage {
			if c.ok > c.failed {
				up++
			}
		}
		u.Observed++
		if up*2 > len(byVantage) {
			u.Up++
		}
	}
	return u
}

// MedianLatency is the median latency of the successful samples, or zero
// when none succeeded.
func MedianLatency(samples []Sample) time.Duration {
	var ls []time.Duration
	for _, s := range samples {
		if s.OK {
			ls = append(ls, s.Latency)
		}
	}
	if len(ls) == 0 {
		return 0
	}
	slices.Sort(ls)
	if len(ls)%2 == 1 {
		return ls[len(ls)/2]
	}
	return (ls[len(ls)/2-1] + ls[len(ls)/2]) / 2
}

// State is what a server is doing now.
type State string

const (
	// StateUnknown: no vantage point has probed it recently, so nothing
	// current can be said.
	StateUnknown State = "unknown"
	// StateUp: its latest probe succeeded.
	StateUp State = "up"
	// StateFailing: its latest probe failed, but fewer than OfflineAfter in
	// a row have.
	StateFailing State = "failing"
	// StateOffline: its last OfflineAfter probes failed.
	StateOffline State = "offline"
)

// Verdict says what a server is doing at now from its recent samples. Each
// vantage point that probed it within staleAfter judges it by its own probes;
// the server is up or off-line when most of them say so, and failing when
// they disagree.
func Verdict(samples []Sample, now time.Time, staleAfter time.Duration) State {
	byVantage := make(map[string][]Sample)
	for _, s := range samples {
		byVantage[s.Vantage] = append(byVantage[s.Vantage], s)
	}
	fresh, up, offline := 0, 0, 0
	for _, ss := range byVantage {
		slices.SortFunc(ss, func(a, b Sample) int { return a.At.Compare(b.At) })
		last := ss[len(ss)-1]
		if now.Sub(last.At) > staleAfter {
			continue
		}
		fresh++
		switch {
		case last.OK:
			up++
		case len(ss) >= OfflineAfter && !slices.ContainsFunc(ss[len(ss)-OfflineAfter:], func(s Sample) bool { return s.OK }):
			offline++
		}
	}
	switch {
	case fresh == 0:
		return StateUnknown
	case up*2 > fresh:
		return StateUp
	case offline*2 > fresh:
		return StateOffline
	}
	return StateFailing
}

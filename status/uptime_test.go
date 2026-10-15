// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func at(v string, d time.Duration, ok bool) Sample {
	return Sample{Vantage: v, At: t0.Add(d), OK: ok}
}

func TestTallyCountsOnlyMinutesThatWereProbed(t *testing.T) {
	// Minutes 0 and 1 probed, 2 to 9 nobody probed, 10 probed and down.
	samples := []Sample{
		at("a", 0, true), at("a", 30*time.Second, true),
		at("a", time.Minute, true), at("a", 90*time.Second, true),
		at("a", 10*time.Minute, false), at("a", 10*time.Minute+30*time.Second, false),
	}
	got := Tally(samples)
	if want := (Uptime{Up: 2, Observed: 3}); got != want {
		t.Fatalf("Tally = %+v, want %+v: the eight unprobed minutes are neither up nor down", got, want)
	}
}

func TestTallyTieIsDown(t *testing.T) {
	// One vantage point, one success and one failure in the minute.
	if got := Tally([]Sample{at("a", 0, true), at("a", 30*time.Second, false)}); got.Up != 0 {
		t.Errorf("a vantage point split one to one counted the minute up: %+v", got)
	}
	// Two vantage points, one says up and one down.
	if got := Tally([]Sample{at("a", 0, true), at("b", 0, false)}); got.Up != 0 || got.Observed != 1 {
		t.Errorf("two vantage points split one to one: %+v, want 0 up of 1", got)
	}
}

func TestTallyMostVantagePointsDecide(t *testing.T) {
	samples := []Sample{at("a", 0, true), at("b", 0, true), at("c", 0, false)}
	if got := Tally(samples); got != (Uptime{Up: 1, Observed: 1}) {
		t.Errorf("two of three up: %+v, want the minute up", got)
	}
	samples = []Sample{at("a", 0, false), at("b", 0, false), at("c", 0, true)}
	if got := Tally(samples); got != (Uptime{Up: 0, Observed: 1}) {
		t.Errorf("one of three up: %+v, want the minute down", got)
	}
}

func TestPercentIsCutNotRounded(t *testing.T) {
	if p := (Uptime{}).Percent(); p != nil {
		t.Errorf("no minutes observed: percent %v, want none", *p)
	}
	// 99.99993 % would round to 100.
	p := (Uptime{Up: 1439999, Observed: 1440000}).Percent()
	if p == nil || *p != 99.999 {
		t.Errorf("percent = %v, want 99.999", p)
	}
	if p := (Uptime{Up: 7, Observed: 7}).Percent(); p == nil || *p != 100 {
		t.Errorf("all up: percent %v, want 100", p)
	}
}

func TestVerdict(t *testing.T) {
	now := t0.Add(2 * time.Minute)
	stale := 90 * time.Second
	cases := []struct {
		name    string
		samples []Sample
		want    State
	}{
		{"never probed", nil, StateUnknown},
		{"newest probe too old", []Sample{at("a", 0, true)}, StateUnknown},
		{"latest ok after failures", []Sample{at("a", 30*time.Second, false), at("a", 60*time.Second, false), at("a", 90*time.Second, true)}, StateUp},
		{"two failures", []Sample{at("a", 30*time.Second, true), at("a", 60*time.Second, false), at("a", 90*time.Second, false)}, StateFailing},
		{"three failures", []Sample{at("a", 30*time.Second, false), at("a", 60*time.Second, false), at("a", 90*time.Second, false)}, StateOffline},
		{"only one probe so far, failed", []Sample{at("a", 90*time.Second, false)}, StateFailing},
		{"vantage points disagree", []Sample{
			at("a", 90*time.Second, true),
			at("b", 30*time.Second, false), at("b", 60*time.Second, false), at("b", 90*time.Second, false),
		}, StateFailing},
		{"stale vantage point is not asked", []Sample{
			at("a", 0, true),
			at("b", 30*time.Second, false), at("b", 60*time.Second, false), at("b", 90*time.Second, false),
		}, StateOffline},
	}
	for _, c := range cases {
		if got := Verdict(c.samples, now, stale); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestMedianLatencyIgnoresFailures(t *testing.T) {
	ss := []Sample{
		{OK: true, Latency: 30 * time.Millisecond},
		{OK: true, Latency: 10 * time.Millisecond},
		{OK: false, Latency: 5 * time.Second},
		{OK: true, Latency: 20 * time.Millisecond},
	}
	if got := MedianLatency(ss); got != 20*time.Millisecond {
		t.Errorf("median = %v, want 20ms", got)
	}
	if got := MedianLatency(ss[2:3]); got != 0 {
		t.Errorf("median of failures only = %v, want 0", got)
	}
}

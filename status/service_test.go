// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fixture is a service watching one node whose health the test switches,
// on a clock the test moves.
type fixture struct {
	svc     *Service
	healthy atomic.Bool
	now     time.Time
}

func newFixture(t *testing.T, now time.Time) *fixture {
	t.Helper()
	f := &fixture{now: now}
	f.healthy.Store(true)
	node := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !f.healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(node.Close)
	client := newClient()
	client.Transport.(*http.Transport).TLSClientConfig = node.Client().Transport.(*http.Transport).TLSClientConfig

	store, err := OpenStore(filepath.Join(t.TempDir(), "status.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := Server{ID: "n1", Kind: KindNode, URL: node.URL, Operator: "Test", GitHub: "test", Added: "2026-10-01",
		added: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	f.svc = &Service{
		Servers: []Server{srv}, Store: store, Prober: &Prober{Client: client},
		Vantage: "test", AllowOrigin: "https://usechirp.app",
		Now: func() time.Time { return f.now },
	}
	return f
}

func (f *fixture) round(t *testing.T) ServerStatus {
	t.Helper()
	f.svc.Round(context.Background())
	snap, err := f.svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(Interval)
	return snap.Servers[0]
}

func TestOfflineAfterThreeFailedRoundsAndSinceWhen(t *testing.T) {
	f := newFixture(t, t0)
	if st := f.round(t); st.State != StateUp || st.LatencyMS == nil {
		t.Fatalf("healthy: %+v", st)
	}
	lastOK := t0
	f.healthy.Store(false)
	for i, want := range []State{StateFailing, StateFailing, StateOffline, StateOffline} {
		st := f.round(t)
		if st.State != want {
			t.Fatalf("failed round %d: %s, want %s", i+1, st.State, want)
		}
		if st.LastOK == nil || !st.LastOK.Equal(lastOK) {
			t.Fatalf("failed round %d: last ok %v, want %v", i+1, st.LastOK, lastOK)
		}
		if st.LastError != ErrStatus || st.LatencyMS != nil {
			t.Fatalf("failed round %d: error %q, latency %v", i+1, st.LastError, st.LatencyMS)
		}
	}
	f.healthy.Store(true)
	if st := f.round(t); st.State != StateUp {
		t.Fatalf("back: %s", st.State)
	}

	// The service itself stops probing: after three missed rounds it no
	// longer claims to know.
	f.now = f.now.Add(staleAfter)
	snap, err := f.svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st := snap.Servers[0]; st.State != StateUnknown {
		t.Fatalf("nobody probing: %s, want unknown", st.State)
	}
}

// fill records one probe every Interval from from up to to, as up or down.
func fill(t *testing.T, st *Store, from, to time.Time, ok bool) {
	t.Helper()
	var ss []Sample
	for at := from; at.Before(to); at = at.Add(Interval) {
		ss = append(ss, Sample{Vantage: "test", At: at, OK: ok, Latency: 20 * time.Millisecond})
	}
	if err := st.Add("n1", ss...); err != nil {
		t.Fatal(err)
	}
}

func TestDaysAreTotalledAndGapsStayGaps(t *testing.T) {
	today := DayOf(t0)
	f := newFixture(t, t0)
	st := f.svc.Store

	fill(t, st, (today - 100).Start(), (today - 99).Start(), true) // past retention
	fill(t, st, (today - 3).Start(), (today - 2).Start(), true)    // a full day up
	// today-2: the service was down all day. No probes, no data.
	half := (today - 1).Start().Add(12 * time.Hour)
	fill(t, st, (today - 1).Start(), half, true) // up for twelve hours
	fill(t, st, half, today.Start(), false)      // then down for twelve
	fill(t, st, today.Start(), t0, true)         // up since midnight

	snap := f.round(t)

	days := getHistory(t, f.svc, "n1", "?days=4")
	want := []struct {
		up, observed int
		percent      *float64
	}{
		{1440, 1440, ptr(100)},
		{0, 0, nil},
		{720, 1440, ptr(50)},
		{720, 720, ptr(100)}, // the round probes at 12:00, a minute not over yet
	}
	if len(days) != len(want) {
		t.Fatalf("%d days, want %d", len(days), len(want))
	}
	for i, w := range want {
		d := days[i]
		if d.Up != w.up || d.Observed != w.observed || !samePercent(d.Percent, w.percent) {
			t.Errorf("%s: %d/%d %v, want %d/%d %v", d.Day, d.Up, d.Observed, d.Percent, w.up, w.observed, w.percent)
		}
		if (d.P50MS == nil) != (w.up == 0) {
			t.Errorf("%s: p50 %v with %d minutes up", d.Day, d.P50MS, w.up)
		}
	}
	if days[0].Day != (today-3).String() || days[3].Day != today.String() {
		t.Errorf("days run %s to %s", days[0].Day, days[3].Day)
	}

	// The 30-day figure is the sum of the days the history shows.
	u30 := snap.Uptime["30d"]
	if u30.Up != 1440+720+720 || u30.Observed != 1440+1440+720 {
		t.Errorf("30d = %+v", u30.Uptime)
	}
	// The last 24 hours: 12 h down, then 12 h up.
	if u24 := snap.Uptime["24h"]; u24.Up != 720 || u24.Observed != 1440 {
		t.Errorf("24h = %+v", u24.Uptime)
	}

	// The day past retention was totalled before its probes went.
	old, err := st.Samples("n1", (today - 100).Start(), (today - 99).Start())
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 0 {
		t.Errorf("%d probes past retention kept", len(old))
	}
	totals, err := st.Totals("n1", today-100, today-100)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 || totals[0].Uptime.Observed != 1440 {
		t.Errorf("the pruned day's total: %+v", totals)
	}
	if u90 := snap.Uptime["90d"]; u90.Observed != u30.Observed {
		t.Errorf("90d counted the day a hundred days ago: %+v", u90.Uptime)
	}
}

func TestAPI(t *testing.T) {
	f := newFixture(t, t0)
	f.round(t)
	h := f.svc.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/servers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/servers: %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://usechirp.app" {
		t.Errorf("CORS origin %q", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "Date" {
		t.Errorf("exposed headers %q, want Date", got)
	}
	var snap Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Servers) != 1 || snap.Servers[0].State != StateUp || !snap.Servers[0].New || snap.Interval != 30 {
		t.Errorf("snapshot: %+v", snap)
	}
	for _, w := range []string{"24h", "30d", "90d"} {
		if _, ok := snap.Servers[0].Uptime[w]; !ok {
			t.Errorf("no %s figure", w)
		}
	}

	for path, code := range map[string]int{
		"/v1/servers/n1/history":         http.StatusOK,
		"/v1/servers/n1/history?days=0":  http.StatusBadRequest,
		"/v1/servers/n1/history?days=91": http.StatusBadRequest,
		"/v1/servers/n1/history?days=x":  http.StatusBadRequest,
		"/v1/servers/n2/history":         http.StatusNotFound,
		"/healthz":                       http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != code {
			t.Errorf("%s: %d, want %d", path, rec.Code, code)
		}
	}
	if days := getHistory(t, f.svc, "n1", ""); len(days) != MaxHistoryDays {
		t.Errorf("default history: %d days, want %d", len(days), MaxHistoryDays)
	}
}

func TestHistoryHoldsItsOwnBound(t *testing.T) {
	f := newFixture(t, t0)
	for _, n := range []int{0, -1, MaxHistoryDays + 1, 1 << 40} {
		if _, err := f.svc.history("n1", DayOf(t0), n, t0); err == nil {
			t.Errorf("history of %d days: no error", n)
		}
	}
	if days, err := f.svc.history("n1", DayOf(t0), MaxHistoryDays, t0); err != nil || len(days) != MaxHistoryDays {
		t.Errorf("history of %d days: %d, %v", MaxHistoryDays, len(days), err)
	}
}

func TestServersBeforeTheFirstRefresh(t *testing.T) {
	f := newFixture(t, t0)
	rec := httptest.NewRecorder()
	f.svc.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/servers", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("before any data: %d, want 503", rec.Code)
	}
}

func getHistory(t *testing.T, svc *Service, id, query string) []DayStatus {
	t.Helper()
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/servers/"+id+"/history"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Days []DayStatus `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Days
}

func ptr(f float64) *float64 { return &f }

func samePercent(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

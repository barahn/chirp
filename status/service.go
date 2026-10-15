// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Interval is how often every server is probed.
const Interval = 30 * time.Second

// staleAfter is how old a vantage point's newest probe may be and still say
// what a server is doing now: three missed rounds.
const staleAfter = 3 * Interval

// MaxHistoryDays is the longest history /v1/servers/{id}/history returns.
const MaxHistoryDays = 90

// Service probes the network's servers, keeps what it saw, and serves it.
type Service struct {
	Servers []Server
	Store   *Store
	Prober  *Prober
	// Vantage names the place this service probes from.
	Vantage string
	// AllowOrigin, if set, is the one origin whose pages may read the API:
	// the status page's.
	AllowOrigin string
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	mu       sync.RWMutex
	snapshot []byte
	rolled   Day
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Run probes every server each Interval until ctx ends.
func (s *Service) Run(ctx context.Context) {
	if err := s.Refresh(); err != nil {
		log.Printf("chirp-status: %v", err)
	}
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		s.Round(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Round probes every server once, all at the same time, records what it
// saw, closes the days that have ended, and refreshes what the API serves.
// It logs what goes wrong rather than stopping: a round that cannot be
// recorded is a gap in the record, and the next round is still worth making.
func (s *Service) Round(ctx context.Context) {
	at := s.now()
	samples := make([]Sample, len(s.Servers))
	var wg sync.WaitGroup
	for i, srv := range s.Servers {
		wg.Go(func() {
			sample := s.Prober.Probe(ctx, srv.URL)
			sample.Vantage, sample.At = s.Vantage, at
			samples[i] = sample
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		// Cut short by shutting down: these failures are ours, not theirs.
		return
	}
	for i, srv := range s.Servers {
		if err := s.Store.Add(srv.ID, samples[i]); err != nil {
			log.Printf("chirp-status: recording %s: %v", srv.ID, err)
		}
	}
	if today := DayOf(at); today != s.rolled {
		if s.closeDays(today, at) {
			s.rolled = today
		}
	}
	if err := s.Refresh(); err != nil {
		log.Printf("chirp-status: %v", err)
	}
}

// closeDays totals the days before today and drops probes past Retention. It
// reports whether every server's days were totalled; pruning waits for that.
func (s *Service) closeDays(today Day, at time.Time) bool {
	for _, srv := range s.Servers {
		if err := s.Store.Rollup(srv.ID, today); err != nil {
			log.Printf("chirp-status: totalling %s: %v", srv.ID, err)
			return false
		}
	}
	if err := s.Store.Prune(at.Add(-Retention)); err != nil {
		log.Printf("chirp-status: pruning: %v", err)
	}
	return true
}

// ServerStatus is one server as /v1/servers reports it.
type ServerStatus struct {
	ID          string            `json:"id"`
	Kind        Kind              `json:"kind"`
	URL         string            `json:"url"`
	Operator    string            `json:"operator"`
	Region      string            `json:"region,omitempty"`
	BehindCDN   bool              `json:"behind_cdn"`
	Added       string            `json:"added"`
	New         bool              `json:"new"`
	Listing     Listing           `json:"listing"`
	State       State             `json:"state"`
	LastChecked *time.Time        `json:"last_checked"`
	LastError   string            `json:"last_error,omitempty"`
	LatencyMS   *int64            `json:"latency_ms"`
	LastOK      *time.Time        `json:"last_ok"`
	Uptime      map[string]Window `json:"uptime"`
}

// Window is an uptime figure over one period.
type Window struct {
	Uptime
	Percent *float64 `json:"percent"`
}

func window(u Uptime) Window { return Window{Uptime: u, Percent: u.Percent()} }

// Snapshot is what /v1/servers returns.
type Snapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Vantage     string         `json:"vantage"`
	Interval    int            `json:"interval_seconds"`
	Servers     []ServerStatus `json:"servers"`
}

// Refresh recomputes what /v1/servers serves from the store.
func (s *Service) Refresh() error {
	snap, err := s.Status()
	if err != nil {
		return err
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.snapshot = b
	s.mu.Unlock()
	return nil
}

// Status computes every server's status now.
//
// The 24-hour figure counts the last 1,440 whole minutes. The 30- and 90-day
// figures count whole UTC days, today's minutes so far included, so they
// move at midnight: they are the sum of the days the history shows.
func (s *Service) Status() (Snapshot, error) {
	now := s.now()
	minute := now.Truncate(time.Minute)
	today := DayOf(now)
	snap := Snapshot{GeneratedAt: now, Vantage: s.Vantage, Interval: int(Interval / time.Second), Servers: []ServerStatus{}}
	for _, srv := range s.Servers {
		st := ServerStatus{
			ID: srv.ID, Kind: srv.Kind, URL: srv.URL, Operator: srv.Operator, Region: srv.Region,
			BehindCDN: srv.BehindCDN, Added: srv.Added, New: srv.IsNew(now), Listing: srv.Listing(),
			Uptime: map[string]Window{},
		}
		recent, err := s.Store.Samples(srv.ID, now.Add(-staleAfter-OfflineAfter*Interval), now.Add(time.Millisecond))
		if err != nil {
			return Snapshot{}, err
		}
		st.State = Verdict(recent, now, staleAfter)
		if last, ok, err := s.Store.Latest(srv.ID); err != nil {
			return Snapshot{}, err
		} else if ok {
			st.LastChecked = &last.At
			st.LastError = last.Err
			if last.OK {
				ms := last.Latency.Milliseconds()
				st.LatencyMS = &ms
			}
		}
		if t, ok, err := s.Store.LastOK(srv.ID); err != nil {
			return Snapshot{}, err
		} else if ok {
			st.LastOK = &t
		}

		day, err := s.Store.Samples(srv.ID, minute.Add(-24*time.Hour), minute)
		if err != nil {
			return Snapshot{}, err
		}
		st.Uptime["24h"] = window(Tally(day))
		days, err := s.history(srv.ID, today, MaxHistoryDays, minute)
		if err != nil {
			return Snapshot{}, err
		}
		var u30, u90 Uptime
		for i, d := range days {
			u90 = u90.Add(d.Uptime)
			if i >= len(days)-30 {
				u30 = u30.Add(d.Uptime)
			}
		}
		st.Uptime["30d"] = window(u30)
		st.Uptime["90d"] = window(u90)
		snap.Servers = append(snap.Servers, st)
	}
	return snap, nil
}

// history returns n days up to today, oldest first, one entry for every day
// whether or not it has data. Today is counted from the probes, up to until.
func (s *Service) history(id string, today Day, n int, until time.Time) ([]DayTotal, error) {
	from := today - Day(n-1)
	totals, err := s.Store.Totals(id, from, today-1)
	if err != nil {
		return nil, err
	}
	byDay := make(map[Day]DayTotal, len(totals))
	for _, t := range totals {
		byDay[t.Day] = t
	}
	samples, err := s.Store.Samples(id, today.Start(), until)
	if err != nil {
		return nil, err
	}
	byDay[today] = DayTotal{Day: today, Uptime: Tally(samples), P50: MedianLatency(samples)}
	out := make([]DayTotal, 0, n)
	for d := from; d <= today; d++ {
		t, ok := byDay[d]
		if !ok {
			t = DayTotal{Day: d}
		}
		out = append(out, t)
	}
	return out, nil
}

// Handler serves the API:
//
//	GET /healthz                       200 while serving
//	GET /v1/servers                    every server's status now
//	GET /v1/servers/{id}/history?days= one server's days, oldest first
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /v1/servers", s.serveServers)
	mux.HandleFunc("GET /v1/servers/{id}/history", s.serveHistory)
	return s.headers(mux)
}

func (s *Service) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-cache")
		if s.AllowOrigin != "" {
			h.Set("Access-Control-Allow-Origin", s.AllowOrigin)
			h.Set("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) serveServers(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	b := s.snapshot
	s.mu.RUnlock()
	if b == nil {
		http.Error(w, "starting", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// DayStatus is one day as the history reports it.
type DayStatus struct {
	Day string `json:"day"`
	Window
	P50MS *int64 `json:"p50_ms"`
}

func (s *Service) serveHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	known := false
	for _, srv := range s.Servers {
		known = known || srv.ID == id
	}
	if !known {
		http.Error(w, "no such server", http.StatusNotFound)
		return
	}
	n := MaxHistoryDays
	if v := r.URL.Query().Get("days"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 || n > MaxHistoryDays {
			http.Error(w, "days must be 1 to "+strconv.Itoa(MaxHistoryDays), http.StatusBadRequest)
			return
		}
	}
	now := s.now()
	days, err := s.history(id, DayOf(now), n, now.Truncate(time.Minute))
	if err != nil {
		log.Printf("chirp-status: history of %s: %v", id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := struct {
		ID   string      `json:"id"`
		Days []DayStatus `json:"days"`
	}{ID: id, Days: make([]DayStatus, 0, len(days))}
	for _, d := range days {
		ds := DayStatus{Day: d.Day.String(), Window: window(d.Uptime)}
		if d.Uptime.Up > 0 {
			ms := d.P50.Milliseconds()
			ds.P50MS = &ms
		}
		out.Days = append(out.Days, ds)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

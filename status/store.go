// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package status

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite" // the pure-Go driver: chirp-status builds without cgo
)

// Retention is how long single probes are kept. A day's totals are kept for
// good once the day is over.
const Retention = 90 * 24 * time.Hour

const schema = `
CREATE TABLE IF NOT EXISTS probes (
	server_id  TEXT    NOT NULL,
	vantage    TEXT    NOT NULL,
	ts         INTEGER NOT NULL, -- Unix milliseconds
	ok         INTEGER NOT NULL,
	latency_ms INTEGER NOT NULL,
	err_class  TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS probes_server_ts ON probes (server_id, ts);
CREATE TABLE IF NOT EXISTS daily (
	server_id        TEXT    NOT NULL,
	day              INTEGER NOT NULL, -- days since 1970-01-01, UTC
	up_minutes       INTEGER NOT NULL,
	observed_minutes INTEGER NOT NULL,
	p50_ms           INTEGER NOT NULL,
	PRIMARY KEY (server_id, day)
);
CREATE TABLE IF NOT EXISTS last_ok (
	server_id TEXT    PRIMARY KEY,
	ts        INTEGER NOT NULL
);
`

const msPerDay = 24 * 60 * 60 * 1000

// Day is a UTC day, counted from 1970-01-01.
type Day int64

// DayOf is the UTC day t falls in.
func DayOf(t time.Time) Day { return Day(t.UnixMilli() / msPerDay) }

// Start is the day's first instant.
func (d Day) Start() time.Time { return time.UnixMilli(int64(d) * msPerDay).UTC() }

// String is the day as YYYY-MM-DD.
func (d Day) String() string { return d.Start().Format(time.DateOnly) }

// DayTotal is one server's record for one day.
type DayTotal struct {
	Day    Day
	Uptime Uptime
	P50    time.Duration
}

// Store keeps the probes in SQLite.
type Store struct {
	db *sql.DB
}

// OpenStore opens, or creates, the database at path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// One writer, and few enough readers that one connection serves them:
	// it rules out SQLITE_BUSY between the service's own goroutines.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (st *Store) Close() error { return st.db.Close() }

// Add records samples of the server with id.
func (st *Store) Add(id string, samples ...Sample) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, s := range samples {
		if _, err := tx.Exec(`INSERT INTO probes (server_id, vantage, ts, ok, latency_ms, err_class) VALUES (?, ?, ?, ?, ?, ?)`,
			id, s.Vantage, s.At.UnixMilli(), s.OK, s.Latency.Milliseconds(), s.Err); err != nil {
			return err
		}
		if s.OK {
			if _, err := tx.Exec(`INSERT INTO last_ok (server_id, ts) VALUES (?, ?)
				ON CONFLICT (server_id) DO UPDATE SET ts = max(ts, excluded.ts)`, id, s.At.UnixMilli()); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// Samples returns the server's samples from from up to, not including, to,
// oldest first.
func (st *Store) Samples(id string, from, to time.Time) ([]Sample, error) {
	rows, err := st.db.Query(`SELECT vantage, ts, ok, latency_ms, err_class FROM probes
		WHERE server_id = ? AND ts >= ? AND ts < ? ORDER BY ts`, id, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Sample
	for rows.Next() {
		var (
			s       Sample
			ts, lat int64
		)
		if err := rows.Scan(&s.Vantage, &ts, &s.OK, &lat, &s.Err); err != nil {
			return nil, err
		}
		s.At = time.UnixMilli(ts).UTC()
		s.Latency = time.Duration(lat) * time.Millisecond
		out = append(out, s)
	}
	return out, rows.Err()
}

// Latest returns the server's most recent sample, if it has one.
func (st *Store) Latest(id string) (Sample, bool, error) {
	var (
		s       Sample
		ts, lat int64
	)
	err := st.db.QueryRow(`SELECT vantage, ts, ok, latency_ms, err_class FROM probes
		WHERE server_id = ? ORDER BY ts DESC LIMIT 1`, id).Scan(&s.Vantage, &ts, &s.OK, &lat, &s.Err)
	if errors.Is(err, sql.ErrNoRows) {
		return Sample{}, false, nil
	}
	if err != nil {
		return Sample{}, false, err
	}
	s.At = time.UnixMilli(ts).UTC()
	s.Latency = time.Duration(lat) * time.Millisecond
	return s, true, nil
}

// LastOK is when the server last answered a probe. It outlives Retention.
func (st *Store) LastOK(id string) (time.Time, bool, error) {
	var ts int64
	err := st.db.QueryRow(`SELECT ts FROM last_ok WHERE server_id = ?`, id).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return time.UnixMilli(ts).UTC(), true, nil
}

// Rollup totals every day before today that has probes of the server and no
// total yet. A day is totalled once, after it ends, and a day with no probes
// gets no total: in the record it stays a day without data.
func (st *Store) Rollup(id string, today Day) error {
	rows, err := st.db.Query(`SELECT DISTINCT ts / ? FROM probes WHERE server_id = ? AND ts < ?
		AND ts / ? NOT IN (SELECT day FROM daily WHERE server_id = ?)`,
		msPerDay, id, today.Start().UnixMilli(), msPerDay, id)
	if err != nil {
		return err
	}
	var days []Day
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d); err != nil {
			_ = rows.Close()
			return err
		}
		days = append(days, d)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, d := range days {
		samples, err := st.Samples(id, d.Start(), (d + 1).Start())
		if err != nil {
			return err
		}
		u := Tally(samples)
		if _, err := st.db.Exec(`INSERT INTO daily (server_id, day, up_minutes, observed_minutes, p50_ms) VALUES (?, ?, ?, ?, ?)`,
			id, d, u.Up, u.Observed, MedianLatency(samples).Milliseconds()); err != nil {
			return err
		}
	}
	return nil
}

// Totals returns the server's day totals from from to to, both included.
// Days without a total are left out.
func (st *Store) Totals(id string, from, to Day) ([]DayTotal, error) {
	rows, err := st.db.Query(`SELECT day, up_minutes, observed_minutes, p50_ms FROM daily
		WHERE server_id = ? AND day >= ? AND day <= ? ORDER BY day`, id, from, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []DayTotal
	for rows.Next() {
		var (
			t   DayTotal
			p50 int64
		)
		if err := rows.Scan(&t.Day, &t.Uptime.Up, &t.Uptime.Observed, &p50); err != nil {
			return nil, err
		}
		t.P50 = time.Duration(p50) * time.Millisecond
		out = append(out, t)
	}
	return out, rows.Err()
}

// Prune deletes the probes older than before. Run Rollup first, or the days
// it deletes are never totalled.
func (st *Store) Prune(before time.Time) error {
	_, err := st.db.Exec(`DELETE FROM probes WHERE ts < ?`, before.UnixMilli())
	return err
}

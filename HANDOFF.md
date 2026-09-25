# Chirp — Handoff Prompt for Continuation Agents

Paste this whole file as the opening prompt of a new agent session on this
repository. It is written to be self-contained: it assumes the agent has the
repo checked out and has read nothing else.

---

## 1. Your role

You are continuing development of **Chirp**, an open-source (AGPL-3.0-only)
on-demand remote support tool owned by **Fabrintek Engenharia Digital Ltda**.
Work on the `develop` branch. Commit with `git commit -s` (a DCO
`Signed-off-by` trailer is mandatory and enforced in CI) and push with
`git push -u origin develop`. Do not open pull requests unless asked.

Read `README.md`, `CONTRIBUTING.md` and `LICENSING.md` before your first
commit. They are not boilerplate — they encode hard constraints (below).

## 2. What Chirp is

A technician generates a 6-digit PIN. The person being helped opens a web page
("the Nest"), enters the PIN, grants explicit consent on their own machine, and
the remote session begins. **No installation, no standing agent, no account.**

This is deliberately the opposite of a persistent-agent RMM product. Every
design decision should be checked against that sentence.

## 3. Where the code stands today

`git log --oneline` (5 commits, all on `develop`):

```
f9c25c6 docs(legal): freeze contributions and state the dual-licensing reservation
b9ddd82 ci: add the pipeline
772d332 feat: stand up chirp as an independent module
197a143 feat(license): tri-license the repo along the three-repository split
80f5531 feat(chirp): implement ad-hoc session engine, consent flow, and Nest /join page (#12)
```

Layout:

```
go.mod                  module github.com/barahn/chirp, go 1.26.5, only dep: google/uuid
session/code.go         PIN generation (crypto/rand, 100000-999999), Format/Normalize
session/consent.go      ConsentRequest / ConsentDecision types, DefaultPermissions
session/session.go      Session, SessionState machine, SessionManager (in-memory)
session/session_test.go tests for the above (currently passing)
scripts/gen-third-party-notices.sh
.github/workflows/ci.yml
LICENSE, LICENSING.md, CONTRIBUTING.md, THIRD_PARTY_NOTICES.md, README.md
```

**The session layer is all that exists.** There is no binary, no server, no
transport, no UI. `go build ./...` builds a single library package.

### The state machine (already implemented)

`pending → connected → consent_requested → consent_granted → active →
terminated`, with `consent_denied` and `expired` as terminal branches.
`SessionManager` holds sessions in a `map` keyed by UUID plus a `codeIndex`
keyed by the 6-digit PIN, reaps expired sessions on a 10-second ticker, and
exposes `Subscribe(code)` returning a channel of snapshots (intended to back
SSE/WebSocket push to the Nest page) and `WaitForConsent(ctx, code)`.

## 4. Known defects — fix these before building on top

### 4.1 `reapExpired` deadlocks the entire SessionManager (critical, confirmed)

`session/session.go`, `reapExpired()` takes `sess.mu.Lock()` and then calls
`sess.Clone()`, which takes `sess.mu.RLock()`. `sync.RWMutex` is **not
reentrant**, so the reaper blocks forever — while also holding
`sm.mu.Lock()`. Every subsequent call on the manager blocks. The first PIN that
is allowed to expire freezes the process.

The existing tests do not catch this because none of them lets a session expire
and then lets the reaper run. Reproduced with:

```go
func TestReaperDeadlock(t *testing.T) {
    sm := NewSessionManager(1 * time.Second)
    defer sm.Close()
    if _, err := sm.CreateSession("t", "T", 1*time.Second); err != nil { t.Fatal(err) }
    time.Sleep(1200 * time.Millisecond)
    done := make(chan struct{})
    go func() { sm.reapExpired(); close(done) }()
    select {
    case <-done:
    case <-time.After(3 * time.Second):
        t.Fatal("reapExpired deadlocked")   // <-- fails today
    }
}
```

Fix by introducing an unexported `cloneLocked()` that assumes the lock is held,
and calling that from any path that already holds `sess.mu`. Add the test above
(and a similar one for `ListActiveSessions`, which does `RLock` then `Clone` —
recursive read-locking, legal only while no writer is queued, so it is a
latent deadlock too).

### 4.2 Other issues worth folding into the same pass

- `Session` has a `sync.RWMutex` field carrying a `json:"-"` tag, which does
  nothing on an unexported field; `go vet` is quiet but the struct is copied by
  value nowhere — confirm and simplify.
- `CI runs only on `main`** (`on: push: branches: [main]`, `pull_request:
  branches: [main]`), but all work is on `develop`. Nothing has ever run in CI.
  Add `develop` to both triggers early, then fix whatever the pipeline reports
  (lint via golangci-lint v2.13.2, gosec, govulncheck, cross-compile for
  windows/amd64 and darwin/amd64, `go test -race`).
- `CreateSession` retries PIN generation 10 times against collisions but does
  not distinguish "collision" from "entropy failure"; the error message is
  generic.
- There is no rate limiting or attempt cap on PIN redemption. 900,000 PINs with
  unlimited guesses and a 10-minute TTL is brute-forceable. `CONTRIBUTING.md`
  itself uses `"expire a PIN after three failed redemptions"` as its example
  commit message — that is the intended behaviour and it is not implemented.

## 5. The roadmap — what is missing, in dependency order

From `README.md`'s own "not here yet" list:

1. **A signalling server.** Chirp needs its own, deliberately simplified: no
   multi-tenancy, no RBAC. The existing Barahn one is proprietary and is *not*
   being relicensed — do not port it, do not copy from it. Design it fresh
   around `SessionManager`: PIN redemption, SDP/ICE relay, and a push channel
   for session state (`Subscribe` already exists for this).
2. **The Nest** — the `/join` page where the end user types the PIN, sees who is
   asking and which permissions are requested (`DefaultPermissions`), and
   allows or denies. Commit `80f5531` mentions a Nest `/join` page but no such
   code is in the tree; assume it must be written.
3. **A `cmd/chirp` binary.** There is no entry point at all yet.
4. **A TURN relay.** Until then, self-hosting means bring-your-own STUN/TURN
   (coturn). Document, do not vendor.

**The remote-session mechanism is not yours to write.** Screen capture, VP8
encoding, input injection, clipboard, file transfer, WebRTC and the reverse
tunnel live in [remotekit](https://github.com/barahn/remotekit) under
Apache-2.0. Chirp consumes it as a dependency. If you find yourself
implementing capture or encoding inside Chirp, stop — it belongs upstream.

Tracking issue / separation plan: the
[barahn/barahn](https://github.com/barahn/barahn) repository.

## 6. Hard constraints — do not trade these away

**Product commitments** (from `README.md`, stated as constraints on what Chirp
may become):

- **No feature gates.** Screen sharing, input control, multi-monitor, file
  transfer and clipboard sync are the free core, permanently. Never add a
  licence check, tier check or entitlement lookup around any of them.
- **Explicit consent.** A session cannot start without the end user agreeing on
  their own machine, in a prompt Chirp controls. No "unattended mode", no
  pre-authorisation, no consent bypass flag — not even behind a config key.
- **Self-hostable with zero restrictions.** Anyone may run their own signalling
  and relay. Any hosted service is a convenience, never a requirement. No
  phone-home, no mandatory endpoint.

**Licensing and dependency constraints:**

- Every new Go file starts with:
  ```go
  // SPDX-License-Identifier: AGPL-3.0-only
  // Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
  ```
- **No copyleft dependencies.** Chirp is AGPL, so a GPL/AGPL dependency would
  be licence-*compatible* and still unacceptable: Fabrintek relicenses Chirp
  for its proprietary platform on the strength of owning all of Chirp, and it
  owns none of its dependencies. Permissive only (MIT/BSD/Apache-2.0). CI
  enforces this via `scripts/gen-third-party-notices.sh`; after any dependency
  change, re-run it and commit `THIRD_PARTY_NOTICES.md`.
- **Dependencies point inwards only.** remotekit depends on neither of the
  others; Chirp may depend on remotekit; the Barahn platform may depend on
  both. A change that needs the arrow reversed is a design problem, not a
  licensing exception.
- **Contributions are closed.** Do not merge external pull requests. The DCO
  sign-off CI check is *not* CLA acceptance and must never be described as
  such.

## 7. Working agreement

- Small, reviewable commits with Conventional Commit subjects
  (`feat(session):`, `fix(session):`, `ci:`, `docs(legal):` — follow the
  existing log).
- Every commit signed off: `git commit -s`.
- Tests alongside behaviour; run `go test -race ./...` and `go vet ./...`
  locally before pushing. Prefer a failing test first for each defect in
  section 4.
- Keep `README.md`'s "Status" section honest. It currently says Chirp is not a
  runnable product; when that stops being true, it is part of the same change
  to say so.

## 8. Suggested first milestone

1. Fix 4.1 with a regression test; audit every lock/`Clone` pairing in
   `session.go` while you are there.
2. Add `develop` to the CI triggers and get the pipeline green.
3. Implement PIN redemption attempt limiting (4.2, last bullet), with tests.
4. Only then start the signalling server (section 5.1), designed around the
   existing `SessionManager` API rather than a new abstraction.

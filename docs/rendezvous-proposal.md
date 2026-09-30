# Proposal: a rendezvous mechanism for remotekit

**Status:** proposal, written in Chirp for remotekit's maintainers. Nothing has
moved yet. This session could only read remotekit, so the proposal lives here
until it is taken upstream.

## Why this belongs in remotekit

Chirp's [product model](product-model.md) needs two strangers' machines to find
each other by a short code, or by a link that mints one, and then to exchange
WebRTC signalling. Almost none of that is Chirp-specific — it is mechanism, and
remotekit is where Fabrintek's remote-access mechanism lives
([ECOSYSTEM.md](../ECOSYSTEM.md)).

remotekit's own documents already point here:

- `docs/implementation-phases.md`, Phase 2, plans a pluggable `Signaler` with
  transports "including local rendezvous".
- `HANDOFF.md` says the relay path's value "is rendezvous … and it belongs to
  Chirp first, not Barahn".
- `bark` already carries `ConsentRequestPayload` / `ConsentResponsePayload`, and
  Phase 0 plans consent *enforcement* at the agent.

What remotekit has today does not cover it. `tunnel` redeems pairing codes, but
those are enrolment codes: single-use, stored by hash, binding a *standing*
agent to a server with persistent credentials. That is Barahn's model, and it is
the one Chirp is defined against. There is no mechanism for two *ephemeral*
peers to meet.

## The split

| Mechanism — remotekit | Product — Chirp |
|---|---|
| Allocate a short code for a peer: random, collision-free, expiring, single-use | That each Chirp shows its code at start, and when it is replaced |
| Issue a link token for a peer; mint a fresh single-use code per use | How long a link lives, and how it is revoked (Chirp decided: while the sharer's Chirp runs, revocable, replaced on restart) |
| Redeem a code, pairing the redeemer with its owner | That the redeemer controls the owner's machine |
| Throttle failed redemptions per origin | The limits (5 per 10 minutes today) |
| Relay opaque signalling between the paired peers; announce presence | Role names, and when a session counts as connected |
| Consent request/response on the wire (`bark`, already there) | The prompt, its wording, the self-asserted name, the default permissions |
| Consent enforcement at the agent (remotekit Phase 0) | — |

Rule of thumb, from `ECOSYSTEM.md`: if Barahn — or a product with entirely
different rules — would use it unchanged, it is mechanism.

## Sketch of the API

A package `rendezvous`, with no notion of a user, a role, a consent policy or a
default value. Values are parameters; policy is the caller's.

```go
// A peer that can be reached by a code or a link.
type PeerID string

// Allocate returns a code by which owner can be reached. It is random
// (crypto/rand), unique among live codes, valid for ttl, and redeemable once.
func (r *Registry) Allocate(owner PeerID, ttl time.Duration) (Code, error)

// IssueLink returns a token from which codes for owner can be minted.
// Lifetime and revocation are the caller's policy, passed in.
func (r *Registry) IssueLink(owner PeerID, p LinkPolicy) (Token, error)
func (r *Registry) RevokeLink(t Token) error

// MintFromLink creates a fresh single-use code for one connection. It is
// metered against origin exactly like a redemption.
func (r *Registry) MintFromLink(t Token, origin string) (Code, error)

// Redeem consumes code, metered per origin, and pairs the redeemer with the
// code's owner. Both ends then signal through the returned Match.
func (r *Registry) Redeem(code Code, origin string) (Match, error)

// Limits sets the per-origin budget for failed attempts.
func (r *Registry) SetLimits(maxFailures int, window time.Duration)
```

`Match` is where Phase 2's `Signaler` plugs in: the relay that carries the two
peers' signalling, over whatever transport. The mechanism answers *who meets
whom*; the `Signaler` answers *how they talk*.

## What Chirp already has that is this mechanism

Reuse, not rewrite. Chirp's code was written for its own session model, but
these parts carry no Chirp policy and are tested:

- **Codes** — `session/code.go`: `GenerateCode` (crypto/rand, 100000–999999),
  `FormatCode`, `NormalizeCode`.
- **The per-origin throttle** — `session/redemption.go`: the fixed-window
  limiter, including the rules that matter — malformed codes count, a
  throttled origin is refused even for a live code, success clears the origin,
  a missing origin shares a bucket rather than escaping, records are reaped.
- **Metered lookup** — `SessionManager.LookupCode` in `session/session.go`,
  and the lesson that produced it: every entry point that takes a code from the
  network must be metered, or it is an oracle that sweeps the keyspace.
- **Code index and expiry** — the code index and reaper in
  `session/session.go`, including the fixed reaper deadlock and its regression
  tests (`session/reaper_internal_test.go`).
- **The two-peer relay** — `signalling/hub.go` and `signalling/relay.go`: an
  opaque relay that never parses what it forwards, refuses a second peer in a
  taken role, announces presence (`chirp.peer_joined` / `chirp.peer_left`), and
  sends outside its lock. Only its role names are Chirp's.

What stays in Chirp: the session state machine's product meaning
(`consent_requested` and so on), the consent rules, and the defaults.

## Observations for remotekit

- `bark.ChirpStatePayload` encodes Chirp's session state machine, and
  `ConsentRequestPayload.TechnicianName` presumes a technician role. Both sit in
  the shared envelope, against remotekit's rule that it carries no product
  opinions. Consent request and response are mechanism and fit there; Chirp's
  state machine arguably does not, and in Chirp's model the "technician" is just
  whoever typed a code, with a self-asserted name.
- `tunnel.HandlePairing` has no limit of its own on failed attempts — nothing
  in `tunnel` meters it, and its tests mount it directly. A consumer may wrap it
  in rate limiting (Barahn was not visible from here); one that mounts it as it
  stands exposes an unmetered oracle for pairing codes. The lesson above
  applies whatever the code's lifetime.

## Preconditions for moving the code

- **Licence.** The code above is AGPL-3.0-only in Chirp. remotekit is
  Apache-2.0. Moving it is a relicence, which Fabrintek can grant only as the
  sole copyright holder. Before moving a file, check its history —
  `git log --format='%an <%ae>' -- <file>` — for any author who is not
  Fabrintek's. Chirp has never accepted an external contribution, so none is
  expected, but it has to be checked, not assumed.
- **Direction.** After the move Chirp depends on remotekit, which the
  dependency rule allows. remotekit must not import anything from Chirp in the
  process — not a type, not a test helper.
- **Access.** Writing to remotekit needs access this session was not granted.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

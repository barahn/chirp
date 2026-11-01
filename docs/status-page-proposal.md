# Proposal: the public relay network and its status page

**Status:** proposal, not decided. It records the maintainer's answers of
2026-10-15 and the design that follows from them. Phase 1, the status service
measuring only, is built: `cmd/chirp-status` and the `status` package. So
is phase 2, the page.

The maintainer decided on 2026-10-31, in the review of barahn/chirp#170:

- A node keeps no record of lookups, in the relay or in its proxy
  ([What a node still sees](#what-a-node-still-sees)).
- An owner's Nostr key lives for one registration and is never reused; what a
  node can link through it is written down rather than hidden (same section).
- The key identifies a node and admits nothing
  ([Joining automatically](#joining-automatically)).
- No node says anything about its version, sealed or not
  ([Versions](#versions)).
- Phase 4's node key is the identity that relay selection in remotekit
  verifies, and that selection does not answer how a code finds its node
  ([The precondition](#the-precondition-finding-the-right-node)).
- Applications to join are suspended until the network's design is approved
  as a whole ([`joining-the-network.md`](joining-the-network.md)).

Whether a public node keeps the owner's socket is still open (open question
6), and the rest of the document is still a proposal.

The goal is a page on usechirp.app that shows which servers make up Chirp's
public network, how long each has been up, which are new, which are dedicated
nodes, which are off-line and since when, and which are out of date. Every
number on it must be measured, not claimed, and it must update within seconds
of a change.

The page is the visible end of something larger: a public, decentralised
network that anyone may join with their own relay. Most of this document is
about that network, because the page can only be as honest as the rules
behind it.

## Terms

- **The network** is the published list of servers a Chirp depends on when it
  uses the public network. It has three kinds of member:
  - **Nodes:** `chirp-relay` instances running in public mode. These are the
    *dedicated* servers: each one is a relay someone has joined to the network.
  - **Nostr relays:** third-party relays the network recommends for signalling
    (`HANDOFF.md` §4.7).
  - **TURN servers:** for sessions that cannot connect directly
    ([`stun-turn.md`](stun-turn.md)).
- **The list** is `network/servers.json` in this repository, plus a signed copy
  published from it (see [The signed list](#the-signed-list)).
- **The status service** is a new program, `cmd/chirp-status`. It probes every
  member, keeps the history, runs the admission and conformance checks, and
  serves the data the page shows.

## Two modes of the relay

`chirp-relay` gains `-mode private|public` (`CHIRP_RELAY_MODE`). The default
is `private`.

- **Private** is the relay as it is today: a self-hoster's server for their
  own people, announced nowhere, configured as they like.
- **Public** is a node of the network. It serves only what the network needs,
  with the network's limits, and publishes a node identity at
  `/.well-known/chirp-node`.

### The invariant: no mode can read the traffic

In neither mode may a relay read a session. This holds today only on the
Nostr path:

- **Nostr path (§4.7, §4.8).** The server sees six routing digits and returns
  a key. Signalling travels as NIP-59 gift wraps. The PAKE over the four secret
  digits binds the two ends, so a relay that hands out the wrong key fails the
  PAKE. Media and data run over WebRTC (DTLS/SRTP), keyed by fingerprints both
  ends have signed. A TURN server carries only DTLS-encrypted packets.
- **The trusted relay path (§4.5)** has no PAKE and no signed viewers. The
  relay forwards SDP, including the DTLS fingerprints, between the two ends. A
  hostile relay could swap the fingerprints and sit in the middle. The
  operator is trusted, which the invariant does not allow.

So:

1. **A public node does not offer the trusted path.** `GET /ws`,
   `POST /api/redeem`, `POST /api/links/{token}/sessions`,
   `GET /api/sessions/{id}` and `POST /api/peers/{peer}/consent` answer `404`.
   A public node does lookup only: `POST /api/peers`, `GET`/`DELETE
   /api/peers/{peer}`, `PUT /api/peers/{peer}/contact`, `POST /api/lookup` and
   `POST /api/links/{token}/lookup`. Whether an owner's `GET /ws` stays is
   open question 6.
2. **The private mode needs the PAKE too.** Either the PAKE also runs over the
   relay's own WebSocket, or the private mode moves to the Nostr path with a
   private Nostr relay. This is a separate change, and a precondition for
   saying the invariant holds in both modes.

### What a node still sees

End-to-end encryption hides content, not metadata. A node sees the addresses
of the Chirps that use it, the routing digits, and when and how often lookups
happen. A Nostr relay sees that a key received gift wraps, when, and how big
they were. Operators therefore pledge not to log or keep any of this beyond
the throttle's in-memory window (see [Conformance](#conformance)). The pledge
cannot be verified from outside, and the documentation for users must say so.

**No record of lookups.** Every attempt on a code or a link passes through
`POST /api/lookup` or `POST /api/links/{token}/lookup`, failures and successes
alike, so a node that kept them would hold who tried to reach whom, and when.
The relay keeps none: it logs only when it starts, stops and fails. That
absence is a commitment, not an accident of the code, and it covers the proxy
in front of the relay too. The shipped Caddyfile does not meet it yet: it
logs one line per request with the client's address and the time, which for a
lookup is exactly that record. A public node's Caddyfile must log no client
address, and phase 4 ships one.

**The owner's Nostr key.** A lookup hands out the owner's contact: a Nostr key
and its relays. Chirp makes that key for one registration, and a new one when
it registers again (a new code asked for, a restart, the relay's grace
exceeded) or when every Nostr relay drops it (`client/nostr.go`). A key is
never reused across registrations, and that is now a rule, not a detail of
the client.

One registration does span several codes: a lookup spends the code shown and
the node issues the next one under the same peer, and each use of a link
mints one. For the whole registration, the node can therefore watch that one
key on the public Nostr relays: when gift wraps reach it, and how big they
are, for every session the registration has. Turning the key over with each
code would not hide this from the node, because the node registers every key
it hands out, and already knows which codes belong to which peer. What the
key gives away ends with the registration: the next one has a key the node
cannot link to it except by timing and address.

## The precondition: finding the right node

Today a code is valid on the one server that issued it. With many independent
nodes, a controller who types six routing digits must know which node to ask.
The obvious answers have costs:

- **Ask every node.** Sends the routing digits to every operator, and
  multiplies the per-origin guessing budget by the number of nodes.
- **Reserve digits for the node.** Shrinks the routing space each node can
  allocate from, and makes random guesses likelier to hit.
- **Share registrations between nodes.** Gives up "all state in memory, one
  process", and lets one bad node read or forge every registration.
- **Carry the node in the code.** Lengthens the code, which decision 5 fixed at
  6 + 4 digits.

A link can name its node without cost, because nobody reads it aloud. Codes
cannot. This needs its own decision in [`product-model.md`](product-model.md)
before the public network can carry real sessions. The status page does not
depend on it and can be built first.

remotekit's
[`relay-path-selection.md`](https://github.com/barahn/remotekit/blob/main/docs/relay-path-selection.md)
chooses the TURN relay a session's media crosses, by round trips the peers
measure themselves. It does not answer this question: a measurement says which
node is near, not which node holds the registration behind six digits. The
two documents meet at the node key instead. Before a peer trusts a
measurement, it checks that the candidate signs a fresh nonce with the key the
list names for it, so that a CDN or a DNS answer in front of a listed node
cannot inherit its measured record. That check is phase 4's
`/.well-known/chirp-node`. So phase 4 supplies what selection needs, and phase
6 still waits on this decision.

## Membership

### Joining by pull request

Until pull requests from outside Fabrintek can be merged (`CONTRIBUTING.md`),
an operator applies with an issue form and a maintainer writes the entry:
[`joining-the-network.md`](joining-the-network.md) is that guide.

An operator adds an entry to `network/servers.json`:

```json
{
  "id": "node-sp-01",
  "kind": "node",
  "url": "https://relay.example.org",
  "operator": "Example Org",
  "github": "example-ops",
  "contact": "ops@example.org",
  "region": "BR",
  "behind_cdn": false
}
```

`github` is required: alerts reach the operator through it (see
[Alerts](#alerts)). `behind_cdn` declares a proxy such as Cloudflare, as
Invidious requires.

### Joining automatically

Automatic entry follows the same rules, with no human in the checks:

1. **The operator applies.** `chirp-relay -mode public` creates an ed25519
   node key on first start and publishes it at `/.well-known/chirp-node`. The
   operator submits the node's URL and their GitHub handle to the status
   service: `POST /v1/applications`.
2. **The node proves control of its domain.** The status service fetches
   `/.well-known/chirp-node?nonce=…` over HTTPS and checks that the node key
   signed the nonce.
3. **Probation.** For 30 days the node is probed like a member and shown on the
   page as *new (in probation)*. It is not in the signed list, so no Chirp
   uses it.
4. **Admission.** If probation ends with uptime ≥ 95 % and every conformance
   check passing, the status service opens a pull request adding the entry.
   The maintainer merges it. Making the merge automatic as well is possible,
   but the repository's ruleset currently requires review on `main`.

A failed probation closes the application with the reasons, and the operator
may apply again.

A valid key is not admission. Making a key costs nothing, so the key only
says which node is answering; probation and the measurements that follow are
what earn it a place in the list, and losing them loses the place, whatever
key it holds.

### Leaving

An operator leaves with a pull request removing the entry. A node that
breaks the rules is suspended and then removed (see [Alerts](#alerts)).

## Conformance

Every check is run from outside, against behaviour, on every member. A
member that fails a check is *non-conforming*. The page says so, and the
signed list leaves it out.

**Nodes:**

| Check | How |
|---|---|
| HTTPS with a valid certificate, TLS ≥ 1.2, a domain name rather than an IP | TLS handshake |
| Up | `GET /healthz` returns `200` within 5 s |
| Identity | `/.well-known/chirp-node` signs a fresh nonce with the listed key |
| Public mode only | the trusted-path endpoints answer `404` |
| Throttle on | the prober sends six malformed codes from its own address within the window and expects `429`. It spends only its own budget |
| Up to date | passes every check added by a security release, within its grace (see [Versions](#versions)) |
| No version on the wire | no `Server` or other header names the relay's version |

**Nostr relays:**

| Check | How |
|---|---|
| Up | WebSocket `REQ` with `limit: 0` reaches `EOSE` within 5 s |
| Compatible | the NIP-11 document does not set `auth_required`, which Chirp cannot meet until barahn/remotekit#71 |
| Carries gift wraps | kind 1059 is accepted, and the maximum message size fits one |

**TURN servers:** a STUN Binding Request is answered over UDP and TCP.

**Pledges** that cannot be checked from outside, and to which the operator
agrees by joining: no logging of URIs, routing digits or link tokens (the
Caddyfile's filtered logs, kept as shipped); no record of lookups, and, from
phase 4's Caddyfile on, no client addresses in any log, the proxy's included
(see [What a node still sees](#what-a-node-still-sees)); no analytics; no access
restrictions by country; any changes to the code published under the AGPL.

## Versions

A node must never tell the public its version, because that tells an attacker
which flaws it has. Invidious, by contrast, publishes its version in
`/api/v1/stats` and so does not have this problem to solve.

A node does not tell the status service its version either. An earlier draft
had the node seal a signed version to the status service's key. That is the
node's own word about itself, which nothing else on this page counts: a
modified node would attest whatever it liked, and the seal only kept others
from reading the claim. A version signed by the release pipeline does not
help here either. It proves that a release exists, not that the node runs it.

**What is measured instead:** where a release fixes something that can be
seen from outside, the conformance suite gains a check for it, and passing
that check is the evidence that cannot be faked.

**The rule:**

- After a release marked **security** that adds a check, a node has **7
  days** to pass it.
- Past that, it is **out of date**. The page says "out of date for N days",
  never which release it fails, and the signed list drops it.

**Limit:** a fix that changes nothing visible from outside cannot be enforced.
The page cannot say a node runs the latest release, only that it passes every
check the releases have added.

## Alerts

When a member becomes non-conforming, or a security release starts a node's
grace period, the status service:

1. opens a GitHub issue on this repository, labelled `network`, mentioning
   the operator's `github` handle, naming the failed check and the deadline;
2. emails the `contact` address with the same text.

If the grace period ends without a fix, it opens a pull request setting the
entry's `state` to `suspended`. When the member conforms again, it opens one
setting it back to `active` and closes the issue. After 90 days suspended, it
opens a pull request removing the entry.

An issue never states the version, only that the node is behind.

## The signed list

The status service publishes `network.json` at
`https://status.usechirp.app/v1/network.json`: the active, conforming members,
signed with an ed25519 key whose public half is compiled into Chirp.

- With `-network public`, a Chirp takes its lookup node, Nostr relays and TURN
  servers from that list, and checks the signature first.
- The list carries an expiry, so a Chirp that cannot reach the status service
  keeps using the last list it verified, until it expires.
- `-relay`, `-nostr` and `CHIRP_ICE` still override it. Self-hosting needs no
  list and no permission.

This ends "no default relay list" (`HANDOFF.md` §4.7). That is product
decision 5's "public network", and it should be recorded there when decided.

## The status service

```
 network/servers.json ──(pull request)──┐
                                        ▼
 ┌─────────────── cmd/chirp-status ───────────────────┐
 │ probers (one or more vantage points) ─► SQLite      │
 │ conformance suite · admission · alerts (GitHub API) │
 │ aggregator ─► API JSON + SSE · signed list · exports│
 └─────────────────────────┬──────────────────────────┘
                           ▼  status.usechirp.app
            site/status/index.html  (usechirp.app)
```

- It runs on a different host and network from every node, or it goes down
  with what it watches.
- **Probes** run every 30 s from each vantage point. A minute counts as up
  when most vantage points succeeded. It starts with one vantage point; the
  schema has a `vantage` column from the start.
- **Storage:** SQLite through a pure-Go driver, so the binary needs no CGO.
  `probes(server_id, vantage, ts, ok, latency_ms, err_class)` is kept for 90
  days; `daily(server_id, day, up_minutes, observed_minutes, p50_ms)` is kept
- **API:**
  - `GET /v1/servers`: the current state of every member and applicant;
  - `GET /v1/servers/{id}/history?days=90`: one member's history;
  - `GET /v1/stream`: server-sent events, one per change of state;
  - `GET /v1/network.json`: the signed list;
  - `GET /v1/exports/{day}.json`: that day's raw probes, signed.
- **Authenticity:** every response and export is signed by the status
  service. Anyone can run `chirp-status` against the same list and compare.
  Notes written by people (incidents, maintenance) are kept apart from the
  measurements, marked as written by a person, and never change a number.

## The page

`site/status/index.html` on usechirp.app, with the site's stylesheet and its
English and Portuguese texts (`data-i18n`).

- **Summary:** "7 of 8 operational · checked 12 s ago".
- **Filters:** All, Nodes (dedicated), Nostr relays, TURN, New, Off-line, Out
  of date.
- **One card per member:** name, operator, region, kind, badges, uptime over
  24 hours, 30 days and 90 days, a strip of 90 daily bars, median latency,
  conformance.

The page uses these definitions, each linked from the number it explains:

| Shown | Meaning |
|---|---|
| Uptime % | Up minutes over observed minutes, cut (never rounded) at three decimals. A vantage point succeeded in a minute when more than half its probes in it succeeded; the minute was up when more than half the vantage points that probed in it succeeded, so a tie is down. Minutes when the status service itself was down are *no data*: a gap in the strip, counted neither way. The 24-hour figure counts the last 1,440 whole minutes; the 30- and 90-day figures count whole UTC days, today so far included, and are the sum of the days the history shows. |
| Unknown | No vantage point has probed the server in the last three rounds, so the page does not say what it is doing now. |
| Off-line | Three failed probes in a row, about 90 s. The same rule as the relay's Podman health check. |
| Off-line for … | Time since the last successful probe, kept current by the page between updates. |
| New | In probation, or listed for under 30 days. Shown with "short history" instead of a 90-day figure. |
| Dedicated | A node: a `chirp-relay` in public mode that has joined the network. |
| Out of date for … | Past the grace period of [Versions](#versions). No version is shown. |
| Incompatible | A Nostr relay that requires NIP-42 authentication. |
| Suspended | Non-conforming past its grace period, and not in the signed list. |

**Real time:** changes arrive over SSE within about 30 s, the probe interval.
If the stream drops, the page polls `/v1/servers` every 60 s. If the status
service stops answering, the page says the monitor is unavailable and shows
the last data with its age. It never shows old data as current.

**Content Security Policy:** the site declares `default-src 'none'` with no
`connect-src`, so a page cannot fetch anything. The status page adds
`connect-src https://status.usechirp.app` to its own policy. The Cloudflare
response-header ruleset (`.cloudflare/rulesets/response-headers.json`) also
sets a policy, and must allow the same for `/status/`.

## Phases

1. **Status service, measuring only. Built**, and run as
   [`status-service.md`](status-service.md) says. `network/servers.json`,
   `/healthz` probes, SQLite, the JSON API (`/v1/servers`,
   `/v1/servers/{id}/history`). Uptime arithmetic tested with a fake clock,
   including *no data* gaps. The registry lists one node, `pve-dc-01` at
   `relay.usechirp.app`, Fabrintek's relay on its Proxmox host, and accepts
   only `node` entries until phase 3. Listing it means it is measured; Chirps
   use it on their own only after phases 4 and 6.
2. **The page. Built.** `site/status/` and `site/assets/status.js`: polling,
   both languages, the CSP change, a footer link on every page. Ages are
   measured by the service's clock, read from its `Date` header, not the
   visitor's. After merging, run `scripts/apply-cloudflare.sh` to load the
   new rule, or `scripts/check-live-site.sh` fails on `/status/`.
3. **Nostr and TURN members**, server-sent events, the conformance suite.
4. **The relay's public mode:** `-mode public`, `/.well-known/chirp-node`,
   the node key, the trusted path disabled, and a Caddyfile that logs no
   client address. No version attestation. The node key is also what
   remotekit's relay selection verifies before it trusts a measurement.
5. **Admission and alerts:** applications, probation, pull requests and
   issues opened by the status service, emails, suspension.
6. **The signed list and `-network public` in Chirp.** Needs the decision on
   [finding the right node](#the-precondition-finding-the-right-node), which
   remotekit's relay selection does not make.
7. **More vantage points**, signed daily exports.

Separately, before the invariant can be claimed for both modes: the PAKE on
the private mode's path.

## Open questions

1. How a code finds its node, among the options above.
   [`network-paths-proposal.md`](network-paths-proposal.md) proposes
   rendezvous hashing over the routing digits.
2. Whether admission ends with an automatic merge, which needs a change to
   the `main` ruleset, or keeps a maintainer's review.
3. The thresholds: 95 % uptime over a 30-day probation; 7 days of grace
   after a security release; 90 days suspended before removal.
4. Who holds the status service's signing key, and how it is rotated. A Chirp
   release pins the list key, so rotating it needs a release.
5. Whether operators must state the jurisdiction they run in, beyond a region.
6. Whether a public node keeps the owner's socket. With it, the node knows
   which owners are running, from when and for how long, through a live
   connection tied to each registration. Without it, an owner re-registers on
   an interval and the registration expires, so the node learns only that a
   write was recent. The socket is how the grace of product-model decision 1
   is measured today, so dropping it changes that measure as well.
   [`network-paths-proposal.md`](network-paths-proposal.md) proposes
   renewals in public mode.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

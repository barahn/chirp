# Proposal: signing the public relay list with Sigstore

**Status:** design accepted by the maintainer on 2026-11-16 (barahn/chirp#242). Nothing here is built, and nothing here
changes a workflow. It is a design for phase 6 of
[`status-page-proposal.md`](status-page-proposal.md) (*Phases*), offered as
an answer to that document's open question 4: who holds the status service's
signing key, and how it is rotated.

That proposal's section *The signed list* says the list is "signed with an
ed25519 key whose public half is compiled into Chirp". Nothing of it is built
yet: the only ed25519 key in the code is phase 4's node key (`node/node.go`).

This document proposes keeping that ed25519 signature as what Chirp checks,
and adding a second, Sigstore keyless signature over the same bytes:
`cosign sign-blob` in a GitHub Actions workflow, its certificate issued by
Fulcio against the workflow's OIDC identity, and the signature recorded in
Rekor. The first protects clients; the second makes every list ever signed
publicly auditable. Why both, and why not keyless alone, is in
[Which signature Chirp checks](#which-signature-chirp-checks). If the
maintainer accepts it, *The signed list* and open question 4 change to point
here.

It also records, deliberately, why the same mechanism must never be used for
Chirp's sessions (see [Where transparency would be a leak](#where-transparency-would-be-a-leak)).

## What "log" means here, and what it does not

In this document, *the log* is always Rekor: Sigstore's public, append-only
transparency log. What goes into it is one entry per **relay list** the
project publishes — the list's hash, the signature, and the identity of the
workflow that signed it. The project's own workflow writes it; nothing else
does.

Nothing about people goes into it. Chirp writes nothing to Rekor and records
nothing about its sessions, its users or its lookups. Relays write nothing to
it either, and keep no record of lookups, as *What a node still sees* and the
operators' pledges already require. Where this document says Chirp "requires
the log", it means only that Chirp, before trusting a list, checks a proof
that this list was published there, the way it checks a signature. Checking
that proof is a read of data shipped with the list; it reveals nothing about
the Chirp doing it.

## Why consider it at all

A long-lived ed25519 key has to live somewhere. On the status service's host
it is exposed to whoever compromises that host; in a secret store it is
exposed to whoever can read the store; and a Chirp release pins its public
half, so rotating it after a leak needs a release and leaves every older
Chirp trusting the leaked key until it expires. Nobody outside can tell what
that key has signed.

Keyless signing removes the key. Each signature is made with an ephemeral key
whose certificate, valid for minutes, says *which workflow, in which
repository, at which ref* asked for it. Every signature is appended to a
public, append-only log. The project already does this for its binaries:
`.github/workflows/release.yml` runs `sigstore/cosign-installer` with
`id-token: write` and signs `SHA256SUMS` with
`cosign sign-blob --yes --bundle SHA256SUMS.sigstore.json`, and attests build
provenance for each binary. The tooling, and the habit of verifying against an
identity rather than a key, are already in the project.

What it costs is a dependency on Sigstore's public-good infrastructure at the
moment of *signing* (not of verifying), and a verification policy that has to
be written carefully, because it, and not the signature, is the security
mechanism.

## Which signature Chirp checks

Three designs were compared.

**ed25519 alone**, as the status proposal has it. Chirp verifies with
`crypto/ed25519` from Go's standard library: no new dependency, nothing to
refresh, a few lines of code. Its weakness is the key. Whoever steals it can
sign a list and serve it to chosen clients only, and nobody else would ever
see that list. Rotation after a theft needs a release, and older Chirps trust
the stolen key until they are updated.

**Keyless alone.** No key to steal, and every list is in Rekor. But Chirp
would have to verify a Sigstore bundle: an X.509 chain to Fulcio, the
certificate's identity extensions, Rekor's signed entry timestamp or
inclusion proof, and the CT log's SCT. Done with
[sigstore-go](https://github.com/sigstore/sigstore-go), that is possible
offline (see [3](#3-how-chirp-verifies-and-what-it-does-when-that-fails)),
but `pkg/verify`, `pkg/root` and `pkg/bundle` pull in about 340 packages
from outside the standard library (`go list -deps`, sigstore-go at
`787fb87`, 2026-11-12): TUF, Rekor's and the timestamp authority's clients,
go-openapi, OpenTelemetry, protobuf and parts of `k8s.io`. Chirp's whole
`go.mod` lists about 80 modules today. For a client whose job is a remote
session, that is a large addition to what has to be trusted, patched and
licence-checked, for one file fetched every few hours. Writing a minimal
bundle verifier by hand instead would avoid the dependencies but put
hand-written certificate and log-proof parsing into Chirp, which is the
kind of code that is easy to get subtly wrong.

The trust also moves rather than vanishes: a list is then as safe as GitHub's
OIDC issuer, Fulcio's CA and the protection of one branch, and Sigstore's
roots rotate on their own schedule, so an embedded root ages and needs a
refresh path of its own.

**Both, which this document proposes.** The workflow signs the canonical list
twice: with an ed25519 key Chirp pins, and keyless, which logs it to Rekor.
Chirp checks only the ed25519 signature, with the standard library. Anyone
else — the project, operators, an independent monitor — checks the Sigstore
bundle and watches Rekor for the workflow's identity.

This is the most secure of the three for this project:

- Clients gain no dependency and no new code beyond what the ed25519 design
  already needed.
- A stolen ed25519 key alone cannot produce a list that the published,
  logged lists match. A targeted list exists only on the clients it was
  served to, and any one of them, or a monitor fetching through many
  vantage points, can show it is missing from Rekor. That turns a silent
  compromise into a detectable one. It does not prevent it, which is the
  honest limit of this design.
- Compromising GitHub's OIDC or Fulcio alone signs nothing a Chirp accepts.
- The ed25519 key lives only as a secret of a GitHub environment that only
  the list workflow on `main` may use, as the macOS signing key does in
  `release.yml`'s `release` environment. It never sits on the status
  service's host.

A later step is for Chirp to also require proof that the list is in Rekor.
It is planned, not part of phase 6's first version; see
[Chirp requiring proof of publication](#chirp-requiring-proof-of-publication).

### Chirp requiring proof of publication

**What it would add.** Chirp would accept a list only if it also carries
proof that Rekor has logged it, bound to the list's hash and to the pinned
ed25519 key. Two things follow:

- **Every list a Chirp accepts is public.** Today a stolen key can sign a
  list served to a few clients only, and it surfaces only if one of them
  reports it. With the proof required, a thief has to log the list first,
  and a monitor watching the log sees it within minutes.
- **A trusted time.** The log's integration time is a clock the signer does
  not control, so `issued` can be checked against it, and a list dated in the
  future cannot outlive its `expires`.

**What it would not add.** It is not prevention. Rekor accepts entries
signed with any key, so a thief can log a forged list and clients will accept
it. The gain is that the attack becomes certainly public and dated, instead
of possibly invisible. That is much stronger than detection by chance, and
weaker than refusing the first use.

**What it costs depends on Rekor's version.** An earlier draft of this
document called it "one ECDSA signature". That holds only for Rekor v1:

- **Rekor v1** gives a *signed entry timestamp*: an ECDSA signature by the
  log over the canonical entry, its integration time, index and log ID.
  The standard library verifies it, but Chirp would still parse the
  `hashedrekord` body to check it names the list's hash and the ed25519 key.
- **Rekor v2** (rekor-tiles) gives no signed entry timestamp. In
  sigstore-go (`pkg/verify/tlog.go`, `787fb87`), a v2 entry is checked by a
  Merkle inclusion proof against a checkpoint signed as a note. That is
  still possible with the standard library and `golang.org/x/mod/sumdb/note`,
  but it is more code that parses another project's formats, and the
  integration time then comes from the checkpoint or a timestamp authority,
  not from the entry.
- **Either way**, Chirp pins the log's public key. Rekor's keys rotate and
  the log moves to new shards, so the pinned key ages and needs its own
  update path, or a release.
- **Availability:** if Rekor is down, no new list is signed. The keyless
  signature already depends on that, and the seven-day `expires` absorbs it.

**Plan, before any code.**

1. Find out which Rekor version the public-good instance will serve when
   phase 6 is implemented, and whether v1 will still be written to. The
   choice of format follows from that, not from what is easier today.
2. Write the verifier's specification here, for that version only: the
   exact bytes checked, the fields compared, the failure for each mismatch.
3. Decide how the pinned log key is refreshed, and how long Chirp trusts one
   it could not refresh, with the same fail-closed rule as an expired list.
4. Build the test vectors first (entries produced by the real workflow, and
   mutated ones that must fail), then the verifier, test-first.
5. Ship it after phase 6's first version, as a separate release: until then
   Chirp checks the ed25519 signature and the monitor watches the log.

Phase 6 itself waits on finding the right node, so there is time for steps 1
to 3 without delaying anything.

Where the rest of this document speaks of a Chirp verifying the bundle, it
describes what an auditor does, and what Chirp would do only if keyless
alone were chosen.

## 1. What is signed

### One artefact, the list

The signed artefact is `network.json`: the members a Chirp may use. It is
derived from two inputs that already exist:

- **Membership** from `network/servers.json` in this repository: who has been
  admitted, by a pull request the maintainer merged (*Membership* in the
  status proposal).
- **State** from the status service: which of those members are active and
  conforming right now (*Conformance*, *Versions*, *Alerts*). A suspended,
  non-conforming or out-of-date member is left out.

The list carries only what a Chirp needs to pick a server and check it:

```json
{
  "format": "chirp-network/v1",
  "serial": 4182,
  "issued": "2026-11-16T12:00:00Z",
  "expires": "2026-11-23T12:00:00Z",
  "source": "https://github.com/barahn/chirp/blob/<commit>/network/servers.json",
  "members": [
    {"id": "node-sp-01", "kind": "node", "url": "https://relay.example.org", "node_key": "ed25519:…"},
    {"id": "nostr-a", "kind": "nostr", "url": "wss://nostr.example.net"},
    {"id": "turn-a", "kind": "turn", "url": "turn:turn.example.org:3478"}
  ]
}
```

Operator names, contacts, GitHub handles, regions and uptime figures are not
in it. They are public on the page already, but a Chirp has no use for them,
and anything in the list is something a client has to parse and a signature
has to cover.

### Canonical form

A signature over JSON is reproducible only if the bytes are. The list is
therefore serialised in one form, and the workflow refuses to sign anything
else:

- [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785) JSON Canonicalization
  Scheme: UTF-8, object keys sorted by code unit, no insignificant
  whitespace, numbers in their shortest form. No floats are used, so the
  number rules never bite.
- `members` sorted by `id`, which is already unique and permanent
  ([`joining-the-network.md`](joining-the-network.md), the `id` field).
- URLs as written in `servers.json`, after the checks CI already applies;
  no further normalisation, so that what is signed is what is listed.
- Times in UTC, RFC 3339, to the second, with `Z`.
- Absent fields are omitted, never `null`.
- `format` names the schema. A Chirp rejects a format it does not know rather
  than guessing.

Anyone with the same `servers.json` commit and the same state can rebuild the
bytes and check that the published list is the one they would have made.

### The daily exports are a different artefact

Phase 7's signed daily exports (`/v1/exports/{day}.json`, *The status
service*) are measurements: every probe, from every vantage point, for one
day. They are large, they are read by people and by other monitors, not by
Chirp, and nothing routes through them. They are a second artefact with
their own schema, signed by the same workflow identity (below) under a
different `format`, so that one can never be mistaken for the other. Signing
both from one identity is a convenience; a Chirp's policy accepts only
`chirp-network/v1`.

## 2. Which identity signs

### The status service cannot sign keyless; a workflow can

Keyless signing needs an OIDC token that Fulcio trusts. A GitHub Actions job
gets one with `id-token: write`; a long-running status service on a VPS, or
the Cloudflare Worker in `chirp-status`, does not. So the signature does not
come from the service. It comes from a scheduled workflow that:

1. checks out `network/servers.json` from `barahn/chirp` at a named commit;
2. reads the status service's current state for those members (the same
   `GET /v1/servers` the page reads);
3. builds the canonical list, with a `serial` one above the last published
   one and an `expires` a fixed interval ahead;
4. signs it with the ed25519 key (`network.json.sig`) and keyless with
   `cosign sign-blob --bundle network.sigstore.json`;
5. publishes both files where the service serves `/v1/network.json`.

It runs on a schedule (proposed: every six hours) and when `servers.json`
changes.

### In which repository

The status proposal and [`ECOSYSTEM.md`](../ECOSYSTEM.md) put the hosted
monitor in `barahn/chirp-status`, and "who is in the network and the rules
for joining it" in `barahn/chirp`. Publishing the list is the hosted
monitor's job, so the workflow lives in **`barahn/chirp-status`**, in a file
of its own that does nothing else: `.github/workflows/network-list.yml`,
running only on `main`. A file of its own matters because the identity is
the file: any other workflow in that repository, however it is changed,
cannot produce a certificate the policy accepts.

Two consequences should be said plainly:

- **The signature attests publication, not truth.** It says this workflow, at
  this ref, published these bytes. It does not say the measurements behind
  the state were honest. That is checked the way every figure on the page is
  checked: anyone can run `chirp-status` against the same list and compare
  (*The status service*, *Authenticity*), and, because of Rekor, compare
  against every list ever published, not only the current one.
- **Whoever can merge to `chirp-status`'s `main` can sign a list.** That is
  the maintainer, through the same review the repository already requires.
  The protection of that branch becomes part of the security of the network,
  and the workflow file should be listed in `CODEOWNERS` there.

### The verification policy

A verifier — a monitor, an operator, or Chirp itself if keyless alone were
chosen — accepts a list's bundle only if it verifies and the certificate in
it says:

| Field | Required value |
|---|---|
| OIDC issuer (`--certificate-oidc-issuer`) | `https://token.actions.githubusercontent.com`, exactly |
| Identity, the certificate's SAN (`--certificate-identity`) | `https://github.com/barahn/chirp-status/.github/workflows/network-list.yml@refs/heads/main`, exactly |
| Source repository identifier | the numeric ID of `chirp-status` |
| Source repository owner identifier | the numeric ID of its owner |
| Workflow trigger | `schedule` or `push` |

Exact strings, not the regular expressions cosign's `--certificate-identity-regexp`
allows: a pattern such as `^https://github.com/barahn/` would accept any
workflow in any of the owner's repositories, and a loose pattern is the
classic way a keyless policy is made worthless.

The numeric identifiers are carried in Fulcio's certificate extensions
(source repository identifier, source repository owner identifier). They
matter because names move: the open repositories are moving from `barahn` to
the `chirp-remote` organisation. A transfer changes the SAN, and a deleted
repository's name could in principle be recreated by someone else; the IDs
survive the first and do not follow the second.

**The organisation move must happen before the policy ships.** If
`chirp-status` moves after Chirp releases with this policy, every released
Chirp rejects every new list. The order is: transfer the repository, then
pin its new SAN and IDs in the release that introduces `-network public`.
If a move ever has to happen later, the release before it accepts both
identities, and the one after drops the old.

Rotation, which open question 4 asks about, splits in two. The keyless side
has no key: what is pinned is a workflow path and two numbers, which change
only when the maintainer moves or renames something on purpose. The ed25519
key still rotates by release, as the status proposal says; what changes is
that a theft is now visible in Rekor rather than silent, so the reason to
rotate is known when it happens.

## 3. How Chirp verifies, and what it does when that fails

### Offline, against an embedded root

A Sigstore bundle carries the signature, the short-lived Fulcio certificate,
and the transparency-log entry for it: its `integratedTime`, a signed entry
timestamp, and an inclusion proof against a signed checkpoint
([bundle format](https://docs.sigstore.dev/about/bundle/)). The bundle
documentation states that for short-lived certificates verified after they
expire, the bundle must carry a log's signed entry timestamp or an RFC 3161
timestamp "to provide proof that signing occurred during the certificates
validity window", and that the signed entry timestamp proves inclusion in the
log "without actually checking the log itself"
([cosign verify](https://docs.sigstore.dev/cosign/verifying/verify/)).

So verification needs the bundle, the list, and Sigstore's trusted root (the
Fulcio CA certificates and the Rekor and CT log public keys). It does not
need Fulcio or Rekor to be reachable. This was confirmed in sigstore-go's
source (commit `787fb87`, 2026-11-12), not only in its documentation:

- `root.NewTrustedRootFromJSON(bytes)` builds the trusted material from a
  `trusted_root.json` held in memory, so it can come from `go:embed`. Only
  `FetchTrustedRoot` and `NewLiveTrustedRoot` go through TUF.
- `verify.NewVerifier(root, verify.WithTransparencyLog(1),
  verify.WithObserverTimestamps(1), verify.WithSignedCertificateTimestamps(1))`
  checks the log entry from the bundle itself: for Rekor v1 entries,
  `tlog.VerifyInclusion` checks the signed entry timestamp and inclusion
  proof against the log's key in the root; for Rekor v2 entries, the
  checkpoint is checked as a signed note with the log's key, using only the
  log's host name from the root. None of `pkg/verify` imports `net/http`.
- `WithObserverTimestamps` makes the log's timestamp, not the clock, the time
  at which the short-lived certificate is checked, which is what lets an
  expired certificate verify. `WithCurrentTime` exists and its own comment
  says it must not be used with short-lived certificates.
- The identity policy is `verify.NewCertificateIdentity(san, issuer,
  certificate.Extensions{SourceRepositoryIdentifier: …,
  SourceRepositoryOwnerIdentifier: …})`, which compares the extensions at
  OIDs `1.3.6.1.4.1.57264.1.15` and `.17` as well as SAN and issuer.

What was not run is an end-to-end test with the network cut; that belongs to
the implementation. For people verifying by hand, `cosign verify-blob
--bundle` with `--certificate-identity` and `--certificate-oidc-issuer` is
the documented command; which flags make the cosign CLI use a local trusted
root instead of TUF was not confirmed.

Under the design proposed here, this path is the auditors', and Chirp checks
the ed25519 signature. sigstore-go is Apache-2.0, which the licence gate
accepts ([`CONTRIBUTING.md`](../CONTRIBUTING.md), *What is enforced today*),
and no rule of the repository forbids third-party packages: the rules are a
permissive licence, dependencies pointing inwards
([`ECOSYSTEM.md`](../ECOSYSTEM.md)), and pinning. Its weight is the reason
it stays out of the client, not a rule.

### Freshness: rollback and replay

A list that was validly signed stays validly signed. An attacker on the path,
or a stale cache, can serve last month's list, perhaps one that still names a
node since suspended. Sigstore says when something was signed, not that it is
the latest. Freshness therefore lives in the list and in the client:

- **`expires`** bounds how long any list is usable. Proposed: seven days,
  with a new list every six hours, so a Chirp that cannot fetch survives a
  long outage of the status service and a withdrawn node stops being used
  within a week at worst.
- **`serial`** only grows. A Chirp stores the highest serial it has accepted
  and refuses any lower one, so once it has seen a newer list it cannot be
  rolled back to an older, still unexpired one.
- **`issued`** must not be later than the log's `integratedTime` plus a small
  skew, and the `integratedTime` must fall inside the certificate's validity.
  This stops a list dated in the future from outliving its intended expiry.

What this does not stop: a Chirp that has never seen a newer list can be held
on an older unexpired one, for at most `expires`. That is the window, and it
should be stated to users rather than hidden.

### When verification fails

| Situation | Chirp's behaviour |
|---|---|
| New list verifies, serial higher | Use it, store it as the last good list. |
| New list fails verification, or wrong identity, or lower serial | Discard it, keep the last good list, and tell the user. Never use the failed one, not even partly. |
| Cannot fetch, last good list unexpired | Keep using the last good list. |
| Last good list expired, nothing newer | Stop using the public network. Say that the public list has expired and that `-relay`, `-nostr` and `CHIRP_ICE` still work. |
| `-relay`, `-nostr` or `CHIRP_ICE` set | Those win, as *The signed list* already says. No list is fetched for what they cover. |

Chirp does not fall back silently to hand-configured relays, nor to an
expired list. Falling back silently would turn a forged or stale list into a
way of steering users, and hand-configured relays are the user's own choice,
not something Chirp should make on their behalf. An expired list fails
closed: the public network stops, self-hosting does not.

## 4. Why public transparency helps here

For the list, Rekor being public is the point. Every list ever signed is
recorded with the time it was signed. Anyone can ask: what did the network
tell clients on a given day? Was a node listed while it was suspended? Was a
list signed that was never served from `/v1/network.json`, which would mean
someone had the workflow sign something and served it to a few clients only?
With an ed25519 key, such a targeted list would be indistinguishable from the
real ones; with Rekor, it is in a log the project does not control. A monitor
can watch the log for the identity above and alert on any entry that does not
match a published list.

The list is public data by design: the page already shows every member. So
logging it reveals nothing.

## Where transparency would be a leak

The same mechanism must not be carried into Chirp's sessions, and this
section exists so that the idea is not reused there later.

A session is authenticated by the PAKE over the four secret digits
(*The invariant: no mode can read the traffic*): a secret shared by two
people, read aloud, checked by the two ends with no third party. The relay
learns six routing digits and hands out a key; if it hands out the wrong one,
the PAKE fails. Nothing online is trusted, and nothing is recorded.

Putting Sigstore there would undo each of those properties:

- **An online authority enters the session.** A Fulcio certificate is issued
  on the strength of an OIDC login. Each end's identity would then rest on an
  identity provider and on Fulcio, both of which can be compelled, coerced or
  broken, where today it rests only on four digits the two people share.
- **Every session would be published.** Each signature becomes a public Rekor
  entry carrying the signer's identity and the time. That is a permanent,
  world-readable record of who connected and when: exactly the record that
  *What a node still sees* and *No record of lookups* commit the network
  never to keep, and that operators pledge not to keep even in memory beyond
  the throttle's window. Writing it to a public append-only log would be
  strictly worse than an operator keeping a log, because it can never be
  deleted.
- **It would require identities Chirp does not have.** Chirp's users have no
  accounts. Adding them to make signatures possible would change the product,
  not just its cryptography.

The distinction is what is being signed. The list is a public statement by
the project, meant to be audited. A session is a private act between two
people, meant to leave no trace. Transparency logs are for the first kind
only.

## 5. Dependencies and what can be decided now

Phase 6 waits on *The precondition: finding the right node*, open question 1
of the status proposal. remotekit's relay selection does not answer it: it
measures which node is near, not which holds a registration.
[`network-paths-proposal.md`](network-paths-proposal.md) proposes rendezvous
hashing over the node keys in the signed list; it is not decided.

**Can be decided now**, because none of it depends on how a code finds its
node:

- Whether to sign twice (ed25519 for Chirp, keyless for the log), as
  proposed, or with one of the two alone.
- The signing identity: which repository, which workflow file, which
  trigger, and the order with the move to `chirp-remote`.
- The verification policy, exact match on issuer, SAN and repository IDs.
- The canonical form, `serial`, `expires`, and the failure table.
- That the daily exports are a separate artefact.
- That this mechanism is never used for sessions.

**Waits on finding the right node:**

- The members' fields beyond `id`, `kind`, `url` and `node_key`. Rendezvous
  hashing needs `node_key` and the list's length (its miss-budget rule scales
  with N); another answer may need other fields, or a list that every node
  must also fetch and verify, which would put the verifier in `chirp-relay`
  too.
- How quickly a list change must reach clients and nodes, which sets the
  publication interval and `expires`. Under rendezvous hashing, clients with
  different lists compute different nodes (*When the list changes*), so a
  shorter `expires` may be needed than the seven days proposed here.
- Whether nodes, not only clients, enforce the `serial` rule.

**Also open, and not blocked:**

- Chirp requiring proof of publication: which Rekor version, the verifier's
  specification, and how the log key is refreshed
  (see [Chirp requiring proof of publication](#chirp-requiring-proof-of-publication)).
- Who runs the monitor that compares served lists with Rekor, and from where.
- Signing depends on Sigstore's public-good Fulcio and Rekor being up when
  the workflow runs. An outage delays a new list; the seven-day expiry is
  meant to absorb that.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

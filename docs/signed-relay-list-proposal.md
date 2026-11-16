# Proposal: signing the public relay list with Sigstore

**Status:** proposal, not decided. Nothing here is built, and nothing here
changes a workflow. It is a design for phase 6 of
[`status-page-proposal.md`](status-page-proposal.md) (*Phases*), offered as
an answer to that document's open question 4: who holds the status service's
signing key, and how it is rotated.

That proposal's section *The signed list* says the list is "signed with an
ed25519 key whose public half is compiled into Chirp". This document proposes
replacing that key with Sigstore keyless signing: `cosign sign-blob` in a
GitHub Actions workflow, its certificate issued by Fulcio against the
workflow's OIDC identity, and the signature recorded in Rekor. If the
maintainer accepts it, *The signed list* and open question 4 change to point
here; if not, this document records why the ed25519 key was kept.

It also records, deliberately, why the same mechanism must never be used for
Chirp's sessions (see [Where transparency would be a leak](#where-transparency-would-be-a-leak)).

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
4. signs it with `cosign sign-blob --bundle network.sigstore.json`;
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

A Chirp accepts a list only if its bundle verifies and the certificate in it
says:

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

Rotation, which open question 4 asks about, mostly disappears: there is no
key to rotate. What is pinned is a workflow path and two numbers, and they
change only when the maintainer moves or renames something on purpose.

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
need Fulcio or Rekor to be reachable. Chirp would verify in-process with
[sigstore-go](https://github.com/sigstore/sigstore-go), whose verifier takes
the trusted material, a requirement for transparency-log and timestamp
evidence, and a certificate-identity policy. Its documented examples load the
trusted root over TUF from Sigstore's public-good repository; it also supports
a custom trusted root in the `sigstore_trustroot` format.

Chirp embeds a trusted root in each release and refreshes it over TUF when it
can. **To be confirmed before implementation:** the exact sigstore-go call
that loads a root from an embedded file and verifies with no network access at
all, and whether the cosign CLI offers an equivalent for people verifying by
hand. The documentation read for this proposal shows the shape but not that
call; the claim above rests on the bundle format, not on a tested path.

`sigstore-go` is Apache-2.0, which the licence gate accepts
([`CONTRIBUTING.md`](../CONTRIBUTING.md), *What is enforced today*). It is a
substantial new dependency for Chirp, and adding it is part of what this
proposal asks the maintainer to decide.

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
| New list fails verification, or wrong identity, or lower serial | Discard it, keep the last good list, say so in the log. Never use the failed one, not even partly. |
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

- Whether to use Sigstore keyless signing instead of a pinned ed25519 key.
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

- Exactly how sigstore-go verifies offline against an embedded root, to be
  confirmed in its code and tests before anyone writes the client side.
- How Chirp refreshes the embedded Sigstore trusted root between releases,
  and how long it trusts one that it could not refresh.
- Signing depends on Sigstore's public-good Fulcio and Rekor being up when
  the workflow runs. An outage delays a new list; the seven-day expiry is
  meant to absorb that.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

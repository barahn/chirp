# Joining the public network with your relay

This page is for people who run a Chirp relay and want to add it to Chirp's
public network, so that it is listed and measured at
[status.usechirp.app](https://status.usechirp.app/). How to run a relay at all
is in [`relay.md`](relay.md); the design behind the network is in
[`status-page-proposal.md`](status-page-proposal.md).

## What joining means today

The network is being built in phases (see the proposal's *Phases*). Today:

- **Your relay is measured, in public.** It is probed every 30 seconds from
  outside, and the status page shows whether it is up, since when it has
  been off-line, its latency, and how much of each of the last 90 days it
  answered. It is marked *new* for its first 30 days.
- **Nobody is sent to it yet.** No Chirp picks a relay from the list until
  the network publishes a signed list and Chirp learns to use it (phase 6).
  Until then, people use your relay only if you give them its address.
- **It sees what any relay sees.** Relays do not have a public mode yet
  (phase 4). Until they do, a relay on the default path sees the whole code
  and the signalling, as `relay.md`'s security checklist says. It never sees
  the screen or the input: those go end to end, encrypted, between the two
  Chirps.

Joining now gets your relay a public track record before it is used, which
is what the later phases will ask for.

## What your relay needs

The list is checked by CI, and an entry that breaks one of these rules is
refused:

- **HTTPS on a domain name**, such as `https://relay.example.org`: not an IP
  address, not plain HTTP, and nothing after the host. The certificate must
  be valid.
- **`GET /healthz` answers `200` within 5 seconds** from the internet. That
  is the probe: nothing else about your relay is counted.
- **A GitHub account** for the operator. Problems with your relay will be
  raised as GitHub issues mentioning it.

And from [`relay.md`](relay.md):

- Every item of its **security checklist**, above all: only your proxy can
  reach the relay, and no log records request URIs.
- **Not behind a CDN**, if you can help it. A relay behind Cloudflare's proxy
  sees every client as Cloudflare unless it trusts Cloudflare's ranges, and
  then one client can lock out everyone (`relay.md`, *How the pieces
  connect*). If yours is behind one, say so in the entry; it is shown on the
  page.
- **`-security-contact` set**, so that whoever finds a problem with your
  relay knows where to report it.
- **Kept up to date** with Chirp's releases. Today this is on your word; the
  conformance checks of phase 3 will measure it, without your relay ever
  stating its version in public.

### What you agree to

Some things cannot be checked from outside. By joining, you agree to:

- log no request URIs, routing digits or link tokens (the shipped
  Caddyfile's filtered logs, kept as they are);
- run no analytics on the relay's users;
- not restrict access by country;
- publish, under the AGPL, any change you make to the relay's code.

## How to join

1. **Run your relay** as [`relay.md`](relay.md) says, and check it from a
   machine outside your network:

   ```bash
   curl -fsS https://relay.example.org/healthz
   ```

   It must print `ok`.

2. **Apply** with the
   [*Join the public network* form](https://github.com/barahn/chirp/issues/new?template=join-network.yml).
   It asks for what the entry needs (below) and for you to confirm the
   requirements and the commitments.

3. **A maintainer reviews it**, probes the relay once by hand, and adds your
   entry to [`network/servers.json`](../network/servers.json). Questions, if
   any, are asked on the issue.

4. **Your relay appears on [status.usechirp.app](https://status.usechirp.app/)
   within about five minutes of the merge**, with nothing more to do: the
   status service reads the list from this repository.

Why a form and not a pull request: Chirp does not merge pull requests from
outside Fabrintek until its Contributor License Agreement is enforced
([`CONTRIBUTING.md`](../CONTRIBUTING.md)). A maintainer writing your entry
needs none.

### The entry

What the form asks for becomes your entry:

```json
{
  "id": "example-sp-01",
  "kind": "node",
  "url": "https://relay.example.org",
  "operator": "Example Org",
  "github": "example-ops",
  "region": "BR",
  "behind_cdn": false,
  "added": "2026-10-27"
}
```

| Field | |
|---|---|
| `id` | Lowercase letters and digits joined by single hyphens, at most 40. Unique in the list, and kept for good: it is in the page's links. |
| `kind` | `node` for a Chirp relay. Nostr relays and TURN servers will be accepted from phase 3. |
| `url` | The relay's root, `https://` and the domain. |
| `operator` | You or your organisation, as the page shows it. |
| `github` | Your GitHub handle. |
| `contact` | Optional: an email address for alerts. |
| `region` | Optional: the ISO 3166 two-letter code of the country the relay runs in. |
| `behind_cdn` | `true` if a CDN or proxy you do not run sits in front. |
| `added` | The day it was added; the maintainer sets it. |

**Everything in the entry is public**, in this repository and on the page,
the contact address included. Leave it out if you would rather not publish
one.

## Once you are in

- **Your relay's figures are public** and stay so: an outage is on the page
  for 90 days. Every figure is measured from outside, and a minute nobody
  probed is shown as *no data*, never as down.
- **Keep the URL.** Moving the relay to another address is a change to the
  entry: ask on an issue first, so its history is not split.
- **If something goes wrong** with your relay, expect an issue mentioning
  you. Once phase 5 is built, a relay that stays non-conforming past its
  grace period is suspended, and removed after 90 days suspended.

## Leaving

Open an issue asking for the entry to be removed. It leaves the page; its
past figures are not republished.

## What is coming

Phases 4 and 5 make joining automatic, and they replace the form:

1. `chirp-relay -mode public` gives the relay its own key, published at
   `/.well-known/chirp-node`.
2. You apply with the relay's URL and your GitHub handle, and the status
   service checks that the relay holds the key, which proves you control the
   domain.
3. For 30 days the relay is probed and shown as *new (in probation)*.
4. If it ends with at least 95 % uptime and every conformance check passing,
   the status service opens the pull request adding it, and a maintainer
   merges it.

---

`SPDX-License-Identifier: AGPL-3.0-only`

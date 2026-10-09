# Chirp

On-demand remote support. Download Chirp and run it — nothing to install, no
account. It shows your own code and a share link. Read the code (or send the
link) to someone you trust; they type it into their Chirp, you accept in a
prompt on your own screen, and they can help you. Every Chirp is the same: the
one who types the code controls, the one who shared it decides.

How it works, and what that commits Chirp to, is in
[docs/product-model.md](docs/product-model.md).

[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![CI](https://github.com/barahn/chirp/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/barahn/chirp/actions/workflows/ci.yml)
[![CodeQL](https://github.com/barahn/chirp/actions/workflows/codeql.yml/badge.svg?branch=develop)](https://github.com/barahn/chirp/actions/workflows/codeql.yml)

## Status: not yet a runnable product

Read this before trying to use it.

What is here today:

- **The session layer** — each running Chirp registers as a peer and gets its
  own code and share link; whoever redeems the code controls that peer's
  machine once its user consents. Codes, links and the per-origin throttle are
  remotekit's `rendezvous` package; the state machine, consent rules and values
  are Chirp's.
- **A signalling relay** — relays WebRTC signalling between the two ends of a
  session and announces each end to the other. Written for Chirp — no
  multi-tenancy, no RBAC — rather than taken from the proprietary Barahn
  control plane. `cmd/chirp-relay` runs it, for a relay you host yourself.
- **The `cmd/chirp` binary, up to the consent request** — the one program
  both sides run. Its window shows this machine's code and link, and lets its
  user type someone else's code or paste their link; the other machine is then
  asked for consent. The window is Fyne in its default theme
  ([docs/ux.md](docs/ux.md) has the plan for the rest).

All of it is tested, AGPL-3.0, and follow the [product model](docs/product-model.md):
the code belongs to the machine to be controlled, and consent is asked of that
machine's user over its own socket, as a remotekit `bark` consent request.

What is **not** here yet:

- **The consent prompt and the screen.** `cmd/chirp` stops at the consent
  request: drawing the prompt and answering it (barahn/chirp#11), and sharing
  the screen through remotekit (barahn/chirp#12), come next. Until then nobody
  can be let in.
- **A TURN relay.** Until Chirp ships one, self-hosting means bringing your own
  STUN/TURN (coturn works): see [docs/stun-turn.md](docs/stun-turn.md).

To try what exists: `cmd/chirp` needs cgo, and on Linux the OpenGL, X11 and
Wayland headers (`libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev` on
Debian and Ubuntu). Start a relay, then two Chirps, and type one's code into
the other:

```sh
go run ./cmd/chirp-relay
CHIRP_RELAY=http://localhost:8080 go run ./cmd/chirp
```

The remote-session mechanism — screen capture, VP8 encoding, input injection,
clipboard, file transfer, WebRTC, the reverse tunnel — is not missing; it lives
in [remotekit](https://github.com/barahn/remotekit) under Apache-2.0, and Chirp
will consume it as a dependency.

Tracking: planned work is in this repository's issues and the
[Chirp roadmap](https://github.com/orgs/barahn/projects/2) project.

## Where Chirp sits

Chirp is one of three repositories, and the one you want depends on what you are
looking for: the reusable mechanism is [remotekit](https://github.com/barahn/remotekit)
(Apache-2.0), this repository is the on-demand support product (AGPL-3.0-only),
and Fabrintek's commercial fleet platform is separate and proprietary.
Dependencies point inwards only — Chirp may depend on remotekit, never on the
platform.

[ECOSYSTEM.md](ECOSYSTEM.md) states each repository's role, why each licence is
the one it is, and how to decide which repository a change belongs in.

## Design commitments

These are constraints on what Chirp may become, not features to be traded away:

- **No feature gates.** Screen sharing, input control, multi-monitor, file
  transfer and clipboard sync are the free core, permanently.
- **Explicit consent.** A session cannot start without the end user agreeing to
  it on their own machine, in a prompt Chirp controls.
- **Self-hostable with zero restrictions.** Anyone may run their own signalling
  and relay infrastructure. Any hosted service is a convenience, never a
  requirement.

## Contributions are closed for now

Chirp is not accepting external contributions yet. Issues and bug reports are
welcome; pull requests from outside Fabrintek will not be merged.

This is deliberate and temporary. Chirp is dual-licensed (see
[LICENSING.md](LICENSING.md)), which works only while Fabrintek holds the
copyright in all of it. Merging a contribution before Contributor License
Agreement acceptance is enforced here would leave code in the tree that
Fabrintek cannot relicense — quietly, and in a way no build or test would
catch.

Rather than accept contributions under terms that are written down in another
repository and shown to nobody, the door stays shut until the CLA is presented
and recorded where pull requests actually arrive. See
[CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Report vulnerabilities privately to
[security@mendsec.com](mailto:security@mendsec.com) — not in a public issue. See
[SECURITY.md](SECURITY.md).

Chirp grants remote control of an end user's machine, so its pipeline treats
that as the threat model rather than an afterthought: every pull request runs
gosec, govulncheck, gitleaks and a third-party licence gate, with every action
pinned to a commit SHA. CodeQL and dependency review are wired in too, and turn
on when the repository has code scanning available -- see
[docs/devsecops.md](docs/devsecops.md). What each gate is for,
and the repository settings that back it, are in
[docs/devsecops.md](docs/devsecops.md).

`develop` is the integration branch: branch from it and open pull requests
against it. `main` receives releases from `develop`.

## Licence

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE).

The AGPL is deliberate: Chirp is network-delivered software, and the AGPL is
what keeps a hosted fork from taking the code without returning changes.

Fabrintek, as sole copyright holder, also licenses Chirp under separate
commercial terms. This does not affect your rights under the AGPL — see
[LICENSING.md](LICENSING.md).

# Chirp

On-demand remote support: a technician generates a 6-digit PIN, the person
being helped enters it, grants explicit consent, and the session begins. No
installation, no standing agent, no account.

[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![CI](https://github.com/barahn/chirp/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/barahn/chirp/actions/workflows/ci.yml)
[![CodeQL](https://github.com/barahn/chirp/actions/workflows/codeql.yml/badge.svg?branch=develop)](https://github.com/barahn/chirp/actions/workflows/codeql.yml)

## Status: not yet a runnable product

Read this before trying to use it.

What is here today is the **session layer** — PIN generation and redemption,
the session lifecycle state machine, and the consent flow. It is extracted, it
is tested, and it is AGPL-3.0.

What is **not** here yet:

- **A signalling server.** Chirp needs its own, simplified: no multi-tenancy,
  no RBAC. The existing one belongs to the proprietary Barahn control plane and
  is not being relicensed.
- **The Nest** — the `/join` page where an end user enters their PIN.
- **A TURN relay.** Until Chirp ships one, self-hosting means bringing your own
  STUN/TURN (coturn works).
- **A `cmd/chirp` binary.** There is no entry point yet.

The remote-session mechanism — screen capture, VP8 encoding, input injection,
clipboard, file transfer, WebRTC, the reverse tunnel — is not missing; it lives
in [remotekit](https://github.com/barahn/remotekit) under Apache-2.0, and Chirp
will consume it as a dependency.

Tracking: [the separation plan](https://github.com/barahn/barahn) in the Barahn
repository.

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

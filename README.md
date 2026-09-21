# Chirp

On-demand remote support: a technician generates a 6-digit PIN, the person
being helped enters it, grants explicit consent, and the session begins. No
installation, no standing agent, no account.

[![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)

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

## Design commitments

These are constraints on what Chirp may become, not features to be traded away:

- **No feature gates.** Screen sharing, input control, multi-monitor, file
  transfer and clipboard sync are the free core, permanently.
- **Explicit consent.** A session cannot start without the end user agreeing to
  it on their own machine, in a prompt Chirp controls.
- **Self-hostable with zero restrictions.** Anyone may run their own signalling
  and relay infrastructure. Any hosted service is a convenience, never a
  requirement.

## Licence

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE).

The AGPL is deliberate: Chirp is network-delivered software, and the AGPL is
what keeps a hosted fork from taking the code without returning changes.

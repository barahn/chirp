# The three repositories

Fabrintek's remote-access work is split across three repositories with three
different licences. The split is not organisational tidiness: it is what lets
an AGPL product and a proprietary product share one implementation of the hard
parts without either one's licence reaching the other.

This page is the same in all three repositories, so that each one states its own
role and acknowledges the other two. If you are reading it in one repository and
acting on another, read that one's copy too — the roles are reciprocal, and a
change that looks right from one side is often the arrow pointing the wrong way.

## The repositories

| Repository | Licence | Role |
|---|---|---|
| [remotekit](https://github.com/barahn/remotekit) | Apache-2.0 | The mechanism. Screen capture and VP8 encoding, input injection, clipboard, chunked file transfer, a WebRTC wrapper, a reverse tunnel, tray and OS-service installation. |
| [chirp](https://github.com/barahn/chirp) | AGPL-3.0-only | On-demand remote support. One binary, the same on both sides, with nothing to install and no account: it shows its own code and share link; whoever types someone's code controls that machine once its owner consents there. |
| [barahn](https://github.com/barahn/barahn) | Proprietary | Fabrintek's commercial platform: fleet management over standing agents, with tenancy, roles and persistence. |

**This repository is Chirp** — the AGPL-3.0-only on-demand support layer.

## Dependencies point inwards only

```
        barahn  (proprietary)
          │  may depend on
          ├──────────────► chirp  (AGPL-3.0-only)
          │                  │  may depend on
          └──────────────────┴──► remotekit  (Apache-2.0)
```

- **remotekit depends on neither of the others.** It is the innermost layer.
- **Chirp may depend on remotekit.** Never on the platform.
- **The platform may depend on both.**

A change that needs one of those arrows reversed is a design problem, not a
licensing exception. There is no approval that makes it acceptable, because the
constraint is not a policy — reversing an arrow either breaks a licence or
breaks the dual-licensing arrangement described in
[LICENSING.md](LICENSING.md).

## Why each licence is the one it is

**remotekit is permissive** so that both an AGPL project and a proprietary one
can build on it. That is its entire reason for existing as a separate
repository: Chirp and the platform have incompatible licences and could not
consume each other's code, so the mechanism both need lives where both can
reach it.

**Chirp is AGPL** because it is network-delivered software, and the AGPL is what
stops a hosted fork from taking the code without returning changes.

**The platform is proprietary**, and Fabrintek's ability to keep it that way
while using Chirp rests on Fabrintek owning all of Chirp — which is why Chirp
requires a CLA and why its contributions are currently closed. See
[CONTRIBUTING.md](CONTRIBUTING.md).

## Where does this change belong?

Work outwards from the innermost repository that can hold it.

**It belongs in remotekit** if it is mechanism: something that manipulates a
screen, a device, a codec, a socket or a file, and that would be just as useful
to a product with entirely different rules. remotekit carries no product
opinions — there is no user, tenant, role or session policy anywhere in it, and
nothing in it shells out to run commands on a caller's behalf. Features layer on
through extension points rather than being compiled in.

**It belongs in Chirp** if it is the ad-hoc support product: how a PIN is
generated and redeemed, how consent is asked for and recorded, what a session
may do and for how long, what the join page shows. Chirp's own commitments
constrain this — no feature gates, consent that cannot be bypassed, and
self-hosting with no restrictions.

**It belongs in the platform** if it presumes accounts, tenants, roles, fleets
or standing agents. Chirp has none of those and is not the place to introduce
them.

If a change seems to belong in two of them, it is usually one mechanism plus one
policy that have not been separated yet. Split it and the arrows resolve.

## What this repository must not acquire

- A dependency on the proprietary platform, in any direction, at any depth.
- Code copied out of the platform. It is not relicensed, and Chirp being AGPL
  does not make it available.
- A reimplementation of something remotekit already provides. Capture, encoding,
  input injection, clipboard, file transfer and the WebRTC plumbing are
  upstream. If you are writing one of those here, it belongs there instead.
- A copyleft dependency. Chirp is AGPL, so a GPL or AGPL dependency would be
  licence-*compatible* with it and still unacceptable: Fabrintek relicenses
  Chirp on the strength of owning all of Chirp, and it owns none of its
  dependencies.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

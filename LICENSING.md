# Licensing

## The short version

Chirp is offered to the public under the **GNU Affero General Public License
v3.0 only** (`AGPL-3.0-only`). That is the licence you receive it under, and
nothing on this page reduces the rights it gives you.

Fabrintek Engenharia Digital Ltda holds the copyright in all of Chirp and, as
copyright holder, also licenses it under separate commercial terms — including
for use in its own proprietary products.

## Why this page exists

A copyright holder is not a licensee of its own work. Fabrintek can release
Chirp under the AGPL to everyone and simultaneously use it under different
terms itself, because it never needed the AGPL's permission in the first
place. This is the ordinary dual-licensing arrangement, the same one MySQL and
Qt have used for decades.

It is written down here rather than left implicit for two reasons. Someone
auditing the licence position of a Fabrintek product should be able to find the
reservation stated by the copyright holder, in the repository it applies to.
And anyone who notices that a proprietary Fabrintek product links AGPL code
deserves a straight answer rather than an apparent contradiction.

## What this means for you

**If you use, modify, fork or self-host Chirp:** the AGPL applies in full, and
this page changes nothing. You get the source, the right to modify it, and the
right to run it — including as a network service — provided you extend the same
terms downstream, including AGPL section 13's requirement to offer source to
users who interact with it over a network.

**If you contribute:** see [CONTRIBUTING.md](CONTRIBUTING.md). Contributions
require a Contributor License Agreement, because a contribution licensed to
Fabrintek under the AGPL alone would end the arrangement described above.
Contributions are currently closed while that process is being put in place.

**If you want Chirp under other terms:** commercial licensing is available.
Contact Fabrintek.

## The boundary

Chirp is one of three repositories:

| Repository | Licence | What it is |
|---|---|---|
| [remotekit](https://github.com/barahn/remotekit) | Apache-2.0 | shared remote-session core: capture, encode, input, transport |
| this repository | AGPL-3.0-only | the on-demand support session layer |
| Barahn | Proprietary | Fabrintek's commercial platform |

`remotekit` is permissive by design, so that both this AGPL project and the
proprietary platform can build on it. Chirp does not depend on the proprietary
platform, and it never will: the dependency runs the other way.

## Third-party code

Chirp's dependencies are listed with their full licence texts in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md), which is generated and
checked in CI.

The dual-licensing above rests on Fabrintek owning what is in this repository.
It is no defence against third-party code arriving here under terms Fabrintek
cannot pass on, which is why that check exists and why it treats an
unrecognised licence as a failure rather than a warning.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

# Security Policy

## Reporting a vulnerability

**Do not open a public issue.** Email
[security@fabrintek.com.br](mailto:security@fabrintek.com.br).

Chirp lets one person control another person's machine. A bug in code or link
generation, redemption, the lookup, the session lifecycle or the consent flow is
not an ordinary defect: it is unauthorised access to a desktop belonging to a
person who was told they had to agree first. Treat anything in that area as
security-sensitive even when it looks cosmetic.

Include what you have: the affected version or commit, what an attacker gains,
and a reproduction case if you have one. A report without a proof of concept is
still worth sending.

If you would like a reply encrypted, say so in your first message and a key will
be provided.

### What to expect

| Stage | Target |
| --- | --- |
| Acknowledgement of your report | 3 working days |
| Initial assessment and severity | 10 working days |
| Fix or documented mitigation for a confirmed high/critical issue | 90 days |

These are targets, not contractual terms. Chirp is pre-release and maintained by
a small team; if a date is going to slip you will be told rather than left
waiting.

### Disclosure

Coordinated disclosure. Please give the 90-day window above before publishing.
If a fix is not shipping in that time, you are free to disclose — the window
exists to protect users, not to bury a report. Reporters are credited in the
advisory unless they ask not to be.

## Supported versions

Chirp is **not yet a runnable product** (see [README.md](README.md)):
`cmd/chirp` goes as far as the consent decision, and no screen is shared yet.
Until it is, only the tip of `develop` and the last release on `main` are
supported, and the answer to "which versions get fixes" is "the one you get by
pulling".

| Version | Supported |
| --- | --- |
| `develop` (tip) | ✅ |
| `main` (`v0.2.0`, the last release) | ✅ |
| `v0.2.0` tag | fixes land on `develop` and ship in the next release; no backports. |
| `v0.1.0` | no. It predates the product model, the reaper deadlock fix and the redemption throttle — do not run it. |

## What is in scope

- The session layer and the relay: code and link generation and entropy,
  redemption and its per-origin throttle, the per-origin caps on sessions and
  registered peers, the release of absent peers, the lookup, session state
  transitions, the consent flow.
- `cmd/chirp` and `cmd/chirp-relay`, including the consent prompt: anything
  that lets the controlling side answer it, or makes it misstate who is asking
  or for what.
- Anything that lets a session start, continue or regain control **without the
  end user's explicit consent on their own machine**. That is a design
  commitment, so breaking it is always a vulnerability.
- The build and release path: CI configuration, dependency and action pinning,
  the third-party licence gate.

## Running a relay

`cmd/chirp-relay` is the relay the first runnable Chirp ships with, and it is
**trusted by design** (`docs/product-model.md`, *What follows for security* and
decision 5). It sees the codes it routes by, so whoever runs it could
impersonate either end of a session. Run one for yourself or for people who
trust you; do not offer one to the public yet. A public relay waits for the
blind design: a lookup that never sees the code's secret part, signalling over
Nostr, and a PAKE (barahn/chirp#21, #43).

It speaks plain HTTP. Put a TLS-terminating proxy in front of it for anything
beyond one machine. `chirp-relay` counts every per-origin limit against the
connection's address, so behind a proxy all of its users share one origin and
are throttled together. A program that embeds `signalling.Server` can set
`Server.ClientIP` to read the proxy's forwarded header instead, but only when
that proxy is the sole way in: a client that can reach the relay directly can
write the header itself and dodge every limit. STUN and TURN are yours to
bring: [docs/stun-turn.md](docs/stun-turn.md).

A weakness in how a relay is meant to be run is in scope, as is any way to get
past its per-origin limits.

## Out of scope

The proprietary Barahn control plane (report anything there to Fabrintek
directly), and [remotekit](https://github.com/barahn/remotekit), which has its
own policy.

## What this pipeline enforces, and what it does not

| Gate | Question it answers | Runs |
| --- | --- | --- |
| `go vet`, golangci-lint | Is the code well-formed | every push and PR |
| `go test -race` | Do the tests pass, free of data races | every push and PR |
| gosec | Does the code match a known-bad Go pattern | every push and PR |
| govulncheck | Is a known vulnerability reachable from Chirp's own code | every push and PR |
| gitleaks | Is a credential being committed | every push and PR, weekly |
| Third-party licence gate | Can Fabrintek pass every dependency on | every push and PR |
| DCO sign-off | Is the origin of each commit certified | every PR |
| CodeQL (`security-extended`) | Does untrusted data reach a dangerous sink | every push and PR, weekly |
| Dependency Review | What does this pull request *add*, and under what licence | every PR |

CodeQL and dependency review run while the `CODE_SCANNING` variable is set, as
it is on this public repository. A private copy needs GitHub Advanced Security
for them; there the variable stays unset, and every run warns that they are
off. See [docs/devsecops.md](docs/devsecops.md).

None of these is a substitute for reading the diff. They catch the classes of
mistake a reviewer reliably misses; they do not catch a consent check that is
present, correct and applied to the wrong session.

Security tooling is pinned to commit SHAs (actions) or exact versions (Go
tools) and updated by Dependabot on a schedule. A pinned tool that is never
updated is a tool that reports last year's vulnerabilities.

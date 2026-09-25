# Security Policy

## Reporting a vulnerability

**Do not open a public issue.** Email
[security@mendsec.com](mailto:security@mendsec.com).

Chirp grants a remote technician control of someone else's machine. A bug in
PIN generation, PIN redemption, session lifecycle or the consent flow is not an
ordinary defect: it is unauthorised access to a desktop belonging to a person
who was told they had to agree first. Treat anything in that area as
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

Chirp is **not yet a runnable product** (see [README.md](README.md)): there is
no `cmd/chirp` binary and no tagged release. Until the first release, only the
tip of `develop` is supported, and the answer to "which versions get fixes" is
"the one you get by pulling".

| Version | Supported |
| --- | --- |
| `develop` (tip) | ✅ |
| `main` (last integrated) | ✅ |
| anything tagged | none exist yet |

## What is in scope

- The session layer: PIN generation and entropy, PIN redemption and its
  rate limiting, session state transitions, the consent flow.
- Anything that lets a session start, continue or regain control **without the
  end user's explicit consent on their own machine**. That is a design
  commitment, so breaking it is always a vulnerability.
- The build and release path: CI configuration, dependency and action pinning,
  the third-party licence gate.

Out of scope: the proprietary Barahn control plane (report those to Fabrintek
directly), and [remotekit](https://github.com/barahn/remotekit), which has its
own policy.

## What this pipeline enforces, and what it does not

Every pull request into `develop` or `main` runs, and must pass:

| Gate | Question it answers |
| --- | --- |
| `go vet`, golangci-lint | Is the code well-formed |
| `go test -race` | Do the tests pass, free of data races |
| gosec | Does the code match a known-bad Go pattern |
| CodeQL (`security-extended`) | Does untrusted data reach a dangerous sink |
| govulncheck | Is a known vulnerability reachable from Chirp's own code |
| Dependency Review | What does this pull request *add*, and under what licence |
| gitleaks | Is a credential being committed |
| Third-party licence gate | Can Fabrintek pass every dependency on |
| DCO sign-off | Is the origin of each commit certified |

None of these is a substitute for reading the diff. They catch the classes of
mistake a reviewer reliably misses; they do not catch a consent check that is
present, correct and applied to the wrong session.

Security tooling is pinned to commit SHAs (actions) or exact versions (Go
tools) and updated by Dependabot on a schedule. A pinned tool that is never
updated is a tool that reports last year's vulnerabilities.

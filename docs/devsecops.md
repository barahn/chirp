# DevSecOps in this repository

Two things this document is not: a list of tools, and a promise that the
pipeline makes review unnecessary. It records what is automated, what is a
setting someone has to click, and why each gate exists — because a gate whose
reason is forgotten is a gate that gets disabled the first time it is
inconvenient.

## Branching model

`develop` is the default branch and the integration branch. Work lands there
first; `main` holds what has been integrated and is what a release is tagged
from.

```
feature/*  ──▶  develop  ──▶  main  ──▶  tag v*.*.*
   PR, CI        PR, CI       release
```

- **Branch from `develop`**, not `main`. `git switch develop && git pull`.
- **Open pull requests against `develop`.** CI runs the same gates on both
  branches, so a change that could not pass on `develop` cannot pass later.
- **`main` receives merges from `develop` only.** No feature branch targets
  `main` directly; a hotfix is still a branch off `develop` that is merged
  forward, so `main` never holds a commit `develop` lacks.
- **Tags are cut from `main`** and match `v*.*.*`, which is what triggers the
  SBOM job.

Chirp requires a DCO sign-off on every non-merge commit (`git commit -s`), which
CI verifies. See [CONTRIBUTING.md](../CONTRIBUTING.md); note that a sign-off is
not CLA acceptance, and that external pull requests are not merged yet.

## The pipeline

| Workflow | Runs on | Gate |
| --- | --- | --- |
| `ci.yml` | push and PR to `develop`/`main`, tags | DCO, licence gate, build, vet, cross-compile, lint, `go test -race`, website check, `HANDOFF.md` snapshot, gosec, govulncheck |
| `codeql.yml` | push and PR, weekly — **when `CODE_SCANNING` is set** | CodeQL `security-extended` for Go |
| `secret-scan.yml` | push and PR, weekly | gitleaks over the PR commit range, or the tree and full history |
| `dependency-review.yml` | PR — **when `CODE_SCANNING` is set** | what the PR adds: advisories at `low` and above, denied licences |
| `sbom.yml` | release tags | SPDX SBOM attached to the release |
| `pages.yml` | push to `develop` touching the site | website check, then publishes usechirp.app |
| `site-live.yml` | after each deploy, weekly | usechirp.app serves the pages, scripts and headers the repository has |

### CodeQL and dependency review are gated on `CODE_SCANNING`

Both jobs run only when the repository variable `CODE_SCANNING` is `true`.
This repository is public, where both are free, and the variable is set: they
run on every pull request and are required checks on `develop` and `main`.

The gate exists for private copies. On a private repository GitHub runs both
only with GitHub Advanced Security (Code Security). That was observed, not
assumed: while this repository was private, on barahn/chirp#1, dependency
review failed with *"not supported on this repository ... ensure Dependency
graph is enabled along with GitHub Advanced Security"*, and CodeQL's analysis
completed but the job ended in a configuration error. The CodeQL licence also
permits analysing private code only under that entitlement, so this is not a
gate to force through. On such a copy leave the variable unset, or set it once
Code Security is enabled. Until then each run carries a warning saying the gate
is off, so its absence is visible rather than quietly green.

Note the interaction with required checks: a skipped job **counts as passing**.
Where `CODE_SCANNING` is not set, marking `Analyze (Go)` or
`Review dependency changes` as required enforces nothing.

Why the overlaps are not redundant:

- **gosec vs CodeQL.** gosec matches known-bad patterns in a single function.
  CodeQL follows data from a source to a sink across packages. A PIN that
  reaches a log or a shell in another package is the second tool's finding.
- **govulncheck vs Dependency Review.** govulncheck asks whether a known
  vulnerability is *reachable* from Chirp's code — stricter, and the one that
  fails the build. Dependency Review asks what this pull request introduces,
  which catches a dependency that has no advisory yet and a licence Chirp
  cannot carry.
- **The licence gate vs Dependency Review.** `scripts/gen-third-party-notices.sh`
  is the authority: it resolves the module set for linux, windows and darwin and
  fails on copyleft or unrecognised licences. Dependency Review is the earlier,
  cheaper warning on the same rule. Chirp is AGPL, so a copyleft dependency is
  licence-*compatible* and still unacceptable — Fabrintek relicenses Chirp
  because it owns all of Chirp, and it owns none of its dependencies. See
  [LICENSING.md](../LICENSING.md).

### Supply chain

- Every GitHub Action is pinned to a **full commit SHA** with the version in a
  trailing comment. A tag can be moved; a SHA cannot.
- Go security tools are pinned to exact versions and run via `go run`, so the
  tool version lives in the workflow and is reviewed like any other change.
- Dependabot rewrites both, weekly, into `develop`. Pinning without an update
  path is just freezing on a known-vulnerable version.
- Workflows declare `permissions: contents: read` at the top level; a job that
  needs more (SARIF upload, release assets, PR comments) asks for it and nothing
  else.
- `persist-credentials: false` on checkout, so the workflow token is not left in
  `.git/config` where a later step — or a dependency's build script — can read
  it.
- harden-runner runs in `egress-policy: audit`. Read the egress report on a few
  green runs, then move it to `block` with an allowlist. Flipping it blind
  breaks the Go module proxy, the linter download and the action downloads at
  once.

## Settings that are not in this repository

Workflows cannot grant themselves these. They are repository settings, and they
are the half of DevSecOps that a pull request cannot deliver:

1. **Default branch → `develop`**
   Settings → Branches → Default branch → switch to `develop`.

2. **Branch protection on `develop` and `main`.** The rules live as code in
   [`.github/rulesets/`](../.github/rulesets/), and
   [branching.md](branching.md) explains each one — including why `main` has
   neither linear history nor required signatures yet, and why `develop`
   requires no approval: a single maintainer cannot approve their own
   pull requests. Import
   those files rather than configuring rules by hand: a hand-made rule that is
   not in the files is drift.

3. **Code security** (Settings → Advanced Security):
   - Secret scanning **and** push protection. gitleaks in CI catches a secret
     before merge; push protection catches it before it ever reaches GitHub.
   - Private vulnerability reporting, so [SECURITY.md](../SECURITY.md) has a
     mechanism behind it.
   - Dependabot alerts and security updates.

4. **Actions** (Settings → Actions → General):
   - Workflow permissions: read-only by default.
   - Require approval for all outside collaborators' workflow runs.

5. **Tag protection** for release tags, so a tag cannot be moved after an SBOM
   has been published against it. It is a ruleset too,
   `.github/rulesets/release-tags-protected.json`, imported like the branch
   rules: no one may delete, move or force-push a `v*` tag
   ([branching.md](branching.md#release-tags)).

6. **usechirp.app behind Cloudflare**, which adds the security headers GitHub
   Pages cannot set. See [The website's headers](#the-websites-headers).

The first five are applied as of 2026-10-10 (barahn/chirp#17), when the
repository went public. The sixth is applied as of 2026-10-12.

### The website's headers

The site is static and served by GitHub Pages, which sets no custom headers.
Each page carries its Content-Security-Policy in a `<meta>` tag instead, and
that cannot hold `frame-ancestors`, the rule that stops another site from
framing the page. So Cloudflare's proxy sits in front of GitHub Pages and adds
the headers, from
[`.cloudflare/rulesets/response-headers.json`](../.cloudflare/rulesets/response-headers.json):

| Header | Value | Why |
| --- | --- | --- |
| `Content-Security-Policy` | the pages' policy, plus `frame-ancestors 'none'` | no framing, so no clickjacking of the link page |
| `X-Frame-Options` | `DENY` | the same, for browsers that predate `frame-ancestors` |
| `X-Content-Type-Options` | `nosniff` | a file is only ever what its type says |
| `Referrer-Policy` | `no-referrer` | as the `<meta>` tag already says |
| `Cross-Origin-Opener-Policy` | `same-origin` | a page that opens the site keeps no handle on it |
| `Cross-Origin-Resource-Policy` | `same-origin` | other sites cannot embed its files |
| `Permissions-Policy` | camera, microphone, geolocation, display capture, USB and payment off | the site needs none of them |

HSTS is not among them: `.app` is on the browsers' HSTS preload list, so a
browser never asks for it over plain HTTP in the first place.

Both policies are enforced, the header's and the `<meta>` tag's, so a page
served without Cloudflare (a misconfigured DNS, a preview) keeps every rule but
`frame-ancestors`. `scripts/check-site.sh`, run in CI, fails if the two differ
by anything else, and if `site/` uses an HTML sink: the pages require Trusted
Types, which keeps sinks out in the browsers that enforce them, and the check
keeps them out of the source for the rest.

As of 2026-10-12, set up as follows:

| Piece | State |
| --- | --- |
| Registrar | Unstoppable Domains, nameservers delegated to Cloudflare (`anuj` and `nancy.ns.cloudflare.com`) |
| DNS | `A` and `AAAA` for the apex to GitHub Pages' addresses; `www` a `CNAME` to `barahn.github.io`; the `_github-pages-challenge-barahn` `TXT` that verifies the domain on GitHub |
| Proxy | on for the apex and `www` |
| SSL/TLS | **Full (strict)**: Cloudflare checks GitHub's certificate, so the hop to GitHub is authenticated |
| Edge certificate | Cloudflare's, for `usechirp.app` and `*.usechirp.app` |
| DNSSEC | signed by Cloudflare; DS `2371 13 2 25CFA0663341592F9436373DCB5E87C3B16F0FE043C7169BB7C47D11F9B9AF23` at the registrar, and resolvers validate the chain |
| Header rules | loaded from `response-headers.json` |
| Page rewrites | off: Email Address Obfuscation, Web Analytics (RUM), Bot Fight Mode; `config-settings.json` keeps the first two, Rocket Loader and Zaraz off |

The rules in [`.cloudflare/rulesets/`](../.cloudflare/rulesets/) are loaded
with `scripts/apply-cloudflare.sh`, which asks for an API token allowed to
edit the zone's transform rules and configuration rules, and puts each file
into its phase. The token reaches `curl` on its standard input, not in its
arguments, which other processes on the machine can read. A `PUT` replaces every rule in that phase, which is the point:
the files are the reference, and a rule added in the dashboard that is not in
them is drift. Run it again after any change to them.

#### Cloudflare must not rewrite the pages

Several Cloudflare features edit the HTML on its way through, and on a site
whose CSP allows only its own scripts each one is at best blocked and at worst
breaks the page. On 2026-10-12 three did:

- **Email Address Obfuscation** replaced the security contact in every page's
  footer with a link reading `[email protected]`, plus a script to decode it,
  which the CSP blocked: the page showed no way to report a vulnerability.
- **Web Analytics** added a beacon from `static.cloudflareinsights.com`, a
  third-party script on the link page.
- **JavaScript Detections** added an inline script loading
  `/cdn-cgi/challenge-platform/scripts/jsd/main.js`.

The first two are off, in the dashboard and in `config-settings.json`, which
also keeps Rocket Loader and Zaraz off. Bot Fight Mode is off too: it cannot
be set by a rule, and it challenges visitors on its own judgement.

JavaScript Detections cannot be turned off on the Free plan: it stays on with
Bot Fight Mode off and its own toggle off, a limitation others report too. It
is accepted. Its inline script carries parameters that change on every
response, so no hash or nonce could allow it, and allowing it would take
`'unsafe-inline'`, which would undo the policy. Blocked, it never runs; the
cost is an error in the console. The Pro plan makes it optional.

`scripts/check-live-site.sh`, run by `site-live.yml` after each deploy and
weekly, fetches the pages and scripts from usechirp.app and fails unless they
match the repository byte for byte, with that one snippet as the only
exception, and unless every header in `response-headers.json` comes back. It
also requires `www.usechirp.app` to answer every path with a 301 to the same
path on `usechirp.app`, carrying the same headers, which is why both rules
match both hosts: a page served on `www` would sit outside the check. A
setting changed in the dashboard shows up there within a week.

To move the domain again, the order matters, and some steps can take the site
down if skipped. Two cases:

**Another registrar, Cloudflare still the DNS provider.** The nameservers and
the keys do not change, so DNSSEC can stay on. Carry the nameservers and the
DS `2371 13 2 25CF…AF23` over with the transfer, and afterwards check that the
parent still publishes both (the commands in steps 1 and 5 below).

**Another DNS provider.** The keys change, so the old DS has to be gone before
the new provider answers:

1. **DNSSEC off, measured, before the nameservers change.** Read the DS's TTL
   from the parent itself, not from a resolver's cache, *before* removing it:
   `dig DS usechirp.app @ns-tld1.charlestonroadregistry.com +norec`. Turn
   DNSSEC off at the registrar. Wait until the parent no longer returns the
   DS, and neither do several independent resolvers (`@1.1.1.1`, `@8.8.8.8`,
   `@9.9.9.9`), then wait that TTL again from the parent's removal before
   changing nameservers. Do not assume an hour: it was one on 2026-10-12, and
   the parent sets it.
2. Create every record at the new provider. Before changing the nameservers,
   read the NS TTL from the parent too, in the authority section of
   `dig NS usechirp.app @ns-tld1.charlestonroadregistry.com +norec`: it is how
   long resolvers keep asking the old provider. Change the nameservers, and
   leave the old provider's records in place for at least that TTL after the
   parent shows the new ones. It was three hours on 2026-10-12; the parent
   sets it.
3. **Anything in front of Pages that terminates TLS** (Cloudflare's proxy, or
   another provider's equivalent) goes on only once its certificate for
   `usechirp.app` is active, and checks GitHub's certificate on the way back
   (on Cloudflare, **Full (strict)**). On 2026-10-12 Cloudflare's proxy was on
   before its edge certificate was issued, and for a while HTTPS through it
   failed its handshake. `.app` is HTTPS-only, so there was no plain-HTTP
   fallback: the site and every link page were down for anyone whose resolver
   had picked up the new nameservers. Test it before turning it on:
   `curl -sI --resolve usechirp.app:443:<edge address> https://usechirp.app/`.
4. **The headers belong to that layer.** A provider without one, or without
   response-header rules, serves the pages with their `<meta>` policy only:
   everything but `frame-ancestors` and the other headers above. Port
   `.cloudflare/rulesets/response-headers.json` to the new provider, moving
   the path `scripts/check-site.sh` reads with it, or record here that they
   are gone.
5. **DNSSEC on, keys first.** Turn signing on at the new provider and wait
   until *every* one of its authoritative servers returns the DNSKEY whose
   digest is the DS the provider gives: `dig DNSKEY usechirp.app @<server>`
   for each, and compare the key tag and digest the KSK (flags 257) gives
   with `dnssec-dsfromkey -2`, from BIND's tools. Only then publish the DS at the
   registrar, check that the parent returns it, and that several independent
   resolvers answer `dig A usechirp.app +dnssec` with the `ad` flag.

GitHub renews its certificate through the proxy. If a renewal fails, Pages
says so in its settings; switch the records to *DNS only* until it is renewed,
then back.

## When a gate fails

Fix the finding. The escape hatches, in order of preference:

1. Fix the code. Almost always the right answer on a codebase this size.
2. Suppress narrowly, in code, with a comment naming *why it is not exploitable*
   — `//nolint:` or `//#nosec` on the specific line, never a file or a rule
   globally.
3. Change the gate. This is a review decision and gets its own pull request, not
   a line buried in a feature branch. The `ALLOWED` list in
   `scripts/gen-third-party-notices.sh` is the model: an entry there must name
   the obligation that was checked.

Never disable a gate to get a green build. A red pipeline is information; a
green one bought by deleting the check is a lie that outlives whoever told it.

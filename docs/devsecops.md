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
| `ci.yml` | push and PR to `develop`/`main`, tags | DCO, licence gate, build, vet, cross-compile, lint, `go test -race`, gosec, govulncheck |
| `codeql.yml` | push and PR, weekly | CodeQL `security-extended` for Go |
| `secret-scan.yml` | push and PR, weekly | gitleaks over the PR commit range, or the tree and full history |
| `dependency-review.yml` | PR | what the PR adds: advisories at `low` and above, denied licences |
| `sbom.yml` | release tags | SPDX SBOM attached to the release |

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

2. **Branch protection on `develop` and `main`** (Settings → Rules → Rulesets, or
   Branches → Add rule):
   - Require a pull request before merging; require 1 approval.
   - Dismiss stale approvals when new commits are pushed.
   - Require review from Code Owners (this is what makes
     [`.github/CODEOWNERS`](../.github/CODEOWNERS) a gate rather than a
     suggestion).
   - Require status checks to pass, and require branches to be up to date:
     `Build, Test & SAST`, `Analyze (Go)`, `gitleaks`, and on `develop`,
     `Review dependency changes`.
   - Require signed commits.
   - Require linear history.
   - Block force pushes and deletions; apply the rule to administrators.
   - On `main`, additionally restrict who may push, so it can only be reached
     through a pull request from `develop`.

3. **Code security** (Settings → Advanced Security):
   - Secret scanning **and** push protection. gitleaks in CI catches a secret
     before merge; push protection catches it before it ever reaches GitHub.
   - Private vulnerability reporting, so [SECURITY.md](../SECURITY.md) has a
     mechanism behind it.
   - Dependabot alerts and security updates.

4. **Actions** (Settings → Actions → General):
   - Workflow permissions: read-only by default.
   - Require approval for all outside collaborators' workflow runs.

5. **Tag protection** for `v*.*.*`, so a release tag cannot be moved after an
   SBOM has been published against it.

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

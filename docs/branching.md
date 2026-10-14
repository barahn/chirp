# Branches, protection and merge flow

## Model

Chirp follows **Trunk-Based Development**.

`main` is the primary trunk: every working branch starts from it, and every pull
request targets it. `main` is the repository's default branch.

```
feature/*  fix/*  deps/*  ──(rebase)──▶  main  ──(tag)──▶  vX.Y.Z
```

Work lands on `main` via pull request with linear history. Releases are tagged
points (`vX.Y.Z`) on `main` — CI runs on those tags, and `sbom.yml` attaches an
SBOM to the release.

Branch names: `feature/<topic>`, `fix/<topic>`, `deps/<topic>`, `docs/<topic>`.

There is no `hotfix/*` branch. A released version gets no backports
([SECURITY.md](../SECURITY.md#supported-versions)), so an urgent fix is a
`fix/*` pull request like any other, shipped as the next patch tag.

## What the protection guarantees

`main` accepts neither a direct push, a force-push nor deletion. The rules are
versioned in [`.github/rulesets/`](../.github/rulesets/) and are the reference:
anything configured on GitHub that is not there is drift.

| Rule | `main` |
| --- | --- |
| Direct push | blocked |
| Force-push / deletion | blocked |
| Pull request required | yes |
| Approvals | 0 — see below |
| Approval dismissed on each push | yes |
| Review threads resolved | yes |
| Merge method | rebase |
| Linear history | yes |
| Branch up to date with base before merge | yes |
| Signed commits | not yet — see below |

Required checks: `Build, Test & SAST` (build, vet, lint, `-race` tests, DCO,
third-party licences, gosec, govulncheck), `gitleaks`, `Analyze (Go)` (CodeQL),
`Review dependency changes`, and `Relay image & deploy files` (the relay's image
builds and serves, its deploy scripts pass shellcheck, its Quadlet units generate;
[relay.md](relay.md)). CodeQL and dependency review run only while the
`CODE_SCANNING` variable is set — see [devsecops.md](devsecops.md). Where it is
not set they are skipped, and a skipped required check counts as passing, so they
enforce nothing there.

`bypass_actors` is empty on all rulesets, on purpose: the rules bind administrators
too.

### Why linear history and rebase are enforced on `main`

Linear history keeps `git log` readable and bisectable. Pull requests are merged
using **rebase**, ensuring that each commit lands with its verified DCO
`Signed-off-by` trailer intact and without merge bubbles. Squash merges are
disallowed because GitHub's automated squash commit can bypass or drop the
individual DCO attestations.

Signed commits are not required yet, and a DCO `Signed-off-by` trailer is not a
cryptographic signature. Signing keys alone will not be enough to require them:
GitHub cannot sign the commits of a rebase merge — it rewrites each one and has
no key of the author's — so with rebase as the only merge method every commit
lands on `main` unsigned, which is exactly what `required_signatures` refuses.
Turning it on is a merge-method decision as well as a keys one, to be weighed
against the reason squash is off above.

## The approval policy, as decided

**One maintainer cannot approve their own pull requests**, so requiring approvals
on a single-maintainer repository would block every merge unless bypass is granted.
GitHub's bypass lifts *every* rule in the ruleset for that actor (including status
checks), which is unacceptable.

The maintainer's decision:
- **`main`: no approval required, and no bypass.** A pull request is still the
  only way in, and every other rule — the required checks, linear history, up to
  date with base, review threads resolved — binds the maintainer like anyone else.
- When a second maintainer joins with write access, raise
  `required_approving_review_count` to 1.

Nothing pushes directly to a protected branch. The `HANDOFF.md` snapshot is
checked in CI instead of being committed by a bot, so no bot needs a bypass.

### Before trunk-based development

Up to v0.3.0 (2026-10-14) there were two long-lived branches: `develop`, for
integration, with no approval required; and `main`, for releases, reached
by a merge commit from `develop` and requiring a code owner's approval from
someone other than the last pusher. That approval was the only thing the second
branch bought, and with one maintainer it was never a second person: the one
release made under the rule, v0.3.0 (barahn/chirp#97), was a pull request
opened by a workflow and approved by the maintainer. An earlier version of this
document had rejected exactly that, since it records the author reviewing their
own code as if it were a second pair of eyes. Meanwhile the model cost a
workflow to keep the release pull request open, a cherry-pick back to `develop`
for every hotfix, and a `develop` that reported itself behind `main` after each
release.

`main`'s history up to v0.3.0 keeps those merge commits; linear history applies
to what lands after it.

### Release tags

A release tag, `v*`, cannot be deleted, moved or force-pushed once it exists,
by anyone: `release-tags-protected.json` has no bypass. The SBOM attached to a
release describes the commit its tag points at, and a tag that could move would
leave the SBOM describing something else. Creating a tag is not restricted, so
tagging a release needs nothing new.

## Applying the rules

In the UI: **Settings → Rules → Rulesets → New ruleset → Import a ruleset**,
once per file in `.github/rulesets/`.

Through the API (needs a token with `administration:write`):

```sh
gh api -X POST repos/barahn/chirp/rulesets --input .github/rulesets/main-protected.json
gh api -X POST repos/barahn/chirp/rulesets --input .github/rulesets/release-tags-protected.json
```

To update a ruleset that already exists, `PUT` it by id —
`gh api repos/barahn/chirp/rulesets --jq '.[] | "\(.id) \(.name)"'` lists them:

```sh
gh api -X PUT repos/barahn/chirp/rulesets/24844665 --input .github/rulesets/main-protected.json
```

Any ruleset on GitHub that is not one of these files is drift: compare with
`gh api repos/barahn/chirp/rulesets --jq '.[].name'`, and remove one by id with
`gh api -X DELETE repos/barahn/chirp/rulesets/<id>`.

To ensure `main` is the default branch:

```sh
gh repo edit barahn/chirp --default-branch main
```

## Working flow

```sh
git switch main && git pull
git switch -c feature/consent-expiry

# ... changes ...
git commit -s          # -s is required: CI checks the DCO trailer

git push -u origin feature/consent-expiry
# open the pull request against main:
gh pr create --base main
```

To release: tag the release commit on `main` and push the tag:

```sh
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

The security posture behind all of this — which gates run, why they overlap,
and the repository settings that cannot live in a file — is in
[devsecops.md](devsecops.md).

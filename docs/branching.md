# Branches, protection and merge flow

## Model

`develop` is the integration branch: every working branch starts from it, and
every pull request targets it. Making it the repository's default branch is a
setting, listed under [Applying the rules](#applying-the-rules).

`main` is the release branch. It receives only merges from `develop`, or a
`hotfix/*`, and each merge into `main` is a release point tagged `vX.Y.Z` — CI
already recognises those tags.

```
feature/*  fix/*  deps/*  ──▶  develop  ──▶  main  ──▶  tag vX.Y.Z
                                   ▲                      │
                                   └──── hotfix/* ────────┘
```

A `hotfix/*` starts from `main`, reaches `main` by pull request, and returns to
`develop` the same day **by cherry-pick** — `develop` keeps a linear history,
so a merge back is refused. Skip that step and the fix is lost at the next
release.

Branch names: `feature/<topic>`, `fix/<topic>`, `hotfix/<topic>`,
`deps/<topic>`, `docs/<topic>`.

## What the protection guarantees

Neither branch accepts a direct push, a force-push or deletion. The rules are
versioned in [`.github/rulesets/`](../.github/rulesets/) and are the reference:
anything configured on GitHub that is not there is drift.

| Rule | `main` | `develop` |
| --- | --- | --- |
| Direct push | blocked | blocked |
| Force-push / deletion | blocked | blocked |
| Pull request required | yes | yes |
| Approvals | 1, from a code owner | 1 — the admin may bypass, via pull request only |
| Approval dismissed on each push | yes | yes |
| Last push needs another person's approval | yes | no |
| Review threads resolved | yes | yes |
| Merge method | merge commit | squash or rebase |
| Linear history | no | yes |
| Branch up to date with base before merge | no | yes |
| Signed commits | not yet — see below | no |

Required checks, on both: `Build, Test & SAST` (build, vet, lint, `-race`
tests, DCO, third-party licences, gosec, govulncheck), `gitleaks`,
`Analyze (Go)` (CodeQL) and `Review dependency changes`. The last two run
only while the `CODE_SCANNING` variable is set, as it is here — see
[devsecops.md](devsecops.md). Where it is not set they are skipped, and a
skipped required check counts as passing, so they enforce nothing there.

On `main`, `bypass_actors` is empty on purpose: the rules bind administrators
too. On `develop` the repository admin may bypass them — see below.

### Why the rules differ between the branches

The first version of these rulesets could not have worked, and the reasons
are worth keeping:

- **Linear history is off on `main`.** Releases are merge commits, so that
  `main`'s history points at the real commits rather than squashed copies.
  Linear history forbids merge commits, and a ruleset allowing only
  `merge` while requiring linear history admits nothing at all.
- **"Up to date before merge" is off on `main`.** After a release, `main` holds
  a merge commit `develop` does not contain. Requiring the next release's head
  to contain it means merging `main` into `develop` — a merge commit, which
  `develop`'s linear history refuses. From the second release on, nothing could
  ship. On `develop` the rule stays: feature branches are not protected, so
  they can be rebased onto `develop` freely.
- **`develop` allows squash and rebase, not merge.** A merge button that linear
  history always refuses is not an option, it is a trap. Rebase is there so a
  multi-commit branch can land with its commits intact.
- **Signed commits are not required yet.** Every commit here is unsigned
  except the merge commits GitHub makes, and a DCO `Signed-off-by` trailer is
  not a signature. With merge-only releases, requiring signatures on `main`
  would refuse every release pull request. Turn `required_signatures` on once
  every author — including the coding agents that work in this repository —
  has a signing key.

## The approval policy, as decided

**One maintainer cannot approve their own pull requests**, so both rulesets as
first written would have blocked every merge. The maintainer's decision:

- **`develop`: the repository admin may bypass, via pull request only.** In
  `develop-protected.json` that is `bypass_actors` with `actor_type`
  `RepositoryRole`, `actor_id` `5` and `bypass_mode` `pull_request`. The admin
  can merge a pull request into `develop` without a second approval, but cannot
  push to it directly. Role `5` is the repository admin role as far as this
  file's author knows; **after importing, confirm in the UI that the bypass
  list names "Repository admin"**, since the numeric role ids were not checked
  against the API.
- **`main`: no bypass.** A release still needs a second person with write
  access to approve it.

What the bypass costs, stated so it is a known trade: GitHub's bypass lifts
*every* rule in the ruleset for that actor, not only the approval. On `develop`
the admin can therefore merge with a failing check too. The checks still run and
still show red; merging past them is a choice the admin makes in the open, not
one the rules make impossible.

Nothing else in the repository pushes to a protected branch. The `HANDOFF.md`
snapshot used to be committed to `main` by a workflow; it is now checked in CI
instead, so no bot needs a bypass.

## Applying the rules

In the UI: **Settings → Rules → Rulesets → New ruleset → Import a ruleset**,
once per file in `.github/rulesets/`.

Through the API (needs a token with `administration:write`):

```sh
gh api -X POST repos/barahn/chirp/rulesets --input .github/rulesets/main-protected.json
gh api -X POST repos/barahn/chirp/rulesets --input .github/rulesets/develop-protected.json
```

To make `develop` the default branch:

```sh
gh api -X PATCH repos/barahn/chirp -f default_branch=develop
```

## Working flow

```sh
git switch develop && git pull
git switch -c feature/consent-expiry

# ... changes ...
git commit -s          # -s is required: CI checks the DCO trailer

git push -u origin feature/consent-expiry
# open the pull request against develop
```

To release: a pull request from `develop` to `main`, merged with a merge commit,
and a `vX.Y.Z` tag on that merge.

The security posture behind all of this — which gates run, why they overlap,
and the repository settings that cannot live in a file — is in
[devsecops.md](devsecops.md).

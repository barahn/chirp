# Releasing and verifying Chirp

A release is a tag on `main` (`docs/branching.md`) and a GitHub Release
written by the maintainer. Publishing the release starts
[`release.yml`](../.github/workflows/release.yml), which builds the binaries
and attaches them to it (barahn/chirp#69). [`sbom.yml`](../.github/workflows/sbom.yml)
attaches the SBOM.

## Versions

Every repository of the Barahn organisation versions the same way. remotekit's
`docs/release.md` says the same for a library.

- **Semantic Versioning, `vMAJOR.MINOR.PATCH`**, always with the `v`: MAJOR
  for a breaking change, MINOR for something new that breaks nothing, PATCH
  for a fix. Before 1.0 a breaking change may go in a MINOR.
- **Pre-releases** are `vX.Y.Z-rc.N` (and `-alpha.N` or `-beta.N` when one
  is wanted earlier). They are published as GitHub pre-releases, which
  `releases/latest` never serves.
- **Tags are annotated and never move.** The *release tags protected*
  ruleset enforces it. A wrong tag is answered with a new version. Deleting
  one needs the maintainer's explicit confirmation and a note of what it
  breaks.
- A major version from 2 on changes the module path to `github.com/barahn/chirp/vN`
  and every import with it.

**A release validates a set of changes, not a pull request.** Merging to
`main` does not make a version. Each version is a milestone (*v0.5*, *v0.6*,
…) with one theme, and is tagged only when the whole milestone is done and
its release candidate has passed the end-to-end checks below. remotekit is
tagged the same way: only once the Chirp release candidate that uses it has
passed. Until then Chirp's `go.mod` pins a commit of remotekit's `develop`.

## Making one

1. **Propose it.** From the commits since the last tag (`feat`, `fix`, and
   `feat!`/`BREAKING CHANGE` for breaking ones; `docs`, `chore`, `refactor`,
   `test` and `ci` alone change no version), write down:
   - the proposed version and why it is a MAJOR, MINOR or PATCH;
   - the changes it carries;
   - their impact: breaking changes, new dependencies, migration;
   - the exact commands.

   Then ask the maintainer: *"Posso criar e publicar a tag vX.Y.Z?"*
   Nothing is tagged without that yes.
2. **Check `main` at the commit to release.** It must be green in CI, and
   locally clean:

   ```sh
   go mod tidy && git diff --exit-code go.mod go.sum
   go build ./... && go vet ./... && go test ./...
   ```

   Every file the release changes (notices, docs) goes in through a pull
   request first: `main` takes no direct commits.
3. **Tag a release candidate:** `git tag -a vX.Y.Z-rc.1 <commit> -m "Chirp
   vX.Y.Z-rc.1"`, push the tag, then `gh release create vX.Y.Z-rc.1
   --verify-tag --prerelease` with its notes. `release.yml` builds and attaches
   the binaries as for a final release.
4. **Run the end-to-end checks** with the candidate's own binaries, in the
   VMs and on at least one real machine of each supported system. Use one
   machine as the owner and one as the controller, then swap:
   - code and link, the consent prompt and each switch, Decline, timeout;
   - screen, mouse and keyboard, End from either side, closing either window;
   - every feature of the milestone, both ways where it goes both ways;
   - a relay of one's own, and the public network.

   A failure is fixed through a pull request, and the next candidate is
   `-rc.2`. A version number is never spent on a broken candidate.
5. **Tag the version** at the commit the last candidate was built from:
   `git tag -a vX.Y.Z <commit>`, push it, and
   `gh release create vX.Y.Z --verify-tag` with the notes.
6. `release.yml` attaches the files below in about ten minutes. If it fails,
   fix the cause and run it again from the Actions tab with the tag. It
   replaces what it uploaded.

The release notes are the changelog: new, fixed, security, dependencies, and
what upgrading needs.

## Road to 1.0

1.0 is a promise that Chirp works for the people it is for, so it is tagged
only when all of these hold:

- **The core works on both sides on every supported system:** screen,
  control, clipboard, files, the tray and its alerts, with the decisions
  behind them recorded in `docs/product-model.md`.
- **The open decisions are settled:** administrator rights (#117), what
  Chirp writes to disk (#49), and macOS's permissions (#51) if macOS is in
  1.0, which the platform list (#132) decides.
- **The binaries are signed** (#69).
- **No security issue of high severity or above is open**, in Chirp or in
  remotekit.
- **No flaky test in CI**, in either repository (barahn/remotekit#135).
- **A public beta (v0.9)** ran for some weeks with outside users and left
  no serious bug open.
- **Basic accessibility** (#84) and the **help centre** (#83) are published.

The milestones are on GitHub: *v0.5* (automatic clipboard, alerts and tray,
files both ways), *v0.6* (transfer manager, files through the clipboard,
monitor picker, the bird), *v0.7* (signing, accessibility, code expiry,
sound), *v0.9* (public beta, fixes only) and *v1.0*.

## What a release carries

| File | What |
| --- | --- |
| `chirp-windows-amd64.exe` | Chirp for Windows 10 and 11, 64-bit. One file, no installer. |
| `chirp-linux-amd64` | Chirp for 64-bit Linux with glibc 2.35 or newer (Ubuntu 22.04, Debian 12 and later). It needs `libGL` and `libwayland-client`, which every desktop has. |
| `chirp-relay-linux-amd64`, `chirp-relay-linux-arm64` | The relay, static, for a server (`docs/relay.md`). |
| `chirp-notices.zip` | `LICENSE`, `THIRD_PARTY_NOTICES.md` and the fonts' OFL texts: the notices the binaries' third-party code and fonts require. |
| `SHA256SUMS`, `SHA256SUMS.sigstore.json` | Checksums of every file above, and their Sigstore signature. |
| `chirp-sbom.spdx.json` | The SBOM, from `sbom.yml`. |

The names carry no version, so
`https://github.com/barahn/chirp/releases/latest/download/chirp-windows-amd64.exe`
always serves the newest one.

There is no macOS build yet: remotekit cannot capture the screen or inject
input there (barahn/remotekit#72). The binaries are not code-signed:
Authenticode and Apple notarization need certificates Fabrintek does not hold
yet. Windows SmartScreen therefore warns on the first run ("Windows protected
your PC"): *More info*, then *Run anyway*.

## Checking a download

Each check proves something different. The checksum proves the file is the
one listed. The signature proves the list came from this repository's release
workflow, at this tag. The attestation proves that workflow built the file.

```sh
# The file matches the published checksums.
sha256sum --check --ignore-missing SHA256SUMS

# The checksums were signed by release.yml, for a v* tag of barahn/chirp.
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/barahn/chirp/\.github/workflows/release\.yml@refs/tags/v'

# The binary was built by that workflow from this repository.
gh attestation verify chirp-linux-amd64 --repo barahn/chirp
```

On Windows, `Get-FileHash chirp-windows-amd64.exe` prints the SHA-256 to
compare with `SHA256SUMS`.

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

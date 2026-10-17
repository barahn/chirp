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
| `chirp-linux-amd64` | Chirp for 64-bit Linux with glibc 2.35 or newer (Ubuntu 22.04, Debian 12 and later). It needs `libGL` and `libwayland-client`, which every desktop has, and on Wayland GStreamer with its PipeWire plugin (`gstreamer1.0-tools gstreamer1.0-pipewire` on Debian and Ubuntu) to share the screen. |
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

## Platforms and distribution

Where Chirp runs, how its binaries reach people, and what comes next.
barahn/chirp#132 tracks the work.

### The rule

Chirp is one program, the same for everyone (product-model decision 4), on
every platform. A platform may ship with less than the full program first,
for example a Chirp that can help but cannot be helped. That build is an
exception. It stays under observation, its download says what it cannot do,
and it ends when the platform reaches the full program.

### Supported systems

| Tier | System | Notes |
| --- | --- | --- |
| 1 | Windows 10 and 11, x64 | Windows 10 is out of Microsoft's support, but it is often the machine that needs help. It needs OpenGL 2.1 (barahn/chirp#107). |
| 1 | Linux x64, GNOME or KDE, X11 or Wayland | On Wayland, the screen goes through the portal and GStreamer (above). Input needs Mutter's RemoteDesktop or the RemoteDesktop portal. |
| 2 | Windows on ARM | The x64 build runs under emulation. A native arm64 build is planned. |
| 2 | Linux arm64 | Planned, on the same glibc 2.35 baseline. |
| 2 | Linux on wlroots (Sway, Hyprland) | No RemoteDesktop portal, so a helper can see the screen but not control it. |
| 3 | macOS | Waits for barahn/remotekit#72. A helper-only build before then would be an exception under the rule above. |
| — | Windows 7 and 8, 32-bit systems | Not supported: Go no longer builds for them. |

### Signing, for now without paid certificates

- **Windows:** the plan is SignPath Foundation's free code signing for
  open-source projects, if Chirp qualifies under its dual licensing
  ([LICENSING.md](../LICENSING.md)). Until then, Chirp stays unsigned and the
  download page explains SmartScreen. Each release's `.exe` is also submitted
  to Microsoft's malware analysis as a remote-support tool, because unsigned
  remote-control tools are often flagged as potentially unwanted. The binaries
  are never packed (UPX or similar), which makes such flags more likely.
- **macOS:** a `.app` in a `.zip`, with a stable bundle identifier, since
  Screen Recording and Accessibility attach to the app's identity
  (barahn/chirp#51). Without the Apple Developer Program it carries only an
  ad-hoc signature. Gatekeeper then needs *Privacy & Security* → *Open
  Anyway*, and the permissions may have to be granted again after each
  update.
- **Everywhere:** `SHA256SUMS`, its Sigstore signature and the build
  attestations ([Checking a download](#checking-a-download)).

### Channels

GitHub Releases is the source of every binary, and the site links
`releases/latest/download/<name>`. Once the Windows build is signed, winget
and Scoop can carry it as a portable package: no installer, nothing left
behind. On Linux, Flathub may follow. Chirp ships no `.deb`, `.rpm` or
installer, because it is not installed. It does not update itself either. At
most, it says that a newer version exists, using the version the relay
reports.

### Mobile

At first, a phone or tablet only helps: it controls someone else's
computer, and is not controlled. Being helped on Android needs the
AccessibilityService for input, which Google keeps restricting because
scammers abuse it. iOS cannot be controlled at all.

- **Android first.** Fyne builds for Android, pion runs there, and
  remotekit's VP8 decoder is pure Go. It ships through F-Droid and Google
  Play.
- **iOS later.** The App Store cannot carry AGPL code, so Fabrintek, as sole
  copyright holder, would publish it there under the commercial licence. The
  source stays AGPL.
- **No web helper.** In a browser, the PAKE would run in JavaScript the
  server sends, and a compromised server could swap it. That breaks
  decision 5.

A helper-only mobile Chirp is an exception under the rule above.

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

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
   fix the cause and run it again with the tag, from the tag:
   `gh workflow run release.yml --ref vX.Y.Z -f tag=vX.Y.Z`. The macOS job
   signs in the `release` environment, which only `v*` tags may use, so a
   run from a branch stops there. It replaces what it uploaded.

The release notes are the changelog: new, fixed, security, dependencies, and
what upgrading needs.

## Road to 1.0

1.0 is a promise that Chirp works for the people it is for, so it is tagged
only when all of these hold:

- **The core works on both sides on every supported system:** screen,
  control, clipboard, files, the tray and its alerts, with the decisions
  behind them recorded in `docs/product-model.md`.
- **The open decisions are settled:** administrator rights (#117), what
  Chirp writes to disk (#49), and macOS's permissions (#51), since macOS is
  in 1.0 (#132).
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
| `chirp-macos-universal.zip` | `Chirp.app` for macOS 12.3 or newer, Intel and Apple silicon in one bundle, signed with Fabrintek's self-signed certificate ([below](#chirpapp-and-its-certificate)). |
| `chirp-linux-amd64` | Chirp for 64-bit Linux with glibc 2.35 or newer (Ubuntu 22.04, Debian 12 and later). It needs `libGL` and `libwayland-client`, which every desktop has, and on Wayland GStreamer with its PipeWire plugin (`gstreamer1.0-tools gstreamer1.0-pipewire` on Debian and Ubuntu) to share the screen. |
| `chirp-relay-linux-amd64`, `chirp-relay-linux-arm64` | The relay, static, for a server (`docs/relay.md`). |
| `chirp-notices.zip` | `LICENSE`, `THIRD_PARTY_NOTICES.md` and the fonts' OFL texts: the notices the binaries' third-party code and fonts require. |
| `SHA256SUMS`, `SHA256SUMS.sigstore.json` | Checksums of every file above, and their Sigstore signature. |
| `chirp-sbom.spdx.json` | The SBOM, from `sbom.yml`. |

The names carry no version, so
`https://github.com/barahn/chirp/releases/latest/download/chirp-windows-amd64.exe`
always serves the newest one.

Until remotekit captures the screen and injects input on macOS
(barahn/remotekit#72), the macOS build can help but cannot be helped. The
Windows binary is not code-signed: Authenticode needs a certificate Fabrintek
does not hold yet. Windows SmartScreen therefore warns on the first run
("Windows protected your PC"): *More info*, then *Run anyway*. On macOS,
Gatekeeper asks for *Open Anyway* ([below](#chirpapp-and-its-certificate)).

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
| 1 from 1.0 | macOS 12.3 and newer, Intel and Apple silicon | In 1.0, but waits for barahn/remotekit#72 to capture the screen and inject input. Until then the build is helper-only, an exception under the rule above. 12.3 is what ScreenCaptureKit needs. |
| — | Windows 7 and 8, 32-bit systems | Not supported: Go no longer builds for them. |

### Signing and antivirus

Chirp is never installed and is not in any store, so what matters is that the
downloaded binary runs without antivirus, SmartScreen or Gatekeeper getting in
the way. Windows decides by reputation. An unsigned file's reputation belongs
to its hash and starts from zero with every release. A signed file's
reputation builds up on the certificate. Remote-control tools also fall under
"RemoteAdmin" and potentially-unwanted heuristics, which metadata, a signature
and a clean history all weigh against. macOS lets an app open without asking
only if it carries a Developer ID signature and is notarized.

This is the plan, in order. barahn/chirp#69 tracks what is done.

**First, at no cost:**

- **Windows metadata:** `chirp.exe` carries a version resource (company,
  product, version), its icon and an application manifest. A binary without
  them looks more suspicious to heuristics.
- **Never packed:** no UPX or similar, since packing makes flags more likely.
- **Microsoft submission:** every release's `.exe` is submitted to Microsoft
  Security Intelligence as a software developer, describing Chirp as a
  remote-support tool.
- **Scans and false positives:** every release is scanned on VirusTotal, as a
  report and never a gate. A detection goes to that vendor's false-positive
  form, with the same account of what Chirp does: consent each time, no
  silent mode, nothing installed or left running.
- **macOS:** a `.app` in a `.zip`, bundle ID `app.usechirp.chirp`, signed
  with Fabrintek's self-signed code-signing certificate:
  - **Why not ad-hoc:** macOS gives Screen Recording and Accessibility to the
    app's identity (barahn/chirp#51). With the self-signed certificate that
    identity is the bundle ID plus the certificate, so the grants survive
    updates and an impostor with the same bundle ID gets none. An ad-hoc
    signature loses the grants on every update. An identifier-only
    requirement would hand them to any app that claims the bundle ID.
  - **The key:** it is a release secret, kept only in the release
    environment. Replacing it makes every user grant the permissions again.
    How it was made and is kept: [Chirp.app and its certificate](#chirpapp-and-its-certificate).
  - **Gatekeeper:** it still rejects the app, so every new download needs
    *Privacy & Security* → *Open Anyway*, and the download page shows how.
- **Everywhere:** `SHA256SUMS`, its Sigstore signature and the build
  attestations ([Checking a download](#checking-a-download)). Vendors can use
  them to check where a binary came from.

**At no cost, if accepted:** SignPath Foundation signs open-source projects
for free. Its published conditions exclude commercial dual licensing, which
Chirp has ([LICENSING.md](../LICENSING.md)), so this needs asking them
directly. If accepted, Windows would show SignPath Foundation as the
publisher, not Fabrintek.

**When there is a budget:**

- **Windows:** Authenticode through Azure Artifact Signing, the cheapest
  managed option, with a monthly plan. Fabrintek's eligibility has to be
  confirmed. The alternative is an OV certificate on a hardware token.
  Reputation then builds up on the certificate across releases.
- **macOS:** the Apple Developer Program as an organization, at US$ 99 a
  year with a free D-U-N-S number, for Developer ID and notarization in the
  release workflow. There is no free route: Apple waives the fee only for
  non-profits, schools and governments.
  - **The switch:** moving from the self-signed certificate to Developer ID
    changes the app's identity, so users grant Screen Recording and
    Accessibility once more. That release's notes must say so.

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

## Chirp.app and its certificate

`chirp-macos-universal.zip` holds `Chirp.app`, bundle ID
`app.usechirp.chirp`, signed with a self-signed code-signing certificate
that Fabrintek holds. macOS grants Screen Recording and Accessibility to
the app's *designated requirement* (barahn/chirp#51). That requirement
decides whether a permission survives an update, and who else could use it:

| Signature | Designated requirement | After an update | Another app with the same bundle ID |
| --- | --- | --- | --- |
| ad-hoc | `cdhash H"…"` | asked again | — |
| ad-hoc, requirement set to the identifier | `identifier "app.usechirp.chirp"` | kept | gets the permissions too |
| **self-signed certificate (Chirp's)** | `identifier "app.usechirp.chirp" and certificate leaf = H"…"` | **kept** | **denied** |

`release.yml` builds the app (`scripts/macapp`: Info.plist with the
version from the tag, `CFBundleLocalizations` `en` and `pt-BR`, the icon),
signs it with `scripts/sign-macos-app.sh`, and fails unless
`codesign --verify --strict` passes and the designated requirement names
exactly the certificate below. CI runs the same steps on every pull
request with a throwaway certificate.

Gatekeeper still refuses the app, because only the Apple Developer Program
($99 a year) can notarize. The first time, and again with each new
download, macOS says it could not verify Chirp: close the warning, then
*System Settings* → *Privacy & Security* → *Open Anyway*. The permissions
stay.

Check a download's signature on a Mac:

```sh
codesign --verify --strict --verbose=2 Chirp.app
codesign -d -r- Chirp.app
```

The second line must print
`designated => identifier "app.usechirp.chirp" and certificate leaf = H"<fingerprint>"`
with the fingerprint below.

**The certificate.** Common name *Chirp Release Signing*, organisation
Fabrintek Engenharia Digital Ltda, RSA 3072, valid from 2026-10-20 to
2056-10-20. SHA-1 fingerprint, as `codesign` prints it:

```
e451a456fe4e7d4e6e95e2f440c57fccac854bd7
```

It was made once, on 2026-10-20, with

```sh
scripts/gen-macos-signing-cert.sh /path/on/a/trusted/machine
```

and is kept like this:

- the `.p12`, base64-encoded, is the secret `MACOS_SIGNING_P12`, and its
  password `MACOS_SIGNING_P12_PASSWORD`, both only in the repository's
  `release` environment, which only `v*` tags may deploy to;
- the fingerprint is that environment's variable `MACOS_SIGNING_SHA1`, and
  is published here;
- one offline copy of the `.p12` and its password, held by the maintainer.
  Nothing else keeps the key.

On the runner, the `.p12` goes into a keychain created for the job and
deleted after it. Replacing the certificate, because it leaked or was lost,
means every Mac asks for both permissions again: say so in that release's
notes, and update the fingerprint here and in the environment.

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

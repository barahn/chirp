# Fonts of the Warm look

These files are **not Fabrintek's**, and neither the AGPL nor Fabrintek's
commercial terms cover them. Each one is under the SIL Open Font License 1.1,
which the maintainer accepted for Chirp in barahn/chirp#54, and stays under it
whichever licence Chirp is used under. The OFL allows that: the fonts may be
bundled with any software, provided their notices go with them and they are
not sold by themselves. `scripts/gen-third-party-notices.sh` gates every font
here one by one and reproduces each licence in full in the
`THIRD_PARTY_NOTICES.md` it generates; that file ships in every release's
`chirp-notices.zip`, which is how the notices reach the people the OFL means
them for.

| File | Family, weight | Licence text | Copyright |
| --- | --- | --- | --- |
| `NunitoSans-Regular.ttf` | Nunito Sans 400 | `OFL-NunitoSans.txt` | 2016 The Nunito Sans Project Authors |
| `NunitoSans-Bold.ttf` | Nunito Sans 700 | `OFL-NunitoSans.txt` | 2016 The Nunito Sans Project Authors |
| `Fredoka-SemiBold.ttf` | Fredoka 600 | `OFL-Fredoka.txt` | 2016 The Fredoka Project Authors |
| `JetBrainsMono-Medium.ttf` | JetBrains Mono 500 | `OFL-JetBrainsMono.txt` | 2020 The JetBrains Mono Project Authors |

## Where they came from

Downloaded on 2026-10-09:

- **The fonts:** static TrueType instances served by Google Fonts for the
  families it publishes under the OFL. The instances are the Latin subset,
  which covers Portuguese and the punctuation Chirp's text uses. The URLs came
  from `https://fonts.googleapis.com/css?family=<family>:<weight>`.
- **The licence texts:** the `OFL.txt` of each family in
  [google/fonts](https://github.com/google/fonts), under `ofl/nunitosans`,
  `ofl/fredoka` and `ofl/jetbrainsmono`.

| File | Source | SHA-256 |
| --- | --- | --- |
| `NunitoSans-Regular.ttf` | `fonts.gstatic.com/s/nunitosans/v19/…1ilXs1Ug.ttf` | `d70a94d2e2c6dc494e8c3c7edaf9f1e210ae2a9dc1df10246ad363d7044da20e` |
| `NunitoSans-Bold.ttf` | `fonts.gstatic.com/s/nunitosans/v19/…GMS5Xs1Ug.ttf` | `7869e9bfdf433762c066204985e2952eb4972b330fc988fb69dbc9c263df3761` |
| `Fredoka-SemiBold.ttf` | `fonts.gstatic.com/s/fredoka/v17/…OLyX8EemP.ttf` | `29bea9015db7cc42c7ca741b5a59ed6afe192377595b969c1caab572ff832965` |
| `JetBrainsMono-Medium.ttf` | `fonts.gstatic.com/s/jetbrainsmono/v24/…qxTOlOQ.ttf` | `3ac5668a41457fd1ef59788d1dc02331725bed7027f2b69c8a90b09d03a58571` |

None of the three licences names a Reserved Font Name. Chirp does not modify
the fonts; a modified one would stay under the OFL.

To replace a font, put the new file here, update the tables above, and run
`scripts/gen-third-party-notices.sh`. An embedded font without an
`EMBEDDED_FONTS` entry, or with a licence that is neither permissive nor in
`ALLOWED`, fails the build.

#!/usr/bin/env bash
#
# Assembles Chirp's website, served at usechirp.app, into _site/ (or the
# directory given). The pages live in site/; the fonts and the bird icon are
# the ones cmd/chirp embeds, copied in here rather than kept twice, so the
# site and the app cannot drift apart.
#
# The pages load nothing from another origin: their Content-Security-Policy
# allows 'self' only, which keeps any script but the site's own off the link
# page, where the token sits in the fragment.
#
# Preview: scripts/build-site.sh && python3 -m http.server -d _site 8000
#
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root/_site}"
warm="$root/cmd/chirp/internal/warm"

rm -rf "$out"
cp -R "$root/site" "$out"
mkdir -p "$out/assets/fonts"
cp "$warm"/fonts/*.ttf "$warm"/fonts/OFL-*.txt "$out/assets/fonts/"
cp "$warm/bird/icon.svg" "$out/favicon.svg"

echo "Site assembled in $out"

#!/usr/bin/env bash
#
# Assembles Chirp's website, served at usechirp.app, into _site/ (or the
# directory given). The pages live in site/; the fonts and the bird icon are
# the ones cmd/chirp embeds, copied in here rather than kept twice, so the
# site and the app cannot drift apart.
#
# The pages load nothing from another origin: their Content-Security-Policy
# allows 'self' only, which keeps any script but the site's own off the link
# page, where the token sits in the fragment. It also requires Trusted Types
# and allows no policy, so even the site's own script cannot parse a string
# as HTML: site.js builds its translations into nodes instead. Cloudflare,
# in front of GitHub Pages, sends the same policy as a header, with
# frame-ancestors added; scripts/check-site.sh keeps the two equal.
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

# Cloudflare tells browsers to keep /assets/ for hours, under the same names
# from one deploy to the next, while the pages expire in minutes. A page from
# a new deploy could then show an old screenshot, or run an old site.js that
# lacks its new translations. Each reference a page makes to an asset, or to
# the favicon, which browsers keep longer still, carries a hash of that file's
# content, so a changed file gets a new URL and an unchanged one stays cached.
for page in "$out"/index.html "$out"/l/index.html "$out"/status/index.html; do
  for ref in $(grep -oE '"/(assets/[^"?]*|favicon\.svg)"' "$page" | tr -d '"' | sort -u); do
    file="$out$ref"
    [ -f "$file" ] || { echo "$page refers to $ref, which is not in the site" >&2; exit 1; }
    hash="$(sha256sum "$file" | cut -c1-12)"
    sed -i "s#\"$ref\"#\"$ref?v=$hash\"#g" "$page"
  done
done

echo "Site assembled in $out"

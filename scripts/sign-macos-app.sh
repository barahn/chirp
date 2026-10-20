#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
#
# Signs Chirp.app with the release's self-signed certificate and checks the
# result (docs/release.md, Signing). release.yml runs it on the macOS runner.
#
# Usage: scripts/sign-macos-app.sh APP P12 P12_PASSWORD_FILE CERT_SHA1
#
# The .p12 goes into a keychain of its own, created here and deleted on
# exit, so the key is never in the login keychain. CERT_SHA1 is the
# certificate's fingerprint from gen-macos-signing-cert.sh: signing by hash
# fails if the .p12 holds any other certificate, and the designated
# requirement must then name exactly that one.
set -euo pipefail

app=${1:?usage: $0 APP P12 P12_PASSWORD_FILE CERT_SHA1}
p12=${2:?}
passfile=${3:?}
sha1=$(printf '%s' "${4:?}" | tr -d ': ' | tr '[:upper:]' '[:lower:]')
id=app.usechirp.chirp

work=$(mktemp -d)
kc="$work/chirp-signing.keychain-db"
# codesign finds identities only in keychains on the search list, so the
# job's keychain joins it for the run, and the list is put back after.
saved=()
while IFS= read -r k; do
  k=${k#"${k%%[![:space:]]*}"}; k=${k#\"}; k=${k%\"}
  [ -n "$k" ] && saved+=("$k")
done < <(security list-keychains -d user)
cleanup() {
  security list-keychains -d user -s "${saved[@]}" 2>/dev/null || true
  security delete-keychain "$kc" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

kcpass=$(openssl rand -hex 24)
security create-keychain -p "$kcpass" "$kc"
security set-keychain-settings -lut 900 "$kc"
security unlock-keychain -p "$kcpass" "$kc"
security import "$p12" -k "$kc" -f pkcs12 -P "$(cat "$passfile")" -T /usr/bin/codesign >/dev/null
# Lets codesign use the key without the dialog nobody on a runner can answer.
security set-key-partition-list -S apple-tool:,apple: -s -k "$kcpass" "$kc" >/dev/null
security list-keychains -d user -s "$kc" "${saved[@]}"

# The designated requirement is set rather than left to codesign, which
# writes "certificate root" for some self-signed certificates and "leaf" for
# others. Both name the one certificate; "leaf" is what barahn/chirp#51
# tested against TCC.
want="identifier \"$id\" and certificate leaf = H\"$sha1\""
# No --options runtime: the hardened runtime matters only for notarization,
# which a self-signed certificate cannot have.
codesign --force --timestamp=none --keychain "$kc" --sign "$sha1" \
  --identifier "$id" --requirements "=designated => $want" "$app"

codesign --verify --strict --verbose=2 "$app"
dr=$(codesign -d -r- "$app" 2>&1 | sed -n 's/^designated => //p')
echo "designated => $dr"
if [ "$dr" != "$want" ]; then
  echo "designated requirement is not: $want" >&2
  exit 1
fi
codesign -dvv "$app" 2>&1 | grep -E '^(Identifier|Authority|Format|TeamIdentifier)='

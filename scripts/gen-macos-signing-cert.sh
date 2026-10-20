#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
#
# Creates the self-signed code-signing certificate that signs Chirp.app
# (docs/release.md, Signing). Run once, on a trusted machine. macOS keys
# Screen Recording and Accessibility to the bundle ID and this certificate,
# so replacing it makes every Mac ask for both again.
#
# Usage: scripts/gen-macos-signing-cert.sh OUTDIR [COMMON_NAME]
#
# OUTDIR must not exist. It receives:
#   chirp-signing.p12       key and certificate, for the MACOS_SIGNING_P12 secret
#   chirp-signing.p12.pass  its password, for MACOS_SIGNING_P12_PASSWORD
#   chirp-signing.pem       the certificate alone (public)
# and the last line printed is the certificate's SHA-1, for release.yml's
# MACOS_SIGNING_SHA1. Store the two secrets, keep an offline copy of the .p12
# and its password, then delete OUTDIR.
set -euo pipefail

out=${1:?usage: $0 OUTDIR [COMMON_NAME]}
cn=${2:-Chirp Release Signing}
[ -e "$out" ] && { echo "$out already exists" >&2; exit 1; }

umask 077
mkdir -p "$out"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/cs.cnf" <<EOF
[req]
distinguished_name=dn
x509_extensions=ext
prompt=no
[dn]
CN=$cn
O=Fabrintek Engenharia Digital Ltda
[ext]
basicConstraints=critical,CA:false
keyUsage=critical,digitalSignature
extendedKeyUsage=critical,codeSigning
subjectKeyIdentifier=hash
EOF

# 30 years: the certificate outlives Chirp's support of any Mac, so no Mac
# is ever asked for its permissions again because the certificate expired.
openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 10958 \
  -config "$tmp/cs.cnf" -keyout "$tmp/key.pem" -out "$out/chirp-signing.pem" 2>/dev/null

openssl rand -base64 32 | tr -d '\n' > "$out/chirp-signing.p12.pass"

# macOS's `security import` reads only the older PKCS#12 algorithms, not
# OpenSSL 3's defaults (AES and PBKDF2).
openssl pkcs12 -export -name "$cn" \
  -keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1 \
  -inkey "$tmp/key.pem" -in "$out/chirp-signing.pem" \
  -passout file:"$out/chirp-signing.p12.pass" -out "$out/chirp-signing.p12"

openssl x509 -in "$out/chirp-signing.pem" -noout -subject -enddate >&2
openssl x509 -in "$out/chirp-signing.pem" -noout -fingerprint -sha1 | cut -d= -f2 | tr -d ':'

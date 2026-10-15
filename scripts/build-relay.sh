#!/usr/bin/env bash
#
# Builds the Chirp relay for Linux and packs it with what deploy/ needs to
# install it, into one archive to copy to the server:
#
#   bin/chirp-relay-<version>-linux-<arch>.tar.gz
#     chirp-relay-<version>-linux-<arch>/
#       chirp-relay                  the static binary
#       install.sh                   deploy/install.sh
#       systemd/, caddy/, proxmox/,  from deploy/
#       proxy/
#
# The relay needs no cgo, so it cross-compiles from any machine with Go.
#
# Usage: scripts/build-relay.sh [amd64|arm64 ...]   (default: amd64)
#
# docs/relay.md shows where each piece goes.
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

ARCHES=("$@")
[ ${#ARCHES[@]} -gt 0 ] || ARCHES=(amd64)

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"

for arch in "${ARCHES[@]}"; do
	case "$arch" in
	amd64 | arm64) ;;
	*) echo "error: unsupported architecture: $arch (amd64 or arm64)" >&2; exit 2 ;;
	esac

	name="chirp-relay-$VERSION-linux-$arch"
	stage="bin/$name"
	rm -rf "$stage"
	mkdir -p "$stage"

	CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
		go build -trimpath -ldflags='-s -w' -o "$stage/chirp-relay" ./cmd/chirp-relay

	cp deploy/install.sh "$stage/"
	cp -r deploy/systemd deploy/caddy deploy/proxmox deploy/proxy "$stage/"

	tar -C bin -czf "bin/$name.tar.gz" "$name"
	rm -rf "$stage"
	echo "bin/$name.tar.gz"
done

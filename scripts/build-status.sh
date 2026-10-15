#!/usr/bin/env bash
#
# Builds the status service for Linux and packs it with what it needs to be
# installed, into one archive to copy to the machine that runs it:
#
#   bin/chirp-status-<version>-linux-<arch>.tar.gz
#     chirp-status-<version>-linux-<arch>/
#       chirp-status            the static binary
#       install.sh              deploy/status/install.sh
#       chirp-status.service    deploy/status/
#       status.env              deploy/status/
#       servers.json            network/servers.json
#
# Usage: scripts/build-status.sh [amd64|arm64 ...]   (default: amd64)
#
# docs/status-service.md shows where it goes.
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

	name="chirp-status-$VERSION-linux-$arch"
	stage="bin/$name"
	rm -rf "$stage"
	mkdir -p "$stage"

	CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
		go build -trimpath -ldflags='-s -w' -o "$stage/chirp-status" ./cmd/chirp-status

	cp deploy/status/install.sh deploy/status/chirp-status.service deploy/status/status.env "$stage/"
	cp network/servers.json "$stage/"

	tar -C bin -czf "bin/$name.tar.gz" "$name"
	rm -rf "$stage"
	echo "bin/$name.tar.gz"
done

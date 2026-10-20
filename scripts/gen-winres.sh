#!/usr/bin/env bash
#
# Builds cmd/chirp/rsrc_windows_amd64.syso: chirp.exe's version information,
# icon and application manifest, from cmd/chirp/winres/winres.json. go build
# links a .syso named *_windows_amd64 into the Windows binary only, so Linux
# and macOS builds never see it.
#
# An unsigned remote-control tool with no publisher, no description and no
# icon looks like what antivirus heuristics call RemoteAdmin or PUA; this is
# what lets them, SmartScreen and Explorer's Properties -> Details see what
# chirp.exe is. cmd/chirp/winres/winres_test.go reads it all back.
#
#   scripts/gen-winres.sh            # the committed copy, version 0.0.0-dev
#   scripts/gen-winres.sh v1.2.3     # what release.yml builds for tag v1.2.3
#
# CI regenerates the committed copy and fails if it differs, so edit
# winres.json or the app icon, run this, and commit the result. A release
# stamps its tag over the committed copy on the runner and commits nothing.
set -euo pipefail

cd "$(dirname "$0")/.."

version="${1:-0.0.0-dev}"
version="${version#v}" # tags are v1.2.3; Windows shows 1.2.3

# The icon is drawn from the same SVG the window and the tray use.
go run ./cmd/chirp/winres/icon cmd/chirp/winres/icon.png

go tool go-winres make \
	--in cmd/chirp/winres/winres.json \
	--out cmd/chirp/rsrc \
	--arch amd64 \
	--product-version "$version" \
	--file-version "$version"

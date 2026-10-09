#!/usr/bin/env bash
#
# Generates THIRD_PARTY_NOTICES.md from the modules Chirp depends on, plus any
# third-party sources vendored under the repository.
#
# Chirp is AGPL-3.0-only, so copyleft dependencies would be license-compatible
# with it. This gate rejects them anyway, and the reason is not compatibility:
#
#   Chirp is dual-licensed. Fabrintek relicenses it for the proprietary Barahn
#   platform, which it may do because it owns all of Chirp. It owns none of its
#   dependencies. A GPL or AGPL dependency would therefore be code Fabrintek
#   cannot pass on under those separate terms -- and unlike a compatibility
#   problem, nothing would fail: the build works, the tests pass, and Chirp's
#   own license identifier does not change.
#
# So every dependency must carry a permissive license (MIT, BSD, ISC,
# Apache-2.0, Zlib). Copyleft (GPL, LGPL, AGPL) and the file-level copyleft of
# MPL-2.0 are reported as review items, not silently accepted. See LICENSING.md.
#
# Usage: scripts/gen-third-party-notices.sh [output-file]
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="${1:-THIRD_PARTY_NOTICES.md}"
PLATFORMS=(linux windows darwin)

# Vendored third-party source trees: <path>|<name>|<upstream URL>|<license file>
#
# Empty, and it should stay that way. Chirp consumes code as modules; a
# vendored tree here is third-party source inside a repository whose whole
# licensing position depends on Fabrintek owning what is in it.
VENDORED=()

# Non-permissive licenses that have been reviewed and accepted, as
# <module path>|<expected SPDX>|<justification>. The SPDX is part of the key:
# if upstream relicenses, the entry stops matching and the component returns to
# the review list rather than staying silently approved.
#
# An entry here is a legal decision, not a lint suppression. Do not add one
# without reading what the license obliges Chirp to do.
ALLOWED=(
	# Empty. Chirp has no copyleft dependencies and should acquire none; see
	# the header for why compatibility with the AGPL is not the test here.
	#
	# An entry added later must name the specific obligation that was checked,
	# not just assert that the license is acceptable.
)

die() {
	echo "error: $*" >&2
	exit 1
}

# Classify a license file into an SPDX identifier. Order matters: the
# distinctive phrase of each license is tested before the boilerplate
# warranty disclaimer that several of them share.
spdx_of() {
	local text
	text="$(tr -d '\r' <"$1")"

	case "$text" in
	*"Apache License"*"Version 2.0"*) echo "Apache-2.0" ;;
	*"Mozilla Public License"*"2.0"*) echo "MPL-2.0" ;;
	*"GNU LESSER GENERAL PUBLIC LICENSE"*) echo "LGPL" ;;
	*"GNU AFFERO GENERAL PUBLIC LICENSE"*) echo "AGPL" ;;
	*"GNU GENERAL PUBLIC LICENSE"*) echo "GPL" ;;
	*"Permission to use, copy, modify, and/or distribute this software"*) echo "ISC" ;;
	*"zlib License"* | *"altered source versions must be plainly marked"*) echo "Zlib" ;;
	*"Redistribution and use in source and binary forms"*)
		case "$text" in
		*"Neither the name"* | *"names of its contributors"*) echo "BSD-3-Clause" ;;
		*) echo "BSD-2-Clause" ;;
		esac
		;;
	*"Permission is hereby granted, free of charge"*) echo "MIT" ;;
	*) echo "UNKNOWN" ;;
	esac
}

# True when <name>|<spdx> has a matching ALLOWED entry. Records the hit in
# allowed_hit so that entries which no longer match anything can be reported
# and removed.
declare -A allowed_hit=()
is_allowed() {
	local name="$1" spdx="$2" entry
	for entry in "${ALLOWED[@]}"; do
		if [ "${entry%%|*}" = "$name" ] && [ "$(echo "$entry" | cut -d'|' -f2)" = "$spdx" ]; then
			allowed_hit["$name"]=1
			return 0
		fi
	done
	return 1
}

find_license_file() {
	find "$1" -maxdepth 1 -type f \
		\( -iname 'LICENSE' -o -iname 'LICENSE.*' -o -iname 'LICENCE' -o -iname 'COPYING' -o -iname 'COPYING.*' \) \
		-print -quit
}

# Collect the module set across every platform we ship, so that
# GOOS-specific dependencies are never missed.
#
# cgo is on for every platform because cmd/chirp ships built with it: its
# window (Fyne) has no files at all without cgo, so listing it without cgo
# misses its dependencies and fails. go list compiles nothing, so this needs
# no C toolchain for the other platforms.
modules_file="$(mktemp)"
trap 'rm -f "$modules_file"' EXIT
for goos in "${PLATFORMS[@]}"; do
	GOOS="$goos" CGO_ENABLED=1 go list -deps -f \
		'{{if and .Module (not .Standard)}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' \
		./... 2>/dev/null
done | grep -v '^github.com/barahn/chirp|' | sort -u | grep -v '^$' >"$modules_file"

[ -s "$modules_file" ] || die "no modules resolved; is the module cache populated?"

review=()
body="$(mktemp)"
trap 'rm -f "$modules_file" "$body"' EXIT

emit() {
	local name="$1" version="$2" spdx="$3" url="$4" license_file="$5"
	{
		echo "### $name"
		echo
		echo "- Version: \`$version\`"
		echo "- License: $spdx"
		echo "- Homepage: $url"
		echo
		echo '```'
		tr -d '\r' <"$license_file"
		echo '```'
		echo
	} >>"$body"
}

count=0
while IFS='|' read -r path version dir; do
	[ -n "$path" ] || continue
	license_file="$(find_license_file "$dir")"
	[ -n "$license_file" ] || die "$path@$version: no license file found in $dir"

	spdx="$(spdx_of "$license_file")"
	case "$spdx" in
	GPL | LGPL | AGPL | MPL-2.0 | UNKNOWN)
		is_allowed "$path" "$spdx" || review+=("$path@$version -> $spdx")
		;;
	esac

	emit "$path" "$version" "$spdx" "https://$path" "$license_file"
	count=$((count + 1))
done <"$modules_file"

vendored_body="$(mktemp)"
trap 'rm -f "$modules_file" "$body" "$vendored_body"' EXIT
vendored_count=0
for entry in "${VENDORED[@]}"; do
	IFS='|' read -r vpath vname vurl vlicense <<<"$entry"
	[ -f "$vlicense" ] || die "vendored $vname: missing license file $vlicense"
	spdx="$(spdx_of "$vlicense")"
	case "$spdx" in
	GPL | LGPL | AGPL | MPL-2.0 | UNKNOWN)
		is_allowed "$vname" "$spdx" || review+=("$vname (vendored) -> $spdx")
		;;
	esac
	{
		echo "### $vname"
		echo
		echo "- Vendored at: \`$vpath\` (modified)"
		echo "- License: $spdx"
		echo "- Upstream: $vurl"
		echo
		echo '```'
		tr -d '\r' <"$vlicense"
		echo '```'
		echo
	} >>"$vendored_body"
	vendored_count=$((vendored_count + 1))
done

{
	cat <<EOF
# Third-Party Notices

Chirp incorporates the third-party components listed below. Each component
remains the property of its respective copyright holders and is distributed
under its own license, reproduced in full.

This file is generated by \`scripts/gen-third-party-notices.sh\` from the
modules Chirp depends on, resolved for $(printf '%s, ' "${PLATFORMS[@]}" | sed 's/, $//').
Do not edit it by hand.

The relay and the protocol packages are pure Go. \`cmd/chirp\` is built with
cgo for its window: it also compiles GLFW's C sources, which come inside
\`github.com/go-gl/glfw/v3.4/glfw\` under the zlib license (\`glfw/LICENSE.md\`
there), and it uses the operating system's own graphics and windowing
libraries, which are part of the system, not of Chirp.

Fyne embeds fonts and icons in every binary built with it, under licenses of
their own that are not listed here yet (barahn/chirp#54). Chirp itself is
AGPL-3.0-only; see LICENSING.md.

## Contents

- $count Go modules
- $vendored_count vendored source tree(s)

## Go modules

EOF
	cat "$body"
	if [ "$vendored_count" -gt 0 ]; then
		echo "## Vendored source"
		echo
		cat "$vendored_body"
	fi
} >"$OUT"

echo "wrote $OUT ($count modules, $vendored_count vendored)"

status=0

if [ "${#review[@]}" -gt 0 ]; then
	{
		echo
		echo "LICENSES REQUIRING REVIEW:"
		printf '  %s\n' "${review[@]}"
		echo
		echo "Each of these is copyleft or unrecognised. Read the license, decide"
		echo "whether Chirp can carry it, and either drop the dependency or add a"
		echo "justified entry to ALLOWED in $0."
	} >&2
	status=2
fi

# A stale allowlist entry is its own problem: it means a copyleft dependency
# was dropped (so the exemption is dead weight) or renamed (so the exemption
# silently stopped protecting anything).
stale=()
for entry in "${ALLOWED[@]}"; do
	name="${entry%%|*}"
	[ -n "${allowed_hit[$name]:-}" ] || stale+=("$name")
done
if [ "${#stale[@]}" -gt 0 ]; then
	{
		echo
		echo "STALE ALLOWLIST ENTRIES (matched nothing; remove them):"
		printf '  %s\n' "${stale[@]}"
	} >&2
	status=2
fi

exit "$status"

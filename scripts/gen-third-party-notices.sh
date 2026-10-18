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
# A module's top-level license does not cover everything inside it. Fonts a
# dependency embeds in the binary (go:embed) carry licenses of their own, so
# every embedded font must match an EMBEDDED_FONTS entry below, and its own
# license is classified and gated like a module's (barahn/chirp#54).
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
	# Chirp has no copyleft code dependencies and should acquire none; see the
	# header for why compatibility with the AGPL is not the test here.
	#
	# The entries below are fonts Fyne embeds in every binary (#24, #54). The
	# maintainer accepted the SIL Open Font License 1.1 and the Bitstream Vera
	# license for them in barahn/chirp#54. Bundling them with Chirp, under the
	# AGPL or Fabrintek's commercial terms, is what both licenses allow; they
	# stay under their own licenses and are never relicensed with Chirp.
	#
	# An entry added later must name the specific obligation that was checked,
	# not just assert that the license is acceptable.
	"fyne.io/fyne/v2:theme/font/NotoSans-*.ttf|OFL-1.1|Obligations: the copyright notice and the license go with every copy (reproduced in THIRD_PARTY_NOTICES.md); the font is not sold by itself; a modified font stays under the OFL and gives up its Reserved Font Names."
	"fyne.io/fyne/v2:theme/font/InterSymbols-Regular.ttf|OFL-1.1|Obligations: as for Noto Sans."
	"github.com/barahn/chirp:cmd/chirp/internal/warm/fonts/NunitoSans-*.ttf|OFL-1.1|Chirp's Warm look (#53). Obligations: as for Noto Sans; the files sit in this repository, so fonts/README.md there also says they are not Fabrintek's."
	"github.com/barahn/chirp:cmd/chirp/internal/warm/fonts/Fredoka-*.ttf|OFL-1.1|Chirp's Warm look (#53). Obligations: as for Nunito Sans."
	"github.com/barahn/chirp:cmd/chirp/internal/warm/fonts/JetBrainsMono-*.ttf|OFL-1.1|Chirp's Warm look (#53). Obligations: as for Nunito Sans."
	"fyne.io/fyne/v2:theme/font/DejaVuSansMono-Powerline.ttf|Bitstream-Vera|Obligations: the copyright, trademark and permission notices go with every copy (reproduced in THIRD_PARTY_NOTICES.md); the font is not sold by itself; a modified font may not be named Bitstream, Vera or Arev."
)

# Fonts embedded in the binary -- by a dependency, or by Chirp itself from
# files kept in this repository -- as
# <module path>|<file glob within the module>|<name>|<license file within the module>.
# Every embedded font must match one; one that matches none is a review item,
# so a font a dependency adds later cannot ship unseen. An entry that matches
# nothing is reported as stale.
EMBEDDED_FONTS=(
	"github.com/barahn/chirp|cmd/chirp/internal/warm/fonts/NunitoSans-*.ttf|Nunito Sans|cmd/chirp/internal/warm/fonts/OFL-NunitoSans.txt"
	"github.com/barahn/chirp|cmd/chirp/internal/warm/fonts/Fredoka-*.ttf|Fredoka|cmd/chirp/internal/warm/fonts/OFL-Fredoka.txt"
	"github.com/barahn/chirp|cmd/chirp/internal/warm/fonts/JetBrainsMono-*.ttf|JetBrains Mono|cmd/chirp/internal/warm/fonts/OFL-JetBrainsMono.txt"
	"fyne.io/fyne/v2|theme/font/NotoSans-*.ttf|Noto Sans|theme/font/LICENSE.txt"
	"fyne.io/fyne/v2|theme/font/InterSymbols-Regular.ttf|Inter Symbols|theme/font/LICENSE_Inter.txt"
	"fyne.io/fyne/v2|theme/font/DejaVuSansMono-Powerline.ttf|DejaVu Sans Mono|theme/font/LICENSE_DejaVu-Powerline.txt"
	"fyne.io/fyne/v2|theme/font/EmojiOneColor.otf|EmojiOne Color|theme/font/LICENSE_EmojiOneColor.txt"
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
	*"SIL OPEN FONT LICENSE"*"1.1"* | *"SIL Open Font License"*"1.1"*) echo "OFL-1.1" ;;
	# Before MIT: the Bitstream Vera grant opens with MIT's own phrase.
	*"Bitstream Vera Fonts Copyright"*) echo "Bitstream-Vera" ;;
	*"Apache License"*"Version 2.0"*) echo "Apache-2.0" ;;
	*"Mozilla Public License"*"2.0"*) echo "MPL-2.0" ;;
	*"GNU LESSER GENERAL PUBLIC LICENSE"*) echo "LGPL" ;;
	*"GNU AFFERO GENERAL PUBLIC LICENSE"*) echo "AGPL" ;;
	*"GNU GENERAL PUBLIC LICENSE"*) echo "GPL" ;;
	*"Permission to use, copy, modify, and/or distribute this software"* | *"Permission to use, copy, modify, and distribute this software"*) echo "ISC" ;;
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

# True for the licenses Chirp accepts without review.
is_permissive() {
	case "$1" in
	MIT | BSD-2-Clause | BSD-3-Clause | ISC | Apache-2.0 | Zlib) return 0 ;;
	esac
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
#
# The sort is byte order (LC_ALL=C) so the file comes out the same on every
# machine: a locale that folds case puts github.com/BurntSushi/toml after
# github.com/anthonynsimon/bild, CI's puts it before, and the check above
# would then fail on whichever machine did not generate the file.
modules_file="$(mktemp)"
trap 'rm -f "$modules_file"' EXIT
for goos in "${PLATFORMS[@]}"; do
	GOOS="$goos" CGO_ENABLED=1 go list -deps -f \
		'{{if and .Module (not .Standard)}}{{.Module.Path}}|{{if .Module.Replace}}{{.Module.Replace.Version}}{{else}}{{.Module.Version}}{{end}}|{{.Module.Dir}}|{{if .Module.Replace}}{{.Module.Replace.Path}}{{end}}{{end}}' \
		./... 2>/dev/null
done | grep -v '^github.com/barahn/chirp|' | LC_ALL=C sort -u | grep -v '^$' >"$modules_file"

[ -s "$modules_file" ] || die "no modules resolved; is the module cache populated?"

# And every file embedded in the binary, as <module path>|<module dir>|<file>.
# Chirp's own packages are included: a font kept in this repository is still
# someone else's, under its own licence.
embeds_file="$(mktemp)"
trap 'rm -f "$modules_file" "$embeds_file"' EXIT
for goos in "${PLATFORMS[@]}"; do
	GOOS="$goos" CGO_ENABLED=1 go list -deps -f \
		'{{if and .Module (not .Standard)}}{{range .EmbedFiles}}{{$.Module.Path}}|{{$.Module.Dir}}|{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{end}}' \
		./... 2>/dev/null
done | LC_ALL=C sort -u | grep -v '^$' >"$embeds_file"

review=()
body="$(mktemp)"
trap 'rm -f "$modules_file" "$embeds_file" "$body"' EXIT
declare -A module_version=()

emit() {
	local name="$1" version="$2" spdx="$3" url="$4" license_file="$5" replaced_by="${6:-}"
	{
		echo "### $name"
		echo
		echo "- Version: \`$version\`"
		if [ -n "$replaced_by" ]; then
			# A go.mod replace: the binary carries the fork, not the original.
			echo "- Built from: \`$replaced_by\`, a modified fork of $name"
		fi
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
while IFS='|' read -r path version dir replaced_by; do
	[ -n "$path" ] || continue
	license_file="$(find_license_file "$dir")"
	[ -n "$license_file" ] || die "$path@$version: no license file found in $dir"

	spdx="$(spdx_of "$license_file")"
	case "$spdx" in
	GPL | LGPL | AGPL | MPL-2.0 | UNKNOWN)
		is_allowed "$path" "$spdx" || review+=("$path@$version -> $spdx")
		;;
	esac

	url="https://$path"
	[ -z "$replaced_by" ] || url="https://$replaced_by"
	emit "$path" "$version" "$spdx" "$url" "$license_file" "$replaced_by"
	module_version["$path"]="$version"
	count=$((count + 1))
done <"$modules_file"

# Embedded fonts: each must match an EMBEDDED_FONTS entry, and each entry's
# license is gated like a module's.
fonts_body="$(mktemp)"
trap 'rm -f "$modules_file" "$embeds_file" "$body" "$fonts_body"' EXIT
declare -A font_files=()
declare -A module_dir=()
while IFS='|' read -r mpath mdir file; do
	[ -n "$mpath" ] || continue
	module_dir["$mpath"]="$mdir"
	rel="${file#"$mdir"/}"
	case "${rel,,}" in
	*.ttf | *.otf | *.ttc | *.woff | *.woff2) ;;
	*) continue ;;
	esac
	matched=""
	for entry in "${EMBEDDED_FONTS[@]}"; do
		IFS='|' read -r fmod fglob _ _ <<<"$entry"
		# shellcheck disable=SC2053 # the glob is meant to match
		if [ "$fmod" = "$mpath" ] && [[ "$rel" == $fglob ]]; then
			font_files["$fmod|$fglob"]+="${rel##*/} "
			matched=1
			break
		fi
	done
	[ -n "$matched" ] || review+=("$mpath: $rel (embedded font) -> no EMBEDDED_FONTS entry")
done <"$embeds_file"

font_count=0
for entry in "${EMBEDDED_FONTS[@]}"; do
	IFS='|' read -r fmod fglob fname flicense <<<"$entry"
	files="${font_files["$fmod|$fglob"]:-}"
	if [ -z "$files" ]; then
		review+=("EMBEDDED_FONTS entry $fmod:$fglob matched no embedded file; remove or fix it")
		continue
	fi
	mdir="${module_dir[$fmod]}"
	[ -f "$mdir/$flicense" ] || die "$fname: license file $flicense not found in $fmod"
	spdx="$(spdx_of "$mdir/$flicense")"
	is_permissive "$spdx" || is_allowed "$fmod:$fglob" "$spdx" || review+=("$fname ($fmod:$fglob, embedded font) -> $spdx")
	read -r -a names <<<"$files"
	listed=""
	for f in "${names[@]}"; do
		listed+="\`$f\` "
	done
	{
		echo "### $fname"
		echo
		if [ -n "${module_version[$fmod]:-}" ]; then
			echo "- Embedded by: \`$fmod\` \`${module_version[$fmod]}\`"
		else
			echo "- Kept in this repository: \`${fglob%/*}/\` (see the README there)"
		fi
		echo "- Files: ${listed% }"
		echo "- License: $spdx"
		echo
		echo '```'
		tr -d '\r' <"$mdir/$flicense"
		echo '```'
		echo
	} >>"$fonts_body"
	font_count=$((font_count + 1))
done

vendored_body="$(mktemp)"
trap 'rm -f "$modules_file" "$embeds_file" "$body" "$fonts_body" "$vendored_body"' EXIT
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

Fyne embeds fonts in every binary built with it, and Chirp's Warm look
embeds three more, kept in this repository. They keep licenses of their own,
listed under *Embedded fonts*; the OFL and the Bitstream Vera license were
accepted for them in barahn/chirp#54. Fyne's icons and shaders have no
license of their own in the module and come under Fyne's. Chirp itself is
AGPL-3.0-only; see LICENSING.md.

## Contents

- $count Go modules
- $font_count embedded font(s)
- $vendored_count vendored source tree(s)

## Go modules

EOF
	cat "$body"
	if [ "$font_count" -gt 0 ]; then
		echo "## Embedded fonts"
		echo
		cat "$fonts_body"
	fi
	if [ "$vendored_count" -gt 0 ]; then
		echo "## Vendored source"
		echo
		cat "$vendored_body"
	fi
} >"$OUT"

echo "wrote $OUT ($count modules, $font_count embedded fonts, $vendored_count vendored)"

status=0

if [ "${#review[@]}" -gt 0 ]; then
	{
		echo
		echo "LICENSES REQUIRING REVIEW:"
		printf '  %s\n' "${review[@]}"
		echo
		echo "Each of these is copyleft, unrecognised, or an embedded font nobody"
		echo "has reviewed. Read the license, decide whether Chirp can carry it, and"
		echo "either drop the dependency or add a justified entry to ALLOWED (and,"
		echo "for a font, to EMBEDDED_FONTS) in $0."
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

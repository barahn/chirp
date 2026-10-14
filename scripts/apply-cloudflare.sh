#!/usr/bin/env bash
#
# Loads every ruleset in .cloudflare/rulesets/ into usechirp.app's zone, each
# into its phase. A PUT replaces every rule in that phase, which is the point:
# the files are the reference, and a rule added in the dashboard that is not in
# them is drift. Run it after any change to those files.
#
# The token needs to edit the zone's transform rules and configuration rules.
# It is read from CLOUDFLARE_API_TOKEN if set, otherwise asked for without
# echoing; it is never written anywhere.
#
# Usage: scripts/apply-cloudflare.sh
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# usechirp.app's zone. A zone ID names the zone; it grants nothing.
zone=588dc7bfe393569ce736dce02eac5795

# Each file and the phase it is the entry point of.
declare -A phase=(
	[response-headers.json]=http_response_headers_transform
	[config-settings.json]=http_config_settings
)

for f in .cloudflare/rulesets/*.json; do
	if [ -z "${phase[$(basename "$f")]:-}" ]; then
		echo "error: $f has no phase in $0" >&2
		exit 1
	fi
done

token="${CLOUDFLARE_API_TOKEN:-}"
if [ -z "$token" ]; then
	read -rsp 'Cloudflare API token: ' token
	echo
fi
# A pasted token can bring a newline or a terminal's control bytes with it.
token="$(printf '%s' "$token" | tr -d '[:space:][:cntrl:]')"
[ -n "$token" ] || { echo "error: no token" >&2; exit 1; }

fail=0
for f in .cloudflare/rulesets/*.json; do
	p="${phase[$(basename "$f")]}"
	# The token goes in on curl's standard input, never in its arguments,
	# which any process on the machine can read.
	resp="$(curl -sS -X PUT \
		"https://api.cloudflare.com/client/v4/zones/$zone/rulesets/phases/$p/entrypoint" \
		-H @- \
		-H "Content-Type: application/json" \
		--data-binary "@$f" \
		<<<"Authorization: Bearer $token")"
	if [ "$(jq -r '.success' <<<"$resp")" = "true" ]; then
		echo "$f → $p: $(jq -r '[.result.rules[].ref] | join(", ")' <<<"$resp")"
	else
		echo "error: $f → $p:" >&2
		jq '.errors' <<<"$resp" >&2
		fail=1
	fi
done
exit "$fail"

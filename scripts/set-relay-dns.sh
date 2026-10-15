#!/usr/bin/env bash
#
# Creates or updates a relay's DNS records in usechirp.app's Cloudflare zone,
# DNS only: a relay must not sit behind Cloudflare's proxy (docs/relay.md).
# Run it from a checkout of this repository, on your own machine.
#
# The token needs Zone → DNS → Edit on usechirp.app. It is read from
# CLOUDFLARE_API_TOKEN if set, otherwise asked for without echoing; it is
# never written anywhere, and reaches curl on its standard input.
#
# Usage: scripts/set-relay-dns.sh --name relay.usechirp.app --ipv4 ADDRESS [--ipv6 ADDRESS]
#
# A record type not given is left as it is. Give --ipv6 none to delete the
# AAAA record, for a relay that IPv6 does not reach.
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
set -euo pipefail

# usechirp.app's zone, as in scripts/apply-cloudflare.sh.
zone=588dc7bfe393569ce736dce02eac5795
name=""
ipv4=""
ipv6=""

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}
while [ $# -gt 0 ]; do
	case "$1" in
	--name) name="${2:?}"; shift 2 ;;
	--ipv4) ipv4="${2:?}"; shift 2 ;;
	--ipv6) ipv6="${2:?}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done
[ -n "$name" ] || usage
[ -n "$ipv4$ipv6" ] || usage
case "$name" in
usechirp.app | *.usechirp.app) ;;
*) echo "error: $name is not in usechirp.app" >&2; exit 1 ;;
esac
if [ -n "$ipv4" ] && ! [[ "$ipv4" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
	echo "error: $ipv4 is not an IPv4 address" >&2
	exit 1
fi
if [ -n "$ipv6" ] && [ "$ipv6" != none ] && ! [[ "$ipv6" =~ ^[0-9a-fA-F:]+$ && "$ipv6" == *:* ]]; then
	echo "error: $ipv6 is not an IPv6 address" >&2
	exit 1
fi

token="${CLOUDFLARE_API_TOKEN:-}"
if [ -z "$token" ]; then
	read -rsp 'Cloudflare API token: ' token
	echo
fi
token="$(printf '%s' "$token" | tr -d '[:space:][:cntrl:]')"
[ -n "$token" ] || { echo "error: no token" >&2; exit 1; }

# api METHOD PATH [BODY]: one call, with the token on curl's standard input.
api() {
	local args=(-sS -X "$1" "https://api.cloudflare.com/client/v4/zones/$zone$2" -H @- -H "Content-Type: application/json")
	[ -z "${3:-}" ] || args+=(--data-binary "$3")
	curl "${args[@]}" <<<"Authorization: Bearer $token"
}

# set_record TYPE CONTENT: create the record, replace it, or with "none"
# delete it.
set_record() {
	local type="$1" content="$2" found id resp body
	found="$(api GET "/dns_records?type=$type&name=$name")"
	[ "$(jq -r .success <<<"$found")" = true ] || { jq .errors <<<"$found" >&2; return 1; }
	id="$(jq -r '.result[0].id // empty' <<<"$found")"
	if [ "$content" = none ]; then
		[ -n "$id" ] || { echo "$type $name: none to delete"; return 0; }
		resp="$(api DELETE "/dns_records/$id")"
	else
		body="$(jq -nc --arg t "$type" --arg n "$name" --arg c "$content" \
			'{type: $t, name: $n, content: $c, ttl: 1, proxied: false, comment: "Chirp relay: DNS only, see docs/relay.md"}')"
		if [ -n "$id" ]; then
			resp="$(api PUT "/dns_records/$id" "$body")"
		else
			resp="$(api POST "/dns_records" "$body")"
		fi
	fi
	if [ "$(jq -r .success <<<"$resp")" = true ]; then
		if [ "$content" = none ]; then echo "$type $name: deleted"; else echo "$type $name → $content (DNS only)"; fi
	else
		echo "error: $type $name:" >&2
		jq .errors <<<"$resp" >&2
		return 1
	fi
}

fail=0
[ -z "$ipv4" ] || set_record A "$ipv4" || fail=1
[ -z "$ipv6" ] || set_record AAAA "$ipv6" || fail=1
exit "$fail"

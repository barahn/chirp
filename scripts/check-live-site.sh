#!/usr/bin/env bash
#
# Checks that usechirp.app serves the site as this repository has it: each page
# and script byte for byte, and the headers in
# .cloudflare/rulesets/response-headers.json. scripts/check-site.sh checks the
# source; this checks what Cloudflare, in front of GitHub Pages, actually
# sends, so a dashboard setting that starts rewriting the pages (email
# obfuscation did, on 2026-10-12, and broke the security contact) fails here.
#
# One rewrite is allowed, exactly: the inline script Cloudflare's JavaScript
# Detections inserts before </body>. On the Free plan it cannot be turned off,
# and the pages' CSP blocks it, so it never runs. Its r and t parameters change
# on every response; anything else in it, or any other change, fails.
#
# Usage: scripts/check-live-site.sh [--wait SECONDS]
#
#   SITE_URL  the site to check, https://usechirp.app by default.
#
#   --wait  retry until the check passes or SECONDS have gone by, for a run
#           right after a deploy, while GitHub Pages' cache still serves the
#           previous version.
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
base="${SITE_URL:-https://usechirp.app}"
wait=0
if [ "${1:-}" = "--wait" ]; then wait="${2:?--wait needs seconds}"; fi

# The JavaScript Detections snippet, with r and t emptied.
jsd="<script>(function(){function c(){var b=a.contentDocument||(a.contentWindow&&a.contentWindow.document);if(b){var d=b.createElement('script');d.innerHTML=\"window.__CF\$cv\$params={r:'',t:''};var a=document.createElement('script');a.src='/cdn-cgi/challenge-platform/scripts/jsd/main.js';document.getElementsByTagName('head')[0].appendChild(a);\";b.getElementsByTagName('head')[0].appendChild(d)}}if(document.body){var a=document.createElement('iframe');a.height=1;a.width=1;a.style.position='absolute';a.style.top=0;a.style.left=0;a.style.border='none';a.style.visibility='hidden';document.body.appendChild(a);if('loading'!==document.readyState)c();else if(window.addEventListener)document.addEventListener('DOMContentLoaded',c);else{var e=document.onreadystatechange||function(){};document.onreadystatechange=function(b){e(b);'loading'!==document.readyState&&(document.onreadystatechange=e,c())}}}})();</script>"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# check_once prints what differs and returns non-zero if anything does.
check_once() {
	local fail=0 path src got name want have html
	for path in "" "l/" "assets/site.js" "assets/site.css"; do
		src="$root/site/${path:-index.html}"
		[ "${path%/}" != "$path" ] && src="$root/site/${path}index.html"
		got="$work/body"
		if ! curl -fsS --max-time 20 -H 'Cache-Control: no-cache' \
			-D "$work/headers" -o "$got" "$base/$path?check=$RANDOM"; then
			echo "/$path: request failed"
			fail=1
			continue
		fi
		if [ "${src##*.}" = "html" ]; then
			# Empty r and t, then drop the snippet once, right before </body>.
			sed -E -i "s/__CF\\\$cv\\\$params=\\{r:'[A-Za-z0-9]*',t:'[A-Za-z0-9+\/=]*'\\}/__CF\$cv\$params={r:'',t:''}/" "$got"
			html="$(cat "$got"; printf x)"
			html="${html%x}"
			html="${html/"$jsd</body>"/</body>}"
			printf '%s' "$html" > "$got"
		fi
		if ! cmp -s "$src" "$got"; then
			echo "/$path differs from ${src#"$root"/}:"
			diff <(sed 's/></>\n</g' "$src") <(sed 's/></>\n</g' "$got") | grep '^[<>]' | cut -c1-200 | head -20 || true
			fail=1
		fi
		# Every header the ruleset sets, on every response.
		while IFS=$'\t' read -r name want; do
			have="$(grep -i "^$name:" "$work/headers" | head -1 | sed 's/^[^:]*:[[:space:]]*//' | tr -d '\r')"
			if [ "$have" != "$want" ]; then
				echo "/$path: $name is \"$have\", want \"$want\""
				fail=1
			fi
		done < <(jq -r '.rules[0].action_parameters.headers | to_entries[] | "\(.key)\t\(.value.value)"' \
			"$root/.cloudflare/rulesets/response-headers.json")
	done
	return "$fail"
}

deadline=$((SECONDS + wait))
until out="$(check_once)"; do
	if [ "$SECONDS" -ge "$deadline" ]; then
		printf '%s\n' "$out" >&2
		echo "error: $base does not serve the site as the repository has it" >&2
		exit 1
	fi
	sleep 30
done
echo "$base: pages and scripts as in the repository, headers as in the ruleset"

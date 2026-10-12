#!/usr/bin/env bash
#
# Checks the website in site/ for what the browser alone cannot be trusted
# to enforce everywhere. Run by CI on every pull request.
#
# 1. No HTML sink. Each page's Content-Security-Policy requires Trusted
#    Types with no policy, so a browser that has them refuses innerHTML and
#    the like; one that does not would run them. This keeps them out of the
#    source, whatever the browser.
#
# 2. One policy. The pages carry their CSP in a <meta> tag, because GitHub
#    Pages sets no headers; Cloudflare, in front of it, sends the same policy
#    as a header from .cloudflare/rulesets/response-headers.json, plus
#    frame-ancestors, which a <meta> policy cannot hold. Both are enforced,
#    so a page served either way keeps the same rules, as long as the two
#    stay equal.
#
# Usage: scripts/check-site.sh
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
fail=0

sinks='\.(innerHTML|outerHTML)\b|insertAdjacentHTML|document\.write|\beval\s*\(|new\s+Function\b|createContextualFragment|DOMParser|setHTMLUnsafe|parseHTMLUnsafe|srcdoc'
if grep -rnE "$sinks" "$root/site" --include='*.js' --include='*.html'; then
	echo "error: site/ uses an HTML sink (above). Build nodes instead; see site/assets/site.js." >&2
	fail=1
fi

header="$(jq -r '.rules[0].action_parameters.headers["Content-Security-Policy"].value' \
	"$root/.cloudflare/rulesets/response-headers.json")"
expected="${header%; frame-ancestors \'none\'}"
if [ "$expected" = "$header" ]; then
	echo "error: the Cloudflare CSP must end with \"; frame-ancestors 'none'\"" >&2
	fail=1
fi
while IFS= read -r page; do
	meta="$(grep -o 'http-equiv="Content-Security-Policy" content="[^"]*"' "$page" | sed 's/.*content="//; s/"$//')"
	if [ "$meta" != "$expected" ]; then
		echo "error: ${page#"$root"/}'s CSP differs from the Cloudflare header's, less frame-ancestors" >&2
		echo "  page:   $meta" >&2
		echo "  header: $expected" >&2
		fail=1
	fi
done < <(find "$root/site" -name '*.html' | sort)

if [ "$fail" -eq 0 ]; then echo "site: no HTML sink, one CSP"; fi
exit "$fail"

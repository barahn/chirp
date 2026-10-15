#!/usr/bin/env bash
#
# Checks a relay from outside, as a Chirp on the internet would reach it:
# its name, its ports, its certificate, /healthz, and the headers Caddy
# sends. Run it from a machine on another network than the relay's. From
# the relay's own LAN, a router without NAT loopback fails the port checks
# even when the relay is reachable from everywhere else.
#
# Usage: scripts/check-relay.sh [domain]     (default: relay.usechirp.app)
#
# Exit status: 0 when every check passes.
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda
set -euo pipefail

domain="${1:-relay.usechirp.app}"
fail=0
ok() { printf '  ok    %s\n' "$*"; }
bad() { printf '  FAIL  %s\n' "$*"; fail=1; }

# resolve TYPE: what Cloudflare's public resolver says, over HTTPS.
resolve() {
	curl -fsS --max-time 10 -H 'accept: application/dns-json' \
		"https://cloudflare-dns.com/dns-query?name=$domain&type=$1" 2>/dev/null |
		grep -o '"data":"[^"]*"' | sed 's/"data":"//; s/"$//' | grep -E '^[0-9a-fA-F.:]+$' || true
}

echo "$domain"
a="$(resolve A)"
aaaa="$(resolve AAAA)"
if [ -z "$a$aaaa" ]; then
	bad "no A or AAAA record: create one (scripts/set-relay-dns.sh)"
	exit 1
fi
[ -z "$a" ] || ok "A $(tr '\n' ' ' <<<"$a")"
[ -z "$aaaa" ] || ok "AAAA $(tr '\n' ' ' <<<"$aaaa")"

for port in 80 443; do
	if timeout 5 bash -c "exec 3<>/dev/tcp/$domain/$port" 2>/dev/null; then
		ok "TCP $port open"
	else
		bad "TCP $port does not answer: check the port forwarding and any firewall"
	fi
done

redirect="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 10 "http://$domain/healthz" 2>/dev/null || true)"
case "$redirect" in
30[178]\ https://"$domain"/*) ok "HTTP redirects to HTTPS" ;;
*) bad "http:// answered \"${redirect:-nothing}\", not a redirect to https://" ;;
esac

headers="$(mktemp)"
trap 'rm -f "$headers"' EXIT
if body="$(curl -fsS --max-time 10 -D "$headers" "https://$domain/healthz" 2>&1)"; then
	ok "HTTPS with a certificate this machine trusts"
	if [ "$body" = ok ]; then ok "/healthz says ok"; else bad "/healthz said \"$body\""; fi
	if grep -qi '^cf-ray:' "$headers"; then
		bad "Cloudflare's proxy is in front: make the record DNS only (docs/relay.md)"
	fi
	if grep -qi '^server:' "$headers"; then
		bad "a Server header names the software: $(grep -i '^server:' "$headers" | tr -d '\r')"
	else
		ok "no Server header"
	fi
	if grep -qi "^strict-transport-security:" "$headers"; then ok "HSTS"; else bad "no Strict-Transport-Security"; fi
else
	bad "https://$domain/healthz: $body"
fi

end="$(echo | timeout 10 openssl s_client -connect "$domain:443" -servername "$domain" 2>/dev/null |
	openssl x509 -noout -enddate 2>/dev/null | sed 's/^notAfter=//' || true)"
if [ -n "$end" ]; then
	days=$((($(date -d "$end" +%s) - $(date +%s)) / 86400))
	if [ "$days" -ge 14 ]; then ok "certificate valid for $days more days"; else bad "certificate expires in $days days"; fi
fi

exit "$fail"

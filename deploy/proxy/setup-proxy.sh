#!/usr/bin/env bash
#
# Sets up one Caddy in front of several services that share a public address,
# in a container or machine of its own, and puts the Chirp relay behind it.
# Run it as root on Debian or Ubuntu, in the proxy's container:
#
#   ./setup-proxy.sh --relay relay.example.org=10.0.0.8:8080
#   ./setup-proxy.sh --site cloud.example.org=10.0.0.20:80
#
# Run it again to add or change a site; the others are kept. It needs nothing
# from the bundle, so it can be fetched on its own.
#
# What it does:
#
#   - installs Caddy (2.7 or newer) from Caddy's own apt repository;
#   - writes /etc/caddy/Caddyfile, unless one it did not write is there: the
#     global logger that keeps request URIs and headers out of Caddy's own
#     log, and an import of every site in /etc/caddy/sites/;
#   - writes /etc/caddy/sites/<name>.caddy for each --relay and --site;
#   - checks the configuration, and reloads Caddy.
#
# A --relay site is the one in deploy/caddy/Caddyfile, with the relay on TCP
# instead of a socket: no request URIs in the log, a 64 KB body cap, and
# WebSockets kept for an hour across reloads. A --site is a plain reverse
# proxy, without an access log.
#
# The relay must then listen on that address and trust this proxy: run
# use-proxy.sh in the relay's container (docs/relay.md, "One Caddy for
# several services").
#
# Usage: setup-proxy.sh [--relay NAME=HOST:PORT]... [--site NAME=HOST:PORT]...
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

MANAGED="# Managed by Chirp's setup-proxy.sh. Sites go in /etc/caddy/sites/."
relays=()
sites=()

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}
while [ $# -gt 0 ]; do
	case "$1" in
	--relay) relays+=("${2:?}"); shift 2 ;;
	--site) sites+=("${2:?}"); shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
command -v apt-get >/dev/null || die "this needs Debian or Ubuntu"

# split NAME=HOST:PORT into $name and $upstream, refusing anything else.
split() {
	name="${1%%=*}"
	upstream="${1#*=}"
	[ "$name" != "$1" ] || die "$1: give NAME=HOST:PORT"
	case "$name" in
	*[!A-Za-z0-9.-]* | .* | *. | "") die "$name: not a host name" ;;
	esac
	[[ "$upstream" =~ ^[A-Za-z0-9.-]+:[0-9]{1,5}$ || "$upstream" =~ ^\[[0-9a-fA-F:]+\]:[0-9]{1,5}$ ]] ||
		die "$upstream: give the service as HOST:PORT"
}
for r in "${relays[@]}" "${sites[@]}"; do split "$r"; done

# --- Caddy ---------------------------------------------------------------------

caddy_version_ok() {
	# stream_close_delay needs Caddy 2.7.
	local major minor
	read -r major minor < <(caddy version 2>/dev/null | sed -n 's/^v\([0-9]*\)\.\([0-9]*\).*/\1 \2/p') || return 1
	[ "$major" -gt 2 ] || { [ "$major" -eq 2 ] && [ "$minor" -ge 7 ]; }
}
if ! caddy_version_ok; then
	log "Installing Caddy from its own apt repository"
	export DEBIAN_FRONTEND=noninteractive
	apt-get update -qq
	apt-get install -y -qq curl gpg debian-keyring debian-archive-keyring apt-transport-https >/dev/null
	curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key |
		gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
	chmod 0644 /usr/share/keyrings/caddy-stable-archive-keyring.gpg
	echo "deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main" \
		>/etc/apt/sources.list.d/caddy-stable.list
	apt-get update -qq
	apt-get install -y -qq caddy >/dev/null
	caddy_version_ok || die "Caddy $(caddy version) is older than 2.7"
fi

install -d -m 0755 /etc/caddy/sites
if [ -f /etc/caddy/Caddyfile ] && [ "$(head -n 1 /etc/caddy/Caddyfile)" != "$MANAGED" ] &&
	grep -qvE '^[[:space:]]*(#|$)' /etc/caddy/Caddyfile &&
	! grep -q 'respond "Hello, world!"\|root \* /usr/share/caddy' /etc/caddy/Caddyfile; then
	die "/etc/caddy/Caddyfile has a configuration this script did not write. Move it into
/etc/caddy/sites/, then run this again; or add the global log block and
'import /etc/caddy/sites/*.caddy' to it by hand (docs/relay.md)."
fi
cat >/etc/caddy/Caddyfile.new <<EOF
$MANAGED

# Peer IDs, session IDs and link tokens travel in the relay's request paths
# and query strings, and each is a credential. Caddy logs the request URI with
# every error, whatever a site's own log says, so the default logger drops it.
{
	log default {
		output stderr
		format filter {
			wrap console
			request>uri delete
			request>headers delete
		}
	}
}

import /etc/caddy/sites/*.caddy
EOF
mv -f /etc/caddy/Caddyfile.new /etc/caddy/Caddyfile
chmod 0644 /etc/caddy/Caddyfile

# --- Sites ---------------------------------------------------------------------

# The relay's site: deploy/caddy/Caddyfile, with the relay on TCP. Keep the
# two in step.
for r in "${relays[@]}"; do
	split "$r"
	cat >"/etc/caddy/sites/$name.caddy" <<EOF
# The Chirp relay. Written by setup-proxy.sh; see docs/relay.md.
$name {
	request_body {
		max_size 64KB
	}

	# Caddy overwrites X-Forwarded-For with the address it sees. The relay
	# believes it because CHIRP_RELAY_TRUSTED_PROXIES names this proxy.
	reverse_proxy $upstream {
		stream_close_delay 1h
	}

	header {
		-Server
		Strict-Transport-Security "max-age=31536000"
		X-Content-Type-Options "nosniff"
		Referrer-Policy "no-referrer"
	}

	# Address, method, status and timing; never the URI or the headers.
	log {
		output stderr
		format filter {
			wrap console
			request>uri delete
			request>headers delete
			resp_headers delete
		}
	}
}
EOF
	log "Site $name → relay at $upstream"
done
for s in "${sites[@]}"; do
	split "$s"
	cat >"/etc/caddy/sites/$name.caddy" <<EOF
# Written by setup-proxy.sh. Edit freely; run it again to replace.
$name {
	reverse_proxy $upstream
	header -Server
}
EOF
	log "Site $name → $upstream"
done

caddy validate --adapter caddyfile --config /etc/caddy/Caddyfile >/dev/null 2>&1 || {
	caddy validate --adapter caddyfile --config /etc/caddy/Caddyfile || true
	die "Caddy rejects the configuration above"
}
systemctl enable caddy.service >/dev/null
if systemctl is-active --quiet caddy.service; then
	systemctl reload caddy.service
else
	systemctl restart caddy.service
fi

here="$(hostname -I 2>/dev/null | awk '{ print $1 }')"
served=""
for f in /etc/caddy/sites/*.caddy; do [ -e "$f" ] && served="$served ${f##*/}"; done
log "Caddy serves:${served//.caddy/}"
log "Next:"
log "  - forward TCP 80 and 443 (and UDP 443) on the router to ${here:-this machine};"
for r in "${relays[@]}"; do
	split "$r"
	# The relay sees this proxy at the address it leaves from towards the
	# relay, which is not this machine's first one when they share a private
	# network of their own.
	host="${upstream%:*}"
	host="${host#[}"
	host="${host%]}"
	from="$(ip route get "$host" 2>/dev/null | sed -n 's/.* src \([^ ]*\).*/\1/p' | head -n 1 || true)"
	log "  - in the relay's container: ./use-proxy.sh --proxy ${from:-${here:-<this address>}} --listen $upstream"
done

#!/usr/bin/env bash
#
# Installs coturn as Chirp's STUN server on a Debian or Ubuntu machine with
# systemd, with turnserver.conf from this directory, and checks it answers.
#
# The machine needs 3478/udp and 3478/tcp reachable from the internet. Behind
# NAT, forward both to it: on Fabrintek's node that is the ONT and then the
# MikroTik, to the proxy container (docs/relay.md, STUN and TURN).
#
# Usage (as root, from a checkout or a copy of deploy/coturn):
#
#   ./install.sh [--config ./turnserver.conf]
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="$HERE/turnserver.conf"

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}
while [ $# -gt 0 ]; do
	case "$1" in
	--config) CONFIG="${2:?}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -r "$CONFIG" ] || die "no config at $CONFIG"

log "installing coturn"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq coturn >/dev/null

if [ -f /etc/turnserver.conf ] && ! cmp -s "$CONFIG" /etc/turnserver.conf; then
	backup="/etc/turnserver.conf.$(date +%Y%m%d%H%M%S)"
	log "keeping the previous config as $backup"
	cp -p /etc/turnserver.conf "$backup"
fi
install -m 0644 "$CONFIG" /etc/turnserver.conf

# Older Debian packages start nothing until this is set.
if [ -f /etc/default/coturn ]; then
	sed -i 's/^#\{0,1\}TURNSERVER_ENABLED=.*/TURNSERVER_ENABLED=1/' /etc/default/coturn
fi

log "starting coturn"
systemctl enable -q coturn
systemctl restart coturn

log "checking it answers STUN on this machine"
for _ in 1 2 3 4 5; do
	if turnutils_stunclient 127.0.0.1 >/dev/null 2>&1; then
		log "coturn answers STUN on 3478"
		log "from outside: turnutils_stunclient <public address> should print that address"
		exit 0
	fi
	sleep 1
done
journalctl -u coturn -n 20 --no-pager >&2 || true
die "coturn does not answer STUN on 127.0.0.1:3478"

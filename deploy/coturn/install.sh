#!/usr/bin/env bash
#
# Installs coturn as Chirp's STUN and TURN server on a Debian or Ubuntu machine
# with systemd, from the turnserver.conf template in this directory, and checks
# it answers.
#
# The machine needs 3478/udp and 3478/tcp, and the relay range (49160-49459/udp
# by default), reachable from the internet. Behind NAT, forward all of them to
# it through every NAT layer, and pass --external-ip (docs/relay.md, STUN and
# TURN).
#
# TURN credentials come from the relay, which must hold the same secret: copy
# the secret file to the relay's machine and point -turn-secret-file at it.
#
# Usage (as root, from a checkout or a copy of deploy/coturn):
#
#   ./install.sh --capacity-mbps N [--external-ip PUBLIC/PRIVATE]
#                [--secret-file PATH] [--ports MIN-MAX] [--config TEMPLATE]
#
#   --capacity-mbps  what the whole server may relay, in Mbit/s: about half
#                    of the uplink's measured upload floor
#   --external-ip    behind NAT: the public address and this machine's
#   --secret-file    the shared secret; created, mode 600, if it does not
#                    exist (default /etc/coturn/chirp-turn.secret)
#   --ports          the relay range (default 49160-49459)
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE="$HERE/turnserver.conf"
SECRET_FILE=/etc/coturn/chirp-turn.secret
EXTERNAL_IP=""
CAPACITY_MBPS=""
PORTS=49160-49459

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}
while [ $# -gt 0 ]; do
	case "$1" in
	--config) TEMPLATE="${2:?}"; shift 2 ;;
	--secret-file) SECRET_FILE="${2:?}"; shift 2 ;;
	--external-ip) EXTERNAL_IP="${2:?}"; shift 2 ;;
	--capacity-mbps) CAPACITY_MBPS="${2:?}"; shift 2 ;;
	--ports) PORTS="${2:?}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -r "$TEMPLATE" ] || die "no template at $TEMPLATE"
[[ "$CAPACITY_MBPS" =~ ^[1-9][0-9]*$ ]] || die "--capacity-mbps N is required: about half of the uplink's measured upload floor, in Mbit/s"
[[ "$PORTS" =~ ^([0-9]+)-([0-9]+)$ ]] || die "--ports must be MIN-MAX"
MIN_PORT="${BASH_REMATCH[1]}"
MAX_PORT="${BASH_REMATCH[2]}"
if [ "$MIN_PORT" -lt 1024 ] || [ "$MAX_PORT" -gt 65535 ] || [ "$MIN_PORT" -ge "$MAX_PORT" ]; then
	die "--ports must be a range within 1024-65535"
fi
if [ -n "$EXTERNAL_IP" ]; then
	[[ "$EXTERNAL_IP" =~ ^[0-9a-fA-F.:]+(/[0-9a-fA-F.:]+)?$ ]] || die "--external-ip must be PUBLIC or PUBLIC/PRIVATE"
fi

log "installing coturn"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq coturn >/dev/null

if [ ! -s "$SECRET_FILE" ]; then
	log "creating the shared secret in $SECRET_FILE"
	install -d -m 0755 "$(dirname "$SECRET_FILE")"
	(umask 077 && head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' >"$SECRET_FILE")
fi
chmod 0600 "$SECRET_FILE"
SECRET="$(tr -d '[:space:]' <"$SECRET_FILE")"
[ -n "$SECRET" ] || die "$SECRET_FILE is empty"
[[ "$SECRET" =~ ^[A-Za-z0-9._~+/=-]+$ ]] || die "the secret in $SECRET_FILE has characters coturn's config cannot hold"

conf="$(<"$TEMPLATE")"
conf="${conf//@SECRET@/$SECRET}"
if [ -n "$EXTERNAL_IP" ]; then
	conf="${conf//@EXTERNAL_IP@/external-ip=$EXTERNAL_IP}"
else
	conf="${conf//@EXTERNAL_IP@/}"
fi
conf="${conf//@MIN_PORT@/$MIN_PORT}"
conf="${conf//@MAX_PORT@/$MAX_PORT}"
conf="${conf//@BPS_CAPACITY@/$((CAPACITY_MBPS * 125000))}"
case "$conf" in *@*@*) die "the template has a value this script does not fill" ;; esac

if [ -f /etc/turnserver.conf ]; then
	backup="/etc/turnserver.conf.$(date +%Y%m%d%H%M%S)"
	log "keeping the previous config as $backup"
	cp -p /etc/turnserver.conf "$backup"
	chmod 0600 "$backup"
fi
# The config holds the secret: readable by root and coturn's own group only.
group=root
getent group turnserver >/dev/null && group=turnserver
(umask 077 && printf '%s\n' "$conf" >/etc/turnserver.conf.new)
chown "root:$group" /etc/turnserver.conf.new
chmod 0640 /etc/turnserver.conf.new
mv /etc/turnserver.conf.new /etc/turnserver.conf

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
		log "coturn answers STUN on 3478, and relays on $MIN_PORT-$MAX_PORT/udp"
		log "give the relay the same secret: copy $SECRET_FILE to its machine, chmod 600,"
		log "owned by the relay's user, and set CHIRP_RELAY_TURN_SECRET_FILE and CHIRP_RELAY_TURN_URLS"
		log "from outside: turnutils_stunclient <public address> should print that address"
		exit 0
	fi
	sleep 1
done
journalctl -u coturn -n 20 --no-pager >&2 || true
die "coturn does not answer STUN on 127.0.0.1:3478"

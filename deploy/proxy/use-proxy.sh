#!/usr/bin/env bash
#
# Puts a relay that install.sh set up behind a proxy on another machine: the
# relay listens on TCP instead of its socket, believes X-Forwarded-For from
# that proxy only, and the Caddy install.sh may have put beside it is turned
# off. Run it as root in the relay's container:
#
#   ./use-proxy.sh --proxy 10.0.0.9
#   ./use-proxy.sh --proxy 10.0.0.9 --listen 10.0.0.8:8080
#
# /etc/chirp-relay/relay.env is backed up first, beside itself. To go back:
# copy the backup over it, then systemctl enable --now caddy and
# systemctl restart chirp-relay.
#
# A request from anywhere but the proxy is attributed to its own address,
# whatever its headers say, so nothing on the network can choose its own
# address for the throttle. Still, let only the proxy reach the port: on the
# Proxmox node, the container's firewall (docs/relay.md).
#
# Usage: use-proxy.sh --proxy ADDRESS [--listen HOST:PORT]
#
#   --proxy ADDRESS     the proxy as the relay sees it: an address or CIDR,
#                       comma-separated if several
#   --listen HOST:PORT  where to listen (default: this machine's first IPv4
#                       address, port 8080)
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

PROXY=""
LISTEN=""
ENV=/etc/chirp-relay/relay.env

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}
while [ $# -gt 0 ]; do
	case "$1" in
	--proxy) PROXY="${2:?}"; shift 2 ;;
	--listen) LISTEN="${2:?}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -n "$PROXY" ] || usage
if [ ! -x /usr/local/bin/chirp-relay ] || [ ! -f "$ENV" ]; then
	die "no relay installed by install.sh here"
fi
[[ "$PROXY" =~ ^[0-9A-Fa-f.:/,]+$ ]] || die "--proxy: give addresses or CIDRs, comma-separated"
case "$PROXY" in
0.0.0.0/0 | ::/0 | *,0.0.0.0/0* | *,::/0*) die "--proxy must name the proxy, not everyone" ;;
esac
if [ -z "$LISTEN" ]; then
	ip4="$(hostname -I | tr ' ' '\n' | grep -E '^[0-9]+\.' | head -n 1 || true)"
	[ -n "$ip4" ] || die "no IPv4 address here: give --listen"
	LISTEN="$ip4:8080"
fi
[[ "$LISTEN" =~ ^[0-9.]+:[0-9]{1,5}$ || "$LISTEN" =~ ^\[[0-9a-fA-F:]+\]:[0-9]{1,5}$ ]] ||
	die "--listen: give HOST:PORT, such as 10.0.0.8:8080"
# The relay checks the list itself; ask it before writing anything.
out="$(CHIRP_RELAY_ADDR=127.0.0.1:0 CHIRP_RELAY_TRUSTED_PROXIES="$PROXY" timeout 2 /usr/local/bin/chirp-relay 2>&1 || true)"
if grep -q 'trusted-proxies' <<<"$out"; then
	die "the relay rejects --proxy $PROXY: $out"
fi

backup="$ENV.$(date +%Y%m%d-%H%M%S).bak"
cp -p "$ENV" "$backup"
log "Backed up $ENV to $backup"

# set_env KEY VALUE: comment out every active line for KEY, then add one.
set_env() {
	sed -i -E "s|^([[:space:]]*$1=)|#\\1|" "$ENV"
	printf '%s=%s\n' "$1" "$2" >>"$ENV"
}
{
	echo
	echo "# Behind the proxy at $PROXY, set by use-proxy.sh on $(date -I)."
} >>"$ENV"
set_env CHIRP_RELAY_ADDR "$LISTEN"
set_env CHIRP_RELAY_TRUSTED_PROXIES "$PROXY"
log "The relay listens on $LISTEN and believes X-Forwarded-For from $PROXY only"

if [ -f /etc/systemd/system/caddy.service.d/chirp-relay.conf ] && systemctl is-enabled --quiet caddy 2>/dev/null; then
	systemctl disable --now caddy >/dev/null 2>&1
	log "Turned off the Caddy install.sh set up here: the proxy does its work now"
fi

systemctl restart chirp-relay
for _ in $(seq 1 20); do
	# shellcheck source=/dev/null
	if (set -a; . "$ENV"; /usr/local/bin/chirp-relay -healthcheck) 2>/dev/null; then
		log "chirp-relay answers on $LISTEN"
		log "From the proxy, check:  curl http://$LISTEN/healthz"
		exit 0
	fi
	sleep 0.5
done
journalctl -u chirp-relay -n 20 --no-pager || true
die "chirp-relay did not answer on $LISTEN; to go back: cp $backup $ENV && systemctl restart chirp-relay"

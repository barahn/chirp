#!/usr/bin/env bash
#
# Installs or upgrades the status service (cmd/chirp-status) on a Debian or
# Ubuntu machine with systemd, from the bundle scripts/build-status.sh makes.
# Run it on a machine apart from every server it watches, or it goes down
# with them (docs/status-service.md).
#
# What it does:
#
#   - installs the binary as /usr/local/bin/chirp-status and the unit;
#   - writes /etc/chirp-status/status.env the first time only;
#   - installs servers.json, the network's list, every time: the repository
#     is its source, so an upgrade brings the list as it is there;
#   - restarts the service and checks it answers. If it does not, it puts
#     back what was there before.
#
# Usage (as root, from the unpacked bundle):
#
#   ./install.sh --binary ./chirp-status [--servers ./servers.json] [--vantage NAME]
#
#   --vantage NAME  where this machine probes from, written to status.env
#                   the first time: a city, a provider, a region
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY=""
SERVERS="$HERE/servers.json"
VANTAGE=""

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}
while [ $# -gt 0 ]; do
	case "$1" in
	--binary) BINARY="${2:?}"; shift 2 ;;
	--servers) SERVERS="${2:?}"; shift 2 ;;
	--vantage) VANTAGE="${2:?}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -n "$BINARY" ] || usage
[ -f "$BINARY" ] || die "no binary at $BINARY"
[ -f "$SERVERS" ] || die "no servers.json at $SERVERS"
[ -f "$HERE/chirp-status.service" ] || die "run from the bundle: chirp-status.service is missing"
command -v systemctl >/dev/null || die "systemd is required"
command -v curl >/dev/null || die "curl is required: apt-get install curl"
"$BINARY" -h >/dev/null 2>&1 || die "$BINARY does not run here (wrong architecture?)"
# Any letter, accented ones included; nothing that would end the quoted
# value in status.env or mean something to sed.
if [[ "$VANTAGE" == *[\"\\\$\`\|\&]* || "$VANTAGE" == *$'\n'* ]]; then
	die "--vantage: a plain name, without quotes, backslashes, \$, \`, | or &"
fi

# What to put back if the new version does not come up.
backup="$(mktemp -d)"
trap 'rm -rf "$backup"' EXIT
[ ! -f /usr/local/bin/chirp-status ] || cp -p /usr/local/bin/chirp-status "$backup/chirp-status"
[ ! -f /etc/chirp-status/servers.json ] || cp -p /etc/chirp-status/servers.json "$backup/servers.json"

log "Installing /usr/local/bin/chirp-status"
install -m 0755 "$BINARY" /usr/local/bin/chirp-status.new
mv -f /usr/local/bin/chirp-status.new /usr/local/bin/chirp-status
install -m 0644 "$HERE/chirp-status.service" /etc/systemd/system/chirp-status.service

install -d -m 0755 /etc/chirp-status
if [ -f /etc/chirp-status/status.env ]; then
	log "Keeping the existing /etc/chirp-status/status.env"
else
	install -m 0644 "$HERE/status.env" /etc/chirp-status/status.env
	[ -z "$VANTAGE" ] || sed -i "s|^CHIRP_STATUS_VANTAGE=.*|CHIRP_STATUS_VANTAGE=\"$VANTAGE\"|" /etc/chirp-status/status.env
fi
install -m 0644 "$SERVERS" /etc/chirp-status/servers.json.new
mv -f /etc/chirp-status/servers.json.new /etc/chirp-status/servers.json
log "Installed the list of $(grep -c '"id"' /etc/chirp-status/servers.json || true) servers"

systemctl daemon-reload
systemctl enable chirp-status.service >/dev/null
systemctl restart chirp-status.service

# Read, not sourced: the file is systemd's syntax, not the shell's.
addr="$(sed -n 's/^CHIRP_STATUS_ADDR=//p' /etc/chirp-status/status.env | tail -n 1 | tr -d '"')"
addr="${addr:-localhost:8081}"
log "Waiting for the status service to answer on $addr"
for _ in $(seq 1 20); do
	if curl -fsS --max-time 2 "http://$addr/healthz" >/dev/null 2>&1; then
		log "chirp-status is up. Publish $addr over TLS: docs/status-service.md"
		exit 0
	fi
	sleep 0.5
done

journalctl -u chirp-status.service -n 20 --no-pager || true
if [ -f "$backup/chirp-status" ]; then
	log "Putting back the previous binary and list"
	cp -p "$backup/chirp-status" /usr/local/bin/chirp-status
	[ ! -f "$backup/servers.json" ] || cp -p "$backup/servers.json" /etc/chirp-status/servers.json
	systemctl restart chirp-status.service || true
fi
die "chirp-status did not answer on $addr"

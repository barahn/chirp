#!/usr/bin/env bash
#
# Installs or upgrades the Chirp relay on a Debian or Ubuntu machine with
# systemd: a Proxmox LXC container (deploy/proxmox/create-ct.sh runs this
# inside the container it creates), a VM, or a bare server.
#
# What it does:
#
#   - installs the binary as /usr/local/bin/chirp-relay, and creates the
#     chirp-relay system user it runs as;
#   - installs deploy/systemd/chirp-relay.service, and writes
#     /etc/chirp-relay/relay.env the first time only;
#   - with --domain, installs Caddy from Caddy's own apt repository (the
#     distributions ship 2.6, older than the Caddyfile needs), adds it to the
#     chirp-relay group so it can open the relay's socket, and points it at
#     the relay, with a certificate for that domain;
#   - starts the relay, or restarts it on an upgrade, and checks it answers.
#
# Run it again with a new binary to upgrade; relay.env is kept.
#
# Usage (as root, from the unpacked bundle; see scripts/build-relay.sh):
#
#   ./install.sh --binary ./chirp-relay [--domain relay.example.org]
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BINARY=""
DOMAIN=""

usage() {
	sed -n '/^# Usage/,/^$/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--binary) BINARY="${2:?--binary needs a path}"; shift 2 ;;
	--domain) DOMAIN="${2:?--domain needs a name}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
[ -n "$BINARY" ] || die "--binary is required"
[ -f "$BINARY" ] || die "no binary at $BINARY"
command -v systemctl >/dev/null || die "systemd is required"
[ -f "$HERE/systemd/chirp-relay.service" ] || die "run from the bundle: $HERE/systemd/ is missing"
"$BINARY" -h >/dev/null 2>&1 || die "$BINARY does not run here (wrong architecture?)"
if [ -n "$DOMAIN" ]; then
	case "$DOMAIN" in
	*[!A-Za-z0-9.-]* | .* | *. | "") die "--domain must be a plain host name, like relay.example.org" ;;
	esac
fi

# --- The relay --------------------------------------------------------------

log "Installing /usr/local/bin/chirp-relay"
install -m 0755 "$BINARY" /usr/local/bin/chirp-relay.new
mv -f /usr/local/bin/chirp-relay.new /usr/local/bin/chirp-relay

if ! getent passwd chirp-relay >/dev/null; then
	log "Creating the chirp-relay system user"
	useradd --system --user-group --no-create-home --home-dir /nonexistent \
		--shell /usr/sbin/nologin chirp-relay
fi

log "Installing chirp-relay.service"
install -m 0644 "$HERE/systemd/chirp-relay.service" /etc/systemd/system/chirp-relay.service
install -d -m 0755 /etc/chirp-relay
if [ -f /etc/chirp-relay/relay.env ]; then
	log "Keeping the existing /etc/chirp-relay/relay.env"
else
	install -m 0644 "$HERE/systemd/relay.env" /etc/chirp-relay/relay.env
fi

systemctl daemon-reload
systemctl enable chirp-relay.service >/dev/null
# restart, not start: on an upgrade the old binary is still running.
if ! systemctl restart chirp-relay.service 2>/dev/null; then
	if journalctl -u chirp-relay.service -n 20 --no-pager 2>/dev/null | grep -q '226/NAMESPACE'; then
		die "the relay's systemd sandbox cannot create namespaces here. In a Proxmox
LXC container, turn on nesting from the Proxmox host and restart the container:
    pct set <ctid> --features nesting=1 && pct reboot <ctid>
then run this again (docs/relay.md, \"Troubleshooting\")."
	fi
	journalctl -u chirp-relay.service -n 20 --no-pager || true
	die "chirp-relay did not start"
fi

# --- Caddy -------------------------------------------------------------------

caddy_version_ok() {
	# stream_close_delay, in the Caddyfile, needs Caddy 2.7.
	local major minor
	read -r major minor < <(caddy version 2>/dev/null | sed -n 's/^v\([0-9]*\)\.\([0-9]*\).*/\1 \2/p') || return 1
	[ -n "$minor" ] || return 1
	[ "$major" -gt 2 ] || { [ "$major" -eq 2 ] && [ "$minor" -ge 7 ]; }
}

if [ -n "$DOMAIN" ]; then
	caddy_was_installed=false
	if command -v caddy >/dev/null; then
		caddy_was_installed=true
	fi

	if ! caddy_version_ok; then
		log "Installing Caddy from its own apt repository"
		export DEBIAN_FRONTEND=noninteractive
		apt-get update -qq
		apt-get install -y -qq ca-certificates curl gnupg >/dev/null
		curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key |
			gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
		chmod 0644 /usr/share/keyrings/caddy-stable-archive-keyring.gpg
		echo "deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main" \
			>/etc/apt/sources.list.d/caddy-stable.list
		apt-get update -qq
		apt-get install -y -qq caddy >/dev/null
		caddy_version_ok || die "Caddy $(caddy version) is older than 2.7"
	fi

	# The domain reaches the Caddyfile through the environment, so the file
	# itself is the repository's, unedited.
	install -d -m 0755 /etc/systemd/system/caddy.service.d
	cat >/etc/systemd/system/caddy.service.d/chirp-relay.conf <<-EOF
		# Written by Chirp's deploy/install.sh.
		[Service]
		Environment=CHIRP_RELAY_DOMAIN=$DOMAIN
	EOF

	# The relay's socket is open to its group, and only Caddy joins it.
	usermod -a -G chirp-relay caddy

	if $caddy_was_installed && ! grep -q 'CHIRP_RELAY_DOMAIN' /etc/caddy/Caddyfile 2>/dev/null; then
		# Caddy was here before us and serves something else: do not
		# replace its configuration, add our site beside it. Global options
		# cannot be imported, so the logger that keeps credentials out of
		# Caddy's error log is left to the operator.
		sed '/^# BEGIN global options$/,/^# END global options$/d' "$HERE/caddy/Caddyfile" \
			>/etc/caddy/chirp-relay.caddy
		chmod 0644 /etc/caddy/chirp-relay.caddy
		log "Caddy already has a configuration. Wrote /etc/caddy/chirp-relay.caddy;"
		log "add this line to /etc/caddy/Caddyfile, then: systemctl restart caddy"
		log "    import /etc/caddy/chirp-relay.caddy"
		log "and copy the global 'log default' block from $HERE/caddy/Caddyfile into"
		log "its global options, or Caddy's error log will record relay credentials."
		systemctl daemon-reload
	else
		install -m 0644 "$HERE/caddy/Caddyfile" /etc/caddy/Caddyfile
		CHIRP_RELAY_DOMAIN="$DOMAIN" caddy validate --adapter caddyfile --config /etc/caddy/Caddyfile >/dev/null 2>&1 ||
			die "Caddy rejects /etc/caddy/Caddyfile: run caddy validate --config /etc/caddy/Caddyfile"
		systemctl daemon-reload
		systemctl enable caddy.service >/dev/null
		systemctl restart caddy.service
	fi
fi

# --- Check -------------------------------------------------------------------

log "Waiting for the relay to answer"
for _ in $(seq 1 20); do
	# shellcheck source=/dev/null
	if (set -a; . /etc/chirp-relay/relay.env; /usr/local/bin/chirp-relay -healthcheck) 2>/dev/null; then
		log "chirp-relay is up: $(systemctl show -p ActiveState --value chirp-relay)"
		if [ -n "$DOMAIN" ]; then
			log "Once DNS for $DOMAIN points here and ports 80 and 443 reach this machine,"
			log "check it from outside:  curl https://$DOMAIN/healthz"
		fi
		exit 0
	fi
	sleep 0.5
done
systemctl --no-pager status chirp-relay.service || true
die "chirp-relay did not answer; see journalctl -u chirp-relay"

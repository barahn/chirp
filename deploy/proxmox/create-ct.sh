#!/usr/bin/env bash
#
# Creates a Proxmox VE LXC container running the Chirp relay, and Caddy in
# front of it when given a domain. Run it as root on a Proxmox VE node, from
# the unpacked bundle that scripts/build-relay.sh makes:
#
#   tar xzf chirp-relay-<version>-linux-amd64.tar.gz
#   cd chirp-relay-<version>-linux-amd64
#   ./proxmox/create-ct.sh --domain relay.example.org --ip 203.0.113.10/24 --gw 203.0.113.1
#
# The container is unprivileged, Debian 13, with 1 core, 256 MiB of memory
# and a 2 GiB disk -- the relay itself uses a few MiB. The "nesting" feature is
# on because the relay's systemd sandbox needs it (docs/relay.md).
#
# To upgrade an existing container, do not run this again: push the new
# bundle into it and run install.sh there (docs/relay.md, "Upgrading").
#
# Usage: create-ct.sh [options]
#
#   --ctid N             container ID (default: the next free one)
#   --hostname NAME      (default: chirp-relay)
#   --domain NAME        the relay's public name; installs Caddy for it.
#                        Without it, the relay listens on loopback only and
#                        you put your own TLS proxy in front (docs/relay.md)
#   --ip CIDR|dhcp       IPv4 address of eth0 (default: dhcp)
#   --gw ADDRESS         IPv4 gateway, with a static --ip
#   --ip6 CIDR|auto|dhcp IPv6 address of eth0 (default: none)
#   --bridge NAME        (default: vmbr0)
#   --vlan TAG           VLAN tag on eth0
#   --storage NAME       storage for the root disk (default: local-lvm)
#   --template-storage NAME  storage holding templates (default: local)
#   --ssh-key FILE       public key(s) to authorise for root in the container
#   --memory MIB         (default: 256)
#   --disk GIB           (default: 2). 0 on a directory storage gives the
#                        container a plain directory instead of a disk image
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

# pct exec hands the node's locale (LANG=en_US.UTF-8 on a usual Proxmox shell)
# to every command it runs in the container, where the Debian template has no
# such locale: apt and perl then print a screen of warnings each. C.UTF-8 is
# built into glibc, so it works in any container and keeps UTF-8 output.
export LC_ALL=C.UTF-8

BUNDLE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

CTID=""
HOSTNAME_="chirp-relay"
DOMAIN=""
IP="dhcp"
GW=""
IP6=""
BRIDGE="vmbr0"
VLAN=""
STORAGE="local-lvm"
TSTORAGE="local"
SSH_KEY=""
MEMORY=256
DISK=2

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--ctid) CTID="${2:?}"; shift 2 ;;
	--hostname) HOSTNAME_="${2:?}"; shift 2 ;;
	--domain) DOMAIN="${2:?}"; shift 2 ;;
	--ip) IP="${2:?}"; shift 2 ;;
	--gw) GW="${2:?}"; shift 2 ;;
	--ip6) IP6="${2:?}"; shift 2 ;;
	--bridge) BRIDGE="${2:?}"; shift 2 ;;
	--vlan) VLAN="${2:?}"; shift 2 ;;
	--storage) STORAGE="${2:?}"; shift 2 ;;
	--template-storage) TSTORAGE="${2:?}"; shift 2 ;;
	--ssh-key) SSH_KEY="${2:?}"; shift 2 ;;
	--memory) MEMORY="${2:?}"; shift 2 ;;
	--disk) DISK="${2:?}"; shift 2 ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root on a Proxmox VE node"
command -v pct >/dev/null || die "pct not found: this is not a Proxmox VE node"
[ -x "$BUNDLE/chirp-relay" ] || die "no chirp-relay binary in $BUNDLE: run this from the unpacked bundle"
[ -f "$BUNDLE/install.sh" ] || die "no install.sh in $BUNDLE"
if [ "$IP" != dhcp ] && [ -z "$GW" ]; then
	die "a static --ip needs --gw"
fi
if [ -n "$SSH_KEY" ] && [ ! -f "$SSH_KEY" ]; then
	die "no key file at $SSH_KEY"
fi

[ -n "$CTID" ] || CTID="$(pvesh get /cluster/nextid)"
if pct status "$CTID" >/dev/null 2>&1; then
	die "container $CTID already exists"
fi

# --- Template ---------------------------------------------------------------

# pveam offers templates for every architecture, and sorting by version alone
# can pick another architecture's newer build. Only this node's will boot.
ARCH="$(dpkg --print-architecture)"
log "Looking for the newest Debian 13 template for $ARCH"
pveam update >/dev/null
TEMPLATE="$(pveam available --section system | awk '{print $2}' |
	grep "^debian-13-standard_.*_${ARCH}\.tar\." | sort -V | tail -n 1)"
[ -n "$TEMPLATE" ] || die "no debian-13-standard template for $ARCH offered by pveam"
if ! pveam list "$TSTORAGE" | grep -q "$TEMPLATE"; then
	log "Downloading $TEMPLATE to $TSTORAGE"
	pveam download "$TSTORAGE" "$TEMPLATE"
fi

# --- Container --------------------------------------------------------------

net0="name=eth0,bridge=$BRIDGE,ip=$IP"
[ -z "$GW" ] || net0="$net0,gw=$GW"
[ -z "$IP6" ] || net0="$net0,ip6=$IP6"
[ -z "$VLAN" ] || net0="$net0,tag=$VLAN"

create_args=(
	"$CTID" "$TSTORAGE:vztmpl/$TEMPLATE"
	--hostname "$HOSTNAME_"
	--ostype debian
	--unprivileged 1
	--features nesting=1
	--cores 1
	--memory "$MEMORY"
	--swap "$MEMORY"
	--rootfs "$STORAGE:$DISK"
	--net0 "$net0"
	--onboot 1
	--timezone host
	--tags chirp
	--description "Chirp relay. docs/relay.md in https://github.com/barahn/chirp"
)
[ -z "$SSH_KEY" ] || create_args+=(--ssh-public-keys "$SSH_KEY")

log "Creating container $CTID ($HOSTNAME_)"
pct create "${create_args[@]}"
pct start "$CTID"

log "Waiting for the container's network"
for _ in $(seq 1 60); do
	if pct exec "$CTID" -- getent hosts deb.debian.org >/dev/null 2>&1; then
		break
	fi
	sleep 1
done
pct exec "$CTID" -- getent hosts deb.debian.org >/dev/null 2>&1 ||
	die "container $CTID cannot resolve deb.debian.org; check --ip, --gw and DNS"

log "Updating the container"
pct exec "$CTID" -- env DEBIAN_FRONTEND=noninteractive apt-get update -qq
pct exec "$CTID" -- env DEBIAN_FRONTEND=noninteractive apt-get -y -qq full-upgrade >/dev/null

# --- Relay ------------------------------------------------------------------

log "Copying the bundle into the container"
dest=/root/chirp-relay-install
pct exec "$CTID" -- mkdir -p "$dest/systemd" "$dest/caddy"
pct push "$CTID" "$BUNDLE/chirp-relay" "$dest/chirp-relay" --perms 0755
pct push "$CTID" "$BUNDLE/install.sh" "$dest/install.sh" --perms 0755
for f in systemd/chirp-relay.service systemd/relay.env caddy/Caddyfile; do
	pct push "$CTID" "$BUNDLE/$f" "$dest/$f" --perms 0644
done

install_args=(--binary "$dest/chirp-relay")
[ -z "$DOMAIN" ] || install_args+=(--domain "$DOMAIN")
pct exec "$CTID" -- "$dest/install.sh" "${install_args[@]}"

addr="$(pct exec "$CTID" -- hostname -I 2>/dev/null | awk '{print $1}')"
log "Container $CTID is running the relay at ${addr:-<no address yet>}."
if [ -n "$DOMAIN" ]; then
	log "Next: point $DOMAIN at the public address that reaches ${addr:-it}, forward"
	log "TCP 80 and 443 (and UDP 443) to it, then check: curl https://$DOMAIN/healthz"
fi

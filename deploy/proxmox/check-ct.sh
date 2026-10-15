#!/usr/bin/env bash
#
# Checks a Chirp relay container on this Proxmox VE node, and says what still
# stands between it and the internet. Run it as root on the node, from the
# unpacked bundle (it ships in proxmox/, next to create-ct.sh):
#
#   ./proxmox/check-ct.sh
#   ./proxmox/check-ct.sh --ctid 105 --domain relay.example.org
#
# It also runs inside the relay's container, or on any host install.sh set
# up, as root. There it checks the same things but those only the node can
# see: nesting, the bridge, and the Proxmox firewall.
#
# It reads, and changes nothing, unless --fix-caddy is given. It reports:
#
#   - the container, its addresses, and whether they are public;
#   - the relay and Caddy inside it, the name Caddy serves, its certificate,
#     and whether /healthz answers through Caddy from inside the container;
#   - the public address the container's traffic leaves from;
#   - whether this node NATs the container, and whether the Proxmox firewall
#     is in the way;
#   - what the relay's name resolves to, asked of Cloudflare's resolver.
#
# Then it prints what is left to do: the DNS record to create, and the ports
# to forward and where to. A router's port forwarding cannot be done from
# here; docs/relay.md, "Let traffic reach it", walks through it.
#
# Whether the relay is reachable from the internet can only be seen from
# outside: run scripts/check-relay.sh from a machine on another network.
#
# Usage: check-ct.sh [options]
#
#   --ctid N        the container (default: the one tagged chirp, or named
#                   chirp-relay, if there is exactly one)
#   --domain NAME   the relay's public name (default: the one Caddy serves)
#   --fix-caddy     install or repoint Caddy for --domain inside the
#                   container, with the installer create-ct.sh left there
#
# Exit status: 0 when nothing is wrong inside the node, 1 otherwise.
#
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

set -euo pipefail

CTID=""
DOMAIN=""
FIX_CADDY=false

usage() {
	sed -n '/^# Usage/,/^# SPDX/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//' >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--ctid) CTID="${2:?}"; shift 2 ;;
	--domain) DOMAIN="${2:?}"; shift 2 ;;
	--fix-caddy) FIX_CADDY=true; shift ;;
	-h | --help) usage ;;
	*) echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

problems=0
todo=()
ok() { printf '  ok    %s\n' "$*"; }
warn() { printf '  warn  %s\n' "$*"; }
bad() { printf '  FAIL  %s\n' "$*"; problems=$((problems + 1)); }
section() { printf '\n%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
command -v curl >/dev/null || die "curl is needed: apt-get install curl"
# On the node, commands run in the container through pct; inside the
# container, or on a host install.sh set up, they run here.
INSIDE=false
if ! command -v pct >/dev/null; then
	[ -x /usr/local/bin/chirp-relay ] || die "neither a Proxmox VE node nor a host with the relay installed"
	INSIDE=true
fi

# is_private ADDRESS: an address the internet cannot reach directly.
is_private() {
	case "$1" in
	10.* | 192.168.* | 127.* | 169.254.*) return 0 ;;
	172.1[6-9].* | 172.2[0-9].* | 172.3[01].*) return 0 ;;
	100.6[4-9].* | 100.[7-9][0-9].* | 100.1[01][0-9].* | 100.12[0-7].*) return 0 ;; # carrier-grade NAT
	fc* | fd* | fe80:*) return 0 ;;
	esac
	return 1
}

# resolve NAME TYPE: the addresses Cloudflare's public resolver gives, over
# HTTPS, so neither this node's resolver nor its cache is asked.
resolve() {
	curl -fsS --max-time 10 -H 'accept: application/dns-json' \
		"https://cloudflare-dns.com/dns-query?name=$1&type=$2" 2>/dev/null |
		grep -o '"data":"[^"]*"' | sed 's/"data":"//; s/"$//' | grep -E '^[0-9a-fA-F.:]+$' || true
}

# --- The container ------------------------------------------------------------

in_hint=""
if $INSIDE; then
	in_ct() { "$@"; }
	section "This host ($(hostname))"
	ok "running the check inside it: nesting, the bridge and the Proxmox firewall are the node's to check"
	net0="" bridge="eth0"
fi
if ! $INSIDE && [ -z "$CTID" ]; then
	found=()
	for id in $(pct list | awk 'NR > 1 { print $1 }'); do
		conf="$(pct config "$id" 2>/dev/null || true)"
		if grep -qE '^tags:.*\bchirp\b' <<<"$conf" || grep -q '^hostname: chirp-relay$' <<<"$conf"; then
			found+=("$id")
		fi
	done
	case "${#found[@]}" in
	0) die "no container tagged chirp or named chirp-relay; give --ctid" ;;
	1) CTID="${found[0]}" ;;
	*) die "several candidates (${found[*]}); give --ctid" ;;
	esac
fi
if ! $INSIDE; then
	pct config "$CTID" >/dev/null 2>&1 || die "no container $CTID"
	conf="$(pct config "$CTID")"
	in_ct() { pct exec "$CTID" -- "$@"; }
	in_hint="pct exec $CTID -- "

	section "Container $CTID ($(sed -n 's/^hostname: //p' <<<"$conf"))"
	if [ "$(pct status "$CTID" | awk '{ print $2 }')" != running ]; then
		bad "it is not running: pct start $CTID"
		exit 1
	fi
	ok "running"
	if grep -q '^features:.*nesting=1' <<<"$conf"; then
		ok "nesting is on"
	else
		bad "nesting is off; the relay's sandbox needs it: pct set $CTID --features nesting=1 && pct reboot $CTID"
	fi
	net0="$(sed -n 's/^net0: //p' <<<"$conf")"
	bridge="$(sed -n 's/.*bridge=\([^,]*\).*/\1/p' <<<"$net0")"
fi
ct_ip4="$(in_ct hostname -I 2>/dev/null | tr ' ' '\n' | grep -E '^[0-9]+\.' | head -n 1 || true)"
ct_ip6="$(in_ct hostname -I 2>/dev/null | tr ' ' '\n' | grep ':' | grep -v '^fe80' | head -n 1 || true)"
[ -n "$ct_ip4" ] || bad "it has no IPv4 address (net0: $net0)"
[ -z "$ct_ip4" ] || ok "IPv4 $ct_ip4 on $bridge$(is_private "$ct_ip4" && echo ", private" || echo ", public")"
[ -z "$ct_ip6" ] || ok "IPv6 $ct_ip6$(is_private "$ct_ip6" && echo ", private" || echo ", public")"

# --- The relay and Caddy --------------------------------------------------------

section "Relay and Caddy"
if [ "$(in_ct systemctl is-active chirp-relay 2>/dev/null || true)" = active ]; then
	ok "chirp-relay is active"
else
	bad "chirp-relay is not active: ${in_hint}journalctl -u chirp-relay -n 50"
fi
if in_ct sh -c 'set -a; . /etc/chirp-relay/relay.env 2>/dev/null; exec /usr/local/bin/chirp-relay -healthcheck' >/dev/null 2>&1; then
	ok "the relay answers /healthz on its socket"
else
	bad "the relay does not answer /healthz on its socket"
fi

caddy_domain="$(in_ct sh -c 'cat /etc/systemd/system/caddy.service.d/chirp-relay.conf 2>/dev/null' |
	sed -n 's/^[[:space:]]*Environment=CHIRP_RELAY_DOMAIN=//p' | head -n 1 || true)"
[ -n "$DOMAIN" ] || DOMAIN="$caddy_domain"
caddy_ok=true
if ! in_ct sh -c 'command -v caddy' >/dev/null 2>&1; then
	bad "Caddy is not installed: the container was created without --domain"
	caddy_ok=false
elif [ -z "$caddy_domain" ]; then
	bad "Caddy is installed, but not by install.sh for a domain"
	caddy_ok=false
elif [ -n "$DOMAIN" ] && [ "$caddy_domain" != "$DOMAIN" ]; then
	bad "Caddy serves $caddy_domain, not $DOMAIN"
	caddy_ok=false
else
	ok "Caddy serves $caddy_domain"
fi

if ! $caddy_ok; then
	if [ -z "$DOMAIN" ]; then
		todo+=("Name the relay: run this again with --domain NAME --fix-caddy")
	elif $FIX_CADDY; then
		in_ct test -x /root/chirp-relay-install/install.sh ||
			die "no installer at /root/chirp-relay-install in the container; push the bundle and run install.sh --domain $DOMAIN there"
		printf '\n==> Installing Caddy for %s%s\n' "$DOMAIN" "${CTID:+ in container $CTID}"
		in_ct /root/chirp-relay-install/install.sh --binary /usr/local/bin/chirp-relay --domain "$DOMAIN"
		problems=$((problems - 1))
		caddy_ok=true
	else
		todo+=("Install Caddy for $DOMAIN: run this again with --domain $DOMAIN --fix-caddy")
	fi
fi

if $caddy_ok && [ -n "$DOMAIN" ]; then
	if [ "$(in_ct systemctl is-active caddy 2>/dev/null || true)" = active ]; then
		ok "caddy is active"
	else
		bad "caddy is not active: ${in_hint}journalctl -u caddy -n 50"
	fi
	listening="$(in_ct ss -Hltn 2>/dev/null | awk '{ print $4 }' || true)"
	for port in 80 443; do
		if grep -qE "[:.]$port\$" <<<"$listening"; then
			ok "something listens on TCP $port"
		else
			bad "nothing listens on TCP $port"
		fi
	done
	# Without a certificate Caddy cannot finish a TLS handshake for the
	# name, so /healthz through it can only be asked once it has one.
	if in_ct sh -c "ls /var/lib/caddy/.local/share/caddy/certificates/*/$DOMAIN/$DOMAIN.crt" >/dev/null 2>&1; then
		ok "Caddy has a certificate for $DOMAIN"
		code="$(in_ct curl -sk -o /dev/null -w '%{http_code}' --max-time 5 \
			--resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/healthz" 2>/dev/null || true)"
		if [ "$code" = 200 ]; then
			ok "/healthz answers through Caddy, inside the container"
		else
			bad "/healthz through Caddy, inside the container, answered ${code:-nothing}"
		fi
	else
		warn "Caddy has no certificate for $DOMAIN yet. It gets one by itself once the name"
		warn "resolves here and port 80 reaches the container. Its last words on it:"
		in_ct journalctl -u caddy -n 200 --no-pager 2>/dev/null |
			grep -iE 'error|challenge|obtain' | tail -n 3 | cut -c1-200 | sed 's/^/          /' || true
	fi
fi

# --- The way out, and in ------------------------------------------------------

section "Network"
public4="$(in_ct curl -4fsS --max-time 8 https://1.1.1.1/cdn-cgi/trace 2>/dev/null | sed -n 's/^ip=//p' || true)"
public6="$(in_ct curl -6fsS --max-time 8 'https://[2606:4700:4700::1111]/cdn-cgi/trace' 2>/dev/null | sed -n 's/^ip=//p' || true)"
if [ -n "$public4" ]; then
	ok "its IPv4 traffic leaves from $public4"
else
	bad "it cannot reach the internet over IPv4"
fi
[ -z "$public6" ] || ok "its IPv6 traffic leaves from $public6"

nat=none
if [ -n "$ct_ip4" ] && [ -n "$public4" ] && [ "$ct_ip4" != "$public4" ]; then
	nat=router
	# A bridge with no physical port behind it is private to this node:
	# the node itself must be doing the NAT.
	if ! $INSIDE && [ -d "/sys/class/net/$bridge/brif" ]; then
		nat=node
		for port in "/sys/class/net/$bridge/brif"/*; do
			case "${port##*/}" in
			veth* | tap* | fwpr* | fwln* | "*") ;;
			*) nat=router ;;
			esac
		done
	fi
fi
case "$nat" in
none) ok "the container's own address is the public one: no port forwarding needed" ;;
node) warn "$bridge has no physical port: this node NATs the container" ;;
router)
	if $INSIDE; then
		warn "it is behind NAT: a router, or the Proxmox node if its bridge is private, forwards to it"
	else
		warn "the container is behind NAT: a router forwards to it"
	fi
	;;
esac

if ! $INSIDE && command -v pve-firewall >/dev/null && pve-firewall status 2>/dev/null | grep -q 'enabled/running'; then
	if grep -q 'firewall=1' <<<"$net0" && grep -qE '^enable: *1' "/etc/pve/firewall/$CTID.fw" 2>/dev/null; then
		if grep -qiE 'dport[ =]?80|dport[ =]?443|HTTPS|HTTP' "/etc/pve/firewall/$CTID.fw" 2>/dev/null; then
			ok "the Proxmox firewall is on for it, with rules for HTTP(S)"
		else
			warn "the Proxmox firewall is on for it, and no rule mentions 80 or 443"
			todo+=("Allow the relay through the Proxmox firewall: add to /etc/pve/firewall/$CTID.fw, under [RULES]:
        IN ACCEPT -p tcp -dport 80,443
        IN ACCEPT -p udp -dport 443")
		fi
	else
		ok "the Proxmox firewall is running, but not for this container"
	fi
fi

# --- The name -----------------------------------------------------------------

if [ -n "$DOMAIN" ]; then
	section "DNS for $DOMAIN (asked of 1.1.1.1)"
	dns4="$(resolve "$DOMAIN" A)"
	dns6="$(resolve "$DOMAIN" AAAA)"
	want4="$public4"
	[ "$nat" = none ] && want4="$ct_ip4"
	if [ -z "$dns4" ]; then
		bad "$DOMAIN has no A record"
		todo+=("Create the DNS record, DNS only (grey cloud), from a machine with the repository:
        scripts/set-relay-dns.sh --name $DOMAIN --ipv4 ${want4:-<public IPv4>}
      or in Cloudflare's dashboard: DNS → Records → Add record:
        Type A, Name ${DOMAIN%%.*}, IPv4 ${want4:-<public IPv4>}, Proxy status: DNS only")
	elif grep -qxF "$want4" <<<"$dns4"; then
		ok "A $dns4"
	else
		bad "A is $(tr '\n' ' ' <<<"$dns4")but the container is reached at ${want4:-?}"
		todo+=("Point the A record at $want4 (DNS only): scripts/set-relay-dns.sh --name $DOMAIN --ipv4 $want4")
	fi
	if [ -n "$dns6" ] && ! grep -qxF "${ct_ip6:-none}" <<<"$dns6"; then
		bad "AAAA is $(tr '\n' ' ' <<<"$dns6")but the container's IPv6 is ${ct_ip6:-none}: remove the AAAA record or fix it"
	elif [ -n "$dns6" ]; then
		ok "AAAA $dns6"
	elif [ -n "$ct_ip6" ] && ! is_private "$ct_ip6"; then
		warn "the container has a public IPv6 address and $DOMAIN no AAAA record"
		todo+=("Optionally, reach it over IPv6 too. IPv6 has no NAT: allow inbound TCP 80 and 443
      to $ct_ip6 in the router's IPv6 firewall, then add the record:
        scripts/set-relay-dns.sh --name $DOMAIN --ipv6 $ct_ip6")
	fi
fi

case "$nat" in
node)
	ext="$(ip -4 route show default | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") print $(i + 1); exit }')"
	todo+=("Forward the ports on this node (public interface ${ext:-?}), and make the rules persistent
      as this node already does, e.g. post-up lines in /etc/network/interfaces:
        iptables -t nat -A PREROUTING -i ${ext:-<public interface>} -p tcp -m multiport --dports 80,443 -j DNAT --to-destination $ct_ip4
        iptables -t nat -A PREROUTING -i ${ext:-<public interface>} -p udp --dport 443 -j DNAT --to-destination $ct_ip4")
	;;
router)
	! $INSIDE || todo+=("Find out who NATs it: run this script on the Proxmox node. If the node does,
      it prints the iptables rules to forward the ports there instead of on the router.")
	todo+=("On the router in front of this network, forward to $ct_ip4 (docs/relay.md, \"Let traffic reach it\"):
        TCP 80  → $ct_ip4:80
        TCP 443 → $ct_ip4:443
        UDP 443 → $ct_ip4:443   (HTTP/3, optional)
      and check the router's WAN address is $public4: if it is not, or is in
      100.64.0.0/10, the provider puts you behind CGNAT and forwarding cannot work.")
	;;
esac
todo+=("From a machine on another network (a phone's data, not this LAN):
        scripts/check-relay.sh ${DOMAIN:-<domain>}")

section "Left to do"
n=1
for item in "${todo[@]}"; do
	printf '  %d. %s\n' "$n" "$item"
	n=$((n + 1))
done
echo
[ "$problems" -eq 0 ]

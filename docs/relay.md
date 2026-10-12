# Running a Chirp relay

This guide takes a Chirp relay from the repository to a public HTTPS address.
It covers three ways to run it:

- [**A Proxmox VE LXC container**](#a-proxmox-ve-lxc-container), created by
  one script on the Proxmox node.
- [**Podman**](#podman), as systemd services through Quadlet, rootful or
  rootless.
- [**Any Linux host with systemd**](#any-linux-host-with-systemd), such as a VM
  or a bare server, optionally behind a TLS proxy you already run.

All three run the same binary with the same TLS proxy (Caddy) in front of it,
and they share the files in [`deploy/`](../deploy). Read the first sections
whichever one you pick. [Local development](#local-development) is at the end.

## What the relay is

`cmd/chirp-relay` is Chirp's server. It hands each running Chirp its code and
share link, matches a typed code or an opened link with the Chirp that owns
it, carries the consent request and answer, and relays the WebRTC offers,
answers and ICE candidates between the two ends. It is `signalling.Server`
behind a small `main`, described in `HANDOFF.md` §4.5.

It does **not** carry the session. Screen, input, clipboard and files travel
over WebRTC, directly between the two Chirps when the network allows it and
through a TURN server when it does not. TURN is a separate server, covered in
[`docs/stun-turn.md`](stun-turn.md) and summarised under
[STUN and TURN](#stun-and-turn) below.

Some properties shape how it is deployed:

- **All of its state is in memory.** Nothing is written to disk and there is
  nothing to back up. A restart forgets every code, link and session (see
  [Restarts and upgrades](#restarts-and-upgrades)).
- **It is one process.** Two relays do not share state, so do not load-balance
  across replicas. One small instance serves a great many users, because
  signalling is a few kilobytes per session.
- **It speaks plain HTTP** and never terminates TLS itself. A reverse proxy in
  front provides HTTPS and WSS. Every deployment here uses Caddy, which also
  obtains and renews the certificate.
- **It throttles code guesses per client address**, along with registrations
  and open sessions (`HANDOFF.md` §4.2, §4.6). Behind a proxy it has to know
  which proxy to believe about that address, and no other process may be able
  to pose as that proxy. Otherwise every user shares one budget, or anyone can
  escape it ([How the pieces connect](#how-the-pieces-connect)).
- **Its URLs carry credentials.** Peer IDs, session IDs and link tokens travel
  in request paths and query strings. Whatever logs requests must leave them
  out, as the Caddyfile here does.

```
                   HTTPS / WSS (443)            HTTP over a Unix socket
  Chirp (owner)  ────────────────────┐          (/run/chirp-relay/relay.sock)
                                     ├──▶  Caddy  ──────────────────▶  chirp-relay
  Chirp (helper) ────────────────────┘     TLS, certificates            codes, consent,
                                           X-Forwarded-For              signalling
        │                                                                (memory only)
        │          WebRTC: screen, input, clipboard, files
        └─────────────────────────────────────────────▶  the other Chirp,
                   directly, or through TURN (docs/stun-turn.md)
```

## Before you start

You need:

- **A DNS name for the relay**, such as `relay.example.org`, with an `A`
  record (and `AAAA` for IPv6) pointing at the public address that reaches
  the machine. Caddy will not get a certificate until the record resolves.
- **TCP 80 and 443 reaching the machine** from the internet, plus UDP 443 for
  HTTP/3, which is optional. Port 80 serves the ACME HTTP challenge and the
  redirect to HTTPS. The relay itself listens on a Unix socket, not a port.
- **Go 1.26** on the machine where you build. The relay needs no cgo, so it
  cross-compiles from any OS.

**If the domain is on Cloudflare**, make the relay's record *DNS only* (grey
cloud), not *Proxied*. A proxied record puts Cloudflare between the users and
Caddy, so every request comes from a Cloudflare address unless you trust
Cloudflare's ranges. Cloudflare also closes idle WebSockets, and a Chirp holds
its owner socket open, mostly idle, for as long as it runs. The
`usechirp.app` website being proxied does not change this: the relay is a
different record.

## Building

Every option starts from a build on your own machine, from a checkout of this
repository.

**The bundle** is used by Proxmox and by any Linux host. It is a binary plus
the files to install it, in one archive:

```bash
scripts/build-relay.sh
```

This writes `bin/chirp-relay-<version>-linux-amd64.tar.gz`. Pass `arm64` (or
`amd64 arm64`) for other architectures. The archive contains:

```
chirp-relay                     static binary
install.sh                      deploy/install.sh
systemd/chirp-relay.service     the hardened unit
systemd/relay.env               its settings, installed once
caddy/Caddyfile                 TLS proxy configuration
proxmox/create-ct.sh            creates the LXC container
```

**The container image** is used by Podman:

```bash
podman build -f deploy/container/Containerfile -t localhost/chirp-relay .
```

The image is a static binary on distroless (about 10 MB), runs as an
unprivileged user and has no shell. Its base images are pinned by digest, and
Dependabot updates them. Chirp does not publish an image yet, so build it
yourself, either on the server or on your machine (see [Podman](#podman)).

## A Proxmox VE LXC container

`deploy/proxmox/create-ct.sh` runs on a Proxmox VE node and does everything:
it downloads the newest Debian 13 template, creates an unprivileged container,
updates it, installs the relay and, when you give it a domain, Caddy.

### 1. Get the bundle onto the Proxmox node

If you reach the node through a bastion (jump host), let SSH do the hop. Add
this to `~/.ssh/config` on your machine, adjusting names and addresses:

```
Host pve-bastion
    HostName bastion.example.org
    User you

Host pve
    HostName 10.0.0.2          # the Proxmox node, as the bastion sees it
    User root
    ProxyJump pve-bastion
```

Then copy the bundle and unpack it:

```bash
scp bin/chirp-relay-*-linux-amd64.tar.gz pve:/root/
```

```bash
ssh pve 'cd /root && tar xzf chirp-relay-*-linux-amd64.tar.gz'
```

### 2. Create the container

On the node, from the unpacked directory:

```bash
cd /root/chirp-relay-*-linux-amd64
./proxmox/create-ct.sh --domain relay.example.org --ip 203.0.113.10/24 --gw 203.0.113.1
```

Run `./proxmox/create-ct.sh --help` for every option. The most useful are:

| Option | Default | |
| --- | --- | --- |
| `--domain` | none | Installs Caddy for this name. Without it, you supply the proxy ([Behind a proxy you already run](#behind-a-proxy-you-already-run)). |
| `--ip`, `--gw` | `dhcp` | IPv4 for `eth0`. `--ip6 auto` adds SLAAC. |
| `--bridge`, `--vlan` | `vmbr0` | Where `eth0` attaches. |
| `--storage` | `local-lvm` | Storage for the root disk. |
| `--ctid` | next free | |
| `--ssh-key` | none | Authorises a key for `root`, if you want to SSH in. `pct enter <ctid>` works without one. |

The container gets 1 core, 256 MiB of memory and 2 GiB of disk, starts on
boot, and is tagged `chirp`. It has the **nesting** feature turned on. The
relay's systemd sandbox needs nesting to create its namespaces, and nesting
does not make an unprivileged container privileged.

### 3. Let traffic reach it

How ports 80 and 443 reach the container depends on your network:

- **The container has a public address** on a bridged network (as in the
  example above). Nothing more to do. Point DNS at that address.
- **The node has the only public address** and containers sit on a private
  bridge. Forward the ports on the node, or on the router in front of it. On
  the node, with the container at `10.10.10.20` and the public interface
  `vmbr0`:

  ```bash
  iptables -t nat -A PREROUTING -i vmbr0 -p tcp -m multiport --dports 80,443 -j DNAT --to-destination 10.10.10.20
  ```

  ```bash
  iptables -t nat -A PREROUTING -i vmbr0 -p udp --dport 443 -j DNAT --to-destination 10.10.10.20
  ```

  Make the rules persistent the way your node already does it, for example in
  `post-up` lines in `/etc/network/interfaces`. A DNAT rule keeps the client's
  source address, which the relay needs.
- **You already run a reverse proxy** (on the node, in another container, or
  on another machine). Create the container without `--domain`, then see
  [Behind a proxy you already run](#behind-a-proxy-you-already-run).

If the Proxmox firewall is on for the container, allow inbound TCP 80 and 443
and UDP 443.

### 4. Check it

```bash
curl https://relay.example.org/healthz
```

It should print `ok`. If it does not, see
[Troubleshooting](#troubleshooting).

### Doing it by hand

Prefer the web UI? Create an unprivileged Debian 13 container with *Nesting*
checked under *Options → Features*, and give it the resources above. Then copy
the bundle into it and run the installer as root:

```bash
./install.sh --binary ./chirp-relay --domain relay.example.org
```

## Podman

The relay and Caddy run as two containers managed by systemd through
[Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html).
The units are in [`deploy/podman/`](../deploy/podman). Podman 4.9 or newer is
needed (5.x recommended).

**Rootful or rootless.** Both work. Rootful (units in
`/etc/containers/systemd/`) is the simpler choice on a server dedicated to the
relay. Rootless (units in `~/.config/containers/systemd/` of an ordinary user)
keeps root out of it. The steps below show rootless and note where rootful
differs.

### How the containers connect

- **The relay has no network** (`Network=none`). It listens on a Unix socket
  in the `chirp-relay-socket` volume, which only the two containers mount.
  Nothing else on the host can reach the relay or pose as Caddy. The relay
  runs with no capabilities, a read-only root filesystem, and the volume as
  its only writable path.
- **Caddy uses the host's network** (`Network=host`), listening on 80 and
  443. A bridge network with published ports would look tidier, but rootless
  Podman then hands Caddy every connection from one internal address, so
  every user would share one throttle. On the host network, Caddy sees each
  user's real address under both rootful and rootless Podman.

### 1. Build or load the image

On the server, from a checkout:

```bash
podman build -f deploy/container/Containerfile -t localhost/chirp-relay .
```

Or build on your machine and send the image over SSH (through the bastion, if
your `~/.ssh/config` has one):

```bash
podman save localhost/chirp-relay | ssh relay-host podman load
```

For rootful Podman, the image must be in root's storage (`sudo podman load`).

### 2. Allow Caddy to bind 80 and 443 (rootless only)

```bash
echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/90-chirp.conf
```

```bash
sudo sysctl --system
```

### 3. Install the units and the Caddyfile

```bash
mkdir -p ~/.config/containers/systemd ~/.config/chirp
cp deploy/podman/chirp-relay.container deploy/podman/caddy.container ~/.config/containers/systemd/
cp deploy/caddy/Caddyfile ~/.config/chirp/Caddyfile
```

Edit `~/.config/containers/systemd/caddy.container` and set your domain:

```ini
Environment=CHIRP_RELAY_DOMAIN=relay.example.org
```

For rootful Podman, copy the units to `/etc/containers/systemd/` and the
Caddyfile to `/root/.config/chirp/Caddyfile`, or change the `Volume=` line to
point wherever you keep it.

### 4. Start

```bash
systemctl --user daemon-reload
systemctl --user start caddy
```

Starting `caddy` starts `chirp-relay` too. Both start on boot through their
`[Install]` sections, which Quadlet applies on its own. A rootless user's
services start at boot only with lingering on:

```bash
sudo loginctl enable-linger "$USER"
```

Rootful: the same commands without `--user`.

### 5. Check it

```bash
systemctl --user status chirp-relay caddy
podman healthcheck run chirp-relay && echo healthy
curl https://relay.example.org/healthz
```

If the health check fails three times in a row, Podman kills the relay
container and systemd restarts it.

### A quick run without Quadlet

To try the image without installing anything:

```bash
podman run --rm -p 127.0.0.1:8080:8080 localhost/chirp-relay
```

Then point Chirp at `http://localhost:8080`. Keep it to your own machine like
this. Anything beyond it needs the proxy in front.

## Any Linux host with systemd

On a Debian or Ubuntu VM or server, use the bundle's installer as root:

```bash
tar xzf chirp-relay-*-linux-amd64.tar.gz && cd chirp-relay-*-linux-amd64
./install.sh --binary ./chirp-relay --domain relay.example.org
```

The installer:

- installs `/usr/local/bin/chirp-relay` and `chirp-relay.service`;
- writes `/etc/chirp-relay/relay.env` the first time, and never overwrites
  it after that;
- creates the `chirp-relay` system user the relay runs as;
- with `--domain`, installs Caddy from Caddy's own apt repository and
  configures it. Debian 12 and 13 and Ubuntu 24.04 ship Caddy 2.6, which is
  older than the Caddyfile needs. It also adds the `caddy` user to the
  `chirp-relay` group, which is the only group that can open the relay's
  socket;
- starts the relay and checks that it answers.

If Caddy was already installed and serving other sites, the installer leaves
`/etc/caddy/Caddyfile` alone. It writes `/etc/caddy/chirp-relay.caddy` and
tells you which `import` line to add. Global options cannot be imported, so
also copy the `log default` block from `deploy/caddy/Caddyfile` into your
global options. Without it, Caddy's error log records request URIs, and with
them relay credentials.

On other distributions, do by hand what the installer does. It is short and
commented.

The unit runs the relay as the `chirp-relay` user in a strict sandbox, which
`systemd-analyze security chirp-relay` rates at 1.1, "OK". The relay listens
on `/run/chirp-relay/relay.sock`, which is mode `0660` in a `0750` directory,
both owned by `chirp-relay`. Only that user and the members of its group
(Caddy) can connect.

### Behind a proxy you already run

If TLS ends at a proxy you already have, skip `--domain` and point that proxy
at the relay. Then:

1. Make the relay reachable from the proxy. In `/etc/chirp-relay/relay.env`,
   set `CHIRP_RELAY_ADDR` to an address the proxy can reach, such as
   `10.10.10.20:8080`. Firewall that port so that only the proxy can connect.
   If the proxy runs on the same machine, keep the socket instead, and add the
   proxy's user to the `chirp-relay` group.
2. Over TCP, trust that proxy and nothing else by setting
   `CHIRP_RELAY_TRUSTED_PROXIES` to the proxy's address as the relay sees it.
3. Make the proxy pass WebSocket upgrades, set `X-Forwarded-For`, and not
   time out idle connections quickly.
4. Keep request URIs out of the proxy's logs. They carry peer IDs, session
   IDs and link tokens.
5. Restart the relay with `systemctl restart chirp-relay`.

For nginx:

```nginx
location / {
    proxy_pass http://10.10.10.20:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;   # map $http_upgrade, as nginx documents
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;     # replace, never append a client's value
    proxy_read_timeout 1h;
    client_max_body_size 64k;
}
```

## Configuration

| Flag | Environment | Default | |
| --- | --- | --- | --- |
| `-addr` | `CHIRP_RELAY_ADDR` | `localhost:8080` (`:8080` in the image) | Where to listen: `host:port`, or `unix:PATH` for a Unix socket. |
| `-socket-mode` | `CHIRP_RELAY_SOCKET_MODE` | `0660` | Permissions of the Unix socket, in octal. |
| `-trusted-proxies` | `CHIRP_RELAY_TRUSTED_PROXIES` | none | Over TCP: comma-separated addresses or CIDRs whose `X-Forwarded-For` is believed. |
| `-healthcheck` | | | Ask the relay on `-addr` for `/healthz`, and exit 0 if it answers. Used by container health checks. |

A flag on the command line overrides the environment. Code lifetimes,
throttles and caps are compiled in (`HANDOFF.md` §4.2, §4.6).

`GET /healthz` returns `200 ok` while the process is serving. It reads no
state and reveals nothing.

### How the pieces connect

The relay counts code guesses, registrations and open sessions per client
address. Behind a proxy, every connection comes from the proxy, so the relay
has to take the client's address from the proxy's `X-Forwarded-For` header.
Two things can go wrong:

- **It believes nobody.** Then it counts every user as one client. Five
  wrong codes from anyone lock everyone out for ten minutes, and the 31st
  Chirp to start gets `429`.
- **It believes someone other than the proxy.** Anything that can reach the
  relay directly and gets believed can write its own header and pick a fresh
  address for every guess, escaping the throttle. That includes any process
  on the same machine when the relay listens on `127.0.0.1`.

There are two ways to get this right:

- **A Unix socket** (`-addr unix:PATH`), which every deployment here uses.
  Only the processes that the socket's permissions admit can connect, and
  those permissions admit only Caddy. Every request on the socket is from
  the proxy, so its `X-Forwarded-For` is believed.
- **TCP with `-trusted-proxies`**, when the proxy is on another machine. A
  request from a listed address is attributed to the rightmost address in
  `X-Forwarded-For` that is not itself listed, which is the client as the
  outermost trusted proxy saw it. Addresses further left were written by the
  client and are ignored. A request from anywhere else is attributed to its
  own address, whatever its headers say. List exactly your proxies, and
  firewall the port so that only they can reach it.

Either way, the proxy must overwrite a client's `X-Forwarded-For` or append
to it. Caddy does this by default.

## Pointing Chirp at the relay

Both ends must use the same relay:

```bash
CHIRP_RELAY=https://relay.example.org chirp
```

`-relay https://relay.example.org` does the same. Add the STUN and TURN
servers with `CHIRP_ICE` (next section). Without them, sessions reach only
machines on the same network.

## STUN and TURN

The relay only introduces the two Chirps. For them to connect across NATs
they need STUN, and often TURN. [`docs/stun-turn.md`](stun-turn.md) explains
why, and gives a coturn configuration. Some notes for these deployments:

- **TURN needs UDP, a range of ports, and an address it can announce.**
  Neither Caddy nor Cloudflare's proxy can carry it. Give coturn a public
  address, or forward `3478/udp+tcp`, `5349/tcp` and the relay range
  (`49152–65535/udp`) to it and set `external-ip` to the public address.
- **coturn fits in the same Proxmox container** as the relay if that
  container has a public address. Install it with `apt install coturn`. A
  separate container keeps the TURN bandwidth apart from the relay and is
  easier to move.
- **TURN carries every byte of a relayed session.** Size the container's
  network accordingly. The relay itself does not need it.

## Operations

### Logs

```bash
journalctl -u chirp-relay -f          # LXC, VM
journalctl --user -u chirp-relay -f   # Podman, rootless
journalctl -u caddy -f
```

The relay logs only when it starts, stops and fails. Caddy logs every
request to the journal: the client's address, the method, the status and the
timing. It does not log the URI or the headers, because peer IDs, session
IDs and link tokens travel there, and each one is a credential. If you change
the Caddyfile's `log` blocks, keep them filtered.

### Restarts and upgrades

The state is in memory, so a restart:

- ends any session being set up, and the users start it again;
- invalidates every code and link shown before the restart. Each running
  Chirp notices, registers again on its own, and shows a new code and link.
  Links that were already shared stop working.

Upgrade when few people are using it, and expect running Chirps to show new
codes afterwards.

- **LXC or VM:** build a new bundle, copy it in (`pct push <ctid> <file>
  <dest>` from the node, or `scp`), and run `./install.sh --binary
  ./chirp-relay` from it again. Your `relay.env` and Caddy configuration are
  kept. Upgrade the system itself with `apt` as usual. `unattended-upgrades`
  is a good idea in the container.
- **Podman:** build or load the new image with the same tag, then run
  `systemctl --user restart chirp-relay`. To update Caddy, change the digest
  on the `Image=` line of `caddy.container` (the current one is listed on
  [Docker Hub](https://hub.docker.com/_/caddy)), then run `systemctl --user
  daemon-reload && systemctl --user restart caddy`. Dependabot keeps the
  relay's own base images current, but cannot read Quadlet units.

### Resources

The relay idles at a few MiB of memory and uses almost no CPU. Each connected
Chirp holds one WebSocket. The 256 MiB container leaves room for Caddy and
`apt`.

### What to keep

Only Caddy's data: the certificates and the ACME account, in
`/var/lib/caddy` (LXC, VM) or the `chirp-caddy-data` volume (Podman). If they
are lost, Caddy obtains them again, subject to Let's Encrypt's rate limits.

## Security checklist

- [ ] Only the proxy can reach the relay: over the Unix socket, or over TCP
      with a firewall that admits only the proxy.
- [ ] Over TCP, `CHIRP_RELAY_TRUSTED_PROXIES` lists your proxies and nothing
      broader.
- [ ] No log records request URIs: they carry peer IDs, session IDs and link
      tokens.
- [ ] The relay's DNS record is *DNS only* if the domain is on Cloudflare.
- [ ] The container or VM receives security updates.
- [ ] coturn, if it runs, has the `denied-peer-ip` list from
      [`docs/stun-turn.md`](stun-turn.md).
- [ ] The relay is trusted for what it sees. It cannot see the screen or
      input, but it does see who connects to whom, and it carries the key
      fingerprints the session is bound to (`docs/product-model.md`, *What
      follows for security*). Run it where you would run anything that
      matters to your users.

## Troubleshooting

**`status=226/NAMESPACE` when the service starts.** The systemd sandbox
cannot create namespaces. In a Proxmox container, turn on nesting from the
node, reboot the container, and run `install.sh` again:

```bash
pct set <ctid> --features nesting=1 && pct reboot <ctid>
```

**Caddy does not get a certificate.** Check `journalctl -u caddy`. Common
causes: the DNS record does not point at the address that reaches the machine
yet, port 80 is not forwarded, or a Cloudflare-proxied record is in the way.
Caddy retries on its own, so once the cause is fixed, wait or restart it.

**Every user gets `429 Too Many Requests` quickly.** The relay is counting
everyone as one client. Over TCP, check that `CHIRP_RELAY_TRUSTED_PROXIES`
names the proxy as the relay sees it. Either way, check that the proxy keeps
client addresses. With rootless Podman, Caddy must use `Network=host`.

**Caddy answers `502` and logs `permission denied` on the socket.** Caddy
cannot open the relay's socket. On a systemd host, check that
`id caddy` lists the `chirp-relay` group, then restart Caddy, because a new
group membership needs a restart. With Podman, check that both containers
mount the `chirp-relay-socket` volume.

**Chirp keeps reconnecting.** Something between the client and the relay
closes idle WebSockets: a Cloudflare-proxied record, or a proxy with a short
read timeout. The relay sends no keepalive pings. Give that hop a timeout of
an hour or more.

**`curl https://…/healthz` works but sessions never show a picture.** The
relay did its job, and ICE found no path between the two ends. Configure
TURN ([`docs/stun-turn.md`](stun-turn.md), *What breaks without them*).

## Local development

No proxy, no TLS, nothing installed:

```bash
go run ./cmd/chirp-relay
```

```bash
CHIRP_RELAY=http://localhost:8080 go run ./cmd/chirp
```

To check a change to the deployment files:

- Build the image and run it: [A quick run without Quadlet](#a-quick-run-without-quadlet).
- Run `shellcheck -x deploy/install.sh deploy/proxmox/create-ct.sh scripts/build-relay.sh`.
- Run `install.sh` in a throwaway systemd container. It needs `--privileged`
  for the sandbox's namespaces, the same thing nesting provides in Proxmox:

  ```bash
  scripts/build-relay.sh
  podman run -d --name relay-sim --privileged --systemd=always \
    -v "$PWD/$(ls bin/chirp-relay-*-amd64.tar.gz | head -n1):/root/b.tgz:ro,Z" \
    docker.io/library/debian:trixie sh -c 'apt-get update && apt-get install -y systemd systemd-sysv curl gnupg && exec /sbin/init'
  ```

  Wait for it to boot (`podman logs -f relay-sim`), then run:

  ```bash
  podman exec relay-sim sh -c 'cd /root && tar xzf b.tgz && cd chirp-relay-*/ && ./install.sh --binary ./chirp-relay'
  ```

CI builds the image, smoke-tests it, lints the scripts and generates the
Quadlet units on every pull request (the *Relay image & deploy files* job).
`create-ct.sh` needs a Proxmox node, so CI only lints it. Test changes to it
on a real node.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

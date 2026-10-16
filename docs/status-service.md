# Running the status service

`cmd/chirp-status` probes every server in [`network/servers.json`](../network/servers.json)
each 30 seconds and serves the figures the status page at
[status.usechirp.app](https://status.usechirp.app/) shows. What it measures
and why is in [`status-page-proposal.md`](status-page-proposal.md). This page
is how to run it.

## Where

**On a machine apart from every server it watches.** A monitor on the same
host, network or power as a relay goes down with it, and then the page says
nothing at the moment it matters. For Fabrintek's relay on pve-dc, that rules
out pve-dc and anything behind its router: a small VPS elsewhere is right.

It needs little: one core, 128 MiB, and a few hundred MiB of disk for 90
days of probes per server. Debian or Ubuntu with systemd.

## Build and install

On your machine, from a checkout:

```bash
scripts/build-status.sh
```

This writes `bin/chirp-status-<version>-linux-amd64.tar.gz` (pass `arm64` for
ARM): the binary, the systemd unit, the settings, `install.sh` and a copy of
`network/servers.json`. Copy it to the machine and install it as root:

```bash
tar xzf chirp-status-*-linux-amd64.tar.gz && cd chirp-status-*-linux-amd64 && ./install.sh --binary ./chirp-status --vantage "São Paulo"
```

`--vantage` names where the probes come from; the page shows it. Use a
place or a provider, never the machine's address.

`install.sh` installs the binary and the unit, writes
`/etc/chirp-status/status.env` the first time only, installs the list every
time, restarts the service and checks it answers on `127.0.0.1:8081`. If it
does not, it puts back the previous binary and list.

The service runs as a user systemd makes for it each start, in the same
sandbox as the relay, and keeps its database in `/var/lib/chirp-status/`.

## Publish it

The API is served at `https://status.usechirp.app`, the only origin the
status page may connect to (its CSP says so). It serves public figures only,
with no credential anywhere in its URLs, so unlike a relay it may sit behind
a CDN. Either way works:

**A Cloudflare Tunnel.** No open port, and the machine's address stays
private. Install `cloudflared` from Cloudflare's package repository, create a
tunnel in *Zero Trust → Networks → Tunnels*, run the command it gives to
install the connector, and add a public hostname: `status.usechirp.app` →
`http://127.0.0.1:8081`. The service sends `Cache-Control: no-cache`, so
Cloudflare does not hold the figures back.

**Caddy on the same machine.** With ports 80 and 443 reaching it and an `A`
record for `status.usechirp.app`, `deploy/proxy/setup-proxy.sh` adds the site:

```bash
./setup-proxy.sh --site status.usechirp.app=127.0.0.1:8081
```

## Check it

```bash
curl -s https://status.usechirp.app/v1/servers
```

It answers JSON with `generated_at` a few seconds old and one entry per
server. `https://status.usechirp.app/` then shows them, once the site's
`/status/` policy is loaded (`scripts/apply-cloudflare.sh`).

On the machine: `systemctl status chirp-status`, and `journalctl -u
chirp-status` for what it could not record.

## Change the list

`network/servers.json` in the repository is the list; the service reads its
copy at start. When a change to it is merged, bring it over and restart:

```bash
curl -fsS -o /etc/chirp-status/servers.json https://raw.githubusercontent.com/barahn/chirp/main/network/servers.json && systemctl restart chirp-status
```

If the new list is invalid, the service refuses to start and says which entry
and why: `journalctl -u chirp-status`. Running `install.sh` from a new bundle
does the same and puts the old list back on failure.

## What to keep

`/var/lib/chirp-status/status.db`: each server's daily totals are kept there
for good, and its probes for 90 days. Back it up with the service stopped, or
with `sqlite3 status.db ".backup copy.db"` while it runs. Losing it loses the
history, not the service: it starts a new one.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

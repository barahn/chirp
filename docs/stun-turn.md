# Bringing your own STUN/TURN

Chirp does not ship a STUN or TURN server. Anyone who self-hosts Chirp brings
their own, and [coturn](https://github.com/coturn/coturn) is the usual choice.
This page explains what those servers are for, what breaks without them, how to
run coturn for Chirp, and what will change upstream.

## What Chirp's relay does, and what it does not

Chirp's own server (`signalling/`) carries **signalling only**: registration,
codes and links, consent, and the offers, answers and ICE candidates the two
ends exchange to set up a connection; [docs/relay.md](relay.md) runs it. The
screen, input, clipboard and files do not pass through it. They travel over WebRTC (remotekit's
`webrtc.PeerSession`), directly between the two Chirps whenever the network
allows it.

Whether "directly" is possible is ICE's job, and ICE needs two kinds of help:

- **STUN** tells each Chirp the public address its NAT gives it, so the other
  end has something to reach. It carries no session traffic and is cheap to
  run.
- **TURN** relays the session itself when no direct path exists. It carries
  every byte of the session, so it costs bandwidth.

## What breaks without them

| What is configured | What works |
| --- | --- |
| Nothing | Two Chirps on the same network (LAN) only. |
| STUN only | Most home and small-office networks, where at least one side's NAT keeps the same public port for every destination. |
| STUN and TURN | Almost everything, including symmetric NAT and carrier-grade NAT on both sides, networks that block UDP (TURN over TCP or TLS on 443), and strict corporate firewalls. |

The failure without TURN is silent from the user's point of view. Both Chirps
reach `consent_requested`, the owner accepts, and then no picture arrives,
because ICE never finds a working path. If support is meant to work for
anyone, anywhere, plan on TURN.

## Pointing Chirp at your servers

remotekit takes ICE servers in `webrtc.PeerConfig`:

```go
webrtc.PeerConfig{
	ICEServers:   []string{"stun:turn.example.org:3478", "turn:turn.example.org:3478", "turns:turn.example.org:5349"},
	TURNUsername: "...",
	TURNPassword: "...",
}
```

`cmd/chirp` takes them as configuration, never constants (barahn/chirp#12):
the servers with `-ice` or in `CHIRP_ICE`, comma-separated, and a TURN
credential only in `CHIRP_TURN_USERNAME` and `CHIRP_TURN_PASSWORD`, which
other users cannot read off the command line:

```sh
CHIRP_ICE=stun:turn.example.org:3478,turn:turn.example.org:3478,turns:turn.example.org:5349 \
CHIRP_TURN_USERNAME=... CHIRP_TURN_PASSWORD=... chirp -relay https://chirp.example.org
```

With none given, a session uses host candidates only, and reaches a controller
on the same network and nothing further. **Chirp passes its ICE servers
explicitly and never relies on `webrtc.DefaultPeerConfig()`.** Today
that default points at Google's public STUN servers. Every session would then
tell a third party that it exists and from which addresses, which does not fit
"self-hostable with zero restrictions" (barahn/remotekit#63 removes the
default upstream).

## Running coturn for Chirp

A minimal `/etc/turnserver.conf` for a server with a public address:

```ini
# Where clients reach it. 3478 serves STUN and TURN over UDP and TCP; 5349
# serves TURN over TLS, which also gets through networks that allow only HTTPS
# when it listens on 443.
listening-port=3478
tls-listening-port=5349
cert=/etc/ssl/turn.example.org/fullchain.pem
pkey=/etc/ssl/turn.example.org/privkey.pem

# Behind a NAT (a cloud VM, for instance): public/private.
#external-ip=203.0.113.10/10.0.0.5

realm=turn.example.org
fingerprint

# Time-limited credentials derived from a shared secret (the TURN REST API
# convention). See "Credentials" below for why not a fixed user/password.
use-auth-secret
static-auth-secret=<a long random secret, kept on the server>

# The relay ports. Open this range for UDP in the firewall.
min-port=49152
max-port=65535

# A TURN server relays to whatever address a client names. Without these lines
# anyone holding credentials can use it to reach your internal network.
# coturn refuses loopback peers by default, but not private ranges.
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
denied-peer-ip=fc00::-fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
denied-peer-ip=fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff

# Bound what one credential, and the whole server, can use (0 = unlimited).
# Example values: size them to your users and your bandwidth.
user-quota=12
total-quota=1200
max-bps=3000000
```

Firewall: `3478/udp`, `3478/tcp`, `5349/tcp` (or `443/tcp`), and the relay
range `49152–65535/udp`.

If the Chirps you support live on the same private network as the TURN server,
remove the matching `denied-peer-ip` line, or replace it with `allowed-peer-ip`
for the exact range. Do not drop the whole list.

Check it with a WebRTC ICE test page (gathering candidates against your
`turn:` URL and a credential pair). A server that returns only `host` and
`srflx` candidates, and never `relay`, is not relaying.

## Credentials

`PeerConfig` takes one TURN username and password, and every Chirp would carry
them. With `use-auth-secret`, that pair is derived from the secret (below)
with an expiry you choose; until something hands out fresh pairs, it has to be
a long one, which makes it a fixed credential in practice. Anyone who unpacks a
Chirp configured with it has a working credential for your TURN server until
it expires or you rotate the secret. The quotas and the `denied-peer-ip` list
above are what limit the damage until that changes.

The fix is per-session credentials that expire, in the format coturn's
`use-auth-secret` already accepts: `username = "<expiry>:<id>"`,
`password = base64(HMAC-SHA1(secret, username))`. The shared secret stays on
the server that hands them out, and each Chirp only ever sees a pair that
stops working on its own. remotekit is adding the generator
(barahn/remotekit#64). Something trusted also has to hand the pairs out. The
natural place is Chirp's relay, which already speaks to both ends, but nothing
in Chirp does it yet.

## What TURN can see

WebRTC encrypts the session end to end (DTLS-SRTP for media, DTLS for the data
channels) between the two Chirps. A TURN server relays ciphertext: it learns
the two ends' addresses, when the session ran and how much data it carried,
and nothing of the screen, input, clipboard or files. The keys are bound to
fingerprints exchanged through the signalling relay, which is why the relay,
not TURN, is the component that has to be trusted (`docs/product-model.md`,
*What follows for security*).

## What will change

- **barahn/remotekit#63:** `DefaultPeerConfig()` loses its Google STUN servers.
  Chirp already must not rely on it.
- **barahn/remotekit#64:** generating ephemeral TURN credentials.
- **A TURN relay that ships with Chirp** is still on the roadmap (`README.md`,
  *What is not here yet*; `HANDOFF.md` §5). Until it exists, this page is the
  answer: document, do not vendor.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

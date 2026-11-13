# Proposal: finding a node, and paths no single operator can see

**Status:** proposal, not decided. It answers open questions 1 and 6 of
[`status-page-proposal.md`](status-page-proposal.md), and turns
[`relay.md`](relay.md)'s "TURN stays off" into a design. Nothing here is
built.

That proposal says a node sees addresses, routing digits and timing, and
that only a pledge stands between those and a log. This one narrows what a
node can see in the first place, by splitting each piece of metadata between
operators who do not see the rest. It does not remove the limit; it moves it
from "one operator" to "operators acting together".

Four parts, in the order they can be built:

1. [TURN on the relay](#1-turn-on-the-relay), with credentials per session.
2. [A code finds its node](#2-a-code-finds-its-node) from its routing digits.
3. [Metadata split between operators](#3-metadata-split-between-operators).
4. [The owner's socket](#4-the-owners-socket) in public mode.

A fifth part, [Volunteers in front](#5-volunteers-in-front), is a direction
rather than a design: it is outside the order of work until its open
questions are answered.

## 1. TURN on the relay

### Why it is off

Fabrintek's coturn runs `stun-only`. A TURN credential built into every
binary would let anyone who unpacks one relay through the node
([`stun-turn.md`](stun-turn.md), *Credentials*). So two Chirps do not
connect today when one is behind a symmetric or carrier-grade NAT and the
other's NAT lets in only replies from the exact port it sent to, as many home
routers do. The same holds when both are behind such NATs. One symmetric side
is enough: a large residential fibre provider was measured giving a new port
for each destination, and its sessions with an ordinary home cable connection
failed (barahn/chirp#151).

remotekit already generates credentials that expire
(`webrtc.GenerateEphemeralTURNCredentials`, barahn/remotekit#64): username
`<expiry>:<id>`, password `base64(HMAC-SHA1(secret, username))`, the format
coturn's `use-auth-secret` checks. Released in v0.8.0, it is what the relay
uses: one implementation of the format, shared with the client, and no new
module, since `cmd/chirp` already imports `remotekit/webrtc`. remotekit
tests it against pion's TURN server only, so Chirp tests it against coturn,
the version `deploy/` runs: an allocation with a fresh credential, refusal
once it has expired, and a session that outlives it. What is missing is
something that hands them out. The relay already speaks to both ends.

### Who gets a credential

Only the owner asks, with `POST /api/peers/{peer}/turn`. The peer ID in the
path is the credential, as for every other owner endpoint. It asks once the
user has let a controller in, for two credentials: its own, and one it sends
the controller over the session's signalling. Over Nostr that is after the
PAKE, signed with the key it proved and encrypted to the controller; over the
relay's own path, through the relay, which issued it anyway.

So the controller never asks the node for one, and a lookup carries none:
whoever only looks codes up, guesses included, gets nothing to relay with,
and every credential comes out of a registration's allowance. Each one is
used within seconds of being issued, which is what lets its lifetime be
short.

The answer, for each:

```json
{
  "urls": ["turn:relay.usechirp.app:3478?transport=udp",
           "turn:relay.usechirp.app:3478?transport=tcp"],
  "username": "1793500000:3f9c1a7e",
  "credential": "…",
  "expires_in": 300
}
```

The `<id>` is random, not the peer ID, so coturn's own records cannot tie an
allocation to a registration.

### Limits

| Limit | Where | Proposed |
|---|---|---|
| Lifetime | relay | 5 minutes |
| Credentials per registration | relay | 8 per hour, two per session |
| Allocations per credential | coturn `user-quota` | 2 |
| Bandwidth per allocation | coturn `max-bps` | sized to the uplink |
| Bandwidth for the whole server | coturn `bps-capacity` | sized to the uplink |
| Peers it may reach | coturn `denied-peer-ip` | private ranges, as `stun-turn.md` lists |

Abuse is bounded by registrations: one origin holds at most 30 peers
(`DefaultMaxPeersPerOrigin`), so at most 30 × 8 credentials an hour, each
capped by `max-bps`, all capped by `bps-capacity`. TURN carries every byte of
a relayed session, so on the reference deployment the caps come from its host uplink, not from a
round number.

### A session outlives its credential

The lifetime bounds when a credential can open an allocation, not how long
the allocation lasts. coturn checks the expiry only on an allocation's first
authenticated request: it then keeps the key for that allocation, and every
Refresh after is checked against the kept key, not the clock. This was read
in `src/server/ns_turn_server.c`, in 4.6.1 (Debian 13's) and in the current
master. An allocation also cannot change credentials midway: coturn answers
`441`, as RFC 8656 requires. So a five-minute credential carries a session of
any length, and a stolen one is useless five minutes later.

pion/turn's server does not behave this way. It checks the expiry on every
Refresh, CreatePermission and ChannelBind (`lt_cred.go`), so its allocations
die within the five minutes a permission lasts once the credential expires.
The network therefore requires the coturn behaviour of its TURN members, and
measures it rather than taking the operator's word: the prober allocates with
a credential of its own, waits past its expiry, and expects the allocation
to keep relaying. It joins the TURN checks in `status-page-proposal.md`'s
*Conformance*.

A new allocation does need a fresh credential, whatever its lifetime. That
happens when a user's network changes midway, from Wi-Fi to mobile data. The
owner then asks for two more and remotekit restarts ICE with them, which
remotekit cannot do yet (open question 1).

### The shared secret

`CHIRP_RELAY_TURN_SECRET` in `relay.env`, or a file named by it, mode 600 like
the node key; coturn's `static-auth-secret` holds the same value. A relay
without it answers `404` on the TURN route.

### Fabrintek's node

`deploy/coturn/turnserver.conf` drops `stun-only` and gains
`use-auth-secret`, the quotas above and a narrow relay range, so gateway NATs
forward a few hundred UDP ports rather than 16,384. TURN over TLS on 443, for
networks that allow only HTTPS, is left for later.

### What TURN still sees

Both ends' addresses, when, and how much. Part 3 splits that.

## 2. A code finds its node

### The rule

Every Chirp and every node holds the same signed list of nodes (phase 6). For
six routing digits `r`, the node is the one with the highest score

```
score(r, node) = SHA-256("chirp-rendezvous/v1" ∥ r ∥ node_key)
```

where `node_key` is the node's ed25519 public key. This is rendezvous
hashing: anyone can compute it, and no node has to agree to it.

- **Registering.** The owner draws six random digits, computes their node and
  registers there with them. The node accepts only if it is the top scorer
  for `r` in its own copy of the list and `r` is free; otherwise the owner
  draws again. Today the server chooses the digits; here the owner does,
  which changes `POST /api/peers`.
- **Looking up.** The controller types the code, computes the same node and
  asks only that one.
- **A link** can name its node, so it needs none of this.

The node key rather than the URL goes into the score, so a node that moves to
another address keeps its codes.

### What it costs, and why that is acceptable

- **No other node sees the digits.** Unlike asking every node.
- **No shared state.** Unlike sharing registrations.
- **No longer code.** Unlike carrying the node in the code. The code stays
  6 + 4.
- **The guessing budget grows with the number of nodes.** Each node meters its
  own lookups, so an origin gets N budgets for N nodes. A routing hit,
  though, only yields a contact and spends the code: the attacker still needs
  the four secret digits, one chance in 10,000, and each code's secret is good
  for one attempt (`client/pake.go`). The space an attacker has to search is
  still 10¹⁰. What would grow is the number of codes an attacker can spend
  to deny them to their owners, and the next section keeps it where it is.

### Spending other people's codes

A lookup that hits the routing digits spends the code, and the owner is
given another (`session/lookup.go`). So an attacker can deny a code without
knowing its secret, though not choose whose: they hit codes at random, and
only misses cost them anything. Each origin may miss 5 times in 10 minutes
(`DefaultMaxRedemptionFailures`) in a space of 900,000 codes. The chance
that a code is spent while someone reads it out for *t* minutes is about

```
p ≈ A · N · t / 1,800,000
```

for an attacker with A addresses against N nodes, however many codes are
live. Links are not affected: their token cannot be guessed.

| A = 1,000 addresses, t = 2 minutes | p |
|---|---|
| One relay, today | 0.11 % |
| 20 nodes, each with today's budget | 2.2 % |
| 20 nodes, with the rule below | 0.11 % |

**The rule.** Each node allows an origin 5 misses per window of
10 minutes × N, where N is the length of the signed list. Across the network
an attacker then has today's budget of one relay. Someone who mistypes the
routing digits lands on any node, so their misses spread across nodes and
rarely meet the limit. Behind a carrier-grade NAT, each node sees 1/N of the
origin's misses and allows 1/N of the budget. The rule needs no shared state:
every node already holds the list, and nodes whose copies differ by a version
differ in their window by a little.

It holds as long as the node sees the client's address. Once requests go
through another node (3.2), the node sees only that node's address, and the
node in front counts the misses instead (3.2, *Who counts the misses*).

Two other ways were set aside. Proof of work per lookup: a cloud core spends
dozens of codes an hour for cents. Letting the node check part of the
secret: it would cut the hits, and give the node a shortcut to sit in the
middle of the session, which the PAKE is there to prevent.

### When the list changes

A node that joins takes over only the digits it now scores highest for, and
codes already registered stay where they are until they expire. A node that
leaves loses its codes; its owners notice as on a restart and register again,
landing elsewhere. A controller whose list is older than the owner's may
compute the wrong node: on a `404` it asks the second-highest, and no more, so
the digits reach at most two nodes.

### A node that misbehaves

It can refuse registrations, and the owner draws other digits, which land on
another node. It can hand out a wrong contact, and the PAKE fails visibly, as
today. It can drop its share of codes, and that share is all it holds.

## 3. Metadata split between operators

| What leaks today | Change | Who sees what afterwards |
|---|---|---|
| The node watches a registration's key on Nostr for every session | [Session key after the PAKE](#31-a-session-key-after-the-pake) | The node sees attempts, not sessions |
| The node sees the address and the digits together | [Requests through another node](#32-requests-through-another-node) | One node sees the address, another the digits |
| TURN sees both ends' addresses | [Two TURN servers in a chain](#33-two-turn-servers-in-a-chain) | Each TURN sees one end |
| The two Chirps see each other's addresses | [Relay only](#34-relay-only) | Neither does |

### 3.1 A session key after the PAKE

The controller already listens under a Nostr key made for each attempt
(`pakeSessionID` in `client/pake.go`). The owner listens under the key it
registered, and every session's signalling reaches that key, which the node
handed out.

After the PAKE, the owner makes a new Nostr key for that session and sends its
public half to the controller in a message signed with the ed25519 key the
PAKE proved, gift-wrapped to the controller's key. Everything after goes to
keys the node never saw. The registered key then receives only the PAKE's
first messages: the node can count attempts and time them, and no more.

This needs no new cryptography and no change to the server, and it works with
one node as well as many. It is the first part of section 3 to build.

### 3.2 Requests through another node

A Chirp sends its registration, its renewals and its lookups to its node B
through another node A, as in Oblivious HTTP (RFC 9458): the request is sealed
with HPKE to a key B publishes in `/.well-known/chirp-node`, signed by its node
key. A forwards it and the answer without being able to open either. Go 1.26
has `crypto/hpke`, so this adds no dependency.

- A sees the Chirp's address, the time and the size, and that it is talking
  to B. Not the digits, the contact or the answer.
- B sees the digits and answers. Not the address: only A's.
- The Chirp picks A from a different operator than B (the list carries
  `operator`).

**What it costs.** A lookup takes one more hop, and the throttle on guessing
moves: it is per origin, and B no longer sees the origin.

#### Who counts the misses

A sees the origin and B knows whether a lookup missed, so B tells A. Next to
the sealed answer, in the clear, B marks a lookup that missed.

- **A counts misses per origin**, by part 2's rule unchanged: 5 per window of
  10 minutes × N. A Chirp picks A anew for each request, so its misses spread
  across nodes as they do without A.
- **B bounds the misses that arrive through each A**, with a window that
  grows with N likewise. The bound is sized for a node's worth of users, and
  its value waits for measurements. An A that does not limit its clients
  uses the bound up, and its own users are the ones refused.
- **Conformance measures A.** The prober sends six misses in a row through A
  from one origin and expects the sixth refused. An A that forwards it is
  non-conforming, as in the other proposal's *Conformance*.

What A learns is whether a lookup missed. It could already tell: a contact
and a `404` differ in size, and A sees sizes and times. What remains is an
operator using its own node as A, which skips its own limit. B's bound still
holds it, and the node went through probation under a known key and
operator.

**Anonymous tokens were set aside** (Privacy Pass, RFC 9576). Someone has to
issue them against an address. A central issuer is what this network is
built to do without. If each node issues the tokens it will later check, a
Chirp cannot know which node its next code lands on, so it fetches tokens
from every node, and every node then sees every user's address and when
they are online; today only A does. Tokens would hide the miss from A at
that price, and only once every answer is padded to one size. If
measurements show the miss is a leak that matters, this is where to return.

### 3.3 Two TURN servers in a chain

When the two Chirps cannot connect directly, each allocates on a TURN server
of a different operator and offers only that relay candidate. ICE pairs the
two relay candidates, and the path is

```
owner ─► TURN X ─► TURN Y ◄─ controller
```

X sees the owner and Y; Y sees the controller and X. Neither sees both ends.

The owner gets its credential from its own node, as in part 1. The controller
picks Y by the round trips it measures (remotekit's
[`relay-path-selection.md`](https://github.com/barahn/remotekit/blob/main/docs/relay-path-selection.md))
and tells the owner. The owner's node then signs a voucher,
`chirp-turn-voucher/v1 ∥ Y's id ∥ expiry ∥ nonce`, which the owner passes to
the controller as it would a credential. The controller presents it to Y,
which checks it against that node's key in the list and issues a credential.
Y learns the controller's address but not which node or owner sent it there,
beyond the voucher's signer. The credential still comes out of the owner's
registration.

**What it costs.** One more hop of latency, and the network carries every
relayed byte twice. Whether a chain is the default whenever TURN is needed, or
an option, is open question 2.

### 3.4 Relay only

With one TURN and only relay candidates (`iceTransportPolicy: relay`), the two
Chirps no longer learn each other's addresses, but the TURN sees both. With
the chain, nobody does. Offered as an option for the helper and the helped
alike.

### 3.5 What remains

- **Operators acting together.** A and B in 3.2, or X and Y in 3.3, can put
  back together what each saw. The list's operator field, and the status
  page's view of who runs what, are what keep them apart; neither proves they
  do not share data.
- **An observer of both ends' networks** can correlate timing, whatever the
  path.
- **Nostr relays** see the PAKE's first messages reach the registered key, and
  the sizes and times of every gift wrap after.
- **The status service** decides what the list holds. That stays central.

Users' documentation must say these, not "anonymous".

## 4. The owner's socket

Open question 6 of the other proposal: whether a public node keeps the
owner's `GET /ws`.

**Proposed:** in public mode, no socket. The owner renews its registration
with a new `PUT /api/peers/{peer}` every 40 s, and a registration not
renewed for 2 minutes expires, which keeps product-model decision 1's grace
as it is. The node learns that a write was recent, not a live connection's
start and length.

It also fits 3.2: a renewal is a request, so it travels through another node
like the rest, while a WebSocket would tie the owner's address to its
registration for as long as it ran. Private mode keeps the socket, since the
trusted path uses it to tell the owner a controller arrived.

## 5. Volunteers in front

Section 3 is as strong as the number of operators who would have to act
together. Dedicated relays grow that number slowly: each needs a server, a
domain and someone to keep it. A Chirp on a desktop that is on and reachable
can add to it, if its user chooses, as Snowflake's volunteers do for Tor.

**What a volunteer does.** Only the part of the node in front, A in 3.2: it
forwards sealed requests to nodes of the list and counts misses per origin.
It cannot open what it forwards, it sees addresses and sizes as any A does,
and a Chirp whose volunteer leaves picks another.

**What a volunteer does not do.**

- **Hold codes.** Rendezvous hashing needs a list that changes rarely, and a
  node that leaves loses its codes (part 2). Desktops come and go.
- **Relay sessions.** Bandwidth is what a session costs, and the network's
  answer to that cost is the dedicated relays and their TURN (part 1), run
  by whoever offers one. A home uplink is not asked to carry a stranger's
  screen.

So volunteers add operators, which is what section 3 needs, and add no
capacity, which the relays provide.

**What it would change.**

- **A class of its own in the signed list**, never in the rendezvous set. A
  node bounds the misses of the whole class together, not of each volunteer:
  a volunteer costs a click, and a bound for each would hand an attacker one
  budget per click.
- **Reached by address and port, known by its key.** A desktop has no domain
  or certificate, which joining asks of a node today
  ([`joining-the-network.md`](joining-the-network.md)). Requests are already
  sealed to the node's key, so a volunteer needs none for what it carries.
- **Consent that says what is published.** The list is public and signed,
  so a volunteer's home address is in it.

None of this is designed yet. Open questions 4 to 8 are what stands between
this section and a step in the order of work.

## Order of work

Each step is test-first, and each lands only once this proposal is approved.

1. **TURN credentials on the relay** (part 1). Independent of the network,
   and it lets two Chirps behind carrier-grade NAT connect. Moves Chirp to
   remotekit v0.8.0.
2. **The session key after the PAKE** (3.1). Client only.
3. **Renewals instead of the socket in public mode** (part 4), replacing the
   owner's socket that barahn/chirp#170 kept.
4. **Rendezvous hashing** (part 2), with phase 6's signed list and the miss
   budget divided by its length.
5. **Requests through another node** (3.2).
6. **The TURN chain** (3.3), with remotekit's relay selection.

## Open questions

1. ICE restart with fresh credentials, in remotekit, for a session whose
   network changes midway. Not needed for step 1. (A credential's lifetime
   against long sessions was open here, and so was where the credential
   generator comes from; part 1 settles both.)
2. Whether the TURN chain is the default whenever TURN is needed, or an
   option.
3. Whether section 3's requests go through a node of the network, or also
   through relays outside it.

4. Volunteers: whether their addresses are published in the signed list, or
   handed out one at a time by a node, which then chooses each Chirp's node
   in front.
5. Volunteers: the bound a node sets on the whole class, and whether a
   Chirp may use a volunteer when a dedicated node of another operator is
   free.
6. Volunteers: how one is reached. Chirp opens no port today; a mapping
   asked of the router (PCP, UPnP) leaves out everyone behind a
   carrier-grade NAT, and a WebRTC path as in Snowflake does not, at a
   price in complexity.
7. Volunteers: what replaces 95 % uptime for a machine that sleeps, and
   what probation means for one.
8. Volunteers: how one stays current with security releases without an
   operator watching it.

Settled here, and no longer open: spending other people's codes (part 2),
and anonymous tokens against counting misses in the node in front (3.2).

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

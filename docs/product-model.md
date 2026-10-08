# How Chirp works

This is the authoritative description of the product: what a person does, what
each side sees, and what that commits Chirp to. Where the code or another
document disagrees with it, this page wins and the other is out of date.

## One binary, the same on both sides

Anyone downloads Chirp from the site and runs it. There is nothing to install
and no account to create. Every running Chirp is identical, and every one
shows two things as soon as it starts:

- **its own code** — a short number that lets someone reach this machine;
- **a share link** — the same capability, in a form that can be sent instead of
  read aloud.

There is no "technician build" and no "customer build". Who helps whom is
decided by who reads a code to whom, not by which program they installed.

## Direction of control

User 2 needs help. User 2 reads their code to user 1. User 1 types it into
their own Chirp. **User 1 then controls user 2's machine** — but only after
user 2 agrees, in a prompt their own Chirp shows on their own machine.

Control never flows the other way as a side effect. If user 2 wants to see
user 1's screen, that is a separate connection, started from user 1's code, with
user 1's own consent.

## The two ways to connect

**Dictated code.** User 2 reads the code shown on their screen. User 1 types it.

**Shared link.** User 2 sends the link instead. Every time someone uses the
link, a **new single-use code** is created for that one connection. A code that
leaks after it has been used opens nothing. Someone who opens the link and does
not have Chirp yet gets it — and from then on is a Chirp user like any other,
with their own code and their own link.

Both ways end at the same place: user 2's Chirp asks user 2 whether to let this
connection in.

## Consent

Consent happens **on the controlled machine, in Chirp's own prompt** — never in
a web page, never on the controlling side. The prompt says who is asking and
what they are asking for (see the permissions below), and nothing starts until
user 2 accepts. Declining ends that connection for good.

Because nobody registers, **the name a controlling user gives is whatever they
typed**. The prompt must present it as a claim, not as an identity. What user 2
is really deciding is whether they are expecting this connection right now.

Permissions requested by default: viewing the screen, remote control, clipboard
sync and file transfer. They are the free core and are never gated behind a
licence or a tier.

## What follows for security

These are consequences of the model, stated so nobody has to rediscover them:

- **A code is a one-time capability.** Guessing a live code does not grant
  control, but it does put a consent prompt on a stranger's screen. The defences
  are the per-origin redemption throttle (remotekit's `rendezvous`) and a prompt
  unmistakable enough that an unexpected one is declined.
- **A link is a bearer capability that keeps working.** Codes it mints are
  single-use, but the link itself mints again until it stops being valid. Every
  connection it creates still needs user 2's consent. A link lives only while
  its sharer's Chirp runs, and can be revoked — see Decisions, below.
- **The relay is trusted, and self-hosting is how you avoid trusting someone
  else's.** A relay sees the codes it routes by, so it could impersonate either
  end. That is the same model as Barahn and every mainstream remote-support tool,
  and it is compatible with Chirp's commitment that a hosted service is a
  convenience and never a requirement: anyone who does not want to trust a hosted
  relay runs their own. Removing that trust entirely would need a
  password-authenticated key exchange over a secret the relay never sees, which
  means changing the code format. That is possible future hardening, not a
  requirement — see `HANDOFF.md` §5.5.

## Decisions

Decided by the maintainer:

1. **Link lifetime and revocation.** A link is valid only while the sharer's
   Chirp is running, can be revoked from it, and is replaced on restart. A
   Chirp that disappears without saying so (a crash, a lost connection) is
   taken to have stopped once it has been away from the relay for **2
   minutes**: its code and link stop working and any session with it ends.
   Coming back within that time keeps both, so a network blip does not break a
   link already sent. Operators can change it (`SetPeerGrace`).
2. **The code shown at start is single-use.** Once it is used it is replaced,
   for the same reason a link mints a fresh code per use: a code that has done
   its job opens nothing.

3. **At most 30 simultaneous sessions per address.** One network address may
   hold at most 30 sessions open as controller at once; the 31st redemption is
   refused (`429`) without spending the code, and ending a session frees a
   place. Since nobody has an account, the address is the only "user" the
   relay can count — people behind one NAT share the 30. Operators can change
   it (`SetMaxSessionsPerOrigin`).

   The same address may also have at most 30 running Chirps registered with
   a live code at once; the 31st is refused (`429`). A Chirp that quits
   without saying so gives its place back when its code expires. Operators
   can change it (`SetMaxPeersPerOrigin`).

Still open, to decide when the download site exists:

4. **How a link reaches a freshly downloaded Chirp.** With no installation there
   is no pre-registered handler, so the link's token has to get from the browser
   into the binary somehow: a custom URL scheme registered on first run, the
   token carried in the download itself, or pasted on first start. Each trades
   convenience against how much the download page must be trusted.

## Where the pieces live

The mechanism behind this — allocating and redeeming codes, minting codes from a
link, throttling guesses, pairing two peers and relaying their signalling — is
not specific to Chirp, and belongs in
[remotekit](https://github.com/barahn/remotekit). The flow, the defaults, the
consent prompt and the binary are Chirp's. The proposed split is in
[rendezvous-proposal.md](rendezvous-proposal.md); the general rule is in
[ECOSYSTEM.md](../ECOSYSTEM.md).

## What the code does today

The session layer and the signalling relay implement this model: a running
Chirp registers and gets its code and link, whoever redeems the code opens a
session to control it, and consent is asked of the owner over its own socket.
It also offers the lookup that decision 5 reduces the public server to: an
owner registers how it can be reached over Nostr, and a code or a link returns
exactly that, with nothing relayed. There is no binary yet, so no prompt is
drawn. `HANDOFF.md` §4.5 and §4.6 describe the endpoints and the known
limits.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

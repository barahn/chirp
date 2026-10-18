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
not have Chirp yet gets it and pastes the link into it (decision 4) — and from
then on is a Chirp user like any other, with their own code and their own link.

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

Permissions requested by default: viewing the screen, remote control and file
transfer. They are the free core and are never gated behind a licence or a
tier. The clipboard is part of that core too. It is a switch in the prompt,
off until the owner turns it on, worded so they know that what either of them
copies during the session goes to the other. Once on, it syncs both ways on
its own, as the tools people know do, and the owner is told of each paste into
their computer and each copy that leaves it (barahn/chirp#134). What a
password manager marks as secret never leaves, and nothing copied before the
session does (barahn/remotekit#130). From v0.3.1 until then the owner's Chirp
withheld the clipboard altogether, because it went to the controller without
the owner being asked or told (GHSA-8jh9-jw95-3v27).

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
- **Today the relay is trusted; before any public relay it must not be.** A
  relay that sees the codes it routes by could impersonate either end. That is
  acceptable for a relay its users run or trust themselves, which is what the
  first runnable Chirp ships with. It is not acceptable for a public service
  run for strangers, so no public relay is offered on that model. Decision 5
  removes the trust: a secret part of the code that the network never sees,
  and a password-authenticated key exchange (PAKE) over it that binds the two
  ends.

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

4. **A link reaches a freshly downloaded Chirp by being pasted into it.** The
   page a share link opens says what to do: if you have Chirp, open it and
   paste this link; if not, download it, run it, and paste this link. The page
   offers a button to copy the link. Chirp has a place to paste a link, beside
   the place to type a code, and both lead to the same consent prompt on the
   sharer's machine.

   - **The download is the same for everyone.** The token is never carried in
     the download: not in its contents, not in its file name. Every copy of a
     release is byte-identical, so it can be checked against the published
     hash and signature, and the site never has to be trusted to personalise
     a binary.
   - **Chirp reads the clipboard only when the user pastes.** It never looks
     at the clipboard on its own, at start or at any other time, outside a
     session whose owner turned the clipboard on (barahn/chirp#134).
   - **Chirp registers no URL handler.** A handler is a change to the system
     that outlives the program, which is installation by another name, and it
     would not help the first link anyway.
   - **Pasting a link makes you the one who controls, never the one
     controlled.** The person who pastes is user 1; the consent prompt appears
     on the sharer's machine. "Paste this link into Chirp" therefore cannot be
     used to take over the machine of the person who does it.

   Chirp accepts the link as the page shows it or as the bare token, and
   checks its form before contacting the relay, where every use is metered
   (`POST /api/links/{token}/sessions`). The token should travel in the link's
   fragment (after `#`), which browsers do not send, so that the site serving
   the page never receives it.

   The cost is one step more than a click. It was chosen over a token carried
   in the download, which leaves the token in the downloads folder and history
   and makes the page a party to every binary, and over reading the clipboard
   at start, which looks at data the user did not offer.

5. **The public server is blind to the traffic, and signalling travels over
   Nostr relays. It comes after the first runnable Chirp.** This replaces the
   idea that every running Chirp would be a node relaying for others: remotekit
   already carries signalling over public Nostr relays, encrypted end to end,
   so no node network has to be invented to get a carrier that sees nothing.

   - **The code becomes 6 + 4 digits**, read as `482-913-5071`. The first six
     are the *routing* part: the network sees them, and they lead to the
     owner. The last four are the *secret*: they never leave the two people,
     and are the password of a PAKE that binds the two ends. Someone in the
     middle gets one guess in 10,000 per attempt, and a wrong guess fails
     visibly to both ends. A link carries both parts.
   - **Chirp's public server does three things only:** it offers the download,
     serves the page a link opens, and looks up a routing part. The lookup
     returns the owner's session key and the relays it listens on, which the
     owner's Chirp registered when it started. It never sees the secret or a
     single signalling message.
   - **Signalling goes over Nostr relays** through remotekit's `signal/nostr`.
     Every message is gift-wrapped and encrypted to the other end under keys
     made for that session. A relay learns that a key received something,
     when and how much, and nothing of the content. remotekit's signed
     signalling and consent enforcement run on top, as they do today.
   - **Abuse limits stay where the requests arrive.** The lookup keeps the
     per-origin throttle on routing parts and the cap on registrations
     (decision 3), and the PAKE bounds guessing the secret.
   - **Self-hosting keeps working with zero restrictions.** A self-hoster runs
     their own lookup and, if they want, their own Nostr relay. The relay that
     exists today also remains, for anyone who prefers a single server they
     trust.
   - **The order of work.** The first runnable Chirp ships with today's relay
     and six-digit codes, for a relay its users trust. The blind design is the
     next milestone, and it must be in place before Chirp offers a public
     relay to anyone. Changing the code format then is accepted: Chirp is
     pre-1.0.

6. **Chirp's domain is `usechirp.app`.** It was registered on 2026-10-10.
   It carries the website, and the page a link opens is part of it, so the
   links people send each other carry the domain too. That makes it the
   address people will learn to trust. "Use Chirp" reads the same in
   Portuguese and in English, and `.app` is preloaded as HTTPS-only in
   browsers, so a link to it never opens over plain HTTP.

   - **The site and the link use it first.** The site is static, served by
     GitHub Pages behind Cloudflare's proxy, which adds the security headers
     Pages cannot set (docs/devsecops.md): it offers the download and the
     page a link opens (decision 4), and needs no relay. *Copy link* copies
     `https://usechirp.app/l#<token>`, which works whatever relay its users
     run, because the page only says what to do with the link.
   - **The relay and the lookup do not, yet.** They come with the public
     server of decision 5. Until then a Chirp reaches only a relay its users
     run or trust, given with `-relay`, so relay examples and tests keep
     names reserved for examples (RFC 2606). Showing `usechirp.app` there
     would suggest a public relay that is not offered yet.

## Where the pieces live

The mechanism behind this — allocating and redeeming codes, minting codes from a
link, throttling guesses, pairing two peers and relaying their signalling — is
not specific to Chirp, and belongs in
[remotekit](https://github.com/barahn/remotekit). The flow, the defaults, the
consent prompt and the binary are Chirp's. The proposed split is in
[rendezvous-proposal.md](rendezvous-proposal.md); the general rule is in
[ECOSYSTEM.md](../ECOSYSTEM.md).

## What the code does today

The session layer and the signalling relay implement this model, through a
relay that sees the codes (the trusted arrangement decision 5 replaces before
any public relay): a running Chirp registers and gets its code and link,
whoever redeems the code opens a session to control it, and consent is asked
of the owner over its own socket. It also offers the lookup that decision 5
reduces the public server to: an owner registers how it can be reached over
Nostr, and a code or a link returns exactly that, with nothing relayed. With
`-nostr`, the two Chirps then meet over Nostr relays, and before anyone is
asked they run the PAKE over the code's four secret digits, which this Chirp
makes and the server never sees: the code reads `482-913-5071`, and each end
comes out with the other's key, which signs the session that follows.
`HANDOFF.md` §4.5 to §4.8 describe the endpoints and the known limits.

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

# How Chirp looks, sounds and behaves

This page proposes Chirp's user experience: the words it uses, what each window
shows in each state, how it asks for attention, and how its help is organised.
It is modelled on how [TunnelBear](https://help.tunnelbear.com/hc/en-us/categories/360000518011-Getting-to-know-your-Bear)
makes a security product feel simple, with one deliberate exception, explained
below. [product-model.md](product-model.md) is the authority on what Chirp
does; where this page disagrees with it, this page is wrong.

**Status: proposal.** The open decisions are listed at the end. Nothing here is
drawn yet: `cmd/chirp` does not exist (barahn/chirp#10), and its UI will be
Fyne (HANDOFF §5.6).

## The name is the metaphor

A chirp is the short call a bird makes so that another one can find it. That
is exactly what Chirp's code and link are: a call you send to one person you
trust, so they can find your machine. Everything friendly in Chirp grows out
of that single image, and nothing else needs inventing.

| Thing | Word on screen | Word in marketing and help |
| --- | --- | --- |
| The product | Chirp | Chirp |
| The code | your code | your chirp ("read your chirp to someone you trust") |
| The link | your link | your chirp, sent as a link |
| Typing someone else's code | Help someone | answer a chirp |
| The mascot | the bird | your bird |
| Consent | Allow / Decline | — |
| A session | session | — |

**Brand words never reach a control that gives or takes control, and never
an error message.** Remote-support scams work by making the victim feel safe
enough to click Allow, and a cheerful prompt does the scammer's work for them.
So the bird, the puns and the namesake sound live in the main window, the idle
and waiting states, the help centre and the site. The consent prompt and the
session bar are plain and serious (§ Consent, § During a session). This is
the exception to TunnelBear: a VPN's playful tone costs nothing, Chirp's would.

**The word alone is common in software, so the name never stands alone where
it identifies the product.** The wordmark always appears with the bird, and
store listings, the site title and the download page carry a fixed
descriptor: *Chirp — remote help* (*Chirp — ajuda remota*).

The mascot is a small, round songbird. It is deliberately not a named species
and has no name of its own: it is *your bird*, as TunnelBear's is *your Bear*.

## What Chirp takes from TunnelBear

| TunnelBear | Chirp |
| --- | --- |
| One switch; the map turns green and the bear sits in the tunnel | One hero element, the code. The bird and a status label show the state; the tray icon shows it when the window is hidden |
| "How do I know TunnelBear is working?" | "How do I know nobody is on my screen?" — the session bar and the tray icon answer it, always |
| *Fastest* is preselected; countries hide behind an arrow | The default server is preselected; a self-hosted server and STUN/TURN hide under *Advanced* |
| GhostBear's help says when **not** to turn it on | The same for *Advanced*: "if Chirp already connects, leave this alone" |
| The OS warning is explained before it appears ("this is a standard warning… TunnelBear will NEVER do this") | Gatekeeper, SmartScreen, macOS Screen Recording and Accessibility, and the Wayland portal are explained before they appear (§ Before the system asks) |
| "Why should I trust TunnelBear?" and a published audit | "Why should I trust Chirp?" and a list of what Chirp never does (§ Help) |
| Feature names in the mascot's voice (VigilantBear, GhostBear, SplitBear) | A few names in the bird's voice, for help articles only (§ Feature names) |
| A skippable Quick Guide | Hints inside the main window, so nothing has to remember that the guide was seen (open decision 1, barahn/chirp#49) |

What Chirp does **not** take: the installer, the account, email
verification and two-factor login. *Download it, run it, read your chirp* is
the whole setup, and it is the thing to sell.

## The bird's states

TunnelBear's map tells you its state at a glance. Chirp's bird does the same
in the main window. The bird is never drawn in the consent prompt or the
session bar.

**On the machine that shares its code (the owner):**

| State | What the server says (HANDOFF §4.5) | Window | Bird | Tray |
| --- | --- | --- | --- | --- |
| Starting | `POST /api/peers` in flight | code placeholder | landing | outline |
| Ready | registered, owner socket open | code and link | perched, blinks now and then | outline |
| Reconnecting | owner socket closed, within the 2-minute grace | code kept, dimmed: "Reconnecting — your code still works when you're back" | looking around | outline, dimmed |
| Offline | grace passed, or the server unreachable | "Your old code and link stopped working." New ones on reconnect | asleep, head tucked | outline, crossed |
| Code replaced | the start code was used or expired (decision 2; `DefaultCodeTTL`, 10 min) | the new code is highlighted for a few seconds so nobody reads the old one | a short hop | — |
| Asked | `chirp.consent_request` received | the consent prompt (§ Consent) | not drawn | dot added |
| In a session | `active` | the session bar (§ During a session) | not drawn | changes shape and colour |
| Session ended | `terminated` | "Session ended. You have a new code." | back on its perch | outline |

The code shown is `formatted_code` as the server returns it: six digits today,
`482-913-5071` once decision 5 lands. The window shows how long the code has
left only in its last two minutes, so a code read aloud does not expire
mid-sentence without warning.

**On the machine that types a code (the controller):**

| State | Window | Bird |
| --- | --- | --- |
| Typing | the field checks the form of a code or link before anything is sent (decision 4) | perched |
| Asking | "Waiting for them to allow you in. Nothing starts without their OK." | chirping, notes rising |
| Allowed | the remote screen opens, and the namesake sound plays (§ Sound) | flies off-screen |
| Refused, expired or failed | the message from § Messages | perched again |

## Sound

Chirp makes its namesake sound in one place only: **on the helper's side,
when the other person allows them in.** It is the one moment that is good news
for both people and gives nothing away.

It never plays on the machine being controlled. There, the consent prompt asks
for attention with the system's own means — a flashing taskbar button, a
bouncing dock icon — so that a friendly sound can never become the cue to
click Allow.

## The main window

From top to bottom:

1. **Header.** The bird, the wordmark, and a status label (*Ready*,
   *Reconnecting*, *Offline*).
2. **Your code.** Large, monospaced, grouped as the server formats it, with a
   copy button. Under it: *Copy link* and *New code*. *New code* replaces both
   the code and the link (decision 1); today that is a `DELETE` and a fresh
   `POST /api/peers`. The hint under the code: "Read it only to someone you
   called."
3. **Help someone.** One field that takes a code or a pasted link, and a
   *Connect* button. Chirp reads the clipboard only when the user pastes
   (decision 4). The hint: "Typing their code lets you control their
   computer, after they allow it."

These hints replace TunnelBear's Quick Guide. They are always there, short
enough not to need dismissing.

## Consent

The prompt Chirp draws on the owner's machine when someone redeems its code.
It is the most important screen in the product (barahn/chirp#11), and it is
written to make an unexpected request easy to decline.

- **Decline has the focus.** *Allow* becomes clickable only after three
  seconds, so neither a reflexive Enter nor a click aimed at something else
  lets anyone in.
- **The name is a claim.** It is shown in quotes, followed by "Chirp can't
  check names", because `name_verified` is always false.
- **The permissions are listed** in words, from `session.DefaultPermissions`.
  Granting only some of them is not possible today: remotekit's
  `bark.ConsentResponsePayload` carries a single `Granted`. Per-permission
  consent is open decision 4 (barahn/chirp#52).
- **A fixed warning** against the usual scam script.
- **A countdown** to the session's deadline, after which the prompt declines
  by itself. The consent request carries only a timestamp; the deadline is
  `expires_at` from `GET /api/sessions/{id}`, or a field to add to
  `chirp.consent_request`.
- **It stays on top** of other windows, and carries no bird and no brand word.

| | English | Português (Brasil) |
| --- | --- | --- |
| Title | Someone wants to control this computer | Alguém quer controlar este computador |
| Name | They say they're "João da TI". Chirp can't check names. | Diz se chamar "João da TI". O Chirp não verifica nomes. |
| Permissions | See your screen · use your mouse and keyboard · share your clipboard · send and receive files | Ver sua tela · usar seu mouse e teclado · compartilhar a área de transferência · enviar e receber arquivos |
| Warning | Allow only if you asked this person for help just now. Banks, government agencies and Microsoft never ask for this. | Só permita se você pediu ajuda a essa pessoa agora. Bancos, órgãos do governo e a Microsoft nunca pedem isso. |
| Countdown | Declines by itself in 1:40 | Recusa sozinho em 1:40 |
| Buttons | Decline · Allow (3) | Recusar · Permitir (3) |

The wording of this prompt is security-critical in every language it ships
in. A translation is reviewed as a change to the prompt, not as a string.

## During a session

**On the owner's machine,** a bar stays on top of every window for as long as
the session lasts: *"João da TI" is controlling this computer · 04:12 ·
End*. *End* acts at once, without asking for confirmation. The tray icon
changes shape as well as colour, so the state does not depend on telling
colours apart. When the session ends the bar goes, and the main window says
the code has been replaced.

Fyne may not offer a portable always-on-top window; check before building the
bar, and plan for a small platform-specific shim if it does not.

**On the controller's machine,** the remote screen fills the window, with a
toolbar for monitors, clipboard, files and *End*.

## The tray

The tray icon exists only while Chirp runs. It has three shapes: idle (the
bird's outline), asked (outline with a dot), in a session (a distinct, filled
shape). Its menu: *Show Chirp*, *Copy code*, *Copy link*, *New code*,
*End session* (only during one), *Quit*.

## Messages

Each says what happened and what to do, with no error codes, no brand words,
and no first person.

| Situation | English | Português (Brasil) |
| --- | --- | --- |
| Server unreachable | You're offline. Your code works again once you're back. | Sem conexão. Seu código volta a funcionar quando a conexão voltar. |
| `404` — unknown, used, expired or malformed | That code doesn't exist or was already used. Ask for a new one. | Esse código não existe ou já foi usado. Peça um novo. |
| `409` — the owner is busy | That person is already getting help from someone else. | Essa pessoa já está recebendo ajuda de outra pessoa. |
| `429` — throttled or over a cap | Too many tries from this network. Try again in a few minutes. | Muitas tentativas a partir desta rede. Tente de novo em alguns minutos. |
| `503` — no codes left | Chirp's server is busy. Try again shortly. | O servidor do Chirp está ocupado. Tente de novo daqui a pouco. |
| Declined | They declined. | A pessoa recusou. |
| Expired undecided | They didn't answer in time. | A pessoa não respondeu a tempo. |
| PAKE failed (decision 5), shown at both ends | The last four digits don't match. Check them with the other person. | Os quatro últimos dígitos não conferem. Confirme com a outra pessoa. |

`404` deliberately does not say which of its causes applies: telling them
apart would help someone sweeping codes.

## Before the system asks

TunnelBear explains each operating-system warning before it appears. Chirp
needs the same, mostly on the machine being controlled:

- **Download warnings** (SmartScreen, Gatekeeper). The download page shows,
  for each system, the warning it will show and which button to press.
- **macOS Screen Recording and Accessibility.** macOS usually applies Screen
  Recording only after the app is reopened, and reopening Chirp replaces its
  code and link (decision 1). If that holds, asking for the permission after
  someone has been allowed in would end the session it was meant for. The
  main window should therefore check these permissions at start, before the
  code is shared (open decision 3, barahn/chirp#51). Confirm against
  remotekit's `screen` on macOS.
- **Wayland portals.** After *Allow*, the system asks once more which screen
  to share. The prompt's last line says so, so the second dialog is expected.
- **Windows administrator prompts** appear on a secure desktop the helper
  cannot see. A help article tells both people what is happening.

## Help: "Getting to know Chirp"

The help centre follows TunnelBear's four sections:

- **Setup guides** — Windows, macOS, Linux. Each is *download, run, read your
  chirp*, plus the system warnings above. None mentions installing, because
  there is nothing to install.
- **FAQ** — What does Chirp do? How do I know nobody is on my screen? I got a
  request I wasn't expecting: what now? Which systems does Chirp run on?
- **Privacy and security** — Why should I trust Chirp? What does the server
  see (today's relay, and decision 5's blind lookup)? What Chirp never does.
- **Features** — Homing, Pigeon Post, running your own server.

**What Chirp never does**, the counterpart of TunnelBear's "will NEVER":

- installs itself, or starts with your computer;
- reads your clipboard unless you paste;
- lets anyone in without you pressing *Allow* on your own screen;
- keeps your code alive after you quit;
- puts screen sharing, control, files, clipboard or multiple monitors behind
  a payment.

Help articles may end on a light line in the bird's voice. Articles about
consent, scams and security do not.

## Feature names

Names in the bird's voice go only to features that have a help article or a
setting, and never onto a control that gives or takes control.

- **Homing** — Chirp finds its way back after a network blip of up to two
  minutes, with the same code and link (decision 1).
- **Pigeon Post** — sending and receiving files during a session.

Avoid *Nest* (a Google brand, and the name of the abandoned web page) and
*Tweet*.

## Open decisions

Each has an issue, on the roadmap project.

1. **Does Chirp write anything to disk** — a name, a self-hosted server
   address, a muted sound? Decision 4 treats a URL handler as installation by
   another name, and a settings file is a lighter form of the same thing. The
   proposal assumes nothing is written, which is why hints replace a guide
   (barahn/chirp#49).
2. **Does closing the window quit Chirp?** The proposal says yes. Hiding to
   the tray with a live code would be a quiet standing agent, which the
   product model rejects (barahn/chirp#50).
3. **When does Chirp ask for macOS permissions?** At start, before the code
   is shared, if Screen Recording does require a relaunch (barahn/chirp#51).
4. **Per-permission consent.** Needs a change to remotekit's `bark`
   consent response. Out of the first runnable Chirp (barahn/chirp#52).

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

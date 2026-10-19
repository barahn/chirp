# How Chirp looks, sounds and behaves

This page proposes Chirp's user experience: the words it uses, what each window
shows in each state, how it asks for attention, and how its help is organised.
It is modelled on how [TunnelBear](https://help.tunnelbear.com/hc/en-us/categories/360000518011-Getting-to-know-your-Bear)
makes a security product feel simple, with one deliberate exception, explained
below. [product-model.md](product-model.md) is the authority on what Chirp
does; where this page disagrees with it, this page is wrong.

**Status: proposal, partly built.** The open decisions are listed at the end.
`cmd/chirp` has the main window, the helper's flow (barahn/chirp#10), the
consent prompt (barahn/chirp#11), the controller's view of the other screen
(barahn/chirp#12) and the owner's session bar (barahn/chirp#77), in the Warm
look (§ Visual direction). Not built yet: keeping the prompt and the bar on
top, the controller's toolbar beyond *End*, the tray and the sound; each has
an issue. Mockups of the possible looks are linked from § Visual direction.

## The name is the metaphor

A chirp is the short call a bird makes so that another one can find it. That
is exactly what Chirp's code and link are: a call you send to one person you
trust, so they can find your machine. Everything friendly in Chirp grows out
of that single image, and nothing else needs inventing.

| Thing | Word on screen | Word in marketing and help |
| --- | --- | --- |
| The product | Chirp | Chirp |
| The code | your code | your chirp ("send your chirp to someone you trust") |
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
verification and two-factor login. *Download it, run it, send your chirp* is
the whole setup, and it is the thing to sell.

*Send* covers both ways to share: reading the code aloud and sending the
link. In Portuguese it is *Baixe, abra, mande seu chirp*; "manda o código" is
what a technician already says on the phone. The site stacks the three parts
as its title, one under the other, so their initials read down a column:
check what they spell in every language before changing a word. *Baixe. Abra.
Dite seu chirp.* once read B-A-D.

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

The code shown is `formatted_code` as the server returns it, six digits, when
sessions go through a relay its users trust. A Chirp that signals over Nostr
adds four secret digits of its own, which the server never sees:
`482-913-5071` (decision 5). Its link carries its own four after a dot. The window shows how long the code has
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
3. **Help someone.** A field for the helper's name, one that takes a code or
   a pasted link, and a *Connect* button. The name is what the other
   person's prompt shows, as a claim, so Chirp asks for it rather than send
   a blank; with nothing written to disk (open decision 1) it is typed each
   time. Chirp reads the clipboard only when the user pastes (decision 4).
   The hint: "Typing their code lets you control their computer, after they
   allow it."

These hints replace TunnelBear's Quick Guide. They are always there, short
enough not to need dismissing.

## Consent

The prompt Chirp draws on the owner's machine when someone redeems its code.
It is the most important screen in the product (barahn/chirp#11), and it is
written to make an unexpected request easy to decline. It is built in
`cmd/chirp/internal/ui/consent.go`, with a test for each point below.

- **Decline has the focus.** *Allow* becomes clickable only after three
  seconds, so neither a reflexive Enter nor a click aimed at something else
  lets anyone in.
- **The name is a claim.** It is shown in quotes, followed by "Chirp can't
  check names", because `name_verified` is always false. The controller
  chose it, so it is cleaned before it is shown: no line breaks or other
  control characters, no right-to-left overrides or other formatting
  characters, no double quotes that could close its own quotation, and at
  most 40 characters. A name like `Ana\nVerified by your bank` cannot pass
  for a line of the prompt.
- **Each permission is a switch, and only *See your screen* starts on**
  (open decision 4, barahn/chirp#52). The permissions are listed in words, from
  `session.DefaultPermissions`, each as a row that changes colour and shows a
  check mark when on, so the state never rests on colour alone. *See your
  screen* is fixed on, marked *Always*: no session goes without it, and
  remotekit's runner requires it. Mouse and keyboard, the clipboard and
  files start off; the owner turns on what the call needs, and *Allow* lets in
  what is on and nothing else. A hurried *Allow* during a scam call
  therefore gives away the picture alone. Tab reaches the rows and Space
  turns one; a click on a row hands the focus back to *Decline*, so Enter
  still declines. A permission Chirp does not know is shown as it came,
  off, so nothing is asked for unseen. With everything off, *Allow* stays
  disabled.
- **What is allowed is enforced on the owner's machine.** The owner's Chirp
  grants its runner only what was switched on, whatever the relay or the
  controller's Chirp do, and the runner tells the controller's Chirp what it
  may do (barahn/remotekit#118), so a refused permission shows as such
  rather than as a bug.
- **A fixed warning** against the usual scam script.
- **A countdown** to the session's deadline, after which the prompt declines
  by itself. The consent request carries only a timestamp, so the owner asks
  `GET /api/sessions/{id}` for the time left as the request arrives.
- **Closing the prompt declines.** Nothing answers on its own: an answer is
  the user pressing a button, or the prompt closing or running out of time,
  which both decline.
- **It asks for attention and carries no bird and no brand word.** It opens
  as its own window and asks for focus. Keeping it above every other window
  needs the per-OS shim the session bar uses (§ During a session).
- **After the answer** the main window says what was decided. A session that
  was let in can be ended there; since the relay ends a session only when its
  owner gives the registration up, ending one also replaces the code and link.
- **The prompt answers the relay; it does not stream anything.** Making the
  screen and input obey the answer is the session runner's job: remotekit's
  `stream.Runner`, granted what the owner allowed (barahn/chirp#12). The
  controller sees the screen in a window of its own (§ During a session).

| | English | Português (Brasil) |
| --- | --- | --- |
| Title | Someone wants to control this computer | Alguém quer controlar este computador |
| Name | They say they're "João da TI". Chirp can't check names. | Diz se chamar "João da TI". O Chirp não verifica nomes. |
| Permissions | See your screen *(Always)* · use your mouse and keyboard · share what you copy, both ways, while the session lasts · send and receive files | Ver sua tela *(Sempre)* · usar seu mouse e teclado · compartilhar o que vocês copiarem, nos dois sentidos, durante a sessão · enviar e receber arquivos |
| Under the permissions | Turn on only what this call needs. Allow lets in what is on. | Ligue só o que este atendimento precisa. Permitir libera o que estiver ligado. |
| Warning | Allow only if you asked this person for help just now. Banks, government agencies and big tech companies never ask for this. | Só permita se você pediu ajuda a essa pessoa agora. Bancos, órgãos do governo e grandes empresas de tecnologia nunca pedem isso. |
| Countdown | Declines by itself in 1:40 | Recusa sozinho em 1:40 |
| Buttons | Decline · Allow (3) | Recusar · Permitir (3) |

The wording of this prompt is security-critical in every language it ships
in. A translation is reviewed as a change to the prompt, not as a string.

## During a session

**On the owner's machine,** a bar stays on top of every window for as long as
the session lasts: *"João da TI" is controlling this computer · 04:12 ·
End* — or *is seeing this screen*, when the mouse and keyboard were not
allowed. *End* acts at once, without asking for confirmation. Under that
line, each permission allowed besides *See your screen* is the same switch
as in the prompt, and it only turns off: the owner takes one back at any
moment, it stops at once, and it stays off (*Taken back*) until a new
request. Turning one on mid-session is what a scammer asks for, so the bar
cannot. Taking the screen back is *End*. While files are allowed, the bar has
*Send a file*; when a file arrives it names it, with *Open folder*. Files land
in a Chirp folder in Downloads at either end, never replacing one already
there. Everything the person helping does with the clipboard and files --
each paste into the computer, each copy that leaves it, each file, a password
that was held back -- is told in a system notification (barahn/chirp#134).

While a session is on, Chirp has an icon in the tray, at either end, with
*Open Chirp* and *End session*; closing the main window then hides it there
instead of quitting. With no session there is no icon, and closing the window
quits (#50). When a session is over, the main window is the only one left. The tray icon
changes shape as well as colour, so the state does not depend on telling
colours apart. When the session ends the bar goes, and the main window says
the code has been replaced.

The bar is built (`cmd/chirp/internal/ui/sessionbar.go`, barahn/chirp#77):
it opens as its own window when a session the owner allowed starts being
served and closes when it ends, whichever end ended it. The name is cleaned
as in the prompt. Closing the bar ends the session, like *End*. Fyne has no
portable way to keep a window above the others, so a small per-OS shim does
(`cmd/chirp/pinbar_*.go`): on Windows the bar is owned by the main window,
which keeps it off the taskbar, and topmost; on X11 it asks the window
manager for `_NET_WM_STATE_ABOVE` and to skip the taskbar. Under Wayland and
on macOS nothing is done yet. The bar grows to show what it adds after it
opens. The consent prompt does not use the shim yet.

**On the controller's machine,** the remote screen fills the window, with a
toolbar for monitors, clipboard, files and *End*. So far the toolbar has
*Send a file* and *End*, and lines for what the other person has not allowed,
what the other computer says instead of its picture, and how the last file
went; the monitor picker is still to come (#79). *Send a file* (Pigeon Post)
sends one file, or several dropped on the window, one at a time, with the
progress as the owner's Chirp acknowledges it, while the owner allows files.
The clipboard needs no button: while the owner allows it, it syncs both ways
on its own. Closing the window ends the session too. The mouse and keyboard over the
picture go to the other computer, keys as the physical keys they are, so its
own layout decides what they type.

## The tray

The tray icon exists only while a session is on (barahn/chirp#134), at either
end. Its menu: *Open Chirp* and *End session*. Fyne cannot take its tray icon
down once shown, so Chirp hides it between sessions with `SetVisible`, from
Barahn's fork of `fyne.io/systray` until upstream has it. The shapes for a
waiting request and a session (#78) and the bird's balloons (#136) build on
it.

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

## Languages

Chirp speaks English and Brazilian Portuguese, and follows the language the
system shows itself in; Portuguese from Portugal gets the Brazilian words
rather than English (barahn/chirp#80). Where that language comes from:

- **Linux:** `LANGUAGE`, `LC_ALL`, `LC_MESSAGES` or `LANG`, so
  `LANGUAGE=pt_BR chirp` shows Portuguese on an English desktop.
- **Windows:** the display languages (*Settings → Time & language →
  Language*), the first in the user's order that Chirp speaks, and not the
  regional format, which only says how dates and numbers are written
  (barahn/chirp#104). Fyne's own words, such as an entry's Copy and Paste
  menu, still follow the regional format, so the two can differ when the
  display language and the format do.
- **macOS:** the system locale (`AppleLocale`), or the app's preferred
  localization once Chirp ships as an `.app` that declares its
  localizations.

The words are written in English in `cmd/chirp/internal/ui`, and each English
text is the key to its translation in `cmd/chirp/internal/ui/translations/`.
A sentence that changes in English therefore shows in English until it is
translated again, never as the old translation of what it used to say, and
`TestEveryWordIsTranslated` fails until every language catches up. The tables
above are the source for the prompt and the messages; the translation files
follow them.

The Warm fonts' Latin subset covers Portuguese. Check their coverage before
adding a language that needs other letters.

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

- **Setup guides** — Windows, macOS, Linux. Each is *download, run, send your
  chirp*, plus the system warnings above. None mentions installing, because
  there is nothing to install.
- **FAQ** — What does Chirp do? How do I know nobody is on my screen? I got a
  request I wasn't expecting: what now? What can the other person do, and
  how do I choose? Which systems does Chirp run on?
- **Privacy and security** — Why should I trust Chirp? What does the server
  see (today's relay, and decision 5's blind lookup)? What Chirp never does.
- **Features** — Homing, Pigeon Post, running your own server.

**What Chirp never does**, the counterpart of TunnelBear's "will NEVER":

- installs itself, or starts with your computer;
- reads your clipboard unless you paste, or allowed it for a session;
- lets anyone in without you pressing *Allow* on your own screen;
- puts screen sharing, control, files, clipboard or multiple monitors behind
  a payment.

**You choose what they can do**, the article every setup guide links to:

- When someone asks to help you, Chirp lists what they want to do. Each item
  is a switch: coloured with a check mark when on, grey when off.
- Only *See your screen* is on at first, and it always is: helping starts
  with seeing. If they need to click or type for you, turn on *Use your
  mouse and keyboard*; files work the same way. Turn on only what the call
  needs.
- *Allow* lets in what is on, and nothing else. If you did not ask for
  help, press *Decline*: switching everything off is not the way to say no.
- During the session, the bar on your screen shows the same switches. Turn
  one off and it stops at once. To give it back they have to ask again.
- *End*, on the bar, stops everything.

It does not promise that a code dies the moment Chirp quits. Quitting
releases the code and link at once, but a Chirp that cannot reach its server
as it closes is released only when the server's grace runs out, two minutes
by default (decision 1). Nobody can be let in meanwhile, because no prompt
can be shown.

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

## Visual direction

**The direction is A · Warm**, for the product and for all of its marketing:
the site, the store listings and the help centre (barahn/chirp#53). In
`cmd/chirp` it is package `cmd/chirp/internal/warm`: a Fyne theme with the
tokens below in both variants, the three fonts, and the bird's poses as SVG.
Its contrast is measured in a test, and every text pair in both variants is
above 4.5:1 (the lowest is 5.38:1). The
design canvas holds it with the two that were weighed against it: the bird in
every state, the app and tray icons, the main screens in light and dark, and
the colour tokens mapped to Fyne —
[Chirp visual directions](https://claude.ai/artifact/JroqdRHaFU7wochyTrfMNB).
The screens there switch direction, dark mode and state from their *Tweaks*,
and the consent prompt, the helper's flow and the session bar can be clicked
through.

- **A · Warm, chosen.** An illustrated, filled bird, rounded shapes, yolk
  yellow for the bird and deep teal for actions; Fredoka, Nunito Sans and
  JetBrains Mono, all under the OFL, which barahn/chirp#54 accepted. The
  closest to TunnelBear.
- **B · Calm.** A line-drawn bird, quieter surfaces, teal and blue; IBM Plex
  Sans and IBM Plex Mono.
- **C · Native.** A silhouette bird used as a logo and little else, close to
  what Fyne draws by default; Noto Sans and Noto Sans Mono. The least work.

Some things were fixed whichever was chosen, and still are:

- **The consent prompt and the session bar look the same in every
  direction:** no bird, no accent colour, the warning in the warning colour,
  *Decline* focused. They change only between light and dark.
- **The tray icon changes shape, not only colour:** an outline when idle, a
  dot when someone is asking, a filled shape during a session. macOS gets
  one-colour template images.
- **Every colour has a light and a dark value** and maps to a Fyne theme
  colour name. The few Fyne has no name for — muted text, the bird's accent,
  the highlight tint, the warning background — are Chirp's own (`chirp.*`)
  and served by the same theme.

What that means in Fyne:

- **Fonts follow style, not role.** Fyne picks a font by text style (regular,
  bold, italic, monospace), so a separate display face (Warm's Fredoka) needs
  Bold mapped to it, or the wordmark drawn as an image. Each bundled font
  file adds to the binary.
- **The bird is SVG**, one file per pose, bundled with the binary and drawn
  with `canvas.NewImageFromResource`; Fyne rasterises SVG itself. Keep the
  paths plain: no filters, masks or text. Motion is swapping the pose and
  moving or fading it with Fyne's animation API; the canvas lists the
  durations.
- **The session bar needs a per-OS shim** to stay above every window
  (§ During a session).

The canvas is private until it is shared from its page.

## Open decisions

Each has an issue, on the roadmap project. The fourth and fifth are
decided.

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
4. **Per-permission consent — decided: the owner chooses each permission**
   (§ Consent, § During a session, barahn/chirp#52). Only *See your screen*
   starts on and it is fixed; the bar takes permissions back but does not
   give them.
5. **Which visual direction — decided: A · Warm** (§ Visual direction,
   barahn/chirp#53).

---

`SPDX-License-Identifier: AGPL-3.0-only`
Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

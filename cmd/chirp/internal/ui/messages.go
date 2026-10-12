// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/barahn/chirp/client"
)

// The words the window uses, from docs/ux.md. Kept apart from the widgets so
// they can be checked, and later translated, without a display.
//
// The rule from docs/ux.md holds here: each message says what happened and
// what to do, with no error codes, no brand words and no first person.

const (
	hintCode       = "Read it only to someone you called."
	hintHelp       = "Typing their code lets you control their computer, after they allow it."
	noteReplaced   = "New code. The old one no longer works."
	noteReconnect  = "Reconnecting. Your code still works when you're back."
	noteLost       = "Your old code and link stopped working. You'll get new ones when you're back online."
	noteOffline    = "You're offline. You'll get a code once Chirp reaches its server."
	noteTooMany    = "Too many Chirps are running on this network. Close one and try again."
	waitTitle      = "Waiting for them to let you in"
	waitText       = "Nothing starts without their OK. They're seeing your request on their own screen now."
	noCode         = "––– –––"
	noServerTitle  = "No Chirp server is set"
	noServerText   = "Start Chirp with -relay and the address of your server, or set CHIRP_RELAY. There is no public Chirp server yet."
	badServerTitle = "That server address doesn't work"
	remoteTitle    = "Their screen"
	remoteEnd      = "End"
)

// remoteNotice is what the remote screen says when the other computer sends a
// notice instead of its picture, or "" for one not worth showing.
func remoteNotice(kind string) string {
	switch kind {
	case "consent_required":
		return "They haven't allowed you to see their screen."
	case "capture_unavailable":
		return "Their computer can't share its screen right now. Ask them whether it allows screen recording."
	}
	return ""
}

// statusText names the owner's phase, for the label beside the wordmark.
func statusText(p client.OwnerPhase) string {
	switch p {
	case client.OwnerReady:
		return "Ready"
	case client.OwnerReconnecting:
		return "Reconnecting"
	case client.OwnerOffline:
		return "Offline"
	default:
		return "Starting"
	}
}

// ownerNote is the line under the code, or "" for none.
func ownerNote(ev client.OwnerEvent) string {
	switch {
	case ev.Phase == client.OwnerReconnecting:
		return noteReconnect
	case ev.Phase == client.OwnerOffline && errors.Is(ev.Err, client.ErrTooMany):
		return noteTooMany
	case ev.Phase == client.OwnerOffline && ev.Lost:
		return noteLost
	case ev.Phase == client.OwnerOffline:
		return noteOffline
	case ev.Replaced:
		return noteReplaced
	}
	return ""
}

// The consent prompt's words (docs/ux.md, Consent). Plain on purpose: no
// brand words, nothing that sounds like the product vouching for anyone.
const (
	promptTitle   = "Someone wants to control this computer"
	promptUnnamed = "They didn't give a name."
	promptNoCheck = "Chirp can't check names."
	promptAsking  = "They're asking to:"
	promptWarning = "Allow only if you asked this person for help just now. Banks, government agencies and big tech companies never ask for this."
	promptDecline = "Decline"
	promptAllow   = "Allow"
)

// claimMax is the longest name the prompt shows, in characters.
const claimMax = 40

// claim makes a typed name safe to show as a claim. The controller chose it,
// so it must not be able to pass for the prompt's own words: control and
// formatting characters go (a line break could start a fake line, and a
// right-to-left override could reverse what follows), double quotes become
// single ones (so the name cannot close its own quotation marks), runs of
// space collapse, and it stops at claimMax characters.
func claim(name string) string {
	var b strings.Builder
	n, space := 0, false
	for _, r := range name {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		case strings.ContainsRune("\"“”„‟«»", r):
			r = '\''
		}
		if n == claimMax {
			b.WriteRune('…')
			break
		}
		if space && n > 0 {
			b.WriteRune(' ')
			n++
		}
		space = false
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// promptName is the line that names who is asking.
func promptName(name string) string {
	c := claim(name)
	if c == "" {
		return promptUnnamed
	}
	return "They say they're “" + c + "”."
}

// permissionText names what a permission lets the other person do. A name
// Chirp does not know is shown as it came, so nothing is granted unseen.
func permissionText(p string) string {
	switch p {
	case "screen_view":
		return "See your screen"
	case "remote_control":
		return "Use your mouse and keyboard"
	case "clipboard":
		return "Share your clipboard"
	case "file_transfer":
		return "Send and receive files"
	}
	return "Something Chirp doesn't know: " + claim(p)
}

// promptCountdown says when the prompt declines by itself.
func promptCountdown(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("Declines by itself in %d:%02d", seconds/60, seconds%60)
}

// allowLabel is the Allow button's label while it is still locked.
func allowLabel(locked int) string {
	if locked > 0 {
		return fmt.Sprintf("%s (%d)", promptAllow, locked)
	}
	return promptAllow
}

// decidedNote tells the owner, in the main window, what they decided.
func decidedNote(name string, granted bool) string {
	who := "them"
	if c := claim(name); c != "" {
		who = "“" + c + "”"
	}
	if granted {
		return "You let " + who + " in. They can see your screen and use your computer until you end the session."
	}
	return "You declined " + who + ". Nobody got in."
}

// sessionOverNote is the main window's line once a session has ended.
const sessionOverNote = "The session is over. Nobody can see or use this computer now."

// answerProblem is shown when an answer did not reach the relay.
func answerProblem(granted bool) string {
	if granted {
		return "Your Allow didn't reach the Chirp server, so nobody got in. The request will decline by itself."
	}
	return "Your Decline didn't reach the Chirp server. The request will decline by itself anyway."
}

// helpProblem says why an attempt to help someone failed, and what to do.
func helpProblem(err error) string {
	var re *client.RelayError
	switch {
	case errors.Is(err, client.ErrNoName):
		return "Enter your name first. They'll see it in their prompt."
	case errors.Is(err, client.ErrNotATarget):
		return "Enter a code or paste a link first."
	case errors.Is(err, client.ErrNotFound):
		return "That code doesn't exist or was already used. Ask for a new one."
	case errors.Is(err, client.ErrBusy):
		return "That person is already getting help from someone else."
	case errors.Is(err, client.ErrTooMany):
		return "Too many tries from this network. Try again in a few minutes."
	case errors.As(err, &re) && re.Status == http.StatusServiceUnavailable:
		return "Chirp's server is busy. Try again shortly."
	case errors.Is(err, client.ErrUnavailable):
		return "Can't reach the Chirp server. Check your connection and try again."
	}
	return "Something went wrong. Try again."
}

// outcome is the title, text and button for an attempt that is over.
func outcome(p client.HelperPhase) (title, text, button string) {
	switch p {
	case client.HelperAllowed:
		return "You're in", "Their screen is open in its own window. End the session there or here when you're done.", "End"
	case client.HelperDeclined:
		return "They declined", "Nothing was shared. If they meant to let you in, ask them for a new code.", "Try again"
	case client.HelperExpired:
		return "They didn't answer in time", "Nothing was shared. Ask them for a new code.", "Try again"
	default:
		return "The session ended", "They closed Chirp, or the session ended.", "Done"
	}
}

// countdown formats seconds left as "Expires in 1:52".
func countdown(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("Expires in %d:%02d", seconds/60, seconds%60)
}

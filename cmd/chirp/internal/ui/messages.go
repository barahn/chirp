// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"errors"
	"fmt"
	"net/http"

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
)

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

// requestNote tells the owner a controller is asking. The prompt that answers
// is barahn/chirp#11; until it exists, nobody can be let in.
func requestNote(req *client.ConsentRequest) string {
	return fmt.Sprintf("Someone typed your code and says they're “%s”. Answering isn't built yet (barahn/chirp#11), so nothing will start.", req.Name)
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
		return "You're in", "Their screen comes in a later version of Chirp (barahn/chirp#12).", "End"
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

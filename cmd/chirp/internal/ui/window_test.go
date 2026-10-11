// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/barahn/chirp/client"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

const token = "aZ0-_aZ0-_aZ0-_aZ0-_aZ0-_aZ0-_aZ0-_aZ0-_abc"

var shown = client.Peer{ID: "p", Code: "482913", Formatted: "482-913", Link: token}

func newWindow(t *testing.T) *Window {
	t.Helper()
	return New(test.NewTempApp(t))
}

func TestTheWindowStartsWithoutACode(t *testing.T) {
	w := newWindow(t)
	if w.code.Text != noCode || w.status.Text != "Starting" {
		t.Errorf("starting window shows %q, %q", w.code.Text, w.status.Text)
	}
	if !w.copyCode.Disabled() || !w.copyLink.Disabled() || !w.newCode.Disabled() {
		t.Error("buttons that need a code are enabled before there is one")
	}
}

func TestTheCodeAndLinkCanBeCopied(t *testing.T) {
	w := newWindow(t)
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
	if w.code.Text != "482-913" || w.status.Text != "Ready" || w.note.Visible() {
		t.Fatalf("ready window shows %q, %q, note %v", w.code.Text, w.status.Text, w.note.Visible())
	}

	test.Tap(w.copyCode)
	if got := w.app.Clipboard().Content(); got != "482-913" {
		t.Errorf("copied code = %q", got)
	}
	test.Tap(w.copyLink)
	got := w.app.Clipboard().Content()
	if got != "https://usechirp.app/l#"+token {
		t.Errorf("copied link = %q", got)
	}
	// What one Chirp copies, another must accept when it is pasted.
	if target, err := client.ParseTarget(got); err != nil || target.Token != token {
		t.Errorf("ParseTarget(copied link) = %+v, %v", target, err)
	}
	if w.copyLink.Text != "Link copied" {
		t.Errorf("copy link button says %q after copying", w.copyLink.Text)
	}
}

func TestOwnerStatesShowWhatTheCodeIsWorth(t *testing.T) {
	for _, tc := range []struct {
		ev      client.OwnerEvent
		status  string
		code    string
		note    string
		canCopy bool
	}{
		{client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Replaced: true}, "Ready", "482-913", noteReplaced, true},
		{client.OwnerEvent{Phase: client.OwnerReconnecting, Peer: shown}, "Reconnecting", "482-913", noteReconnect, true},
		{client.OwnerEvent{Phase: client.OwnerOffline, Lost: true}, "Offline", noCode, noteLost, false},
		{client.OwnerEvent{Phase: client.OwnerOffline, Err: client.ErrUnavailable}, "Offline", noCode, noteOffline, false},
		{client.OwnerEvent{Phase: client.OwnerOffline, Err: &client.RelayError{Status: http.StatusTooManyRequests}}, "Offline", noCode, noteTooMany, false},
	} {
		w := newWindow(t)
		w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
		w.ApplyOwner(tc.ev)
		if w.status.Text != tc.status || w.code.Text != tc.code || w.note.Text != tc.note || !w.note.Visible() {
			t.Errorf("%+v shows %q, %q, %q", tc.ev, w.status.Text, w.code.Text, w.note.Text)
		}
		if w.copyCode.Disabled() == tc.canCopy {
			t.Errorf("%+v: copy enabled = %v, want %v", tc.ev, !w.copyCode.Disabled(), tc.canCopy)
		}
	}
}

func TestAConsentRequestOpensThePrompt(t *testing.T) {
	w := newWindow(t)
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Consent: &client.ConsentRequest{Name: "João da TI", SessionID: "s1", ExpiresIn: 100}})
	if w.prompt == nil || w.prompt.req.SessionID != "s1" {
		t.Fatal("no prompt for the request")
	}
	if w.request.Visible() {
		t.Error("the main window says something before the user has answered")
	}
}

func TestNewCodeAsksTheOwner(t *testing.T) {
	w := newWindow(t)
	asked := false
	w.OnNewCode = func() { asked = true }
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
	test.Tap(w.newCode)
	if !asked {
		t.Error("New code did not reach the owner")
	}
}

func TestConnectChecksTheFormBeforeAnythingIsSent(t *testing.T) {
	for _, tc := range []struct {
		name, target, problem string
	}{
		{"", "482 913", helpProblem(client.ErrNoName)},
		{"Ana", "hello", helpProblem(client.ErrNotATarget)},
	} {
		w := newWindow(t)
		w.OnConnect = func(string, string) { t.Errorf("%q, %q reached OnConnect", tc.name, tc.target) }
		test.Type(w.name, tc.name)
		test.Type(w.target, tc.target)
		test.Tap(w.connect)
		if !w.problem.Visible() || w.problem.Text != tc.problem || !w.form.Visible() {
			t.Errorf("%q, %q: problem %q visible %v", tc.name, tc.target, w.problem.Text, w.problem.Visible())
		}
	}
}

func TestConnectWaitsAndCancelGoesBack(t *testing.T) {
	w := newWindow(t)
	var gotTarget, gotName string
	cancelled := false
	w.OnConnect = func(target, name string) { gotTarget, gotName = target, name }
	w.OnCancel = func() { cancelled = true }

	test.Type(w.name, "Ana")
	test.Type(w.target, "482 913")
	test.Tap(w.connect)
	if gotTarget != "482 913" || gotName != "Ana" || !w.waiting.Visible() || w.form.Visible() {
		t.Fatalf("connect: %q, %q, waiting %v", gotTarget, gotName, w.waiting.Visible())
	}

	w.ApplyHelper(client.HelperEvent{Phase: client.HelperAsking, Session: client.Session{ExpiresIn: 112}})
	w.ApplyHelper(client.HelperEvent{Phase: client.HelperAsking, Session: client.Session{ExpiresIn: 56}})
	if w.remaining.Text != "Expires in 0:56" || w.progress.Value != 0.5 {
		t.Errorf("countdown %q, progress %v", w.remaining.Text, w.progress.Value)
	}

	test.Tap(w.cancel)
	if !cancelled || !w.form.Visible() || w.waiting.Visible() {
		t.Errorf("cancel: called %v, form %v", cancelled, w.form.Visible())
	}
}

func TestOutcomesSayWhatHappened(t *testing.T) {
	for _, phase := range []client.HelperPhase{client.HelperAllowed, client.HelperDeclined, client.HelperExpired, client.HelperEnded} {
		w := newWindow(t)
		ended := false
		w.OnCancel = func() { ended = true }
		w.ApplyHelper(client.HelperEvent{Phase: phase})
		title, _, button := outcome(phase)
		if !w.done.Visible() || w.doneTitle.Text != title || w.again.Text != button {
			t.Errorf("phase %d shows %q / %q", phase, w.doneTitle.Text, w.again.Text)
		}
		test.Tap(w.again)
		if !w.form.Visible() {
			t.Errorf("phase %d: %q did not go back to the form", phase, button)
		}
		// Only a session that was let in has anything left to end.
		if ended != (phase == client.HelperAllowed) {
			t.Errorf("phase %d: OnCancel called = %v", phase, ended)
		}
	}
}

func TestAFailedAttemptSaysWhyAndWhatToDo(t *testing.T) {
	w := newWindow(t)
	w.ApplyHelper(client.HelperEvent{Phase: client.HelperFailed, Err: client.ErrBusy})
	if !w.form.Visible() || w.problem.Text != "That person is already getting help from someone else." {
		t.Errorf("problem = %q", w.problem.Text)
	}
}

func TestHelpProblemNamesEveryRelayRefusal(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&client.RelayError{Status: http.StatusNotFound}, "That code doesn't exist or was already used. Ask for a new one."},
		{&client.RelayError{Status: http.StatusTooManyRequests}, "Too many tries from this network. Try again in a few minutes."},
		{&client.RelayError{Status: http.StatusServiceUnavailable}, "Chirp's server is busy. Try again shortly."},
		{client.ErrUnavailable, "Can't reach the Chirp server. Check your connection and try again."},
		{errors.New("anything else"), "Something went wrong. Try again."},
	} {
		if got := helpProblem(tc.err); got != tc.want {
			t.Errorf("helpProblem(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestNoServerSaysHowToSetOne(t *testing.T) {
	for addr, title := range map[string]string{"": noServerTitle, "localhost:8080": badServerTitle} {
		w := newWindow(t)
		w.ShowNoServer(addr)
		got := strings.Join(labels(w.win.Content()), " | ")
		if !strings.Contains(got, title) || !strings.Contains(got, "CHIRP_RELAY") {
			t.Errorf("ShowNoServer(%q) shows %q", addr, got)
		}
	}
}

// labels collects the text of every label under o.
func labels(o fyne.CanvasObject) []string {
	switch v := o.(type) {
	case *widget.Label:
		return []string{v.Text}
	case *fyne.Container:
		var out []string
		for _, c := range v.Objects {
			out = append(out, labels(c)...)
		}
		return out
	}
	return nil
}

func TestTheBirdSaysHowTheMachineStands(t *testing.T) {
	w := newWindow(t)
	if w.pose != warm.Landing {
		t.Errorf("starting: bird is %s, want landing", w.pose)
	}
	for _, tc := range []struct {
		ev   client.OwnerEvent
		want warm.Pose
	}{
		{client.OwnerEvent{Phase: client.OwnerReady, Peer: shown}, warm.Perched},
		{client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Replaced: true}, warm.Hop},
		{client.OwnerEvent{Phase: client.OwnerReconnecting, Peer: shown}, warm.Looking},
		{client.OwnerEvent{Phase: client.OwnerOffline, Lost: true}, warm.Asleep},
	} {
		w.ApplyOwner(tc.ev)
		if w.pose != tc.want || w.bird.Resource != warm.Bird(tc.want) {
			t.Errorf("%+v: bird is %s, want %s", tc.ev, w.pose, tc.want)
		}
	}
	// A consent request changes what the owner is told, not the bird.
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Consent: &client.ConsentRequest{Name: "Ana"}})
	if w.pose != warm.Asleep {
		t.Errorf("a consent request moved the bird to %s", w.pose)
	}
}

func TestTheBirdFliesOffOnlyWhenLetIn(t *testing.T) {
	w := newWindow(t)
	w.ApplyHelper(client.HelperEvent{Phase: client.HelperAllowed})
	if w.doneBird.Resource != warm.Bird(warm.Flying) {
		t.Error("allowed, but the bird is not flying")
	}
	w.ApplyHelper(client.HelperEvent{Phase: client.HelperDeclined})
	if w.doneBird.Resource != warm.Bird(warm.Perched) {
		t.Error("declined, but the bird is not perched")
	}
}

func TestHandDrawnTextFollowsTheTheme(t *testing.T) {
	a := test.NewTempApp(t)
	w := New(a)
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
	before := w.mark.Color

	a.Settings().SetTheme(warm.New())
	want := warm.New().Color(theme.ColorNameForeground, a.Settings().ThemeVariant())
	if w.mark.Color == before || w.mark.Color != want || w.code.Color != want || w.doneTitle.Color != want {
		t.Errorf("after switching to Warm the wordmark is %v, the code %v; want %v", w.mark.Color, w.code.Color, want)
	}
}

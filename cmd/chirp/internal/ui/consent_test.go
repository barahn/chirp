// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/barahn/chirp/client"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

var allFour = []string{"screen_view", "remote_control", "clipboard", "file_transfer"}

// asked opens the prompt for a request, as an owner event would, and returns
// the window and the answers it passes on: whether each let anyone in.
func asked(t *testing.T, req *client.ConsentRequest) (*Window, *[]bool) {
	t.Helper()
	w, answers, _ := askedGrants(t, req)
	return w, answers
}

// askedGrants is asked, with what each answer granted.
func askedGrants(t *testing.T, req *client.ConsentRequest) (*Window, *[]bool, *[][]string) {
	t.Helper()
	w := newWindow(t)
	var answers []bool
	var grants [][]string
	w.OnConsent = func(sessionID string, grant []string) {
		if sessionID != req.SessionID {
			t.Errorf("answer for session %q, want %q", sessionID, req.SessionID)
		}
		answers = append(answers, len(grant) > 0)
		grants = append(grants, grant)
	}
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown})
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Consent: req})
	if w.prompt == nil {
		t.Fatal("no prompt")
	}
	return w, &answers, &grants
}

func request() *client.ConsentRequest {
	return &client.ConsentRequest{SessionID: "s1", Name: "João da TI", Permissions: allFour, ExpiresIn: 100}
}

func TestThePromptSaysWhoAndWhat(t *testing.T) {
	w, _ := asked(t, request())
	got := strings.Join(labels(w.prompt.win.Content()), " | ")
	for _, want := range []string{
		promptTitle,
		"They say they're “João da TI”.",
		promptNoCheck,
		"See your screen", "Use your mouse and keyboard", "Share your clipboard", "Send and receive files",
		promptWarning,
		"Declines by itself in 1:40",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not say %q; it says %q", want, got)
		}
	}
}

func TestDeclineHasTheFocusAndAllowStartsLocked(t *testing.T) {
	w, answers := asked(t, request())
	p := w.prompt
	if p.win.Canvas().Focused() != p.decline {
		t.Error("Decline does not have the focus")
	}
	if !p.allow.Disabled() || p.allow.Text != "Allow (3)" {
		t.Errorf("Allow starts as %q, disabled %v", p.allow.Text, p.allow.Disabled())
	}
	// A tap on a locked Allow does nothing.
	test.Tap(p.allow)
	if len(*answers) != 0 {
		t.Fatal("a locked Allow answered")
	}
	for i := 0; i < allowLock; i++ {
		p.tick()
	}
	if p.allow.Disabled() || p.allow.Text != "Allow" {
		t.Errorf("after %d s Allow is %q, disabled %v", allowLock, p.allow.Text, p.allow.Disabled())
	}
	test.Tap(p.allow)
	if len(*answers) != 1 || !(*answers)[0] {
		t.Fatalf("answers = %v, want one Allow", *answers)
	}
	if w.prompt != nil || !w.endSession.Visible() || !strings.Contains(w.request.Text, "You let “João da TI” see your screen, without using your computer") {
		t.Errorf("after Allow: prompt %v, end visible %v, note %q", w.prompt != nil, w.endSession.Visible(), w.request.Text)
	}
}

func TestEnterDeclines(t *testing.T) {
	w, answers := asked(t, request())
	w.prompt.win.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	if len(*answers) != 1 || (*answers)[0] {
		t.Fatalf("answers = %v, want one Decline", *answers)
	}
	if w.endSession.Visible() || !strings.Contains(w.request.Text, "You declined") {
		t.Errorf("after Decline: end visible %v, note %q", w.endSession.Visible(), w.request.Text)
	}
}

func TestClosingThePromptDeclines(t *testing.T) {
	w, answers := asked(t, request())
	// What the window manager's close button runs. Fyne's test window keeps
	// the intercept without ever calling it, so it is called here.
	w.prompt.closed()
	if len(*answers) != 1 || (*answers)[0] {
		t.Fatalf("answers = %v, want one Decline", *answers)
	}
}

func TestTheDeadlineDeclines(t *testing.T) {
	req := request()
	req.ExpiresIn = 2
	w, answers := asked(t, req)
	p := w.prompt
	p.tick()
	if len(*answers) != 0 || p.count.Text != "Declines by itself in 0:01" {
		t.Fatalf("one second in: answers %v, countdown %q", *answers, p.count.Text)
	}
	p.tick()
	if len(*answers) != 1 || (*answers)[0] {
		t.Fatalf("answers = %v, want one Decline at the deadline", *answers)
	}
	// Ticks after the answer change nothing.
	p.tick()
	if len(*answers) != 1 {
		t.Errorf("answered again after closing: %v", *answers)
	}
}

func TestANewRequestReplacesAnOpenPrompt(t *testing.T) {
	w, answers := asked(t, request())
	first := w.prompt
	second := request()
	second.SessionID = "s2"
	w.OnConsent = func(string, []string) { t.Error("replacing a prompt answered it") }
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Consent: second})
	if w.prompt == first || w.prompt.req.SessionID != "s2" || !first.answered {
		t.Error("the new request did not replace the open prompt")
	}
	if len(*answers) != 0 {
		t.Errorf("answers = %v", *answers)
	}
}

func TestAnUnknownPermissionIsShownAsItCame(t *testing.T) {
	req := request()
	req.Permissions = append(req.Permissions, "webcam")
	w, _ := asked(t, req)
	if got := strings.Join(labels(w.prompt.win.Content()), " | "); !strings.Contains(got, "Something Chirp doesn't know: webcam") {
		t.Errorf("an unknown permission is not shown: %q", got)
	}
}

func TestEndingASessionAsksForANewCode(t *testing.T) {
	w, _ := asked(t, request())
	ended := false
	w.OnEndSession = func() { ended = true }
	for i := 0; i < allowLock; i++ {
		w.prompt.tick()
	}
	test.Tap(w.prompt.allow)
	test.Tap(w.endSession)
	if !ended || w.endSession.Visible() {
		t.Errorf("End session: called %v, still visible %v", ended, w.endSession.Visible())
	}
}

func TestAFailedAnswerSaysNobodyGotIn(t *testing.T) {
	w := newWindow(t)
	w.AnswerFailed(true)
	if !strings.Contains(w.request.Text, "nobody got in") || w.endSession.Visible() {
		t.Errorf("note %q, end visible %v", w.request.Text, w.endSession.Visible())
	}
}

func TestClaimCannotPassForThePromptsOwnWords(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"João da TI", "João da TI"},
		{"  Ana \t  Souza ", "Ana Souza"},
		{"Ana\nVerified by your bank", "Ana Verified by your bank"},
		{"Ana\nThis request is verified by your bank", "Ana This request is verified by your ban…"},
		{"Ana” — verified. “OK", "Ana' — verified. 'OK"},
		{"Bo\u202eknab ruoy", "Boknab ruoy"},
		{"Zé\u200b\u2066x\u2069", "Zéx"},
		{strings.Repeat("a", 60), strings.Repeat("a", claimMax) + "…"},
		{"", ""},
		{"\n\t ", ""},
	} {
		if got := claim(tc.in); got != tc.want {
			t.Errorf("claim(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := promptName(" \n"); got != promptUnnamed {
		t.Errorf("an empty name shows %q", got)
	}
}

// darkWarm is the Warm theme held in its dark variant, as when the system
// switches to dark while the prompt is open.
type darkWarm struct{ warm.Theme }

func (t darkWarm) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.Theme.Color(n, theme.VariantDark)
}

func TestTheWarningFollowsTheTheme(t *testing.T) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(warm.New())
	w := New(a)
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerReady, Peer: shown, Consent: request()})
	light := w.prompt.warnBg.FillColor
	a.Settings().SetTheme(darkWarm{})
	want := warm.New().Color(warm.ColorNameWarnBackground, theme.VariantDark)
	if got := w.prompt.warnBg.FillColor; got == light || got != want {
		t.Errorf("after switching to dark the warning background is %v, want %v", got, want)
	}
}

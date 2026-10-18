// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package ui is Chirp's one window (docs/ux.md, The main window): this
// machine's code and link above, the place to help someone below.
//
// It draws in whatever theme the app has; cmd/chirp gives it the Warm one
// (package warm, barahn/chirp#53), and the bird and the wordmark come from
// there too. It holds no protocol: cmd/chirp feeds it the events of package
// client and hands its buttons back.
package ui

import (
	"context"
	"image/color"
	"io"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/barahn/chirp/client"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// Window is the main window. Its methods are called on Fyne's goroutine:
// cmd/chirp wraps the client's events in fyne.Do.
type Window struct {
	app fyne.App
	win fyne.Window

	// OnNewCode is called when the person asks for a fresh code and link.
	OnNewCode func()
	// OnConnect is called with what the person typed, already checked to be
	// a code or a link, and the name they gave.
	OnConnect func(target, name string)
	// OnCancel is called when the person stops waiting, or ends a session.
	OnCancel func()
	// OnConsent is called with this machine's user's answer to a consent
	// request: what they allowed, with the switches the prompt had on when
	// they pressed Allow, or nothing when they pressed Decline, closed the
	// prompt or let it run out of time. It is never called otherwise.
	OnConsent func(sessionID string, grant []string)
	// OnEndSession is called when the user ends a session they allowed.
	OnEndSession func()
	// Elevated says Chirp runs with administrator rights, which the consent
	// prompt then states (GHSA-r385-j539-p738). Set it before the first
	// prompt.
	Elevated bool
	// OnRevoke is called when the user turns a permission off in the
	// session bar, mid-session. It cannot be turned on again there.
	OnRevoke func(sessionID, permission string)
	// OnSendFile sends a file the owner picked in the session bar to whoever
	// is in the session sessionID, and returns once they saved or refused
	// it. It is called off Fyne's goroutine.
	OnSendFile func(sessionID, name string, f io.ReadSeeker, progress func(sent, total int64)) error
	// Tray is the system tray icon, shown only during a session. Set it
	// before the first session; without it there is none.
	Tray Tray

	// notify shows a system notification, and quit quits Chirp; tests
	// replace them.
	notify func(*fyne.Notification)
	quit   func()
	// remote is the window onto a computer this machine is helping, while
	// one is open.
	remote *Remote
	// mainHidden is set while the main window is in the tray for a session.
	mainHidden bool

	// The header: the bird, whose pose says how this machine stands, and
	// the wordmark.
	bird      *canvas.Image
	pose      warm.Pose
	mark      *canvas.Text
	animating bool
	bar       *sessionBar

	// This machine's code.
	status     *widget.Label
	code       *canvas.Text
	codeDim    bool
	copyCode   *widget.Button
	note       *widget.Label
	hint       *widget.Label
	copyLink   *widget.Button
	newCode    *widget.Button
	request    *widget.Label
	endSession *widget.Button
	prompt     *prompt
	peer       client.Peer

	// Helping someone: the form, waiting, and how it ended.
	form        *fyne.Container
	name        *widget.Entry
	target      *widget.Entry
	connect     *widget.Button
	problem     *widget.Label
	waiting     *fyne.Container
	waitHeading *canvas.Text
	remaining   *widget.Label
	progress    *widget.ProgressBar
	cancel      *widget.Button
	firstWait   int
	done        *fyne.Container
	doneBird    *canvas.Image
	doneTitle   *canvas.Text
	doneText    *widget.Label
	again       *widget.Button
	allowed     bool
}

// New builds the window, not yet shown.
func New(a fyne.App) *Window {
	w := &Window{app: a, win: a.NewWindow("Chirp"), notify: a.SendNotification, quit: a.Quit}
	w.win.SetCloseIntercept(w.closeMain)
	header := w.header()
	card := w.ownerCard()
	help := w.helpArea()
	w.win.SetContent(container.NewPadded(container.NewVBox(header, card, w.request, container.NewHBox(w.endSession), widget.NewSeparator(), help)))
	w.win.Resize(fyne.NewSize(460, 660))
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerStarting})
	// Text drawn by hand keeps the colour it was given, so it is given the
	// theme's again when the system switches between light and dark.
	a.Settings().AddListener(func(fyne.Settings) { fyne.Do(w.recolor) })
	return w
}

// Window returns the Fyne window, to show and run.
func (w *Window) Window() fyne.Window { return w.win }

// Animate lets the bird move until ctx ends: it blinks now and then while
// perched, and a hop for a replaced code settles back on the perch. Without
// it the bird holds each pose, which is what the tests see.
func (w *Window) Animate(ctx context.Context) {
	w.animating = true
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fyne.Do(w.blink)
			}
		}
	}()
}

func (w *Window) blink() {
	if w.pose != warm.Perched {
		return
	}
	w.bird.Resource = warm.Bird(warm.Blink)
	w.bird.Refresh()
	time.AfterFunc(150*time.Millisecond, func() {
		fyne.Do(func() {
			if w.pose == warm.Perched {
				w.bird.Resource = warm.Bird(warm.Perched)
				w.bird.Refresh()
			}
		})
	})
}

func (w *Window) header() fyne.CanvasObject {
	w.bird = birdImage(warm.Landing, 56)
	w.mark = canvas.NewText("Chirp", theme.Color(theme.ColorNameForeground))
	w.mark.TextSize = 30
	w.mark.FontSource = warm.Display
	w.status = widget.NewLabel("")
	return container.NewHBox(w.bird, container.NewCenter(w.mark), layout.NewSpacer(), container.NewCenter(w.status))
}

func (w *Window) ownerCard() fyne.CanvasObject {
	w.code = canvas.NewText(noCode, theme.Color(theme.ColorNameForeground))
	w.code.TextSize = 40
	w.code.TextStyle = fyne.TextStyle{Monospace: true}
	w.copyCode = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), w.copyTheCode)
	w.note = wrapped("")
	w.hint = wrapped(tr(hintCode))
	w.copyLink = widget.NewButton(tr("Copy link"), w.copyTheLink)
	w.newCode = widget.NewButton(tr("New code"), func() {
		if w.OnNewCode != nil {
			w.OnNewCode()
		}
	})
	w.request = wrapped("")
	w.request.Hide()
	w.endSession = widget.NewButton(tr("End session"), w.endTheSession)
	w.endSession.Importance = widget.DangerImportance
	w.endSession.Hide()
	return widget.NewCard(tr("Your code"), "", container.NewVBox(
		container.NewBorder(nil, nil, nil, w.copyCode, w.code),
		w.note,
		w.hint,
		container.NewHBox(w.copyLink, w.newCode),
	))
}

func (w *Window) helpArea() fyne.CanvasObject {
	w.name = widget.NewEntry()
	w.name.SetPlaceHolder(tr("As they'll see it"))
	w.target = widget.NewEntry()
	w.target.SetPlaceHolder(tr("482 913 5071 or a link"))
	w.target.OnSubmitted = func(string) { w.tryConnect() }
	w.connect = widget.NewButton(tr("Connect"), w.tryConnect)
	w.connect.Importance = widget.HighImportance
	w.problem = wrapped("")
	w.problem.Importance = widget.DangerImportance
	w.problem.Hide()
	hint := wrapped(tr(hintHelp))
	w.form = container.NewVBox(
		widget.NewLabelWithStyle(tr("Help someone"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(tr("Your name")), w.name,
		widget.NewLabel(tr("Their code or link")),
		container.NewBorder(nil, nil, nil, w.connect, w.target),
		w.problem,
		hint,
	)

	w.waitHeading = heading(tr(waitTitle))
	w.remaining = widget.NewLabel("")
	w.progress = widget.NewProgressBar()
	w.progress.TextFormatter = func() string { return "" }
	w.cancel = widget.NewButton(tr("Cancel"), w.stopHelping)
	w.waiting = container.NewVBox(
		container.NewCenter(birdImage(warm.Chirping, 96)),
		container.NewCenter(w.waitHeading),
		wrapped(tr(waitText)),
		w.progress,
		w.remaining,
		container.NewCenter(w.cancel),
	)
	w.waiting.Hide()

	w.doneBird = birdImage(warm.Perched, 96)
	w.doneTitle = heading("")
	w.doneText = wrapped("")
	w.again = widget.NewButton("", w.stopHelping)
	w.done = container.NewVBox(
		container.NewCenter(w.doneBird),
		container.NewCenter(w.doneTitle),
		w.doneText,
		container.NewCenter(w.again),
	)
	w.done.Hide()

	return container.NewStack(w.form, w.waiting, w.done)
}

// ApplyOwner shows a change in this machine's standing with the relay.
func (w *Window) ApplyOwner(ev client.OwnerEvent) {
	defer w.relayout()
	switch {
	case ev.Started != nil:
		w.showSessionBar(*ev.Started)
		return
	case ev.Ended:
		w.sessionEnded()
		return
	}
	w.status.SetText(statusText(ev.Phase))
	switch ev.Phase {
	case client.OwnerReady:
		w.status.Importance = widget.SuccessImportance
	case client.OwnerReconnecting:
		w.status.Importance = widget.WarningImportance
	default:
		w.status.Importance = widget.MediumImportance
	}
	w.status.Refresh()

	if ev.Peer.Code != "" {
		w.peer = ev.Peer
	}
	usable := w.peer.Code != "" && (ev.Phase == client.OwnerReady || ev.Phase == client.OwnerReconnecting)
	if !usable {
		w.peer = client.Peer{}
	}
	w.code.Text = noCode
	if usable {
		w.code.Text = w.peer.Spoken()
	}
	w.codeDim = ev.Phase != client.OwnerReady
	w.recolor()
	setEnabled(usable, w.copyCode, w.copyLink, w.newCode)

	if ev.Consent != nil {
		w.showPrompt(ev.Consent)
		return
	}
	w.setPose(ownerPose(ev))
	w.hint.SetText(tr(hintCode))
	w.copyLink.SetText(tr("Copy link"))
	if note := ownerNote(ev); note != "" {
		w.note.SetText(note)
		w.note.Show()
	} else {
		w.note.Hide()
	}
}

// consentDecided records the user's answer in the main window and passes it
// on. Only the prompt calls it.
func (w *Window) consentDecided(req *client.ConsentRequest, grant []string) {
	defer w.relayout()
	granted := len(grant) > 0
	w.request.Importance = widget.MediumImportance
	w.request.SetText(decidedNote(req.Name, grant))
	w.request.Show()
	if granted {
		w.endSession.Show()
	} else {
		w.endSession.Hide()
	}
	if w.OnConsent != nil {
		w.OnConsent(req.SessionID, grant)
	}
}

// AnswerFailed says that an answer did not reach the relay. The relay then
// declines by itself at the deadline, so nobody gets in either way.
func (w *Window) AnswerFailed(granted bool) {
	defer w.relayout()
	w.request.Importance = widget.WarningImportance
	w.request.SetText(answerProblem(granted))
	w.request.Show()
	w.endSession.Hide()
}

// sessionEnded closes the bar and says the session is over, whichever end
// ended it.
func (w *Window) sessionEnded() {
	if w.bar != nil {
		w.bar.dismiss()
	}
	w.endSession.Hide()
	w.request.Importance = widget.MediumImportance
	w.request.SetText(tr(sessionOverNote))
	w.request.Show()
}

func (w *Window) endTheSession() {
	defer w.relayout()
	w.endSession.Hide()
	w.request.Importance = widget.MediumImportance
	w.request.SetText(tr("Session ended. Your code and link are being replaced."))
	if w.OnEndSession != nil {
		w.OnEndSession()
	}
}

// ownerPose is the bird for the owner's phase (docs/ux.md, The bird's
// states).
func ownerPose(ev client.OwnerEvent) warm.Pose {
	switch {
	case ev.Phase == client.OwnerStarting:
		return warm.Landing
	case ev.Phase == client.OwnerReconnecting:
		return warm.Looking
	case ev.Phase == client.OwnerOffline:
		return warm.Asleep
	case ev.Replaced:
		return warm.Hop
	}
	return warm.Perched
}

func (w *Window) setPose(p warm.Pose) {
	w.pose = p
	w.bird.Resource = warm.Bird(p)
	w.bird.Refresh()
	if p == warm.Hop && w.animating {
		time.AfterFunc(1200*time.Millisecond, func() {
			fyne.Do(func() {
				if w.pose == warm.Hop {
					w.setPose(warm.Perched)
				}
			})
		})
	}
}

// ApplyHelper shows a change in an attempt to help someone.
func (w *Window) ApplyHelper(ev client.HelperEvent) {
	defer w.relayout()
	switch ev.Phase {
	case client.HelperAsking:
		if w.firstWait == 0 {
			w.firstWait = ev.Session.ExpiresIn
		}
		w.remaining.SetText(countdown(ev.Session.ExpiresIn))
		if w.firstWait > 0 {
			w.progress.SetValue(float64(ev.Session.ExpiresIn) / float64(w.firstWait))
		}
		w.show(w.waiting)
	case client.HelperFailed:
		w.problem.SetText(helpProblem(ev.Err))
		w.problem.Show()
		w.show(w.form)
	default:
		title, text, button := outcome(ev.Phase)
		w.doneTitle.Text = title
		w.doneTitle.Refresh()
		w.doneText.SetText(text)
		w.again.SetText(button)
		w.allowed = ev.Phase == client.HelperAllowed
		w.doneBird.Resource = warm.Bird(warm.Perched)
		if w.allowed {
			w.doneBird.Resource = warm.Bird(warm.Flying)
		}
		w.doneBird.Refresh()
		w.show(w.done)
	}
}

// ShowNoServer replaces the window's content when there is no usable relay
// address. addr is what was given, if anything.
// ShowElevated replaces the window's content with why Chirp will not run as
// administrator (barahn/chirp#117, option 1). It is shown instead of a code:
// nothing is registered with the relay.
func (w *Window) ShowElevated() {
	w.win.SetContent(container.NewPadded(container.NewVBox(
		container.NewCenter(birdImage(warm.Asleep, 96)),
		widget.NewLabelWithStyle(tr("Chirp is running as administrator"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		wrapped(tr("Anyone you let in would have full control of this computer. Close Chirp and open it normally, not with \"Run as administrator\". To share an administrator session anyway, start it with -allow-elevated.")),
	)))
}

func (w *Window) ShowNoServer(addr string) {
	title := tr(noServerTitle)
	if addr != "" {
		title = tr(badServerTitle)
	}
	w.win.SetContent(container.NewPadded(container.NewVBox(
		container.NewCenter(birdImage(warm.Asleep, 96)),
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		wrapped(tr(noServerText)),
	)))
}

func (w *Window) tryConnect() {
	if w.name.Text == "" {
		w.ApplyHelper(client.HelperEvent{Phase: client.HelperFailed, Err: client.ErrNoName})
		return
	}
	if _, err := client.ParseTarget(w.target.Text); err != nil {
		w.ApplyHelper(client.HelperEvent{Phase: client.HelperFailed, Err: err})
		return
	}
	w.problem.Hide()
	w.firstWait = 0
	w.remaining.SetText("")
	w.progress.SetValue(1)
	w.show(w.waiting)
	if w.OnConnect != nil {
		w.OnConnect(w.target.Text, w.name.Text)
	}
}

// stopHelping goes back to the form. Cancel stops waiting; after an outcome
// the attempt is already over, except a session that was allowed, which End
// ends.
func (w *Window) stopHelping() {
	if (w.waiting.Visible() || w.allowed) && w.OnCancel != nil {
		w.OnCancel()
	}
	w.allowed = false
	w.show(w.form)
}

func (w *Window) show(c *fyne.Container) {
	defer w.relayout()
	for _, o := range []*fyne.Container{w.form, w.waiting, w.done} {
		if o == c {
			o.Show()
		} else {
			o.Hide()
		}
	}
}

func (w *Window) copyTheCode() {
	w.app.Clipboard().SetContent(w.peer.Spoken())
	w.hint.SetText(tr("Code copied."))
	w.relayout()
}

// linkPage is the page a link opens (product-model decisions 4 and 6). The
// token goes after "#", which the browser never sends, so the site never
// receives it; client.ParseTarget reads it back from there.
const linkPage = "https://usechirp.app/l#"

// copyTheLink copies the link as the person sends it: the page that says
// what to do with it, with the token in its fragment.
func (w *Window) copyTheLink() {
	w.app.Clipboard().SetContent(linkPage + w.peer.LinkFragment())
	w.copyLink.SetText(tr("Link copied"))
	w.relayout()
}

// recolor gives the hand-drawn text the theme's colours.
func (w *Window) recolor() {
	fg := theme.Color(theme.ColorNameForeground)
	for _, t := range []*canvas.Text{w.mark, w.waitHeading, w.doneTitle} {
		t.Color = fg
		t.Refresh()
	}
	w.code.Color = fg
	if w.codeDim {
		w.code.Color = dim()
	}
	w.code.Refresh()
	if w.prompt != nil {
		w.prompt.recolor()
	}
	if w.bar != nil {
		w.bar.recolor()
	}
}

// relayout lays the whole window out again. A label that changes its text,
// or appears, changes its size, and the containers around it do not notice
// on their own.
func (w *Window) relayout() { w.win.Content().Refresh() }

func birdImage(p warm.Pose, size float32) *canvas.Image {
	img := canvas.NewImageFromResource(warm.Bird(p))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	return img
}

// heading is a title in the display face, which only hand-drawn text can
// carry (warm.Display).
func heading(text string) *canvas.Text {
	t := canvas.NewText(text, theme.Color(theme.ColorNameForeground))
	t.TextSize = 20
	t.FontSource = warm.Display
	t.Alignment = fyne.TextAlignCenter
	return t
}

func wrapped(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func setEnabled(on bool, buttons ...*widget.Button) {
	for _, b := range buttons {
		if on {
			b.Enable()
		} else {
			b.Disable()
		}
	}
}

func dim() color.Color { return theme.Color(theme.ColorNameDisabled) }

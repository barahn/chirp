// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package ui is Chirp's one window (docs/ux.md, The main window): this
// machine's code and link above, the place to help someone below.
//
// It draws in Fyne's default theme until a visual direction is chosen
// (barahn/chirp#53). It holds no protocol: cmd/chirp feeds it the events of
// package client and hands its buttons back.
package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/barahn/chirp/client"
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

	// This machine's code.
	status   *widget.Label
	code     *canvas.Text
	copyCode *widget.Button
	note     *widget.Label
	hint     *widget.Label
	copyLink *widget.Button
	newCode  *widget.Button
	request  *widget.Label
	peer     client.Peer

	// Helping someone: the form, waiting, and how it ended.
	form      *fyne.Container
	name      *widget.Entry
	target    *widget.Entry
	connect   *widget.Button
	problem   *widget.Label
	waiting   *fyne.Container
	remaining *widget.Label
	progress  *widget.ProgressBar
	cancel    *widget.Button
	firstWait int
	done      *fyne.Container
	doneTitle *widget.Label
	doneText  *widget.Label
	again     *widget.Button
	allowed   bool
}

// New builds the window, not yet shown.
func New(a fyne.App) *Window {
	w := &Window{app: a, win: a.NewWindow("Chirp")}
	header := w.header()
	card := w.ownerCard()
	help := w.helpArea()
	w.win.SetContent(container.NewPadded(container.NewVBox(header, card, w.request, widget.NewSeparator(), help)))
	w.win.Resize(fyne.NewSize(460, 620))
	w.ApplyOwner(client.OwnerEvent{Phase: client.OwnerStarting})
	return w
}

// Window returns the Fyne window, to show and run.
func (w *Window) Window() fyne.Window { return w.win }

func (w *Window) header() fyne.CanvasObject {
	mark := canvas.NewText("Chirp", theme.Color(theme.ColorNameForeground))
	mark.TextSize = 26
	mark.TextStyle = fyne.TextStyle{Bold: true}
	w.status = widget.NewLabel("")
	return container.NewHBox(mark, layout.NewSpacer(), w.status)
}

func (w *Window) ownerCard() fyne.CanvasObject {
	w.code = canvas.NewText(noCode, theme.Color(theme.ColorNameForeground))
	w.code.TextSize = 40
	w.code.TextStyle = fyne.TextStyle{Monospace: true}
	w.copyCode = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), w.copyTheCode)
	w.note = wrapped("")
	w.hint = wrapped(hintCode)
	w.copyLink = widget.NewButton("Copy link", w.copyTheLink)
	w.newCode = widget.NewButton("New code", func() {
		if w.OnNewCode != nil {
			w.OnNewCode()
		}
	})
	w.request = wrapped("")
	w.request.Importance = widget.WarningImportance
	w.request.Hide()
	return widget.NewCard("Your code", "", container.NewVBox(
		container.NewBorder(nil, nil, nil, w.copyCode, w.code),
		w.note,
		w.hint,
		container.NewHBox(w.copyLink, w.newCode),
	))
}

func (w *Window) helpArea() fyne.CanvasObject {
	w.name = widget.NewEntry()
	w.name.SetPlaceHolder("As they'll see it")
	w.target = widget.NewEntry()
	w.target.SetPlaceHolder("482 913 or a link")
	w.target.OnSubmitted = func(string) { w.tryConnect() }
	w.connect = widget.NewButton("Connect", w.tryConnect)
	w.connect.Importance = widget.HighImportance
	w.problem = wrapped("")
	w.problem.Importance = widget.DangerImportance
	w.problem.Hide()
	hint := wrapped(hintHelp)
	w.form = container.NewVBox(
		widget.NewLabelWithStyle("Help someone", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Your name"), w.name,
		widget.NewLabel("Their code or link"),
		container.NewBorder(nil, nil, nil, w.connect, w.target),
		w.problem,
		hint,
	)

	w.remaining = widget.NewLabel("")
	w.progress = widget.NewProgressBar()
	w.progress.TextFormatter = func() string { return "" }
	w.cancel = widget.NewButton("Cancel", w.stopHelping)
	w.waiting = container.NewVBox(
		widget.NewLabelWithStyle(waitTitle, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		wrapped(waitText),
		w.progress,
		w.remaining,
		container.NewCenter(w.cancel),
	)
	w.waiting.Hide()

	w.doneTitle = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	w.doneText = wrapped("")
	w.again = widget.NewButton("", w.stopHelping)
	w.done = container.NewVBox(w.doneTitle, w.doneText, container.NewCenter(w.again))
	w.done.Hide()

	return container.NewStack(w.form, w.waiting, w.done)
}

// ApplyOwner shows a change in this machine's standing with the relay.
func (w *Window) ApplyOwner(ev client.OwnerEvent) {
	defer w.relayout()
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
		w.code.Text = w.peer.Formatted
	}
	w.code.Color = theme.Color(theme.ColorNameForeground)
	if ev.Phase != client.OwnerReady {
		w.code.Color = dim()
	}
	w.code.Refresh()
	setEnabled(usable, w.copyCode, w.copyLink, w.newCode)

	if ev.Consent != nil {
		w.request.SetText(requestNote(ev.Consent))
		w.request.Show()
		return
	}
	w.hint.SetText(hintCode)
	w.copyLink.SetText("Copy link")
	if note := ownerNote(ev); note != "" {
		w.note.SetText(note)
		w.note.Show()
	} else {
		w.note.Hide()
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
		w.doneTitle.SetText(title)
		w.doneText.SetText(text)
		w.again.SetText(button)
		w.allowed = ev.Phase == client.HelperAllowed
		w.show(w.done)
	}
}

// ShowNoServer replaces the window's content when there is no usable relay
// address. addr is what was given, if anything.
func (w *Window) ShowNoServer(addr string) {
	title := noServerTitle
	if addr != "" {
		title = badServerTitle
	}
	w.win.SetContent(container.NewPadded(container.NewVBox(
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		wrapped(noServerText),
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
	w.app.Clipboard().SetContent(w.peer.Formatted)
	w.hint.SetText("Code copied.")
	w.relayout()
}

// copyTheLink copies the link's token. The page a link opens, which would
// carry the token after "#", comes with the download site (product-model
// decision 4); until then the token itself is what Chirp accepts as a link.
func (w *Window) copyTheLink() {
	w.app.Clipboard().SetContent(w.peer.Link)
	w.copyLink.SetText("Link copied")
	w.relayout()
}

// relayout lays the whole window out again. A label that changes its text,
// or appears, changes its size, and the containers around it do not notice
// on their own.
func (w *Window) relayout() { w.win.Content().Refresh() }

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

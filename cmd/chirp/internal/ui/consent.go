// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"image/color"
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

// The consent prompt (barahn/chirp#11, docs/ux.md, Consent): Chirp's own
// window, on the machine to be controlled, asking its user whether to let a
// controller in. It is written to make an unexpected request easy to decline:
//
//   - Decline has the focus, so Enter declines.
//   - Allow stays locked for the first seconds, so neither a reflexive key
//     nor a click meant for something else lets anyone in.
//   - The name is shown as a claim, cleaned of anything that could pass for
//     the prompt's own words.
//   - Closing the window declines, and so does the deadline.
//   - No bird and no brand words: nothing that sounds like Chirp vouching.
//   - Each permission is a switch, and only Screen view starts on (it is
//     fixed on: no session goes without it). Allow lets in what is on and
//     nothing else, so a hurried Allow gives away the picture alone
//     (barahn/chirp#52).
//
// Nothing here can answer on its own. An answer is only ever the user
// pressing a button, or the absence of one.

// allowLock is how many seconds Allow stays locked.
const allowLock = 3

type prompt struct {
	w        *Window
	win      fyne.Window
	req      *client.ConsentRequest
	left     int
	locked   int
	allow    *widget.Button
	decline  *widget.Button
	count    *widget.Label
	switches []*permSwitch
	answered bool
	stop     chan struct{}
	// closed is what closing the window does, from the window manager.
	closed func()
	// warnBg is the warning's background, recoloured with the theme.
	warnBg *canvas.Rectangle
}

// showPrompt opens the prompt for req. A request that arrives while another
// is open replaces it: the relay holds one session per owner, so the older
// one is already over.
func (w *Window) showPrompt(req *client.ConsentRequest) {
	if w.prompt != nil {
		w.prompt.dismiss()
	}
	p := &prompt{
		w:      w,
		win:    w.app.NewWindow(tr(promptTitle)),
		req:    req,
		left:   req.ExpiresIn,
		locked: allowLock,
		stop:   make(chan struct{}),
	}
	w.prompt = p

	p.decline = widget.NewButton(tr(promptDecline), func() { p.answer(false) })
	p.decline.Importance = widget.HighImportance
	p.allow = widget.NewButton(allowLabel(p.locked), func() {
		if p.locked == 0 && len(p.chosen()) > 0 {
			p.answer(true)
		}
	})
	p.allow.Importance = widget.LowImportance
	p.allow.Disable()
	p.count = widget.NewLabel("")
	p.showCountdown()

	title := widget.NewLabelWithStyle(tr(promptTitle), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapWord
	perms := container.NewVBox()
	for _, name := range req.Permissions {
		s := newPermSwitch(name, false, func(bool) { p.updateAllow() })
		s.tapped = func() { p.win.Canvas().Focus(p.decline) }
		if name == permScreenView {
			s.fix()
		}
		p.switches = append(p.switches, s)
		perms.Add(s)
	}
	who := wrapped(promptName(req.Name))
	who.TextStyle = fyne.TextStyle{Bold: true}

	p.win.SetContent(container.NewPadded(container.NewVBox(
		container.NewBorder(nil, nil, widget.NewIcon(theme.NewWarningThemedResource(theme.WarningIcon())), nil, title),
		who,
		wrapped(tr(promptNoCheck)),
		widget.NewLabel(tr(promptAsking)),
		perms,
		wrapped(tr(promptChoose)),
		p.warningBox(tr(promptWarning)),
		container.NewHBox(p.count, layout.NewSpacer(), p.allow, p.decline),
	)))
	p.win.Resize(fyne.NewSize(480, 0))
	p.win.SetFixedSize(true)
	// Closing the prompt is not consent.
	p.closed = func() { p.answer(false) }
	p.win.SetCloseIntercept(p.closed)
	p.win.Show()
	p.win.RequestFocus()
	p.win.Canvas().Focus(p.decline)

	if w.animating {
		go p.run()
	}
}

// run ticks the prompt once a second until it is answered.
func (p *prompt) run() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			fyne.Do(p.tick)
		}
	}
}

// tick is one second passing: Allow unlocks after allowLock, and at the
// deadline the prompt declines by itself.
func (p *prompt) tick() {
	if p.answered {
		return
	}
	if p.locked > 0 {
		p.locked--
		p.allow.SetText(allowLabel(p.locked))
		p.updateAllow()
	}
	if p.left > 0 {
		p.left--
		p.showCountdown()
		if p.left == 0 {
			p.answer(false)
		}
	}
}

func (p *prompt) showCountdown() {
	if p.left > 0 {
		p.count.SetText(promptCountdown(p.left))
	} else {
		p.count.SetText("")
	}
}

// chosen is what the switches have on, in the order asked for.
func (p *prompt) chosen() []string {
	var on []string
	for _, s := range p.switches {
		if s.on {
			on = append(on, s.name)
		}
	}
	return on
}

// updateAllow enables Allow once it is unlocked and something is on to
// allow: with every switch off, there is nothing to let anyone in to.
func (p *prompt) updateAllow() {
	if p.locked == 0 && len(p.chosen()) > 0 {
		p.allow.Enable()
	} else {
		p.allow.Disable()
	}
}

// answer closes the prompt with the user's decision, once: what the switches
// have on, or nothing.
func (p *prompt) answer(granted bool) {
	if p.answered {
		return
	}
	var grant []string
	if granted {
		grant = p.chosen()
	}
	p.dismiss()
	p.w.consentDecided(p.req, grant)
}

// dismiss closes the prompt without deciding anything.
func (p *prompt) dismiss() {
	if p.answered {
		return
	}
	p.answered = true
	close(p.stop)
	p.win.Close()
	if p.w.prompt == p {
		p.w.prompt = nil
	}
}

// warnBackground is the theme's warning background. A theme without one
// gives the box none.
func warnBackground() color.Color {
	th := fyne.CurrentApp().Settings().Theme()
	if c, ok := th.(interface{ ServesChirpColors() bool }); ok && c.ServesChirpColors() {
		return th.Color(warm.ColorNameWarnBackground, fyne.CurrentApp().Settings().ThemeVariant())
	}
	return color.Transparent
}

// permScreenView is the permission no session goes without, as remotekit
// names it: the prompt shows it fixed on.
const permScreenView = "screen_view"

func permissionIcon(p string) fyne.Resource {
	switch p {
	case permScreenView:
		return theme.VisibilityIcon()
	case "remote_control":
		return theme.ComputerIcon()
	case "clipboard":
		return theme.ContentPasteIcon()
	case "file_transfer":
		return theme.FileIcon()
	}
	return theme.QuestionIcon()
}

// warningBox is the fixed warning, on the warning background.
func (p *prompt) warningBox(text string) fyne.CanvasObject {
	p.warnBg = canvas.NewRectangle(warnBackground())
	p.warnBg.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	l := wrapped(text)
	l.Importance = widget.WarningImportance
	l.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewStack(p.warnBg, container.NewPadded(l))
}

// recolor gives the hand-drawn background the theme's colour again, as the
// system switches between light and dark.
func (p *prompt) recolor() {
	p.warnBg.FillColor = warnBackground()
	p.warnBg.Refresh()
	for _, s := range p.switches {
		s.Refresh()
	}
}

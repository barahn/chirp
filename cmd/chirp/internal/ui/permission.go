// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// A permission the owner turns on or off (barahn/chirp#52, docs/ux.md,
// Consent): a whole row, its icon, its words and a check mark, that changes
// colour when on. The check mark says the same as the colour, so the state
// never rests on telling colours apart.
//
// The prompt's rows turn both ways. The session bar's only turn off, since
// turning a permission on mid-session is what a scammer asks for; a row that
// can no longer change is drawn as it stands and ignores taps. See Screen
// view, which no session goes without: it is fixed on.
//
// Tab reaches the rows, and Space turns the focused one. A click focuses the
// row it lands on, as Fyne does with anything focusable; tapped then hands
// the focus back, so the prompt's Decline keeps it and Enter still declines
// after a click.

type permSwitch struct {
	widget.BaseWidget

	name    string
	on      bool
	fixed   bool // always on: Screen view
	offOnly bool // the session bar's: once off, it stays off
	focused bool
	changed func(on bool)
	tapped  func()

	bg    *canvas.Rectangle
	icon  *widget.Icon
	label *widget.Label
	mark  *widget.Icon
	note  *widget.Label
}

// newPermSwitch is a row for the permission name, on or off.
func newPermSwitch(name string, on bool, changed func(bool)) *permSwitch {
	s := &permSwitch{name: name, on: on, changed: changed}
	s.bg = canvas.NewRectangle(color.Transparent)
	s.bg.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	s.bg.StrokeWidth = 2
	s.icon = widget.NewIcon(nil)
	s.label = widget.NewLabel(permissionText(name))
	s.label.Wrapping = fyne.TextWrapWord
	s.mark = widget.NewIcon(nil)
	s.note = widget.NewLabel("")
	s.note.Importance = widget.LowImportance
	s.ExtendBaseWidget(s)
	s.update()
	return s
}

// fix makes the row always on, with a word saying so.
func (s *permSwitch) fix() *permSwitch {
	s.fixed, s.on = true, true
	s.note.SetText(tr("Always"))
	s.update()
	return s
}

// canChange reports whether a tap would change the row.
func (s *permSwitch) canChange() bool {
	// A fixed row never changes, and a row that only turns off cannot once
	// it is off.
	return !s.fixed && (!s.offOnly || s.on)
}

func (s *permSwitch) toggle() {
	if !s.canChange() {
		return
	}
	s.on = !s.on
	s.update()
	if s.changed != nil {
		s.changed(s.on)
	}
}

// update draws the row as it stands, in the theme's current colours.
func (s *permSwitch) update() {
	icon := permissionIcon(s.name)
	if s.on {
		s.bg.FillColor = withAlpha(theme.Color(theme.ColorNamePrimary), 0x33)
		s.icon.SetResource(theme.NewPrimaryThemedResource(icon))
		s.mark.SetResource(theme.NewPrimaryThemedResource(theme.CheckButtonCheckedIcon()))
		s.label.Importance = widget.MediumImportance
		s.label.TextStyle = fyne.TextStyle{Bold: true}
	} else {
		s.bg.FillColor = color.Transparent
		s.icon.SetResource(theme.NewDisabledResource(icon))
		s.mark.SetResource(theme.NewDisabledResource(theme.CheckButtonIcon()))
		s.label.Importance = widget.LowImportance
		s.label.TextStyle = fyne.TextStyle{}
	}
	switch {
	case s.fixed:
		s.mark.Hide()
		s.note.Show()
	case s.offOnly && !s.on:
		// An empty box would offer to turn it on again.
		s.note.SetText(tr("Taken back"))
		s.mark.Hide()
		s.note.Show()
	default:
		s.mark.Show()
		s.note.Hide()
	}
	if s.focused {
		s.bg.StrokeColor = theme.Color(theme.ColorNameFocus)
	} else {
		s.bg.StrokeColor = color.Transparent
	}
	s.label.Refresh()
	s.bg.Refresh()
}

func (s *permSwitch) CreateRenderer() fyne.WidgetRenderer {
	right := container.NewStack(s.mark, s.note)
	return widget.NewSimpleRenderer(container.NewStack(s.bg, container.NewBorder(nil, nil, s.icon, right, s.label)))
}

// Refresh redraws the row, as the theme changes between light and dark.
func (s *permSwitch) Refresh() {
	s.update()
	s.BaseWidget.Refresh()
}

func (s *permSwitch) Tapped(*fyne.PointEvent) {
	s.toggle()
	if s.tapped != nil {
		s.tapped()
	}
}

// Cursor is a pointing hand over a row a tap would change.
func (s *permSwitch) Cursor() desktop.Cursor {
	if s.canChange() {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (s *permSwitch) FocusGained() { s.focused = true; s.update() }
func (s *permSwitch) FocusLost()   { s.focused = false; s.update() }

func (s *permSwitch) TypedRune(r rune) {
	if r == ' ' {
		s.toggle()
	}
}

func (s *permSwitch) TypedKey(*fyne.KeyEvent) {}

// withAlpha is c with its alpha replaced, for a tint of a theme colour.
func withAlpha(c color.Color, a uint8) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = a
	return n
}

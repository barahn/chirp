// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package warm

import "image/color"

// palette is one variant of the Warm colours: the canvas's Tokens board,
// "Warm light" and "Warm dark" (barahn/chirp#53).
type palette struct {
	bg, surface, button, text, muted, disabled, line color.NRGBA
	accent, tint                                     color.NRGBA
	primary, onPrimary                               color.NRGBA
	warnBg, warnText, onWarn                         color.NRGBA
	danger, onDanger                                 color.NRGBA
	ok, onOk                                         color.NRGBA
	shadow                                           color.NRGBA
}

var light = palette{
	bg:        rgb(0xFF, 0xF8, 0xEC),
	surface:   rgb(0xFF, 0xFF, 0xFF),
	button:    rgb(0xF4, 0xE9, 0xD8),
	text:      rgb(0x2A, 0x21, 0x18),
	muted:     rgb(0x6E, 0x5F, 0x50),
	disabled:  rgb(0xA8, 0x9A, 0x8A),
	line:      rgb(0xEA, 0xDF, 0xCC),
	accent:    rgb(0xF5, 0xB8, 0x3D),
	tint:      rgb(0xFF, 0xE9, 0xB8),
	primary:   rgb(0x1F, 0x6F, 0x78),
	onPrimary: rgb(0xFF, 0xFF, 0xFF),
	warnBg:    rgb(0xFF, 0xF1, 0xD6),
	warnText:  rgb(0x7A, 0x4B, 0x00),
	onWarn:    rgb(0xFF, 0xFF, 0xFF),
	danger:    rgb(0xB9, 0x3A, 0x26),
	onDanger:  rgb(0xFF, 0xFF, 0xFF),
	ok:        rgb(0x2E, 0x6B, 0x2F),
	onOk:      rgb(0xFF, 0xFF, 0xFF),
	shadow:    color.NRGBA{A: 0x33},
}

var dark = palette{
	bg:        rgb(0x1D, 0x19, 0x14),
	surface:   rgb(0x2A, 0x24, 0x1D),
	button:    rgb(0x3A, 0x31, 0x28),
	text:      rgb(0xF6, 0xEF, 0xE6),
	muted:     rgb(0xB9, 0xAC, 0x9C),
	disabled:  rgb(0x6E, 0x5F, 0x50),
	line:      rgb(0x3D, 0x35, 0x2B),
	accent:    rgb(0xF5, 0xB8, 0x3D),
	tint:      rgb(0x4A, 0x3A, 0x16),
	primary:   rgb(0x6C, 0xC3, 0xCC),
	onPrimary: rgb(0x0E, 0x23, 0x26),
	warnBg:    rgb(0x3A, 0x2C, 0x12),
	warnText:  rgb(0xF7, 0xC8, 0x73),
	onWarn:    rgb(0x1D, 0x19, 0x14),
	danger:    rgb(0xFF, 0x8A, 0x73),
	onDanger:  rgb(0x2A, 0x0D, 0x07),
	ok:        rgb(0x8F, 0xD1, 0x8F),
	onOk:      rgb(0x1D, 0x19, 0x14),
	shadow:    color.NRGBA{A: 0x66},
}

func rgb(r, g, b uint8) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 0xFF}
}

// withAlpha returns c at alpha a, for the washes Fyne lays over a widget
// when it is hovered or pressed.
func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

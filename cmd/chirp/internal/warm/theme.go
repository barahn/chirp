// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package warm is Chirp's look, direction A in docs/ux.md (barahn/chirp#53):
// the Warm palette in both variants, its type and its shapes as a Fyne theme,
// and the bird.
//
// The fonts in fonts/ are not Fabrintek's. They are under the SIL Open Font
// License 1.1, accepted in barahn/chirp#54, and stay under it whatever
// licence Chirp is used under; fonts/README.md says where each came from.
package warm

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Chirp's own colour names, for what Fyne has no name for. Chirp's widgets ask
// the theme for them like any other.
const (
	ColorNameMuted          fyne.ThemeColorName = "chirp.muted"
	ColorNameAccent         fyne.ThemeColorName = "chirp.accent"
	ColorNameTint           fyne.ThemeColorName = "chirp.tint"
	ColorNameWarnBackground fyne.ThemeColorName = "chirp.warnBackground"
)

var (
	//go:embed fonts/NunitoSans-Regular.ttf
	nunitoRegular []byte
	//go:embed fonts/NunitoSans-Bold.ttf
	nunitoBold []byte
	//go:embed fonts/Fredoka-SemiBold.ttf
	fredoka []byte
	//go:embed fonts/JetBrainsMono-Medium.ttf
	jetbrainsMono []byte

	textRegular = fyne.NewStaticResource("NunitoSans-Regular.ttf", nunitoRegular)
	textBold    = fyne.NewStaticResource("NunitoSans-Bold.ttf", nunitoBold)
	codeFont    = fyne.NewStaticResource("JetBrainsMono-Medium.ttf", jetbrainsMono)

	// Display is the face for the wordmark and headings: Fredoka, set on a
	// canvas.Text as its FontSource. Fyne picks a theme font by style, not
	// by role, so it cannot be the theme's bold without making every bold
	// label Fredoka too.
	Display fyne.Resource = fyne.NewStaticResource("Fredoka-SemiBold.ttf", fredoka)
)

// Theme is the Warm theme. It follows the system's light or dark preference
// through the variant Fyne passes in.
type Theme struct{}

// New returns the Warm theme.
func New() fyne.Theme { return Theme{} }

func pick(v fyne.ThemeVariant) palette {
	if v == theme.VariantDark {
		return dark
	}
	return light
}

// Color returns the Warm value for name, falling back to Fyne's default for
// the few names Warm leaves alone (window borders, for instance).
func (Theme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	p := pick(v)
	switch name {
	case theme.ColorNameBackground:
		return p.bg
	case theme.ColorNameForeground:
		return p.text
	case theme.ColorNameButton:
		return p.button
	case theme.ColorNameDisabledButton:
		return p.line
	case theme.ColorNameDisabled:
		return p.disabled
	case theme.ColorNamePlaceHolder:
		return p.muted
	case theme.ColorNameInputBackground, theme.ColorNameMenuBackground,
		theme.ColorNameOverlayBackground, theme.ColorNameHeaderBackground:
		return p.surface
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return p.line
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return p.primary
	case theme.ColorNameForegroundOnPrimary:
		return p.onPrimary
	case theme.ColorNameHover:
		return withAlpha(p.primary, 0x1A)
	case theme.ColorNamePressed:
		return withAlpha(p.primary, 0x33)
	case theme.ColorNameSelection:
		return p.tint
	case theme.ColorNameScrollBar:
		return withAlpha(p.muted, 0x99)
	case theme.ColorNameShadow:
		return p.shadow
	case theme.ColorNameError:
		return p.danger
	case theme.ColorNameForegroundOnError:
		return p.onDanger
	case theme.ColorNameWarning:
		return p.warnText
	case theme.ColorNameForegroundOnWarning:
		return p.onWarn
	case theme.ColorNameSuccess:
		return p.ok
	case theme.ColorNameForegroundOnSuccess:
		return p.onOk
	case ColorNameMuted:
		return p.muted
	case ColorNameAccent:
		return p.accent
	case ColorNameTint:
		return p.tint
	case ColorNameWarnBackground:
		return p.warnBg
	}
	return theme.DefaultTheme().Color(name, v)
}

// Font is Nunito Sans for text and JetBrains Mono for codes. Neither has an
// italic here, and Chirp sets nothing in italic, so italic falls back to the
// upright face. Symbols stay Fyne's.
func (Theme) Font(s fyne.TextStyle) fyne.Resource {
	switch {
	case s.Symbol:
		return theme.DefaultTheme().Font(s)
	case s.Monospace:
		return codeFont
	case s.Bold:
		return textBold
	}
	return textRegular
}

// Icon is Fyne's.
func (Theme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Size rounds the corners as the canvas does: 18 for cards and dialogs, 12
// for buttons and fields.
func (Theme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameCardRadius, theme.SizeNameDialogRadius:
		return 18
	case theme.SizeNameButtonRadius, theme.SizeNameInputRadius, theme.SizeNamePopupRadius:
		return 12
	case theme.SizeNameSelectionRadius:
		return 8
	}
	return theme.DefaultTheme().Size(name)
}

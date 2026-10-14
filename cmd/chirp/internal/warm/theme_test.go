// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package warm

import (
	"image"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestColoursFollowTheVariant(t *testing.T) {
	th := New()
	for _, tc := range []struct {
		name fyne.ThemeColorName
		v    fyne.ThemeVariant
		want any
	}{
		{theme.ColorNameBackground, theme.VariantLight, light.bg},
		{theme.ColorNameBackground, theme.VariantDark, dark.bg},
		{theme.ColorNamePrimary, theme.VariantLight, light.primary},
		{theme.ColorNameForegroundOnPrimary, theme.VariantDark, dark.onPrimary},
		{theme.ColorNameError, theme.VariantLight, light.danger},
		{ColorNameAccent, theme.VariantDark, dark.accent},
		{ColorNameWarnBackground, theme.VariantLight, light.warnBg},
	} {
		if got := th.Color(tc.name, tc.v); got != tc.want {
			t.Errorf("Color(%s, %d) = %v, want %v", tc.name, tc.v, got, tc.want)
		}
	}
	// A name Warm leaves alone is Fyne's.
	if got, want := th.Color(theme.ColorNameInnerWindowBorder, theme.VariantLight),
		theme.DefaultTheme().Color(theme.ColorNameInnerWindowBorder, theme.VariantLight); got != want {
		t.Errorf("an unhandled name gives %v, want Fyne's %v", got, want)
	}
}

func TestFontsAreTheWarmOnes(t *testing.T) {
	th := New()
	for style, want := range map[fyne.TextStyle]string{
		{}:                "NunitoSans-Regular.ttf",
		{Italic: true}:    "NunitoSans-Regular.ttf",
		{Bold: true}:      "NunitoSans-Bold.ttf",
		{Monospace: true}: "JetBrainsMono-Medium.ttf",
	} {
		if got := th.Font(style); got.Name() != want || len(got.Content()) == 0 {
			t.Errorf("Font(%+v) = %s, want %s", style, got.Name(), want)
		}
	}
	if Display.Name() != "Fredoka-SemiBold.ttf" || len(Display.Content()) == 0 {
		t.Errorf("Display = %s", Display.Name())
	}
}

func TestCornersAreRounded(t *testing.T) {
	th := New()
	if th.Size(theme.SizeNameCardRadius) != 18 || th.Size(theme.SizeNameButtonRadius) != 12 {
		t.Error("the card and button radii are not Warm's")
	}
	if th.Size(theme.SizeNameText) != theme.DefaultTheme().Size(theme.SizeNameText) {
		t.Error("the text size should stay Fyne's")
	}
}

// Every pose has to draw: Fyne's SVG rasteriser gives up on what it does not
// support, and a blank bird would only show up on a screen.
func TestEveryPoseDraws(t *testing.T) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(New())
	poses := []Pose{Perched, Blink, Landing, Looking, Asleep, Hop, Chirping, Flying}
	for _, p := range poses {
		img := canvas.NewImageFromResource(Bird(p))
		img.Resize(fyne.NewSize(120, 120))
		w := a.NewWindow(string(p))
		w.SetContent(img)
		w.Resize(fyne.NewSize(120, 120))
		if painted(w.Canvas().Capture()) < 500 {
			t.Errorf("pose %s draws almost nothing", p)
		}
	}
	if Bird("no such pose") != Bird(Perched) {
		t.Error("an unknown pose should fall back to perched")
	}
	if len(AppIcon.Content()) == 0 {
		t.Error("no app icon")
	}
}

// painted counts the pixels that differ from the top-left corner's.
func painted(img image.Image) int {
	b := img.Bounds()
	bg := img.At(b.Min.X, b.Min.Y)
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.At(x, y) != bg {
				n++
			}
		}
	}
	return n
}

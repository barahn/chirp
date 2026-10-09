// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package warm

import (
	"image/color"
	"math"
	"testing"
)

// The contrast barahn/chirp#53 asked to be measured before the palette was
// adopted: WCAG 2's ratio, 4.5:1 for text and 3:1 for the edges of controls,
// in both variants.

func luminance(c color.NRGBA) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

func contrast(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestTheWarmPaletteIsReadable(t *testing.T) {
	for name, p := range map[string]palette{"light": light, "dark": dark} {
		for _, pair := range []struct {
			what   string
			fg, bg color.NRGBA
			min    float64
		}{
			{"text on the window", p.text, p.bg, 4.5},
			{"text on a card or field", p.text, p.surface, 4.5},
			{"text on a button", p.text, p.button, 4.5},
			{"muted text on the window", p.muted, p.bg, 4.5},
			{"muted text on a card", p.muted, p.surface, 4.5},
			{"a primary button's label", p.onPrimary, p.primary, 4.5},
			{"a warning label", p.warnText, p.bg, 4.5},
			{"text in a warning box", p.warnText, p.warnBg, 4.5},
			{"a danger label", p.danger, p.bg, 4.5},
			{"a danger button's label", p.onDanger, p.danger, 4.5},
			{"a success label", p.ok, p.bg, 4.5},
			{"a success button's label", p.onOk, p.ok, 4.5},
			{"text on the new-code highlight", p.text, p.tint, 4.5},
			{"a focused control's edge", p.primary, p.bg, 3},
		} {
			if got := contrast(pair.fg, pair.bg); got < pair.min {
				t.Errorf("%s: %s is %.2f:1, needs %.1f:1", name, pair.what, got, pair.min)
			}
		}
	}
}

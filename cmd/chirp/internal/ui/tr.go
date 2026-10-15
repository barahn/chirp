// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

// The window's words are written in English in the code, and each English
// text is also the key to its translation, as with Fyne's lang.L. A
// translation file maps one to the other, in go-i18n's flat JSON form.
//
// Keying by the English text means a sentence that changes in English shows
// in English until it is translated again, rather than showing the old
// translation of what it used to say. For the consent prompt that matters:
// its wording is reviewed in every language (docs/ux.md, Consent), and
// TestEveryWordIsTranslated fails until the translation catches up.
//
// The language is chosen once, by UseLanguage, before the window is built.
// Until then, and in tests that do not choose one, Chirp speaks English
// whatever the system's language, so tests read the same on every machine.

//go:embed translations/*.json
var translations embed.FS

// languages maps a language, as the base of a locale, to its translation
// file. Portuguese from Portugal gets the Brazilian words, which it reads
// without trouble, rather than English.
var languages = map[string]string{
	"pt": "translations/pt-BR.json",
}

// words is the chosen translation, or nil for English.
var words map[string]string

// Speaks reports whether Chirp has words for locale: English, which the code
// is written in, or a language with a translation file.
func Speaks(locale string) bool {
	base := baseOf(locale)
	_, ok := languages[base]
	return ok || base == "en"
}

// baseOf is the language of a locale, lower case: "pt" for "pt-BR" or
// "pt_BR".
func baseOf(locale string) string {
	base, _, _ := strings.Cut(strings.ReplaceAll(locale, "_", "-"), "-")
	return strings.ToLower(base)
}

// UseLanguage chooses the words for locale, such as "pt-BR" from Fyne's
// lang.SystemLocale. A language Chirp has no translation for stays English.
func UseLanguage(locale string) error {
	file, ok := languages[baseOf(locale)]
	if !ok {
		words = nil
		return nil
	}
	m, err := loadWords(file)
	if err != nil {
		words = nil
		return err
	}
	words = m
	return nil
}

func loadWords(file string) (map[string]string, error) {
	data, err := translations.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return m, nil
}

// tr is s in the chosen language, or s itself when it has no translation.
func tr(s string) string {
	if t, ok := words[s]; ok && t != "" {
		return t
	}
	return s
}

// trf translates format, then formats it with args.
func trf(format string, args ...any) string {
	return fmt.Sprintf(tr(format), args...)
}

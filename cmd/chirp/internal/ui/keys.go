// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// domKeys maps Fyne's key names to the DOM's KeyboardEvent.code and key, the
// form remotekit's injectors take (input.KeyboardEvent). Fyne names keys by
// their position on a US layout, as the DOM's codes do, so a key goes over as
// the same physical key and the other computer's layout decides what it types.
var domKeys = map[fyne.KeyName][2]string{
	fyne.KeyEscape:    {"Escape", "Escape"},
	fyne.KeyReturn:    {"Enter", "Enter"},
	fyne.KeyEnter:     {"NumpadEnter", "Enter"},
	fyne.KeyTab:       {"Tab", "Tab"},
	fyne.KeyBackspace: {"Backspace", "Backspace"},
	fyne.KeyInsert:    {"Insert", "Insert"},
	fyne.KeyDelete:    {"Delete", "Delete"},
	fyne.KeyRight:     {"ArrowRight", "ArrowRight"},
	fyne.KeyLeft:      {"ArrowLeft", "ArrowLeft"},
	fyne.KeyDown:      {"ArrowDown", "ArrowDown"},
	fyne.KeyUp:        {"ArrowUp", "ArrowUp"},
	fyne.KeyPageUp:    {"PageUp", "PageUp"},
	fyne.KeyPageDown:  {"PageDown", "PageDown"},
	fyne.KeyHome:      {"Home", "Home"},
	fyne.KeyEnd:       {"End", "End"},
	fyne.KeySpace:     {"Space", " "},

	fyne.KeyApostrophe:   {"Quote", "'"},
	fyne.KeyComma:        {"Comma", ","},
	fyne.KeyMinus:        {"Minus", "-"},
	fyne.KeyPeriod:       {"Period", "."},
	fyne.KeySlash:        {"Slash", "/"},
	fyne.KeyBackslash:    {"Backslash", "\\"},
	fyne.KeyLeftBracket:  {"BracketLeft", "["},
	fyne.KeyRightBracket: {"BracketRight", "]"},
	fyne.KeySemicolon:    {"Semicolon", ";"},
	fyne.KeyEqual:        {"Equal", "="},
	fyne.KeyAsterisk:     {"NumpadMultiply", "*"},
	fyne.KeyPlus:         {"NumpadAdd", "+"},
	fyne.KeyBackTick:     {"Backquote", "`"},

	desktop.KeyShiftLeft:    {"ShiftLeft", "Shift"},
	desktop.KeyShiftRight:   {"ShiftRight", "Shift"},
	desktop.KeyControlLeft:  {"ControlLeft", "Control"},
	desktop.KeyControlRight: {"ControlRight", "Control"},
	desktop.KeyAltLeft:      {"AltLeft", "Alt"},
	desktop.KeyAltRight:     {"AltRight", "Alt"},
	desktop.KeySuperLeft:    {"MetaLeft", "Meta"},
	desktop.KeySuperRight:   {"MetaRight", "Meta"},
	desktop.KeyMenu:         {"ContextMenu", "ContextMenu"},
	desktop.KeyPrintScreen:  {"PrintScreen", "PrintScreen"},
	desktop.KeyCapsLock:     {"CapsLock", "CapsLock"},
}

func init() {
	for c := 'A'; c <= 'Z'; c++ {
		domKeys[fyne.KeyName(string(c))] = [2]string{"Key" + string(c), string(c + 'a' - 'A')}
	}
	for c := '0'; c <= '9'; c++ {
		domKeys[fyne.KeyName(string(c))] = [2]string{"Digit" + string(c), string(c)}
	}
	for i, name := range []fyne.KeyName{
		fyne.KeyF1, fyne.KeyF2, fyne.KeyF3, fyne.KeyF4, fyne.KeyF5, fyne.KeyF6,
		fyne.KeyF7, fyne.KeyF8, fyne.KeyF9, fyne.KeyF10, fyne.KeyF11, fyne.KeyF12,
	} {
		f := fmt.Sprintf("F%d", i+1)
		domKeys[name] = [2]string{f, f}
	}
}

// domKey returns a key's DOM code and key value, or false for one with no
// counterpart.
func domKey(name fyne.KeyName) (code, key string, ok bool) {
	k, ok := domKeys[name]
	return k[0], k[1], ok
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package alert

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

func show(title, text string) {
	// A console, when there is one, gets it too: a .bat or a terminal keeps
	// the reason after the box is closed.
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)

	// The box runs its own message loop on the calling thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	c, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, t, c, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}

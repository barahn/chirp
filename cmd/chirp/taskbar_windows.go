// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procGetWindowLong = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLong = user32.NewProc("SetWindowLongPtrW")
	procShowWindow    = user32.NewProc("ShowWindow")
)

// keepOffTaskbar makes win a tool window, which Windows leaves off the
// taskbar and out of Alt+Tab: during a session Chirp shows only in the tray
// (barahn/chirp#134). The window is hidden and shown again so the taskbar
// takes the change.
func keepOffTaskbar(win fyne.Window) {
	nw, ok := win.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		wc, ok := ctx.(driver.WindowsWindowContext)
		if !ok || wc.HWND == 0 {
			return
		}
		const (
			gwlExStyle       = ^uintptr(19) // GWL_EXSTYLE, -20
			wsExToolWindow   = 0x00000080
			wsExAppWindow    = 0x00040000
			swHide           = 0
			swShowNoActivate = 4
		)
		style, _, _ := procGetWindowLong.Call(wc.HWND, gwlExStyle)
		style = style&^wsExAppWindow | wsExToolWindow
		_, _, _ = procShowWindow.Call(wc.HWND, swHide)
		_, _, _ = procSetWindowLong.Call(wc.HWND, gwlExStyle, style)
		_, _, _ = procShowWindow.Call(wc.HWND, swShowNoActivate)
	})
}

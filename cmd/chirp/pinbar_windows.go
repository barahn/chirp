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
	procSetWindowPos  = user32.NewProc("SetWindowPos")
	procShowWindow    = user32.NewProc("ShowWindow")
)

// pinBar keeps win above every other window and off the taskbar, owned by
// owner (barahn/chirp#77, #134). Windows leaves a window owned by another off
// the taskbar unless it has WS_EX_APPWINDOW, which GLFW gives every window:
// so win is given its owner, loses that style, and is hidden and shown again
// for the taskbar to take it. It is not made a tool window: that changes its
// frame behind GLFW's back, which then draws the content 20 pixels lower
// than it reads the mouse, and the bar's buttons stop answering where they
// show (seen on Windows 11 with v0.5.0-rc.3).
func pinBar(win, owner fyne.Window) {
	ownerHWND := hwnd(owner)
	nw, ok := win.(driver.NativeWindow)
	if !ok || ownerHWND == 0 {
		return
	}
	nw.RunNative(func(ctx any) {
		wc, ok := ctx.(driver.WindowsWindowContext)
		if !ok || wc.HWND == 0 {
			return
		}
		const (
			gwlpHWNDParent   = ^uintptr(7)  // GWLP_HWNDPARENT, -8: the owner
			gwlExStyle       = ^uintptr(19) // GWL_EXSTYLE, -20
			wsExAppWindow    = 0x00040000
			swHide           = 0
			swShowNoActivate = 4
		)
		_, _, _ = procShowWindow.Call(wc.HWND, swHide)
		_, _, _ = procSetWindowLong.Call(wc.HWND, gwlpHWNDParent, ownerHWND)
		style, _, _ := procGetWindowLong.Call(wc.HWND, gwlExStyle)
		_, _, _ = procSetWindowLong.Call(wc.HWND, gwlExStyle, style&^wsExAppWindow)
		_, _, _ = procShowWindow.Call(wc.HWND, swShowNoActivate)
		topmost(wc.HWND)
	})
}

// keepOnTop keeps win above every other window and leaves it on the taskbar
// (barahn/chirp#147).
func keepOnTop(win fyne.Window) {
	if h := hwnd(win); h != 0 {
		topmost(h)
	}
}

// topmost puts the window h in the topmost band, without moving, resizing or
// activating it.
func topmost(h uintptr) {
	const (
		hwndTopmost   = ^uintptr(0) // HWND_TOPMOST, -1
		swpNoSize     = 0x0001
		swpNoMove     = 0x0002
		swpNoActivate = 0x0010
	)
	_, _, _ = procSetWindowPos.Call(h, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

// hwnd is win's native handle, or 0.
func hwnd(win fyne.Window) uintptr {
	var h uintptr
	if nw, ok := win.(driver.NativeWindow); ok {
		nw.RunNative(func(ctx any) {
			if wc, ok := ctx.(driver.WindowsWindowContext); ok {
				h = wc.HWND
			}
		})
	}
	return h
}

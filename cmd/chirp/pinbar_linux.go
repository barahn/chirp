// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// pinBar asks the X11 window manager to keep the session bar above other
// windows (_NET_WM_STATE_ABOVE, barahn/chirp#77) and off the taskbar and the
// pager (_NET_WM_STATE_SKIP_TASKBAR, _NET_WM_STATE_SKIP_PAGER), so that
// during a session Chirp shows only in the tray (#134) and the bar cannot be
// lost behind other windows. Under Wayland there are no such requests, and
// nothing is done.
func pinBar(bar, _ fyne.Window) {
	addWMState(bar, "_NET_WM_STATE_SKIP_TASKBAR", "_NET_WM_STATE_SKIP_PAGER", "_NET_WM_STATE_ABOVE")
}

// keepOnTop asks the X11 window manager to keep win above other windows
// (_NET_WM_STATE_ABOVE), leaving it on the taskbar (barahn/chirp#147).
func keepOnTop(win fyne.Window) {
	addWMState(win, "_NET_WM_STATE_ABOVE")
}

// addWMState asks the window manager to add the named _NET_WM_STATE states
// to win. It does nothing under Wayland, or where X11 cannot be reached.
func addWMState(win fyne.Window, states ...string) {
	nw, ok := win.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		xc, ok := ctx.(driver.X11WindowContext)
		if !ok || xc.WindowHandle == 0 {
			return
		}
		conn, err := xgb.NewConn()
		if err != nil {
			return
		}
		defer conn.Close()
		atom := func(name string) xproto.Atom {
			r, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply() // #nosec G115 -- constant names
			if err != nil {
				return 0
			}
			return r.Atom
		}
		state := atom("_NET_WM_STATE")
		if state == 0 {
			return
		}
		root := xproto.Setup(conn).DefaultScreen(conn).Root
		// Each request carries at most two states.
		for i := 0; i < len(states); i += 2 {
			first, second := atom(states[i]), xproto.Atom(0)
			if i+1 < len(states) {
				second = atom(states[i+1])
			}
			const netWMStateAdd = 1
			ev := xproto.ClientMessageEvent{
				Format: 32,
				Window: xproto.Window(xc.WindowHandle), // #nosec G115 -- an X11 window ID
				Type:   state,
				Data:   xproto.ClientMessageDataUnionData32New([]uint32{netWMStateAdd, uint32(first), uint32(second), 1, 0}),
			}
			mask := uint32(xproto.EventMaskSubstructureRedirect | xproto.EventMaskSubstructureNotify)
			_ = xproto.SendEventChecked(conn, false, root, mask, string(ev.Bytes())).Check()
		}
	})
}

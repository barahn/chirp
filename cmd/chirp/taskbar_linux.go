// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// keepOffTaskbar asks the X11 window manager to leave win off the taskbar
// and the pager (_NET_WM_STATE_SKIP_TASKBAR, _NET_WM_STATE_SKIP_PAGER), so
// that during a session Chirp shows only in the tray (barahn/chirp#134).
// Under Wayland there is no such request, and nothing is done.
func keepOffTaskbar(win fyne.Window) {
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
		skipTaskbar, skipPager := atom("_NET_WM_STATE_SKIP_TASKBAR"), atom("_NET_WM_STATE_SKIP_PAGER")
		if state == 0 || skipTaskbar == 0 {
			return
		}
		root := xproto.Setup(conn).DefaultScreen(conn).Root
		const netWMStateAdd = 1
		ev := xproto.ClientMessageEvent{
			Format: 32,
			Window: xproto.Window(xc.WindowHandle), // #nosec G115 -- an X11 window ID
			Type:   state,
			Data:   xproto.ClientMessageDataUnionData32New([]uint32{netWMStateAdd, uint32(skipTaskbar), uint32(skipPager), 1, 0}),
		}
		mask := uint32(xproto.EventMaskSubstructureRedirect | xproto.EventMaskSubstructureNotify)
		_ = xproto.SendEventChecked(conn, false, root, mask, string(ev.Bytes())).Check()
	})
}

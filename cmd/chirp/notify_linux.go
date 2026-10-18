// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"bytes"
	"image"
	"image/draw"
	"sync"

	"fyne.io/fyne/v2"
	"github.com/fyne-io/oksvg"
	"github.com/godbus/dbus/v5"
	"github.com/srwiley/rasterx"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// desktopNotifier shows alerts through the freedesktop notification service
// itself. Fyne's notifications never expire (it sends a timeout of 0, which
// means never), so on Xfce they piled up on screen. Chirp's expire after
// alertTimeout, each replaces the one before, and the bird goes with them as
// pixels in the call, so nothing is written to disk for it (#49).
type desktopNotifier struct {
	notify func(replaces uint32, title, body string, hints map[string]dbus.Variant, timeout int32) (uint32, error)

	mu   sync.Mutex
	last uint32
}

// newNotifier returns how alerts are shown: through the session bus, or
// Fyne's notifications when there is no bus to reach.
func newNotifier(a fyne.App) func(*fyne.Notification) {
	conn, err := dbus.SessionBus() // shared connection, not closed
	if err != nil {
		return a.SendNotification
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	n := &desktopNotifier{notify: func(replaces uint32, title, body string, hints map[string]dbus.Variant, timeout int32) (uint32, error) {
		var id uint32
		err := obj.Call("org.freedesktop.Notifications.Notify", 0,
			"Chirp", replaces, "", title, body, []string{}, hints, timeout).Store(&id)
		return id, err
	}}
	return n.send
}

func (n *desktopNotifier) send(no *fyne.Notification) {
	n.mu.Lock()
	defer n.mu.Unlock()
	hints := map[string]dbus.Variant{"urgency": dbus.MakeVariant(byte(1))}
	if icon, ok := birdImageData(); ok {
		hints["image-data"] = dbus.MakeVariant(icon)
	}
	id, err := n.notify(n.last, no.Title, no.Content, hints, int32(alertTimeout.Milliseconds()))
	if err != nil {
		n.last = 0
		return
	}
	n.last = id
}

// imageData is the notification spec's image-data hint, (iiibiiay).
type imageData struct {
	Width, Height, RowStride int32
	HasAlpha                 bool
	BitsPerSample, Channels  int32
	Data                     []byte
}

var (
	birdOnce sync.Once
	birdIcon imageData
	birdOK   bool
)

// birdImageData is the app icon drawn at 64 pixels, once.
func birdImageData() (imageData, bool) {
	birdOnce.Do(func() {
		icon, err := oksvg.ReadIconStream(bytes.NewReader(warm.AppIcon.Content()))
		if err != nil {
			return
		}
		const size = 64
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
		icon.SetTarget(0, 0, size, size)
		icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)
		birdIcon = imageData{Width: size, Height: size, RowStride: int32(img.Stride), HasAlpha: true, BitsPerSample: 8, Channels: 4, Data: img.Pix}
		birdOK = true
	})
	return birdIcon, birdOK
}

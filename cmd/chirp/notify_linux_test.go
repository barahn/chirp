// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"github.com/godbus/dbus/v5"
)

// notifyCall is one org.freedesktop.Notifications.Notify the notifier made.
type notifyCall struct {
	replaces uint32
	title    string
	body     string
	hints    map[string]dbus.Variant
	timeout  int32
}

// Seen end to end on Xfce: Fyne's notifications never expire, and pile up.
// Chirp's go away on their own after alertTimeout, and each replaces the one
// before, so one alert at a time is on screen.
func TestAlertsExpireAndReplaceEachOther(t *testing.T) {
	var calls []notifyCall
	n := &desktopNotifier{notify: func(replaces uint32, title, body string, hints map[string]dbus.Variant, timeout int32) (uint32, error) {
		calls = append(calls, notifyCall{replaces, title, body, hints, timeout})
		return 41 + uint32(len(calls)), nil
	}}
	n.send(fyne.NewNotification("Chirp", "first"))
	n.send(fyne.NewNotification("Chirp", "second"))
	if len(calls) != 2 {
		t.Fatalf("%d calls, want 2", len(calls))
	}
	if calls[0].replaces != 0 || calls[1].replaces != 42 {
		t.Errorf("replaces %d then %d, want 0 then the first's id 42", calls[0].replaces, calls[1].replaces)
	}
	for _, c := range calls {
		if c.timeout != int32(alertTimeout.Milliseconds()) || c.timeout <= 0 {
			t.Errorf("timeout %d ms, want %d", c.timeout, alertTimeout.Milliseconds())
		}
		if _, ok := c.hints["image-data"]; !ok {
			t.Error("no image-data hint: the bird is sent in memory, not from a file")
		}
	}
	if calls[1].title != "Chirp" || calls[1].body != "second" {
		t.Errorf("sent %q / %q", calls[1].title, calls[1].body)
	}
}

// When the notification service fails, the next alert is a new one.
func TestAFailedAlertDoesNotReplaceAnything(t *testing.T) {
	var replaces []uint32
	fail := true
	n := &desktopNotifier{notify: func(r uint32, _, _ string, _ map[string]dbus.Variant, _ int32) (uint32, error) {
		replaces = append(replaces, r)
		if fail {
			return 0, dbus.ErrClosed
		}
		return 7, nil
	}}
	n.send(fyne.NewNotification("Chirp", "a"))
	fail = false
	n.send(fyne.NewNotification("Chirp", "b"))
	if replaces[1] != 0 {
		t.Errorf("after a failure the next alert replaced %d", replaces[1])
	}
}

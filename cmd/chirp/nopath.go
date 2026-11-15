// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import "time"

// noPictureAfter is how long a session the helper was let into may go
// without a picture, and without word from the other side about why, before
// the helper is told the two computers could not connect (barahn/chirp#151).
// A path through TURN connects within seconds; ICE gives up on a direct one
// within about half a minute and says nothing the viewer passes on.
const noPictureAfter = 20 * time.Second

// pictureWatch calls tell once if nothing settles the session in time: a
// picture, or a notice that says why there is none.
type pictureWatch struct {
	timer *time.Timer
}

func watchForPicture(after time.Duration, tell func()) *pictureWatch {
	return &pictureWatch{timer: time.AfterFunc(after, tell)}
}

func (w *pictureWatch) settled() { w.stop() }

func (w *pictureWatch) stop() { w.timer.Stop() }

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

// alertTimeoutMS is how long an alert stays on screen, in milliseconds, as
// the notification service takes it (barahn/chirp#134). Long enough to read
// a sentence, short enough not to pile up while the person helping copies
// and pastes.
const alertTimeoutMS int32 = 6000

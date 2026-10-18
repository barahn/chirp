// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import "time"

// alertTimeout is how long an alert stays on screen (barahn/chirp#134).
// Long enough to read a sentence, short enough not to pile up while the
// person helping copies and pastes.
const alertTimeout = 6 * time.Second

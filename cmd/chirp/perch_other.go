// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows

package main

import "github.com/barahn/chirp/cmd/chirp/internal/ui"

// newPerch has no bird to draw here yet: the balloon is the session window.
func newPerch() ui.Perch { return nil }

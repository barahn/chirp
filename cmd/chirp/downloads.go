// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"os"
	"path/filepath"
)

// receivedDir is where files the controller sends are saved on the owner's
// machine: a Chirp folder in the user's Downloads, so they are where people
// look for what arrived and apart from what they downloaded themselves
// (barahn/chirp#79). remotekit's runner creates it on the first file, and
// never replaces a file already there. Without a home directory, it is the
// system's temporary directory, as remotekit's own default.
func receivedDir() string {
	if d := downloadsDir(); d != "" {
		return filepath.Join(d, "Chirp")
	}
	return filepath.Join(os.TempDir(), "Chirp")
}

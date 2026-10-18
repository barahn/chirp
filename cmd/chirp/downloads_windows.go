// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// downloadsDir is the user's Downloads folder, wherever they moved it, or
// Downloads in the profile if Windows cannot say. It is "" without either.
func downloadsDir() string {
	if d, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil && d != "" {
		return d
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Downloads")
	}
	return ""
}

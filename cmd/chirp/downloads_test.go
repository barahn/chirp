// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTheDownloadsFolderFollowsTheDesktop(t *testing.T) {
	home := t.TempDir()
	dirs := filepath.Join(home, "user-dirs.dirs")
	for _, tc := range []struct{ file, want string }{
		{"# written by xdg-user-dirs-update\nXDG_DESKTOP_DIR=\"$HOME/Área de trabalho\"\nXDG_DOWNLOAD_DIR=\"$HOME/Transferências\"\n", filepath.Join(home, "Transferências")},
		{"XDG_DOWNLOAD_DIR=\"/data/incoming\"\n", "/data/incoming"},
		{"XDG_DOWNLOAD_DIR=\"$HOME/\"\n", ""},
		{"XDG_DOWNLOAD_DIR=\"relative\"\n", ""},
		{"XDG_DOWNLOAD_DIR=unquoted\n", ""},
		{"XDG_MUSIC_DIR=\"$HOME/Music\"\n", ""},
	} {
		if err := os.WriteFile(dirs, []byte(tc.file), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := xdgDownloadDir(dirs, home); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.file, got, tc.want)
		}
	}
	if got := xdgDownloadDir(filepath.Join(home, "missing"), home); got != "" {
		t.Errorf("a missing file gave %q", got)
	}
}

func TestReceivedFilesGoToAChirpFolderInDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "nowhere"))
	if got, want := receivedDir(), filepath.Join(home, "Downloads", "Chirp"); got != want {
		t.Errorf("receivedDir() = %q, want %q", got, want)
	}
}

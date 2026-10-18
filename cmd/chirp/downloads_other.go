// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// downloadsDir is the user's Downloads folder: XDG_DOWNLOAD_DIR from the
// desktop's user-dirs.dirs, which names it in the desktop's language, or
// Downloads in the home directory. It is "" without a home directory.
func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	if d := xdgDownloadDir(filepath.Join(config, "user-dirs.dirs"), home); d != "" {
		return d
	}
	return filepath.Join(home, "Downloads")
}

// xdgDownloadDir reads XDG_DOWNLOAD_DIR from a user-dirs.dirs file. The file
// is shell assignments, each value quoted and either absolute or starting
// with $HOME/; a value set to $HOME alone means the desktop has no Downloads
// folder of its own, and is ignored.
func xdgDownloadDir(file, home string) string {
	f, err := os.Open(file) // #nosec G304 -- the desktop's own settings file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		value, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "XDG_DOWNLOAD_DIR=")
		if !ok {
			continue
		}
		v, err := strconv.Unquote(value)
		if err != nil {
			return ""
		}
		switch {
		case strings.HasPrefix(v, "$HOME/"):
			v = filepath.Join(home, v[len("$HOME/"):])
		case !filepath.IsAbs(v):
			return ""
		}
		if filepath.Clean(v) == filepath.Clean(home) {
			return ""
		}
		return v
	}
	return ""
}

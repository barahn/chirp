// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/barahn/chirp/signalling"
)

// turnConfig turns TURN on when both the secret file and the URLs are given,
// and leaves it off when neither is. The secret is the one coturn holds as
// static-auth-secret; it is read from a file only its owner can read, never
// from a flag or relay.env, which other users of the machine can see.
func turnConfig(secretFile, urlList string) (*signalling.TURN, error) {
	if secretFile == "" && strings.TrimSpace(urlList) == "" {
		return nil, nil
	}
	if secretFile == "" {
		return nil, errors.New("-turn-urls needs -turn-secret-file")
	}
	if strings.TrimSpace(urlList) == "" {
		return nil, errors.New("-turn-secret-file needs -turn-urls")
	}
	secret, err := readSecret(secretFile)
	if err != nil {
		return nil, err
	}
	urls, err := turnURLs(urlList)
	if err != nil {
		return nil, err
	}
	return &signalling.TURN{Secret: secret, URLs: urls}, nil
}

func readSecret(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("-turn-secret-file: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("-turn-secret-file: %s is readable by others (mode %04o); chmod 600 it", path, info.Mode().Perm())
	}
	b, err := os.ReadFile(path) // #nosec G304 -- the operator names this file
	if err != nil {
		return "", fmt.Errorf("-turn-secret-file: %w", err)
	}
	secret := strings.TrimSpace(string(b))
	if secret == "" {
		return "", fmt.Errorf("-turn-secret-file: %s is empty", path)
	}
	return secret, nil
}

func turnURLs(list string) ([]string, error) {
	var urls []string
	for _, u := range strings.Split(list, ",") {
		u = strings.TrimSpace(u)
		if !strings.HasPrefix(u, "turn:") && !strings.HasPrefix(u, "turns:") {
			return nil, fmt.Errorf("-turn-urls: %q is not a turn: or turns: URL", u)
		}
		urls = append(urls, u)
	}
	return urls, nil
}

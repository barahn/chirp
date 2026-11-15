// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func secretFile(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "turn.secret")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

const exampleTURNURLs = "turn:relay.example:3478?transport=udp,turn:relay.example:3478?transport=tcp"

func TestNoTURNSettingsLeaveTURNOff(t *testing.T) {
	turn, err := turnConfig("", "")
	if err != nil || turn != nil {
		t.Fatalf("turnConfig() = %v, %v; want TURN off", turn, err)
	}
}

func TestTURNReadsItsSecretAndURLs(t *testing.T) {
	turn, err := turnConfig(secretFile(t, "  the shared secret\n", 0o600), exampleTURNURLs)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Secret != "the shared secret" {
		t.Errorf("secret = %q, want it trimmed", turn.Secret)
	}
	if strings.Join(turn.URLs, ",") != exampleTURNURLs {
		t.Errorf("urls = %v", turn.URLs)
	}
}

func TestTURNNeedsBothTheSecretAndTheURLs(t *testing.T) {
	if _, err := turnConfig(secretFile(t, "s", 0o600), ""); err == nil {
		t.Error("a secret without URLs was accepted")
	}
	if _, err := turnConfig("", exampleTURNURLs); err == nil {
		t.Error("URLs without a secret were accepted")
	}
}

func TestTURNRefusesABadSecretFile(t *testing.T) {
	if _, err := turnConfig(filepath.Join(t.TempDir(), "missing"), exampleTURNURLs); err == nil {
		t.Error("a missing secret file was accepted")
	}
	if _, err := turnConfig(secretFile(t, " \n", 0o600), exampleTURNURLs); err == nil {
		t.Error("an empty secret was accepted")
	}
	if runtime.GOOS != "windows" {
		if _, err := turnConfig(secretFile(t, "s", 0o640), exampleTURNURLs); err == nil {
			t.Error("a secret file others can read was accepted")
		}
	}
}

func TestTURNURLsMustBeTURN(t *testing.T) {
	for _, urls := range []string{"stun:relay.example:3478", "https://relay.example", "turn:relay.example:3478,", " "} {
		if _, err := turnConfig(secretFile(t, "s", 0o600), urls); err == nil {
			t.Errorf("urls %q were accepted", urls)
		}
	}
}

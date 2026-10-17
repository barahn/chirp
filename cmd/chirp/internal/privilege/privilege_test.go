// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows

package privilege

import (
	"os"
	"testing"
)

func TestElevatedMeansRoot(t *testing.T) {
	if got, want := Elevated(), os.Geteuid() == 0; got != want {
		t.Fatalf("Elevated() = %v with euid %d", got, os.Geteuid())
	}
}

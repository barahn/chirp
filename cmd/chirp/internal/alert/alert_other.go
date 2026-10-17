// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows

package alert

import (
	"fmt"
	"os"
)

func show(title, text string) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, text) }

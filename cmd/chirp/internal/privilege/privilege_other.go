// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

//go:build !windows

package privilege

import "os"

func elevated() bool { return os.Geteuid() == 0 }

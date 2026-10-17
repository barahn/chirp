// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package privilege

import "golang.org/x/sys/windows"

// elevated reads the process token. With UAC on, an administrator's token is
// elevated only when Chirp was started "as administrator", or by an elevated
// parent; with UAC off, an administrator's always is.
func elevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

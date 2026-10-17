// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package privilege tells whether Chirp runs with administrator rights.
//
// Whoever the owner lets in acts with the owner's Chirp's own rights: its
// account, and its elevation if it has any (GHSA-r385-j539-p738,
// barahn/chirp#117). An elevated Chirp hands the helper full control of the
// machine, past every UAC prompt the owner would otherwise answer.
package privilege

// Elevated reports whether this process runs with administrator rights: an
// elevated token on Windows, an effective user ID of 0 elsewhere.
func Elevated() bool { return elevated() }

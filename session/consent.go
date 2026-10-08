// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package session

import (
	"time"
)

// DefaultPermissions requested for on-demand remote support sessions.
//
// The names are remotekit's: they are what tunnel.AgentStreamRunner.Grant
// accepts (tunnel.PermissionScreenView and its siblings), spelled out here
// because Chirp does not import tunnel. A name remotekit does not know grants
// nothing, and the feature behind it stays refused without an error.
var DefaultPermissions = []string{
	"screen_view",
	"remote_control",
	"clipboard",
	"file_transfer",
}

// ConsentDecision represents the end-user authorization response.
type ConsentDecision struct {
	Granted   bool      `json:"granted"`
	Reason    string    `json:"reason,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

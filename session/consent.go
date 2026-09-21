// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package chirp

import (
	"time"
)

// DefaultPermissions requested for on-demand remote support sessions.
var DefaultPermissions = []string{
	"screen_view",
	"remote_control",
	"clipboard_sync",
	"file_transfer",
}

// ConsentDecision represents the end-user authorization response.
type ConsentDecision struct {
	Granted   bool      `json:"granted"`
	Reason    string    `json:"reason,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

// ConsentRequest encapsulates the parameters shown to the end-user in the authorization dialog.
type ConsentRequest struct {
	SessionID            string    `json:"session_id"`
	Code                 string    `json:"code"`
	FormattedCode        string    `json:"formatted_code"`
	TechnicianName       string    `json:"technician_name"`
	RequestedPermissions []string  `json:"requested_permissions"`
	CreatedAt            time.Time `json:"created_at"`
}

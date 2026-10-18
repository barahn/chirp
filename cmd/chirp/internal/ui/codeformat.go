// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import "strings"

// codeGroups are the lengths of a code's groups as people read it: six
// digits that route and four that are secret, 482-913-5071.
var codeGroups = []int{3, 3, 4}

// formatCode puts a code's dashes in as it is typed. Only digits, with the
// spaces, dashes and dots people put between them, and no more than a full
// code's ten, are formatted; anything else -- a link, its token -- is
// returned as it came.
func formatCode(s string) string {
	var digits strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == ' ' || r == '-' || r == '.':
		default:
			return s
		}
	}
	d := digits.String()
	max := 0
	for _, g := range codeGroups {
		max += g
	}
	if len(d) > max {
		return s
	}
	var out strings.Builder
	for _, g := range codeGroups {
		if d == "" {
			break
		}
		if out.Len() > 0 {
			out.WriteByte('-')
		}
		n := min(g, len(d))
		out.WriteString(d[:n])
		d = d[n:]
	}
	return out.String()
}

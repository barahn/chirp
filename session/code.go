package chirp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

var (
	ErrInvalidCodeLength = errors.New("chirp: session code must be exactly 6 digits")
	ErrInvalidCodeFormat = errors.New("chirp: session code must contain only numbers")
)

// GenerateCode produces a cryptographically secure 6-digit numeric PIN string in the range [100000, 999999].
func GenerateCode() (string, error) {
	// Range: 100000 to 999999 inclusive (900,000 possible combinations)
	maxVal := big.NewInt(900000)
	n, err := rand.Int(rand.Reader, maxVal)
	if err != nil {
		return "", fmt.Errorf("failed to generate random session code: %w", err)
	}
	codeNum := n.Int64() + 100000
	return fmt.Sprintf("%06d", codeNum), nil
}

// FormatCode returns the 6-digit code formatted with a hyphen for human readability (e.g., "847291" -> "847-291").
func FormatCode(code string) string {
	normalized, err := NormalizeCode(code)
	if err != nil {
		return code
	}
	return normalized[:3] + "-" + normalized[3:]
}

// NormalizeCode removes dashes, spaces, and whitespace from a user-provided code, validating that it is exactly 6 digits.
func NormalizeCode(code string) (string, error) {
	var sb strings.Builder
	for _, r := range code {
		if unicode.IsSpace(r) || r == '-' || r == '.' {
			continue
		}
		if !unicode.IsDigit(r) {
			return "", ErrInvalidCodeFormat
		}
		sb.WriteRune(r)
	}

	result := sb.String()
	if len(result) != 6 {
		return "", ErrInvalidCodeLength
	}

	return result, nil
}

package validation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// NormalizePostText applies Unicode NFC normalization, trims leading/trailing
// whitespace, and collapses internal whitespace runs to single spaces.
// Returns an error if the result is empty.
func NormalizePostText(raw string) (string, error) {
	s := norm.NFC.String(raw)
	s = strings.TrimSpace(s)
	// Collapse internal whitespace runs.
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	result := b.String()
	if result == "" {
		return "", fmt.Errorf("post text is empty after normalization")
	}
	return result, nil
}

// ValidatePostText checks that text meets all protocol requirements.
// The text must already be normalized (caller should call NormalizePostText first).
func ValidatePostText(text string, maxCodePoints int, maxBytes int) error {
	if !utf8.ValidString(text) {
		return fmt.Errorf("post text is not valid UTF-8")
	}
	if text == "" {
		return fmt.Errorf("post text is empty")
	}
	// Check NFC normalization.
	if !norm.NFC.IsNormalString(text) {
		return fmt.Errorf("post text is not NFC-normalized")
	}
	// Count code points.
	cpCount := utf8.RuneCountInString(text)
	if cpCount > maxCodePoints {
		return fmt.Errorf("post text exceeds maximum length: %d code points (max %d)", cpCount, maxCodePoints)
	}
	// Check byte length.
	if len(text) > maxBytes {
		return fmt.Errorf("post text exceeds maximum byte size: %d bytes (max %d)", len(text), maxBytes)
	}
	return nil
}

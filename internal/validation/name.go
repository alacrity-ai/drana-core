package validation

import (
	"fmt"
	"strings"
)

const (
	MinNameLength = 3
	MaxNameLength = 20
)

// ValidateName checks that a name meets all protocol rules.
func ValidateName(name string) error {
	if len(name) < MinNameLength {
		return fmt.Errorf("name too short: %d characters (min %d)", len(name), MinNameLength)
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("name too long: %d characters (max %d)", len(name), MaxNameLength)
	}
	for i, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("invalid character %q at position %d (allowed: a-z, 0-9, _)", c, i)
		}
	}
	if name[0] == '_' {
		return fmt.Errorf("name must not start with underscore")
	}
	if name[len(name)-1] == '_' {
		return fmt.Errorf("name must not end with underscore")
	}
	if strings.Contains(name, "__") {
		return fmt.Errorf("name must not contain consecutive underscores")
	}
	return nil
}

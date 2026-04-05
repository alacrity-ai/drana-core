package validation

import (
	"strings"
	"testing"
)

func TestValidNamePasses(t *testing.T) {
	valid := []string{"alice", "user_42", "abc", "a_b", "hello_world_123"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Fatalf("ValidateName(%q) should pass: %v", name, err)
		}
	}
}

func TestNameTooShort(t *testing.T) {
	if err := ValidateName("ab"); err == nil {
		t.Fatal("2-char name should fail")
	}
}

func TestNameTooLong(t *testing.T) {
	if err := ValidateName(strings.Repeat("a", 21)); err == nil {
		t.Fatal("21-char name should fail")
	}
}

func TestNameExactBounds(t *testing.T) {
	if err := ValidateName("abc"); err != nil {
		t.Fatalf("3-char name should pass: %v", err)
	}
	if err := ValidateName(strings.Repeat("a", 20)); err != nil {
		t.Fatalf("20-char name should pass: %v", err)
	}
}

func TestNameInvalidChars(t *testing.T) {
	invalid := []string{"Alice", "user!", "user name", "user@home", "UPPER", "with-dash"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Fatalf("ValidateName(%q) should fail", name)
		}
	}
}

func TestNameLeadingUnderscore(t *testing.T) {
	if err := ValidateName("_alice"); err == nil {
		t.Fatal("leading underscore should fail")
	}
}

func TestNameTrailingUnderscore(t *testing.T) {
	if err := ValidateName("alice_"); err == nil {
		t.Fatal("trailing underscore should fail")
	}
}

func TestNameConsecutiveUnderscores(t *testing.T) {
	if err := ValidateName("al__ice"); err == nil {
		t.Fatal("consecutive underscores should fail")
	}
}

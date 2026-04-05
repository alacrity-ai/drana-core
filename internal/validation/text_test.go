package validation

import (
	"strings"
	"testing"
)

func TestNormalizePostText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"plain", "hello world", "hello world", false},
		{"trim spaces", "  hello  ", "hello", false},
		{"collapse internal", "hello   world", "hello world", false},
		{"tabs and newlines", "hello\t\n\nworld", "hello world", false},
		{"empty", "", "", true},
		{"whitespace only", "   \t\n  ", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePostText(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidatePostTextASCII(t *testing.T) {
	err := ValidatePostText("hello world", 280, 1024)
	if err != nil {
		t.Fatalf("valid ASCII text rejected: %v", err)
	}
}

func TestValidatePostTextMultibyteWithinCodePoints(t *testing.T) {
	// 280 CJK characters: each is 3 bytes in UTF-8 = 840 bytes, under 1024.
	text := strings.Repeat("\u4e16", 280) // '世' repeated 280 times
	err := ValidatePostText(text, 280, 1024)
	if err != nil {
		t.Fatalf("280 CJK code points should pass: %v", err)
	}
}

func TestValidatePostTextExactLimit(t *testing.T) {
	text := strings.Repeat("a", 280)
	err := ValidatePostText(text, 280, 1024)
	if err != nil {
		t.Fatalf("exactly 280 code points should pass: %v", err)
	}
}

func TestValidatePostTextOverCodePointLimit(t *testing.T) {
	text := strings.Repeat("a", 281)
	err := ValidatePostText(text, 280, 1024)
	if err == nil {
		t.Fatal("281 code points should fail")
	}
}

func TestValidatePostTextOverByteLimit(t *testing.T) {
	// Use 4-byte emoji to exceed byte limit while under code point limit.
	// 260 x 4-byte emoji = 1040 bytes > 1024, but only 260 code points < 280.
	text := strings.Repeat("\U0001F600", 260) // 😀 repeated
	err := ValidatePostText(text, 280, 1024)
	if err == nil {
		t.Fatal("should fail on byte limit")
	}
}

func TestValidatePostTextEmpty(t *testing.T) {
	err := ValidatePostText("", 280, 1024)
	if err == nil {
		t.Fatal("empty string should fail")
	}
}

func TestValidatePostTextInvalidUTF8(t *testing.T) {
	bad := string([]byte{0xff, 0xfe, 0xfd})
	err := ValidatePostText(bad, 280, 1024)
	if err == nil {
		t.Fatal("invalid UTF-8 should fail")
	}
}

func TestValidatePostTextEmoji(t *testing.T) {
	err := ValidatePostText("Hello 🌍🔥💰", 280, 1024)
	if err != nil {
		t.Fatalf("emoji text should pass: %v", err)
	}
}

package crypto

import (
	"testing"
)

func TestAddressDeterministic(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	a1 := AddressFromPublicKey(pub)
	a2 := AddressFromPublicKey(pub)
	if a1 != a2 {
		t.Fatal("address derivation is not deterministic")
	}
}

func TestAddressRoundTrip(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := AddressFromPublicKey(pub)
	s := addr.String()

	parsed, err := ParseAddress(s)
	if err != nil {
		t.Fatalf("ParseAddress(%q): %v", s, err)
	}
	if parsed != addr {
		t.Fatalf("round-trip failed: got %v, want %v", parsed, addr)
	}
}

func TestAddressValidateGood(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := AddressFromPublicKey(pub)
	if err := addr.Validate(); err != nil {
		t.Fatalf("valid address failed Validate: %v", err)
	}
}

func TestAddressBadChecksum(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := AddressFromPublicKey(pub)
	// Corrupt checksum byte
	addr[AddressLen-1] ^= 0xff

	if err := addr.Validate(); err == nil {
		t.Fatal("corrupted checksum should fail Validate")
	}

	// Also test via ParseAddress
	s := AddressPrefix + hexEncode(addr[:])
	if _, err := ParseAddress(s); err == nil {
		t.Fatal("ParseAddress should reject bad checksum")
	}
}

func TestParseAddressWrongPrefix(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := AddressFromPublicKey(pub)
	s := "wrong1" + hexEncode(addr[:])
	if _, err := ParseAddress(s); err == nil {
		t.Fatal("ParseAddress should reject wrong prefix")
	}
}

func TestParseAddressWrongLength(t *testing.T) {
	if _, err := ParseAddress("drana1aabb"); err == nil {
		t.Fatal("ParseAddress should reject wrong length")
	}
}

func TestParseAddressTooShort(t *testing.T) {
	if _, err := ParseAddress("dr"); err == nil {
		t.Fatal("ParseAddress should reject too-short input")
	}
}

func TestAddressIsZero(t *testing.T) {
	var zero Address
	if !zero.IsZero() {
		t.Fatal("zero address should be zero")
	}
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := AddressFromPublicKey(pub)
	if addr.IsZero() {
		t.Fatal("derived address should not be zero")
	}
}

// helper — avoid importing encoding/hex in test when we only need it for one spot
func hexEncode(b []byte) string {
	const hextable = "0123456789abcdef"
	buf := make([]byte, len(b)*2)
	for i, v := range b {
		buf[i*2] = hextable[v>>4]
		buf[i*2+1] = hextable[v&0x0f]
	}
	return string(buf)
}

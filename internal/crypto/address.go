package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	addressBodyLen     = 20
	addressChecksumLen = 4
	// AddressLen is the total byte length of an Address (body + checksum).
	AddressLen = addressBodyLen + addressChecksumLen
	// AddressPrefix is the human-readable prefix for display addresses.
	AddressPrefix = "drana1"
)

// Address is a 24-byte value: 20-byte pubkey hash body + 4-byte checksum.
type Address [AddressLen]byte

// AddressFromPublicKey derives an Address from an Ed25519 public key.
//
//	pubKeyHash = SHA-256(pubKey)
//	body       = pubKeyHash[:20]
//	checksum   = SHA-256(body)[:4]
//	Address    = body || checksum
func AddressFromPublicKey(pubKey PublicKey) Address {
	pubKeyHash := sha256.Sum256(pubKey[:])
	var body [addressBodyLen]byte
	copy(body[:], pubKeyHash[:addressBodyLen])

	checksumFull := sha256.Sum256(body[:])
	var addr Address
	copy(addr[:addressBodyLen], body[:])
	copy(addr[addressBodyLen:], checksumFull[:addressChecksumLen])
	return addr
}

// String returns the drana1-prefixed hex representation of the address.
func (a Address) String() string {
	return AddressPrefix + hex.EncodeToString(a[:])
}

// Validate checks that the internal checksum is consistent with the body.
func (a Address) Validate() error {
	var body [addressBodyLen]byte
	copy(body[:], a[:addressBodyLen])
	checksumFull := sha256.Sum256(body[:])
	for i := 0; i < addressChecksumLen; i++ {
		if a[addressBodyLen+i] != checksumFull[i] {
			return fmt.Errorf("address checksum mismatch")
		}
	}
	return nil
}

// IsZero returns true if the address is all zeros.
func (a Address) IsZero() bool {
	var zero Address
	return a == zero
}

// ParseAddress parses a drana1-prefixed hex string into an Address.
func ParseAddress(s string) (Address, error) {
	var addr Address
	if len(s) < len(AddressPrefix) {
		return addr, fmt.Errorf("address too short")
	}
	if s[:len(AddressPrefix)] != AddressPrefix {
		return addr, fmt.Errorf("address must start with %q", AddressPrefix)
	}
	hexPart := s[len(AddressPrefix):]
	b, err := hex.DecodeString(hexPart)
	if err != nil {
		return addr, fmt.Errorf("invalid hex in address: %w", err)
	}
	if len(b) != AddressLen {
		return addr, fmt.Errorf("address hex must be %d bytes, got %d", AddressLen, len(b))
	}
	copy(addr[:], b)
	if err := addr.Validate(); err != nil {
		return Address{}, err
	}
	return addr, nil
}

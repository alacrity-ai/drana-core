package crypto

import (
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	// Sanity: not all zeros
	var zeroPub PublicKey
	var zeroPriv PrivateKey
	if pub == zeroPub {
		t.Fatal("public key is all zeros")
	}
	if priv == zeroPriv {
		t.Fatal("private key is all zeros")
	}
}

func TestSignAndVerify(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	// Extract public key from private key (bytes 32..64 of Ed25519 private key)
	var pub PublicKey
	copy(pub[:], priv[32:])

	msg := []byte("hello drana")
	sig := Sign(priv, msg)

	if !Verify(pub, msg, sig) {
		t.Fatal("valid signature rejected")
	}
}

func TestVerifyWrongMessage(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	var pub PublicKey
	copy(pub[:], priv[32:])

	sig := Sign(priv, []byte("message one"))
	if Verify(pub, []byte("message two"), sig) {
		t.Fatal("signature should not verify with wrong message")
	}
}

func TestVerifyWrongPublicKey(t *testing.T) {
	_, priv1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	pub2, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	sig := Sign(priv1, []byte("hello"))
	if Verify(pub2, []byte("hello"), sig) {
		t.Fatal("signature should not verify with wrong public key")
	}
}

func TestVerifyCorruptedSignature(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	var pub PublicKey
	copy(pub[:], priv[32:])

	sig := Sign(priv, []byte("hello"))
	sig[0] ^= 0xff // flip bits

	if Verify(pub, []byte("hello"), sig) {
		t.Fatal("corrupted signature should not verify")
	}
}

func TestVerifyEmptySignature(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if Verify(pub, []byte("hello"), nil) {
		t.Fatal("nil signature should not verify")
	}
	if Verify(pub, []byte("hello"), []byte{}) {
		t.Fatal("empty signature should not verify")
	}
}

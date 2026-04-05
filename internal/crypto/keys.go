package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
)

// PublicKey is a 32-byte Ed25519 public key.
type PublicKey [32]byte

// PrivateKey is a 64-byte Ed25519 private key (seed + public key).
type PrivateKey [64]byte

// GenerateKeyPair generates a new Ed25519 keypair.
func GenerateKeyPair() (PublicKey, PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PublicKey{}, PrivateKey{}, fmt.Errorf("generate keypair: %w", err)
	}
	var pubKey PublicKey
	var privKey PrivateKey
	copy(pubKey[:], pub)
	copy(privKey[:], priv)
	return pubKey, privKey, nil
}

// Sign produces an Ed25519 signature over the given message.
func Sign(privKey PrivateKey, message []byte) []byte {
	priv := ed25519.PrivateKey(privKey[:])
	return ed25519.Sign(priv, message)
}

// Verify checks an Ed25519 signature against a public key and message.
func Verify(pubKey PublicKey, message []byte, sig []byte) bool {
	if len(sig) != ed25519.SignatureSize {
		return false
	}
	pub := ed25519.PublicKey(pubKey[:])
	return ed25519.Verify(pub, message, sig)
}

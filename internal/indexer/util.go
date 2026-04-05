package indexer

import (
	"encoding/hex"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

// parseAddressForIndexer parses a drana1... display address into raw bytes.
func parseAddressForIndexer(display string) (crypto.Address, error) {
	return crypto.ParseAddress(display)
}

// computePostIDHex computes the post ID hex for a given author address and nonce.
func computePostIDHex(addr crypto.Address, nonce uint64) string {
	pid := types.DerivePostID(addr, nonce)
	return hex.EncodeToString(pid[:])
}

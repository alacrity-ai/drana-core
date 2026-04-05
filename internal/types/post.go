package types

import (
	"crypto/sha256"

	"github.com/drana-chain/drana/internal/crypto"
)

// PostID is a 32-byte identifier derived from (author address || author nonce at creation).
type PostID [32]byte

// DerivePostID computes a deterministic PostID from the author's address and nonce.
func DerivePostID(author crypto.Address, nonce uint64) PostID {
	hw := NewHashWriter()
	hw.WriteBytes(author[:])
	hw.WriteUint64(nonce)
	return PostID(sha256.Sum256(hw.buf))
}

// Post is an immutable on-chain text object with associated stake positions.
type Post struct {
	PostID          PostID
	Author          crypto.Address
	Text            string
	Channel         string         // empty = general
	ParentPostID    PostID         // zero = top-level post
	CreatedAtHeight uint64
	CreatedAtTime   int64  // unix seconds
	TotalStaked     uint64 // sum of active stake positions (microdrana)
	TotalBurned     uint64 // cumulative fees burned on this post
	StakerCount     uint64 // count of unique active stakers
	Withdrawn       bool   // true if author unstaked
}

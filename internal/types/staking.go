package types

import (
	"github.com/drana-chain/drana/internal/crypto"
)

// ValidatorStake represents a validator in the active set with their stake.
type ValidatorStake struct {
	Address       crypto.Address
	PubKey        crypto.PublicKey
	StakedBalance uint64
}

// UnbondingEntry represents DRANA in the process of being unstaked.
type UnbondingEntry struct {
	Address       crypto.Address
	Amount        uint64
	ReleaseHeight uint64
}

// PostStake records a staker's position on a post.
type PostStake struct {
	PostID PostID
	Staker crypto.Address
	Amount uint64 // microdrana locked
	Height uint64 // block height when staked
}

// SlashEvidence proves a validator double-signed at a given height.
type SlashEvidence struct {
	VoteA BlockVote
	VoteB BlockVote
}

// IsValid checks that the evidence represents a genuine double-sign.
func (se *SlashEvidence) IsValid() bool {
	// Same voter.
	if se.VoteA.VoterAddr != se.VoteB.VoterAddr {
		return false
	}
	// Same height.
	if se.VoteA.Height != se.VoteB.Height {
		return false
	}
	// Different blocks.
	if se.VoteA.BlockHash == se.VoteB.BlockHash {
		return false
	}
	// Both signatures valid.
	if !VerifyBlockVote(&se.VoteA) {
		return false
	}
	if !VerifyBlockVote(&se.VoteB) {
		return false
	}
	return true
}

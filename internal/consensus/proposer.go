package consensus

import (
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

// ProposerForHeight returns the validator responsible for proposing at the given height.
// Uses stake-weighted selection for PoS, or equal-weight round-robin for legacy.
func ProposerForHeight(validators []types.ValidatorStake, height uint64) types.ValidatorStake {
	if len(validators) == 0 {
		return types.ValidatorStake{}
	}
	totalStake := TotalStake(validators)
	if totalStake == 0 {
		// Fallback: equal-weight if no stake data.
		return validators[height%uint64(len(validators))]
	}
	slot := height % totalStake
	var cumulative uint64
	for _, v := range validators {
		cumulative += v.StakedBalance
		if slot < cumulative {
			return v
		}
	}
	// Should not reach here, but return last validator as safety.
	return validators[len(validators)-1]
}

// IsProposer returns true if the given address is the proposer for the given height.
func IsProposer(validators []types.ValidatorStake, height uint64, addr crypto.Address) bool {
	return ProposerForHeight(validators, height).Address == addr
}

// TotalStake returns the sum of all validators' staked balance.
func TotalStake(validators []types.ValidatorStake) uint64 {
	var total uint64
	for _, v := range validators {
		total += v.StakedBalance
	}
	return total
}

// QuorumThreshold returns the minimum stake weight needed for a quorum.
func QuorumThreshold(validators []types.ValidatorStake) uint64 {
	total := TotalStake(validators)
	return types.MulDiv(total, 2, 3) + 1
}

// ValidatorsFromGenesis converts genesis validators to ValidatorStake for backward compatibility.
func ValidatorsFromGenesis(gv []types.GenesisValidator) []types.ValidatorStake {
	vs := make([]types.ValidatorStake, len(gv))
	for i, g := range gv {
		vs[i] = types.ValidatorStake{
			Address:       g.Address,
			PubKey:        g.PubKey,
			StakedBalance: g.Stake,
		}
		// If no stake set (legacy genesis), give equal weight.
		if vs[i].StakedBalance == 0 {
			vs[i].StakedBalance = 1
		}
	}
	return vs
}

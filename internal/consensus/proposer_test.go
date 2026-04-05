package consensus

import (
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func makeValidatorStakes(t *testing.T, n int, stakeEach uint64) []types.ValidatorStake {
	t.Helper()
	vals := make([]types.ValidatorStake, n)
	for i := range vals {
		pub, _, err := crypto.GenerateKeyPair()
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		vals[i] = types.ValidatorStake{
			Address:       crypto.AddressFromPublicKey(pub),
			PubKey:        pub,
			StakedBalance: stakeEach,
		}
	}
	return vals
}

func TestProposerRotationEqualStake(t *testing.T) {
	vals := makeValidatorStakes(t, 3, 1000)
	// With equal stake, should rotate like round-robin.
	// totalStake = 3000, slot = H % 3000
	// Heights 0-999 -> val 0, 1000-1999 -> val 1, 2000-2999 -> val 2
	// This is different from simple H%N, but deterministic.
	seen := make(map[crypto.Address]int)
	for h := uint64(1); h <= 3000; h++ {
		p := ProposerForHeight(vals, h)
		seen[p.Address]++
	}
	// Each should get exactly 1000 proposals.
	for _, v := range vals {
		if seen[v.Address] != 1000 {
			t.Fatalf("expected 1000 proposals per validator, got %d", seen[v.Address])
		}
	}
}

func TestProposerWeightedStake(t *testing.T) {
	vals := makeValidatorStakes(t, 2, 0)
	vals[0].StakedBalance = 3000
	vals[1].StakedBalance = 1000
	// totalStake = 4000
	// Over 4000 heights: val0 should get 3000, val1 should get 1000.
	seen := make(map[crypto.Address]int)
	for h := uint64(0); h < 4000; h++ {
		p := ProposerForHeight(vals, h)
		seen[p.Address]++
	}
	if seen[vals[0].Address] != 3000 {
		t.Fatalf("val0 (3000 stake): expected 3000 proposals, got %d", seen[vals[0].Address])
	}
	if seen[vals[1].Address] != 1000 {
		t.Fatalf("val1 (1000 stake): expected 1000 proposals, got %d", seen[vals[1].Address])
	}
}

func TestProposerSingleValidator(t *testing.T) {
	vals := makeValidatorStakes(t, 1, 1000)
	for h := uint64(1); h <= 5; h++ {
		if ProposerForHeight(vals, h).Address != vals[0].Address {
			t.Fatalf("single validator should always be selected")
		}
	}
}

func TestIsProposer(t *testing.T) {
	vals := makeValidatorStakes(t, 3, 1000)
	for h := uint64(1); h <= 100; h++ {
		expected := ProposerForHeight(vals, h)
		if !IsProposer(vals, h, expected.Address) {
			t.Fatalf("height %d: correct proposer not recognized", h)
		}
	}
}

func TestQuorumThreshold(t *testing.T) {
	vals := makeValidatorStakes(t, 3, 1000)
	// Total stake = 3000, quorum = 2000 + 1 = 2001
	q := QuorumThreshold(vals)
	if q != 2001 {
		t.Fatalf("quorum: got %d, want 2001", q)
	}
}

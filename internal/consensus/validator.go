package consensus

import (
	"fmt"
	"time"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/types"
)

const AllowedFutureDrift = 15 // seconds

// ValidateProposedBlock performs full consensus-level validation of a proposed block.
func ValidateProposedBlock(
	block *types.Block,
	currentState *state.WorldState,
	lastBlock *types.Block,
	validators []types.ValidatorStake,
	params *types.GenesisConfig,
) error {
	// 1. Height continuity.
	expectedHeight := uint64(1)
	if lastBlock != nil {
		expectedHeight = lastBlock.Header.Height + 1
	}
	if block.Header.Height != expectedHeight {
		return fmt.Errorf("wrong height: got %d, want %d", block.Header.Height, expectedHeight)
	}

	// 2. Parent hash.
	var expectedPrevHash [32]byte
	if lastBlock != nil {
		expectedPrevHash = lastBlock.Header.Hash()
	}
	if block.Header.PrevHash != expectedPrevHash {
		return fmt.Errorf("wrong parent hash")
	}

	// 3. Proposer identity.
	expectedProposer := ProposerForHeight(validators, block.Header.Height)
	if block.Header.ProposerAddr != expectedProposer.Address {
		return fmt.Errorf("wrong proposer: got %s, want %s",
			block.Header.ProposerAddr.String(), expectedProposer.Address.String())
	}

	// 4. Timestamp rules.
	if lastBlock != nil {
		if block.Header.Timestamp <= lastBlock.Header.Timestamp {
			return fmt.Errorf("timestamp must be after parent: got %d, parent %d",
				block.Header.Timestamp, lastBlock.Header.Timestamp)
		}
	}
	maxTime := time.Now().Unix() + AllowedFutureDrift
	if block.Header.Timestamp > maxTime {
		return fmt.Errorf("timestamp too far in future: %d > %d", block.Header.Timestamp, maxTime)
	}

	// 5. Transaction root.
	txRoot := types.ComputeTxRoot(block.Transactions)
	if block.Header.TxRoot != txRoot {
		return fmt.Errorf("tx root mismatch")
	}

	// 6. Transaction count.
	if params.MaxTxPerBlock > 0 && len(block.Transactions) > params.MaxTxPerBlock {
		return fmt.Errorf("too many transactions: %d > %d", len(block.Transactions), params.MaxTxPerBlock)
	}

	// 7. State execution.
	exec := &state.Executor{Params: params}
	newState, err := exec.ApplyBlock(currentState, block)
	if err != nil {
		return fmt.Errorf("block execution failed: %w", err)
	}

	// 8. State root.
	stateRoot := state.ComputeStateRoot(newState)
	if block.Header.StateRoot != stateRoot {
		return fmt.Errorf("state root mismatch")
	}

	return nil
}

// ValidateQuorumCertificate checks that a QC has enough stake-weighted votes.
func ValidateQuorumCertificate(qc *types.QuorumCertificate, blockHash [32]byte, validators []types.ValidatorStake) error {
	if qc.BlockHash != blockHash {
		return fmt.Errorf("QC block hash mismatch")
	}

	// Build validator stake map.
	stakeMap := make(map[crypto.Address]uint64)
	for _, v := range validators {
		stakeMap[v.Address] = v.StakedBalance
	}

	// Sum stake behind valid unique votes.
	seen := make(map[crypto.Address]bool)
	var voteStake uint64
	for _, vote := range qc.Votes {
		stake, isValidator := stakeMap[vote.VoterAddr]
		if !isValidator {
			continue
		}
		if seen[vote.VoterAddr] {
			continue
		}
		if vote.Height != qc.Height || vote.BlockHash != qc.BlockHash {
			continue
		}
		if !types.VerifyBlockVote(&vote) {
			continue
		}
		seen[vote.VoterAddr] = true
		voteStake += stake
	}

	required := QuorumThreshold(validators)
	if voteStake < required {
		return fmt.Errorf("insufficient vote stake: got %d, need %d", voteStake, required)
	}
	return nil
}

package consensus

import (
	"testing"
	"time"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/types"
)

type testSetup struct {
	vals       []types.ValidatorStake
	privKeys   []crypto.PrivateKey
	params     *types.GenesisConfig
	ws         *state.WorldState
	addrA      crypto.Address
	privA      crypto.PrivateKey
}

func newTestSetup(t *testing.T) *testSetup {
	t.Helper()
	s := &testSetup{
		params: &types.GenesisConfig{
			MaxPostLength:      280,
			MaxPostBytes:       1024,
			MinPostCommitment:  1_000_000,
			MinBoostCommitment: 100_000,
			BlockReward:        10_000_000,
			MaxTxPerBlock:      100,
		},
	}

	// 3 validators.
	for i := 0; i < 3; i++ {
		pub, priv, err := crypto.GenerateKeyPair()
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		s.vals = append(s.vals, types.ValidatorStake{
			Address:       crypto.AddressFromPublicKey(pub),
			PubKey:        pub,
			StakedBalance: 1_000_000_000, // 1000 DRANA
		})
		s.privKeys = append(s.privKeys, priv)
	}

	// Funded account.
	pubA, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	s.addrA = crypto.AddressFromPublicKey(pubA)
	s.privA = privA

	s.ws = state.NewWorldState()
	s.ws.SetAccount(&types.Account{Address: s.addrA, Balance: 1_000_000_000, Nonce: 0})
	s.ws.SetActiveValidators(s.vals)

	return s
}

func (s *testSetup) makeValidBlock(t *testing.T, height uint64, lastBlock *types.Block) *types.Block {
	t.Helper()
	proposer := ProposerForHeight(s.vals, height)

	var prevHash [32]byte
	var ts int64 = time.Now().Unix()
	if lastBlock != nil {
		prevHash = lastBlock.Header.Hash()
		if ts <= lastBlock.Header.Timestamp {
			ts = lastBlock.Header.Timestamp + 1
		}
	}

	block := &types.Block{
		Header: types.BlockHeader{
			Height:       height,
			PrevHash:     prevHash,
			ProposerAddr: proposer.Address,
			Timestamp:    ts,
			TxRoot:       types.ComputeTxRoot(nil),
		},
	}

	// Trial execute for state root.
	exec := &state.Executor{Params: s.params}
	newState, err := exec.ApplyBlock(s.ws, block)
	if err != nil {
		t.Fatalf("trial execute: %v", err)
	}
	block.Header.StateRoot = state.ComputeStateRoot(newState)

	return block
}

func TestValidateProposedBlockValid(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err != nil {
		t.Fatalf("valid block rejected: %v", err)
	}
}

func TestValidateProposedBlockWrongHeight(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)
	block.Header.Height = 5

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("wrong height should fail")
	}
}

func TestValidateProposedBlockWrongParentHash(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)
	block.Header.PrevHash = [32]byte{0xff}

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("wrong parent hash should fail")
	}
}

func TestValidateProposedBlockWrongProposer(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)
	// Use a different validator's address.
	block.Header.ProposerAddr = s.vals[(1+1)%3].Address
	// Recompute state root since proposer changed.
	exec := &state.Executor{Params: s.params}
	newState, _ := exec.ApplyBlock(s.ws, block)
	block.Header.StateRoot = state.ComputeStateRoot(newState)

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("wrong proposer should fail")
	}
}

func TestValidateProposedBlockTimestampTooFuture(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)
	block.Header.Timestamp = time.Now().Unix() + 3600 // 1 hour in future

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("future timestamp should fail")
	}
}

func TestValidateProposedBlockTimestampBeforeParent(t *testing.T) {
	s := newTestSetup(t)
	block1 := s.makeValidBlock(t, 1, nil)
	// Apply block1 to advance state.
	exec := &state.Executor{Params: s.params}
	s.ws, _ = exec.ApplyBlock(s.ws, block1)

	block2 := s.makeValidBlock(t, 2, block1)
	block2.Header.Timestamp = block1.Header.Timestamp - 1 // before parent

	err := ValidateProposedBlock(block2, s.ws, block1, s.vals, s.params)
	if err == nil {
		t.Fatal("timestamp before parent should fail")
	}
}

func TestValidateProposedBlockWrongTxRoot(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)
	block.Header.TxRoot = [32]byte{0xde, 0xad}

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("wrong tx root should fail")
	}
}

func TestValidateProposedBlockWrongStateRoot(t *testing.T) {
	s := newTestSetup(t)
	block := s.makeValidBlock(t, 1, nil)
	block.Header.StateRoot = [32]byte{0xba, 0xad}

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("wrong state root should fail")
	}
}

func TestValidateProposedBlockTooManyTxs(t *testing.T) {
	s := newTestSetup(t)
	s.params.MaxTxPerBlock = 1

	// Create a block with 2 txs.
	_, _, addrB := makeKeypair2(t)
	tx1 := &types.Transaction{
		Type: types.TxTransfer, Sender: s.addrA, Recipient: addrB,
		Amount: 100, Nonce: 1,
	}
	types.SignTransaction(tx1, s.privA)
	tx2 := &types.Transaction{
		Type: types.TxTransfer, Sender: s.addrA, Recipient: addrB,
		Amount: 100, Nonce: 2,
	}
	types.SignTransaction(tx2, s.privA)

	proposer := ProposerForHeight(s.vals, 1)
	block := &types.Block{
		Header: types.BlockHeader{
			Height:       1,
			ProposerAddr: proposer.Address,
			Timestamp:    time.Now().Unix(),
			TxRoot:       types.ComputeTxRoot([]*types.Transaction{tx1, tx2}),
		},
		Transactions: []*types.Transaction{tx1, tx2},
	}

	err := ValidateProposedBlock(block, s.ws, nil, s.vals, s.params)
	if err == nil {
		t.Fatal("too many txs should fail")
	}
}

func TestValidateQuorumCertificate(t *testing.T) {
	s := newTestSetup(t)
	blockHash := [32]byte{1, 2, 3}

	qc := &types.QuorumCertificate{
		Height:    1,
		BlockHash: blockHash,
	}
	// Sign with all 3 validators.
	for i, v := range s.vals {
		vote := types.BlockVote{
			Height:    1,
			BlockHash: blockHash,
			VoterAddr: v.Address,
		}
		types.SignBlockVote(&vote, s.privKeys[i])
		qc.Votes = append(qc.Votes, vote)
	}

	err := ValidateQuorumCertificate(qc, blockHash, s.vals)
	if err != nil {
		t.Fatalf("valid QC rejected: %v", err)
	}
}

func TestValidateQuorumCertificateInsufficientVotes(t *testing.T) {
	s := newTestSetup(t)
	blockHash := [32]byte{1, 2, 3}

	// Only 1 of 3 votes — needs 3 (2/3+1 of 3 = 3).
	vote := types.BlockVote{
		Height:    1,
		BlockHash: blockHash,
		VoterAddr: s.vals[0].Address,
	}
	types.SignBlockVote(&vote, s.privKeys[0])

	qc := &types.QuorumCertificate{
		Height:    1,
		BlockHash: blockHash,
		Votes:     []types.BlockVote{vote},
	}

	err := ValidateQuorumCertificate(qc, blockHash, s.vals)
	if err == nil {
		t.Fatal("insufficient votes should fail")
	}
}

func makeKeypair2(t *testing.T) (crypto.PublicKey, crypto.PrivateKey, crypto.Address) {
	t.Helper()
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return pub, priv, crypto.AddressFromPublicKey(pub)
}

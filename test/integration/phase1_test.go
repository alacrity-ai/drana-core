package integration

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/genesis"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/store"
	"github.com/drana-chain/drana/internal/types"
)

// TestPhase1EndToEnd exercises the full Phase 1 stack:
// genesis -> transactions -> block execution -> persistence -> reload -> verify.
func TestPhase1EndToEnd(t *testing.T) {
	// --- Setup: generate three identities ---
	pubA, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen A: %v", err)
	}
	addrA := crypto.AddressFromPublicKey(pubA)

	pubB, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen B: %v", err)
	}
	addrB := crypto.AddressFromPublicKey(pubB)

	pubC, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen C: %v", err)
	}
	addrC := crypto.AddressFromPublicKey(pubC)

	// --- Write a genesis file ---
	genesisDir := t.TempDir()
	genesisPath := filepath.Join(genesisDir, "genesis.json")
	genesisContent := `{
		"chainId": "drana-test-1",
		"genesisTime": 1700000000,
		"accounts": [
			{"address": "` + addrA.String() + `", "balance": 1000000000},
			{"address": "` + addrB.String() + `", "balance": 150000000}
		],
		"validators": [
			{"address": "` + addrC.String() + `", "pubKey": "` + hex.EncodeToString(pubC[:]) + `", "name": "val-1"}
		],
		"maxPostLength": 280,
		"maxPostBytes": 1024,
		"minPostCommitment": 1000000,
		"minBoostCommitment": 100000,
		"maxTxPerBlock": 100,
		"maxBlockBytes": 1048576,
		"blockIntervalSec": 120,
		"blockReward": 10000000
	}`
	if err := os.WriteFile(genesisPath, []byte(genesisContent), 0644); err != nil {
		t.Fatalf("write genesis: %v", err)
	}

	// --- Step 1: Load genesis ---
	cfg, err := genesis.LoadGenesis(genesisPath)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}

	// --- Step 2: Initialize state ---
	ws, err := genesis.InitializeState(cfg)
	if err != nil {
		t.Fatalf("InitializeState: %v", err)
	}

	// Verify initial balances.
	aliceAcct, ok := ws.GetAccount(addrA)
	if !ok || aliceAcct.Balance != 1_000_000_000 {
		t.Fatalf("alice initial balance: %v %v", ok, aliceAcct)
	}
	bobAcct, ok := ws.GetAccount(addrB)
	if !ok || bobAcct.Balance != 150_000_000 {
		t.Fatalf("bob initial balance: %v %v", ok, bobAcct)
	}

	// --- Step 3: Persist initial state ---
	dataDir := t.TempDir()
	kvPath := filepath.Join(dataDir, "state")
	kv, err := store.OpenKVStore(kvPath)
	if err != nil {
		t.Fatalf("OpenKVStore: %v", err)
	}
	if err := kv.SaveState(ws); err != nil {
		t.Fatalf("SaveState (initial): %v", err)
	}

	// --- Step 4: Build transactions ---

	// Tx 1: Alice sends 100 DRANA (100_000_000 microdrana) to Bob.
	tx1 := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100_000_000,
		Nonce:     1,
	}
	types.SignTransaction(tx1, privA)

	// Tx 2: Alice creates a post with 200 DRANA (200_000_000 microdrana).
	tx2 := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "The empire of relevance belongs to the highest bidder.",
		Amount: 200_000_000,
		Nonce:  2,
	}
	types.SignTransaction(tx2, privA)

	// Tx 3: Bob boosts Alice's post with 75 DRANA (75_000_000 microdrana).
	// PostID is derived from Alice's address and her nonce at creation time (2).
	postID := types.DerivePostID(addrA, 2)
	tx3 := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrB,
		PostID: postID,
		Amount: 75_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx3, privB)

	// --- Step 5: Assemble block ---
	txs := []*types.Transaction{tx1, tx2, tx3}
	txRoot := types.ComputeTxRoot(txs)

	block := &types.Block{
		Header: types.BlockHeader{
			Height:       1,
			PrevHash:     [32]byte{}, // genesis has no parent
			ProposerAddr: addrC,
			Timestamp:    1700000120,
			TxRoot:       txRoot,
		},
		Transactions: txs,
	}

	// --- Step 6: Execute block ---
	exec := &state.Executor{Params: cfg}
	newState, err := exec.ApplyBlock(ws, block)
	if err != nil {
		t.Fatalf("ApplyBlock: %v", err)
	}

	// --- Step 7: Verify state matches DESIGN.md section 23 examples ---

	// Under post-staking: alice gets rewards from bob's boost.
	alice, _ := newState.GetAccount(addrA)
	t.Logf("  Alice: balance=%d, postStake=%d, nonce=%d", alice.Balance, alice.PostStakeBalance, alice.Nonce)
	if alice.Nonce != 2 {
		t.Fatalf("alice nonce: got %d, want 2", alice.Nonce)
	}

	bob, _ := newState.GetAccount(addrB)
	t.Logf("  Bob: balance=%d, postStake=%d, nonce=%d", bob.Balance, bob.PostStakeBalance, bob.Nonce)
	if bob.Nonce != 1 {
		t.Fatalf("bob nonce: got %d, want 1", bob.Nonce)
	}

	post, ok := newState.GetPost(postID)
	if !ok {
		t.Fatal("post not found")
	}
	t.Logf("  Post: staked=%d, burned=%d, stakers=%d", post.TotalStaked, post.TotalBurned, post.StakerCount)
	if post.TotalStaked == 0 {
		t.Fatal("post should have stake")
	}
	if post.StakerCount != 2 { // alice + bob
		t.Fatalf("post stakerCount: got %d, want 2", post.StakerCount)
	}
	if post.Text != "The empire of relevance belongs to the highest bidder." {
		t.Fatalf("post text: %q", post.Text)
	}
	if post.Author != addrA {
		t.Fatal("post author mismatch")
	}

	// Proposer (addrC): block reward = 10M
	proposer, ok := newState.GetAccount(addrC)
	if !ok {
		t.Fatal("proposer account not found")
	}
	if proposer.Balance != 10_000_000 {
		t.Fatalf("proposer balance: got %d, want 10000000", proposer.Balance)
	}

	// Burned supply > 0 (fees were burned)
	if newState.GetBurnedSupply() == 0 {
		t.Fatal("burned supply should be > 0")
	}
	t.Logf("  Burned: %d, Issued: %d", newState.GetBurnedSupply(), newState.GetIssuedSupply())

	// Issued supply = 10M (one block reward)
	if newState.GetIssuedSupply() != 10_000_000 {
		t.Fatalf("issued supply: got %d, want 10000000", newState.GetIssuedSupply())
	}

	// Chain height = 1
	if newState.GetChainHeight() != 1 {
		t.Fatalf("chain height: got %d, want 1", newState.GetChainHeight())
	}

	// --- Step 8: Compute state root and set in block header ---
	stateRoot := state.ComputeStateRoot(newState)
	block.Header.StateRoot = stateRoot

	// --- Step 9: Persist block and final state ---
	blockPath := filepath.Join(dataDir, "blocks")
	bs, err := store.OpenBlockStore(blockPath)
	if err != nil {
		t.Fatalf("OpenBlockStore: %v", err)
	}
	if err := bs.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock: %v", err)
	}
	if err := kv.SaveState(newState); err != nil {
		t.Fatalf("SaveState (final): %v", err)
	}

	// --- Step 10: Close everything ---
	if err := kv.Close(); err != nil {
		t.Fatalf("Close KV: %v", err)
	}
	if err := bs.Close(); err != nil {
		t.Fatalf("Close BlockStore: %v", err)
	}

	// --- Step 11: Reopen and verify ---
	kv2, err := store.OpenKVStore(kvPath)
	if err != nil {
		t.Fatalf("Reopen KVStore: %v", err)
	}
	defer kv2.Close()

	bs2, err := store.OpenBlockStore(blockPath)
	if err != nil {
		t.Fatalf("Reopen BlockStore: %v", err)
	}
	defer bs2.Close()

	loadedState, err := kv2.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	// State root after reload must match.
	reloadedRoot := state.ComputeStateRoot(loadedState)
	if reloadedRoot != stateRoot {
		t.Fatalf("state root mismatch after reload:\n  pre:  %x\n  post: %x", stateRoot, reloadedRoot)
	}

	// Verify loaded block.
	loadedBlock, err := bs2.GetBlockByHeight(1)
	if err != nil {
		t.Fatalf("GetBlockByHeight: %v", err)
	}
	if loadedBlock.Header.Height != 1 {
		t.Fatalf("loaded block height: %d", loadedBlock.Header.Height)
	}
	if len(loadedBlock.Transactions) != 3 {
		t.Fatalf("loaded block tx count: %d", len(loadedBlock.Transactions))
	}
	if loadedBlock.Header.StateRoot != stateRoot {
		t.Fatal("loaded block state root mismatch")
	}

	// Verify block by hash.
	blockHash := block.Header.Hash()
	hashBlock, err := bs2.GetBlockByHash(blockHash)
	if err != nil {
		t.Fatalf("GetBlockByHash: %v", err)
	}
	if hashBlock.Header.Height != 1 {
		t.Fatal("hash lookup returned wrong block")
	}

	// Verify latest block.
	latest, err := bs2.GetLatestBlock()
	if err != nil {
		t.Fatalf("GetLatestBlock: %v", err)
	}
	if latest.Header.Height != 1 {
		t.Fatal("latest block height mismatch")
	}

	// Verify tx lookup.
	tx1Hash := tx1.Hash()
	foundTx, foundHeight, err := bs2.GetTransaction(tx1Hash)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if foundHeight != 1 {
		t.Fatalf("tx block height: %d", foundHeight)
	}
	if foundTx.Amount != 100_000_000 {
		t.Fatalf("tx amount: %d", foundTx.Amount)
	}

	// Verify loaded state values.
	loadedAlice, _ := loadedState.GetAccount(addrA)
	t.Logf("  Loaded Alice: balance=%d, postStake=%d", loadedAlice.Balance, loadedAlice.PostStakeBalance)
	loadedBob, _ := loadedState.GetAccount(addrB)
	t.Logf("  Loaded Bob: balance=%d, postStake=%d", loadedBob.Balance, loadedBob.PostStakeBalance)
	loadedPost, ok := loadedState.GetPost(postID)
	if !ok {
		t.Fatal("loaded post not found")
	}
	if loadedPost.TotalStaked == 0 {
		t.Fatal("loaded post should have stake")
	}
	if loadedState.GetBurnedSupply() == 0 {
		t.Fatal("loaded burned supply should be > 0")
	}
	if loadedState.GetIssuedSupply() != 10_000_000 {
		t.Fatalf("loaded issued supply: %d", loadedState.GetIssuedSupply())
	}

	// --- Supply conservation invariant check ---
	// sum(balances) == genesis_supply + issuedSupply - burnedSupply
	var totalBalances uint64
	for _, acct := range loadedState.AllAccounts() {
		totalBalances += acct.Balance + acct.PostStakeBalance
	}
	genesisSupply := uint64(1_000_000_000 + 150_000_000)
	expectedSupply := genesisSupply + loadedState.GetIssuedSupply() - loadedState.GetBurnedSupply()
	if totalBalances != expectedSupply {
		t.Fatalf("supply conservation violated: balances+postStakes=%d != genesis(%d) + issued(%d) - burned(%d) = %d",
			totalBalances, genesisSupply, loadedState.GetIssuedSupply(), loadedState.GetBurnedSupply(), expectedSupply)
	}

	t.Logf("Phase 1 integration test passed.")
	t.Logf("  State root: %x", stateRoot)
	t.Logf("  Block hash: %x", blockHash)
	t.Logf("  Alice:    %d microdrana, nonce %d", loadedAlice.Balance, loadedAlice.Nonce)
	t.Logf("  Bob:      %d microdrana, nonce %d", loadedBob.Balance, loadedBob.Nonce)
	t.Logf("  Proposer: %d microdrana (block reward)", proposer.Balance)
	t.Logf("  Post:     %d microdrana committed, %d boosts", loadedPost.TotalStaked, loadedPost.StakerCount)
	t.Logf("  Burned:   %d microdrana", loadedState.GetBurnedSupply())
	t.Logf("  Issued:   %d microdrana", loadedState.GetIssuedSupply())
	t.Logf("  Net supply change: %+d microdrana", int64(loadedState.GetIssuedSupply())-int64(loadedState.GetBurnedSupply()))
}

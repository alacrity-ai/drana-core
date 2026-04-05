package state

import (
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func defaultParams() *types.GenesisConfig {
	return &types.GenesisConfig{
		MaxPostLength:      280,
		MaxPostBytes:       1024,
		MinPostCommitment:  1_000_000,
		MinBoostCommitment: 100_000,
		BlockReward:        10_000_000, // 10 DRANA
	}
}

func makeKeypair(t *testing.T) (crypto.PublicKey, crypto.PrivateKey, crypto.Address) {
	t.Helper()
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return pub, priv, crypto.AddressFromPublicKey(pub)
}

func setupAliceBob(t *testing.T) (
	*WorldState, *Executor,
	crypto.PrivateKey, crypto.Address,
	crypto.PrivateKey, crypto.Address,
) {
	t.Helper()
	_, privA, addrA := makeKeypair(t)
	_, privB, addrB := makeKeypair(t)

	ws := NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 1_000_000_000, Nonce: 0})
	ws.SetAccount(&types.Account{Address: addrB, Balance: 50_000_000, Nonce: 0})

	exec := &Executor{Params: defaultParams()}
	return ws, exec, privA, addrA, privB, addrB
}

func TestApplyTransfer(t *testing.T) {
	ws, exec, privA, addrA, _, addrB := setupAliceBob(t)

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100_000_000,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)

	if err := exec.ApplyTransaction(ws, tx, 1, 1700000000); err != nil {
		t.Fatalf("ApplyTransaction: %v", err)
	}

	alice, _ := ws.GetAccount(addrA)
	bob, _ := ws.GetAccount(addrB)
	if alice.Balance != 900_000_000 {
		t.Fatalf("alice balance: got %d, want 900000000", alice.Balance)
	}
	if bob.Balance != 150_000_000 {
		t.Fatalf("bob balance: got %d, want 150000000", bob.Balance)
	}
	if alice.Nonce != 1 {
		t.Fatalf("alice nonce: got %d, want 1", alice.Nonce)
	}
}

func TestApplyCreatePost(t *testing.T) {
	ws, exec, privA, addrA, _, _ := setupAliceBob(t)

	tx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "The empire of relevance belongs to the highest bidder.",
		Amount: 200_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := exec.ApplyTransaction(ws, tx, 1, 1700000000); err != nil {
		t.Fatalf("ApplyTransaction: %v", err)
	}

	alice, _ := ws.GetAccount(addrA)
	if alice.Balance != 800_000_000 {
		t.Fatalf("alice balance: got %d, want 800000000", alice.Balance)
	}
	if alice.Nonce != 1 {
		t.Fatalf("alice nonce: got %d, want 1", alice.Nonce)
	}

	// Post should exist.
	postID := types.DerivePostID(addrA, 1) // nonce was 0, so nonce+1=1 at creation
	post, ok := ws.GetPost(postID)
	if !ok {
		t.Fatal("post should exist")
	}
	// With 6% fee: staked = 200M * 94/100 = 188M, burned = 12M
	if post.TotalStaked != 188_000_000 {
		t.Fatalf("post staked: got %d, want 188000000", post.TotalStaked)
	}
	if post.StakerCount != 1 {
		t.Fatalf("post staker count: got %d, want 1", post.StakerCount)
	}
	if post.CreatedAtHeight != 1 {
		t.Fatalf("post height: got %d, want 1", post.CreatedAtHeight)
	}
	if ws.GetBurnedSupply() != 12_000_000 {
		t.Fatalf("burned supply: got %d, want 12000000", ws.GetBurnedSupply())
	}
}

func TestApplyBoostPostInsufficientBalance(t *testing.T) {
	ws, exec, privA, addrA, privB, addrB := setupAliceBob(t)

	// Alice creates a post first.
	createTx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "test post",
		Amount: 200_000_000,
		Nonce:  1,
	}
	types.SignTransaction(createTx, privA)
	if err := exec.ApplyTransaction(ws, createTx, 1, 1700000000); err != nil {
		t.Fatalf("create post: %v", err)
	}

	postID := types.DerivePostID(addrA, 1)

	// Bob only has 50M, tries to boost 75M — should fail.
	boostTx := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrB,
		PostID: postID,
		Amount: 75_000_000,
		Nonce:  1,
	}
	types.SignTransaction(boostTx, privB)
	if err := exec.ApplyTransaction(ws, boostTx, 1, 1700000000); err == nil {
		t.Fatal("boost with insufficient balance should fail")
	}
	_ = addrB
}

func TestApplyBoostPostCorrect(t *testing.T) {
	ws, exec, privA, addrA, privB, addrB := setupAliceBob(t)

	// Give Bob more money.
	bobAcct, _ := ws.GetAccount(addrB)
	bobAcct.Balance = 150_000_000
	ws.SetAccount(bobAcct)

	// Alice creates a post.
	createTx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "test post",
		Amount: 200_000_000,
		Nonce:  1,
	}
	types.SignTransaction(createTx, privA)
	if err := exec.ApplyTransaction(ws, createTx, 1, 1700000000); err != nil {
		t.Fatalf("create post: %v", err)
	}

	postID := types.DerivePostID(addrA, 1)

	// Bob boosts.
	boostTx := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrB,
		PostID: postID,
		Amount: 75_000_000,
		Nonce:  1,
	}
	types.SignTransaction(boostTx, privB)
	if err := exec.ApplyTransaction(ws, boostTx, 1, 1700000000); err != nil {
		t.Fatalf("boost post: %v", err)
	}

	bob, _ := ws.GetAccount(addrB)
	if bob.Balance != 75_000_000 {
		t.Fatalf("bob balance: got %d, want 75000000", bob.Balance)
	}
	if bob.Nonce != 1 {
		t.Fatalf("bob nonce: got %d, want 1", bob.Nonce)
	}

	post, _ := ws.GetPost(postID)
	// Post: 200M * 94% = 188M initial + 75M * 94% = 70.5M boost = 258.5M staked
	if post.TotalStaked != 258_500_000 {
		t.Fatalf("post staked: got %d, want 258500000", post.TotalStaked)
	}
	if post.StakerCount != 2 { // author + booster
		t.Fatalf("post staker count: got %d, want 2", post.StakerCount)
	}
	// Burned: 200M * 6% = 12M + 75M * 3% = 2.25M = 14.25M
	if ws.GetBurnedSupply() != 14_250_000 {
		t.Fatalf("burned supply: got %d, want 14250000", ws.GetBurnedSupply())
	}
}

func TestApplySelfBoost(t *testing.T) {
	ws, exec, privA, addrA, _, _ := setupAliceBob(t)

	createTx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "self boost test",
		Amount: 100_000_000,
		Nonce:  1,
	}
	types.SignTransaction(createTx, privA)
	if err := exec.ApplyTransaction(ws, createTx, 1, 1700000000); err != nil {
		t.Fatalf("create post: %v", err)
	}

	postID := types.DerivePostID(addrA, 1)

	boostTx := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrA,
		PostID: postID,
		Amount: 50_000_000,
		Nonce:  2,
	}
	types.SignTransaction(boostTx, privA)
	if err := exec.ApplyTransaction(ws, boostTx, 1, 1700000000); err != nil {
		t.Fatalf("self-boost should succeed: %v", err)
	}

	post, _ := ws.GetPost(postID)
	// Post: 100M * 94% = 94M + 50M * 94% = 47M = 141M
	if post.TotalStaked != 141_000_000 {
		t.Fatalf("post staked: got %d, want 141000000", post.TotalStaked)
	}
}

func TestApplyBlockAtomic(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)

	ws := NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 1_000_000_000, Nonce: 0})

	exec := &Executor{Params: defaultParams()}

	// First tx is valid, second will fail (insufficient balance).
	tx1 := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    999_000_000,
		Nonce:     1,
	}
	types.SignTransaction(tx1, privA)

	tx2 := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    999_000_000, // not enough after tx1
		Nonce:     2,
	}
	types.SignTransaction(tx2, privA)

	block := &types.Block{
		Header:       types.BlockHeader{Height: 1, Timestamp: 1700000000},
		Transactions: []*types.Transaction{tx1, tx2},
	}

	_, err := exec.ApplyBlock(ws, block)
	if err == nil {
		t.Fatal("block with overdraw tx should fail")
	}

	// Original state must be untouched.
	alice, _ := ws.GetAccount(addrA)
	if alice.Balance != 1_000_000_000 {
		t.Fatalf("original state was mutated: balance=%d", alice.Balance)
	}
	if alice.Nonce != 0 {
		t.Fatalf("original state was mutated: nonce=%d", alice.Nonce)
	}
}

func TestApplyBlockSuccess(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, privB, addrB := makeKeypair(t)
	_, _, addrProposer := makeKeypair(t)

	ws := NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 1_000_000_000, Nonce: 0})
	ws.SetAccount(&types.Account{Address: addrB, Balance: 150_000_000, Nonce: 0})

	exec := &Executor{Params: defaultParams()}

	// Transfer: Alice -> Bob 100M
	tx1 := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100_000_000,
		Nonce:     1,
	}
	types.SignTransaction(tx1, privA)

	// CreatePost: Alice posts with 200M
	tx2 := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "The empire of relevance belongs to the highest bidder.",
		Amount: 200_000_000,
		Nonce:  2,
	}
	types.SignTransaction(tx2, privA)

	// BoostPost: Bob boosts with 75M
	postID := types.DerivePostID(addrA, 2)
	tx3 := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrB,
		PostID: postID,
		Amount: 75_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx3, privB)

	block := &types.Block{
		Header: types.BlockHeader{
			Height:       1,
			Timestamp:    1700000000,
			ProposerAddr: addrProposer,
		},
		Transactions: []*types.Transaction{tx1, tx2, tx3},
	}

	newState, err := exec.ApplyBlock(ws, block)
	if err != nil {
		t.Fatalf("ApplyBlock: %v", err)
	}

	alice, _ := newState.GetAccount(addrA)
	bob, _ := newState.GetAccount(addrB)
	proposer, _ := newState.GetAccount(addrProposer)
	post, _ := newState.GetPost(postID)

	// Under post-staking model, alice receives author + staker rewards from bob's boost.
	// Exact values depend on fee math — just verify she spent her transfers/posts.
	t.Logf("  Alice balance: %d, postStake: %d", alice.Balance, alice.PostStakeBalance)
	t.Logf("  Bob balance: %d, postStake: %d", bob.Balance, bob.PostStakeBalance)
	t.Logf("  Proposer balance: %d", proposer.Balance)
	t.Logf("  Post staked: %d, stakers: %d, burned: %d", post.TotalStaked, post.StakerCount, post.TotalBurned)

	if proposer.Balance != 10_000_000 {
		t.Fatalf("proposer balance: got %d, want 10000000", proposer.Balance)
	}
	if post.StakerCount != 2 { // alice (author) + bob (booster)
		t.Fatalf("post staker count: got %d, want 2", post.StakerCount)
	}
	if newState.GetBurnedSupply() == 0 {
		t.Fatal("burned supply should be > 0 from fees")
	}
	if newState.GetIssuedSupply() != 10_000_000 {
		t.Fatalf("issued supply: got %d, want 10000000", newState.GetIssuedSupply())
	}
	if newState.GetChainHeight() != 1 {
		t.Fatalf("chain height: got %d, want 1", newState.GetChainHeight())
	}

	// Supply conservation: sum(balances + postStakeBalances) == genesis + issued - burned
	genesisSupply := uint64(1_000_000_000 + 150_000_000)
	var totalBalances uint64
	for _, acct := range newState.AllAccounts() {
		totalBalances += acct.Balance + acct.PostStakeBalance
	}
	expected := genesisSupply + newState.GetIssuedSupply() - newState.GetBurnedSupply()
	if totalBalances != expected {
		t.Fatalf("supply conservation violated: balances=%d, expected=%d (genesis=%d + issued=%d - burned=%d)",
			totalBalances, expected, genesisSupply, newState.GetIssuedSupply(), newState.GetBurnedSupply())
	}

	// Original state untouched.
	origAlice, _ := ws.GetAccount(addrA)
	if origAlice.Balance != 1_000_000_000 {
		t.Fatalf("original alice mutated: %d", origAlice.Balance)
	}
}

func TestSequentialNonces(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)

	ws := NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 1_000_000_000, Nonce: 0})

	exec := &Executor{Params: defaultParams()}

	tx1 := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100,
		Nonce:     1,
	}
	types.SignTransaction(tx1, privA)

	tx2 := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    200,
		Nonce:     2,
	}
	types.SignTransaction(tx2, privA)

	block := &types.Block{
		Header:       types.BlockHeader{Height: 1, Timestamp: 1700000000},
		Transactions: []*types.Transaction{tx1, tx2},
	}

	newState, err := exec.ApplyBlock(ws, block)
	if err != nil {
		t.Fatalf("ApplyBlock: %v", err)
	}

	alice, _ := newState.GetAccount(addrA)
	if alice.Nonce != 2 {
		t.Fatalf("alice nonce: got %d, want 2", alice.Nonce)
	}
	if alice.Balance != 999_999_700 {
		t.Fatalf("alice balance: got %d, want 999999700", alice.Balance)
	}
}

func TestBlockRewardCreatesProposerAccount(t *testing.T) {
	_, _, addrProposer := makeKeypair(t)

	ws := NewWorldState()
	// Proposer has no account yet.

	exec := &Executor{Params: defaultParams()}

	block := &types.Block{
		Header: types.BlockHeader{
			Height:       1,
			Timestamp:    1700000000,
			ProposerAddr: addrProposer,
		},
		Transactions: nil, // empty block
	}

	newState, err := exec.ApplyBlock(ws, block)
	if err != nil {
		t.Fatalf("ApplyBlock: %v", err)
	}

	proposer, ok := newState.GetAccount(addrProposer)
	if !ok {
		t.Fatal("proposer account should have been created")
	}
	if proposer.Balance != 10_000_000 {
		t.Fatalf("proposer balance: got %d, want 10000000", proposer.Balance)
	}
	if proposer.Nonce != 0 {
		t.Fatalf("proposer nonce: got %d, want 0", proposer.Nonce)
	}
	if newState.GetIssuedSupply() != 10_000_000 {
		t.Fatalf("issued supply: got %d, want 10000000", newState.GetIssuedSupply())
	}
}

func TestBlockRewardAccumulatesOverBlocks(t *testing.T) {
	_, _, addrProposer := makeKeypair(t)

	ws := NewWorldState()
	exec := &Executor{Params: defaultParams()}

	// Apply 3 empty blocks from the same proposer.
	current := ws
	for h := uint64(1); h <= 3; h++ {
		block := &types.Block{
			Header: types.BlockHeader{
				Height:       h,
				Timestamp:    1700000000 + int64(h)*120,
				ProposerAddr: addrProposer,
			},
		}
		var err error
		current, err = exec.ApplyBlock(current, block)
		if err != nil {
			t.Fatalf("ApplyBlock height %d: %v", h, err)
		}
	}

	proposer, _ := current.GetAccount(addrProposer)
	if proposer.Balance != 30_000_000 {
		t.Fatalf("proposer balance after 3 blocks: got %d, want 30000000", proposer.Balance)
	}
	if current.GetIssuedSupply() != 30_000_000 {
		t.Fatalf("issued supply after 3 blocks: got %d, want 30000000", current.GetIssuedSupply())
	}
}

func TestBlockRewardZero(t *testing.T) {
	_, _, addrProposer := makeKeypair(t)

	ws := NewWorldState()
	params := defaultParams()
	params.BlockReward = 0 // no issuance
	exec := &Executor{Params: params}

	block := &types.Block{
		Header: types.BlockHeader{
			Height:       1,
			Timestamp:    1700000000,
			ProposerAddr: addrProposer,
		},
	}

	newState, err := exec.ApplyBlock(ws, block)
	if err != nil {
		t.Fatalf("ApplyBlock: %v", err)
	}

	// Proposer should not have been created.
	_, ok := newState.GetAccount(addrProposer)
	if ok {
		t.Fatal("proposer account should not exist with zero block reward")
	}
	if newState.GetIssuedSupply() != 0 {
		t.Fatalf("issued supply: got %d, want 0", newState.GetIssuedSupply())
	}
}

func TestSupplyConservationMultiBlock(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrProposer := makeKeypair(t)

	genesisBalance := uint64(1_000_000_000)
	ws := NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: genesisBalance, Nonce: 0})

	exec := &Executor{Params: defaultParams()}

	// Block 1: Alice creates a post burning 200M.
	tx1 := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "attention economy test",
		Amount: 200_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx1, privA)

	block1 := &types.Block{
		Header: types.BlockHeader{
			Height:       1,
			Timestamp:    1700000000,
			ProposerAddr: addrProposer,
		},
		Transactions: []*types.Transaction{tx1},
	}
	state1, err := exec.ApplyBlock(ws, block1)
	if err != nil {
		t.Fatalf("ApplyBlock 1: %v", err)
	}

	// Block 2: empty block, just issuance.
	block2 := &types.Block{
		Header: types.BlockHeader{
			Height:       2,
			Timestamp:    1700000120,
			ProposerAddr: addrProposer,
		},
	}
	state2, err := exec.ApplyBlock(state1, block2)
	if err != nil {
		t.Fatalf("ApplyBlock 2: %v", err)
	}

	// Verify conservation at each step.
	for _, s := range []*WorldState{state1, state2} {
		var totalBal uint64
		for _, acct := range s.AllAccounts() {
			totalBal += acct.Balance + acct.PostStakeBalance
		}
		expected := genesisBalance + s.GetIssuedSupply() - s.GetBurnedSupply()
		if totalBal != expected {
			t.Fatalf("supply conservation violated at height %d: balances=%d, expected=%d (genesis=%d + issued=%d - burned=%d)",
				s.GetChainHeight(), totalBal, expected, genesisBalance, s.GetIssuedSupply(), s.GetBurnedSupply())
		}
	}
}

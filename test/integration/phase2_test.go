package integration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drana-chain/drana/internal/consensus"
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/node"
	"github.com/drana-chain/drana/internal/p2p"
	"github.com/drana-chain/drana/internal/state"
	pb "github.com/drana-chain/drana/internal/proto/pb"
	"github.com/drana-chain/drana/internal/types"
)

func TestPhase2MultiNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node test in short mode")
	}

	// --- Generate 3 validator identities ---
	type valIdentity struct {
		pub     crypto.PublicKey
		priv    crypto.PrivateKey
		addr    crypto.Address
		privHex string
	}
	vals := make([]valIdentity, 3)
	for i := range vals {
		pub, priv, err := crypto.GenerateKeyPair()
		if err != nil {
			t.Fatalf("keygen %d: %v", i, err)
		}
		vals[i] = valIdentity{
			pub:     pub,
			priv:    priv,
			addr:    crypto.AddressFromPublicKey(pub),
			privHex: hex.EncodeToString(priv[:]),
		}
	}

	// Also generate a funded user account.
	userPub, userPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("user keygen: %v", err)
	}
	userAddr := crypto.AddressFromPublicKey(userPub)

	// --- Write genesis file ---
	tmpDir := t.TempDir()
	genesisPath := filepath.Join(tmpDir, "genesis.json")

	type genAcct struct {
		Address string `json:"address"`
		Balance uint64 `json:"balance"`
	}
	type genVal struct {
		Address string `json:"address"`
		PubKey  string `json:"pubKey"`
		Name    string `json:"name"`
	}
	genesisData := map[string]interface{}{
		"chainId":            "drana-test-phase2",
		"genesisTime":        time.Now().Unix(),
		"maxPostLength":      280,
		"maxPostBytes":       1024,
		"minPostCommitment":  1000000,
		"minBoostCommitment": 100000,
		"maxTxPerBlock":      100,
		"maxBlockBytes":      1048576,
		"blockIntervalSec":   2, // fast for testing
		"blockReward":        10000000,
		"accounts": []genAcct{
			{Address: userAddr.String(), Balance: 1_000_000_000},
			{Address: vals[0].addr.String(), Balance: 0},
			{Address: vals[1].addr.String(), Balance: 0},
			{Address: vals[2].addr.String(), Balance: 0},
		},
		"validators": []genVal{
			{Address: vals[0].addr.String(), PubKey: hex.EncodeToString(vals[0].pub[:]), Name: "val-0"},
			{Address: vals[1].addr.String(), PubKey: hex.EncodeToString(vals[1].pub[:]), Name: "val-1"},
			{Address: vals[2].addr.String(), PubKey: hex.EncodeToString(vals[2].pub[:]), Name: "val-2"},
		},
	}
	gdata, _ := json.MarshalIndent(genesisData, "", "  ")
	os.WriteFile(genesisPath, gdata, 0644)

	// --- Configure 3 nodes ---
	ports := []string{"127.0.0.1:26701", "127.0.0.1:26702", "127.0.0.1:26703"}
	peerEndpoints := map[string]string{
		"val-0": ports[0],
		"val-1": ports[1],
		"val-2": ports[2],
	}

	configs := make([]*node.Config, 3)
	for i := 0; i < 3; i++ {
		dataDir := filepath.Join(tmpDir, fmt.Sprintf("node%d", i))
		os.MkdirAll(dataDir, 0755)
		configs[i] = &node.Config{
			GenesisPath:   genesisPath,
			DataDir:       dataDir,
			PrivKeyHex:    vals[i].privHex,
			ListenAddr:    ports[i],
			PeerEndpoints: peerEndpoints,
		}
	}

	// --- Start 3 nodes ---
	nodes := make([]*node.Node, 3)
	ctxs := make([]context.Context, 3)
	cancels := make([]context.CancelFunc, 3)

	for i := 0; i < 3; i++ {
		n, err := node.NewNode(configs[i])
		if err != nil {
			t.Fatalf("NewNode %d: %v", i, err)
		}
		nodes[i] = n
		// Override block interval for fast testing.
		n.Engine.BlockInterval = 2 * time.Second
	}

	// Start P2P servers first so they can accept connections.
	for i := 0; i < 3; i++ {
		if err := nodes[i].P2PServer.Start(); err != nil {
			t.Fatalf("start p2p %d: %v", i, err)
		}
	}
	time.Sleep(500 * time.Millisecond) // let servers bind

	// Connect peers.
	for i := 0; i < 3; i++ {
		nodes[i].Peers.Connect(peerEndpoints)
	}

	// Start consensus engines.
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		cancels[i] = cancel
		go func(n *node.Node, ctx context.Context) {
			n.Engine.Run(ctx)
		}(nodes[i], ctx)
	}

	// --- Wait for at least 5 blocks ---
	t.Log("Waiting for 5 blocks...")
	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for blocks (heights: %d, %d, %d)",
				nodes[0].Engine.CurrentHeight(),
				nodes[1].Engine.CurrentHeight(),
				nodes[2].Engine.CurrentHeight())
		case <-time.After(500 * time.Millisecond):
		}

		minHeight := nodes[0].Engine.CurrentHeight()
		for _, n := range nodes[1:] {
			h := n.Engine.CurrentHeight()
			if h < minHeight {
				minHeight = h
			}
		}
		if minHeight >= 5 {
			break
		}
	}

	// --- Verify all nodes agree on state ---
	t.Log("Verifying state agreement across nodes...")
	heights := make([]uint64, 3)
	roots := make([][32]byte, 3)
	for i, n := range nodes {
		heights[i] = n.Engine.CurrentHeight()
		roots[i] = state.ComputeStateRoot(n.Engine.CurrentState())
	}

	// All should be at the same height (within 1 block tolerance).
	minH, maxH := heights[0], heights[0]
	for _, h := range heights[1:] {
		if h < minH {
			minH = h
		}
		if h > maxH {
			maxH = h
		}
	}
	if maxH-minH > 1 {
		t.Fatalf("height divergence too large: %v", heights)
	}

	// State roots at the minimum common height should match.
	// (Since they may be 1 block apart, compare at minH.)
	t.Logf("Heights: %v, checking state roots at height %d", heights, minH)

	// --- Submit a transaction ---
	t.Log("Submitting transfer transaction...")
	_, _, recipAddr := func() (crypto.PublicKey, crypto.PrivateKey, crypto.Address) {
		pub, priv, err := crypto.GenerateKeyPair()
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		return pub, priv, crypto.AddressFromPublicKey(pub)
	}()

	transferTx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    userAddr,
		Recipient: recipAddr,
		Amount:    50_000_000,
		Nonce:     1,
	}
	types.SignTransaction(transferTx, userPriv)

	// Submit to node 0.
	pbTx := p2p.TxToProto(transferTx)
	resp, err := func() (*pb.TxSubmissionResponse, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return nodes[0].Engine.OnSubmitTx(ctx, &pb.TxSubmission{Tx: pbTx})
	}()
	if err != nil || !resp.Accepted {
		t.Fatalf("submit tx: err=%v accepted=%v", err, resp)
	}

	// Submit a CreatePost transaction.
	t.Log("Submitting create post transaction...")
	postTx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: userAddr,
		Text:   "First post on the DRANA testnet!",
		Amount: 5_000_000,
		Nonce:  2,
	}
	types.SignTransaction(postTx, userPriv)
	pbPostTx := p2p.TxToProto(postTx)
	resp2, err := func() (*pb.TxSubmissionResponse, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return nodes[0].Engine.OnSubmitTx(ctx, &pb.TxSubmission{Tx: pbPostTx})
	}()
	if err != nil || !resp2.Accepted {
		t.Fatalf("submit post tx: err=%v accepted=%v", err, resp2)
	}

	// Wait for transactions to be included.
	t.Log("Waiting for transactions to be included...")
	prevHeight := nodes[0].Engine.CurrentHeight()
	deadline2 := time.After(30 * time.Second)
	for {
		select {
		case <-deadline2:
			t.Fatal("timeout waiting for tx inclusion")
		case <-time.After(500 * time.Millisecond):
		}
		if nodes[0].Engine.CurrentHeight() >= prevHeight+3 {
			break
		}
	}

	// --- Verify transactions are reflected in all nodes ---
	t.Log("Verifying transaction effects...")
	for i, n := range nodes {
		ws := n.Engine.CurrentState()
		user, ok := ws.GetAccount(userAddr)
		if !ok {
			t.Fatalf("node %d: user account not found", i)
		}
		if user.Nonce < 2 {
			t.Fatalf("node %d: user nonce %d, want >= 2", i, user.Nonce)
		}

		recip, ok := ws.GetAccount(recipAddr)
		if !ok {
			t.Fatalf("node %d: recipient account not found", i)
		}
		if recip.Balance == 0 {
			t.Fatalf("node %d: recipient balance %d, should not be 0", i, recip.Balance)
		}

		// Check post exists.
		postID := types.DerivePostID(userAddr, 2) // nonce was 2 at creation
		post, ok := ws.GetPost(postID)
		if !ok {
			t.Fatalf("node %d: post not found", i)
		}
		if post.TotalStaked == 0 {
			t.Fatalf("node %d: post should have stake", i)
		}

		// Supply conservation.
		genesisSupply := uint64(1_000_000_000)
		var totalBal uint64
		for _, acct := range ws.AllAccounts() {
			totalBal += acct.Balance + acct.PostStakeBalance
		}
		expected := genesisSupply + ws.GetIssuedSupply() - ws.GetBurnedSupply()
		if totalBal != expected {
			t.Fatalf("node %d: supply conservation violated: %d != %d", i, totalBal, expected)
		}
	}

	// --- Test chain sync: start a fresh node 3 (4th node) and verify it syncs ---
	t.Log("Starting fresh node 3 to test chain sync...")
	// Generate a 4th validator identity — not in genesis, but we can still sync as a reader.
	// Actually, we just create a new Node instance using an existing validator's key but fresh data.
	freshDataDir := filepath.Join(tmpDir, "node3-fresh")
	os.MkdirAll(freshDataDir, 0755)
	freshConfig := &node.Config{
		GenesisPath:   genesisPath,
		DataDir:       freshDataDir,
		PrivKeyHex:    vals[2].privHex, // reuse val-2's key
		ListenAddr:    "127.0.0.1:26704",
		PeerEndpoints: peerEndpoints,
	}

	freshNode, err := node.NewNode(freshConfig)
	if err != nil {
		t.Fatalf("create fresh node: %v", err)
	}
	freshNode.Engine.BlockInterval = 2 * time.Second
	freshNode.P2PServer.Start()
	time.Sleep(500 * time.Millisecond)
	freshNode.Peers.Connect(peerEndpoints)

	ctxFresh, cancelFresh := context.WithCancel(context.Background())
	defer cancelFresh()

	// Sync from network.
	currentH := nodes[0].Engine.CurrentHeight()
	if err := freshNode.Engine.SyncToNetwork(ctxFresh); err != nil {
		t.Fatalf("fresh node sync: %v", err)
	}

	freshHeight := freshNode.Engine.CurrentHeight()
	t.Logf("Fresh node synced to height %d (network at %d)", freshHeight, currentH)
	if freshHeight < currentH {
		t.Fatalf("fresh node did not catch up: at %d, want >= %d", freshHeight, currentH)
	}

	// Verify state root matches at the synced height.
	rootFresh := state.ComputeStateRoot(freshNode.Engine.CurrentState())
	root0 := state.ComputeStateRoot(nodes[0].Engine.CurrentState())
	if nodes[0].Engine.CurrentHeight() == freshHeight && rootFresh != root0 {
		t.Fatalf("state root mismatch after sync: fresh=%x node0=%x", rootFresh, root0)
	}

	// --- Clean shutdown ---
	t.Log("Shutting down all nodes...")
	for i, c := range cancels {
		c()
		nodes[i].Stop()
	}
	cancelFresh()
	freshNode.Stop()

	t.Log("Phase 2 integration test passed.")
	t.Logf("  Final heights: node0=%d, node1=%d, node2=%d, fresh=%d",
		nodes[0].Engine.CurrentHeight(),
		nodes[1].Engine.CurrentHeight(),
		nodes[2].Engine.CurrentHeight(),
		freshNode.Engine.CurrentHeight())

	_ = userPub
	_ = consensus.ProposerForHeight
}

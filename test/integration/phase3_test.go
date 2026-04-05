package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/node"
	"github.com/drana-chain/drana/internal/rpc"
	"github.com/drana-chain/drana/internal/types"
)

func TestPhase3RPC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping RPC integration test in short mode")
	}

	// --- Setup: 3 validators + funded user ---
	type valID struct {
		pub     crypto.PublicKey
		priv    crypto.PrivateKey
		addr    crypto.Address
		privHex string
	}
	vals := make([]valID, 3)
	for i := range vals {
		pub, priv, err := crypto.GenerateKeyPair()
		if err != nil {
			t.Fatalf("keygen %d: %v", i, err)
		}
		vals[i] = valID{pub: pub, priv: priv, addr: crypto.AddressFromPublicKey(pub), privHex: hex.EncodeToString(priv[:])}
	}

	userPub, userPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("user keygen: %v", err)
	}
	userAddr := crypto.AddressFromPublicKey(userPub)

	tmpDir := t.TempDir()
	genesisPath := filepath.Join(tmpDir, "genesis.json")

	genesisData := map[string]interface{}{
		"chainId":            "drana-test-phase3",
		"genesisTime":        time.Now().Unix(),
		"maxPostLength":      280,
		"maxPostBytes":       1024,
		"minPostCommitment":  1000000,
		"minBoostCommitment": 100000,
		"maxTxPerBlock":      100,
		"maxBlockBytes":      1048576,
		"blockIntervalSec":   2,
		"blockReward":        10000000,
		"accounts": []map[string]interface{}{
			{"address": userAddr.String(), "balance": 1000000000},
			{"address": vals[0].addr.String(), "balance": 0},
			{"address": vals[1].addr.String(), "balance": 0},
			{"address": vals[2].addr.String(), "balance": 0},
		},
		"validators": []map[string]interface{}{
			{"address": vals[0].addr.String(), "pubKey": hex.EncodeToString(vals[0].pub[:]), "name": "val-0"},
			{"address": vals[1].addr.String(), "pubKey": hex.EncodeToString(vals[1].pub[:]), "name": "val-1"},
			{"address": vals[2].addr.String(), "pubKey": hex.EncodeToString(vals[2].pub[:]), "name": "val-2"},
		},
	}
	gdata, _ := json.MarshalIndent(genesisData, "", "  ")
	os.WriteFile(genesisPath, gdata, 0644)

	ports := []string{"127.0.0.1:26801", "127.0.0.1:26802", "127.0.0.1:26803"}
	rpcPorts := []string{"127.0.0.1:26851", "127.0.0.1:26852", "127.0.0.1:26853"}
	peerEndpoints := map[string]string{
		"val-0": ports[0], "val-1": ports[1], "val-2": ports[2],
	}

	nodes := make([]*node.Node, 3)
	cancels := make([]context.CancelFunc, 3)

	for i := 0; i < 3; i++ {
		dataDir := filepath.Join(tmpDir, fmt.Sprintf("node%d", i))
		os.MkdirAll(dataDir, 0755)
		cfg := &node.Config{
			GenesisPath:   genesisPath,
			DataDir:       dataDir,
			PrivKeyHex:    vals[i].privHex,
			ListenAddr:    ports[i],
			RPCListenAddr: rpcPorts[i],
			PeerEndpoints: peerEndpoints,
		}
		n, err := node.NewNode(cfg)
		if err != nil {
			t.Fatalf("NewNode %d: %v", i, err)
		}
		n.Engine.BlockInterval = 2 * time.Second
		nodes[i] = n
	}

	// Start all nodes.
	for i := 0; i < 3; i++ {
		nodes[i].P2PServer.Start()
		nodes[i].RPCServer.Start()
	}
	time.Sleep(500 * time.Millisecond)
	for i := 0; i < 3; i++ {
		nodes[i].Peers.Connect(peerEndpoints)
	}
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		go nodes[i].Engine.Run(ctx)
	}

	rpcBase := "http://" + rpcPorts[0]

	// Wait for 3 blocks.
	waitForHeight(t, rpcBase, 3, 30*time.Second)

	// --- Test: GetNodeInfo ---
	t.Log("Testing GET /v1/node/info")
	var nodeInfo rpc.NodeInfoResponse
	mustGet(t, rpcBase+"/v1/node/info", &nodeInfo)
	if nodeInfo.ChainID != "drana-test-phase3" {
		t.Fatalf("chainId: %q", nodeInfo.ChainID)
	}
	if nodeInfo.LatestHeight < 3 {
		t.Fatalf("height: %d", nodeInfo.LatestHeight)
	}
	if nodeInfo.BlockReward != 10000000 {
		t.Fatalf("blockReward: %d", nodeInfo.BlockReward)
	}
	if nodeInfo.ValidatorCount != 3 {
		t.Fatalf("validatorCount: %d", nodeInfo.ValidatorCount)
	}

	// --- Test: GetLatestBlock ---
	t.Log("Testing GET /v1/blocks/latest")
	var latestBlock rpc.BlockResponse
	mustGet(t, rpcBase+"/v1/blocks/latest", &latestBlock)
	if latestBlock.Height != nodeInfo.LatestHeight {
		t.Fatalf("latest block height %d != node info %d", latestBlock.Height, nodeInfo.LatestHeight)
	}

	// --- Test: GetBlockByHeight ---
	t.Log("Testing GET /v1/blocks/1")
	var block1 rpc.BlockResponse
	mustGet(t, rpcBase+"/v1/blocks/1", &block1)
	if block1.Height != 1 {
		t.Fatalf("block 1 height: %d", block1.Height)
	}

	// --- Test: GetAccount (validator with block reward) ---
	t.Log("Testing GET /v1/accounts/{validator}")
	var valAcct rpc.AccountResponse
	mustGet(t, rpcBase+"/v1/accounts/"+vals[0].addr.String(), &valAcct)
	if valAcct.Balance == 0 {
		t.Fatal("validator should have block reward balance")
	}

	// --- Test: GetAccount (unknown address returns zero) ---
	t.Log("Testing GET /v1/accounts/{unknown}")
	unknownPub, _, _ := crypto.GenerateKeyPair()
	unknownAddr := crypto.AddressFromPublicKey(unknownPub)
	var unknownAcct rpc.AccountResponse
	mustGet(t, rpcBase+"/v1/accounts/"+unknownAddr.String(), &unknownAcct)
	if unknownAcct.Balance != 0 || unknownAcct.Nonce != 0 {
		t.Fatalf("unknown account should be zero: bal=%d nonce=%d", unknownAcct.Balance, unknownAcct.Nonce)
	}

	// --- Test: Submit Transfer ---
	t.Log("Testing POST /v1/transactions (transfer)")
	recipPub, _, _ := crypto.GenerateKeyPair()
	recipAddr := crypto.AddressFromPublicKey(recipPub)

	transferTx := &types.Transaction{
		Type: types.TxTransfer, Sender: userAddr, Recipient: recipAddr,
		Amount: 50_000_000, Nonce: 1,
	}
	types.SignTransaction(transferTx, userPriv)
	transferHash := submitTxRPC(t, rpcBase, transferTx)
	t.Logf("  Transfer tx: %s", transferHash)

	// --- Test: Submit CreatePost ---
	t.Log("Testing POST /v1/transactions (create_post)")
	postTx := &types.Transaction{
		Type: types.TxCreatePost, Sender: userAddr,
		Text: "First post from the CLI test!", Amount: 5_000_000, Nonce: 2,
	}
	types.SignTransaction(postTx, userPriv)
	postTxHash := submitTxRPC(t, rpcBase, postTx)
	postID := types.DerivePostID(userAddr, 2)
	t.Logf("  Post tx: %s, post ID: %s", postTxHash, hex.EncodeToString(postID[:]))

	// Wait for inclusion.
	waitForHeight(t, rpcBase, nodeInfo.LatestHeight+3, 30*time.Second)

	// --- Test: GetTransaction ---
	t.Log("Testing GET /v1/transactions/{hash}")
	var txResp rpc.TransactionResponse
	mustGet(t, rpcBase+"/v1/transactions/"+transferHash, &txResp)
	if txResp.Type != "transfer" {
		t.Fatalf("tx type: %q", txResp.Type)
	}
	if txResp.Amount != 50_000_000 {
		t.Fatalf("tx amount: %d", txResp.Amount)
	}
	if txResp.BlockHeight == 0 {
		t.Fatal("tx should have block height")
	}

	// --- Test: GetTransactionStatus ---
	t.Log("Testing GET /v1/transactions/{hash}/status")
	var statusResp rpc.TxStatusResponse
	mustGet(t, rpcBase+"/v1/transactions/"+transferHash+"/status", &statusResp)
	if statusResp.Status != "confirmed" {
		t.Fatalf("tx status: %q", statusResp.Status)
	}

	// --- Test: GetAccount (recipient) ---
	t.Log("Testing GET /v1/accounts/{recipient}")
	var recipAcct rpc.AccountResponse
	mustGet(t, rpcBase+"/v1/accounts/"+recipAddr.String(), &recipAcct)
	if recipAcct.Balance != 50_000_000 {
		t.Fatalf("recipient balance: %d", recipAcct.Balance)
	}

	// --- Test: GetPost ---
	t.Log("Testing GET /v1/posts/{id}")
	var postResp rpc.PostResponse
	mustGet(t, rpcBase+"/v1/posts/"+hex.EncodeToString(postID[:]), &postResp)
	if postResp.Text != "First post from the CLI test!" {
		t.Fatalf("post text: %q", postResp.Text)
	}
	if postResp.TotalStaked == 0 {
		t.Fatal("post should have stake")
	}
	t.Logf("  Post staked: %d, burned: %d", postResp.TotalStaked, postResp.TotalBurned)

	// --- Test: ListPosts ---
	t.Log("Testing GET /v1/posts?author={addr}")
	var listResp rpc.PostListResponse
	mustGet(t, rpcBase+"/v1/posts?author="+userAddr.String(), &listResp)
	if listResp.TotalCount != 1 {
		t.Fatalf("post list count: %d", listResp.TotalCount)
	}

	// --- Test: Submit BoostPost ---
	t.Log("Testing POST /v1/transactions (boost_post)")
	boostTx := &types.Transaction{
		Type: types.TxBoostPost, Sender: userAddr,
		PostID: postID, Amount: 2_000_000, Nonce: 3,
	}
	types.SignTransaction(boostTx, userPriv)
	boostTxHash := submitTxRPC(t, rpcBase, boostTx)
	t.Logf("  Boost tx: %s", boostTxHash)

	waitForHeight(t, rpcBase, nodes[0].Engine.CurrentHeight()+2, 30*time.Second)

	// Verify boost effect.
	var postAfterBoost rpc.PostResponse
	mustGet(t, rpcBase+"/v1/posts/"+hex.EncodeToString(postID[:]), &postAfterBoost)
	if postAfterBoost.TotalStaked <= postResp.TotalStaked {
		t.Fatalf("post staked should increase after boost: before=%d, after=%d", postResp.TotalStaked, postAfterBoost.TotalStaked)
	}
	// The same user created and boosted — still 1 unique staker.
	if postAfterBoost.StakerCount < 1 {
		t.Fatalf("staker count should be >= 1: got %d", postAfterBoost.StakerCount)
	}
	t.Logf("  Post after boost: staked=%d, stakers=%d", postAfterBoost.TotalStaked, postAfterBoost.StakerCount)

	// --- Test: ListValidators ---
	t.Log("Testing GET /v1/network/validators")
	var valList []rpc.ValidatorResponse
	mustGet(t, rpcBase+"/v1/network/validators", &valList)
	if len(valList) != 3 {
		t.Fatalf("validator count: %d", len(valList))
	}

	// --- Test: 404 for nonexistent post ---
	t.Log("Testing GET /v1/posts/{fake} -> 404")
	resp, err := http.Get(rpcBase + "/v1/posts/" + hex.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	// --- Supply conservation via RPC ---
	t.Log("Verifying supply conservation via RPC")
	var info2 rpc.NodeInfoResponse
	mustGet(t, rpcBase+"/v1/node/info", &info2)

	var userAcct rpc.AccountResponse
	mustGet(t, rpcBase+"/v1/accounts/"+userAddr.String(), &userAcct)

	// Sum balances: we need all accounts but RPC doesn't have a "list all accounts" endpoint.
	// Use the engine directly for this check.
	ws := nodes[0].Engine.CurrentState()
	var totalBal uint64
	for _, acct := range ws.AllAccounts() {
		totalBal += acct.Balance + acct.PostStakeBalance
	}
	genesisSupply := uint64(1_000_000_000)
	expected := genesisSupply + info2.IssuedSupply - info2.BurnedSupply
	if totalBal != expected {
		t.Fatalf("supply conservation: balances=%d != expected=%d", totalBal, expected)
	}

	// --- Shutdown ---
	t.Log("Shutting down...")
	for i, c := range cancels {
		c()
		nodes[i].Stop()
	}

	t.Log("Phase 3 integration test passed.")
	t.Logf("  Final height: %d", nodes[0].Engine.CurrentHeight())
	t.Logf("  Issued: %d, Burned: %d", info2.IssuedSupply, info2.BurnedSupply)

	_ = userPub
}

// --- helpers ---

func waitForHeight(t *testing.T, rpcBase string, targetHeight uint64, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for height %d", targetHeight)
		case <-time.After(300 * time.Millisecond):
		}
		var info rpc.NodeInfoResponse
		if err := rpcGet(rpcBase+"/v1/node/info", &info); err == nil && info.LatestHeight >= targetHeight {
			return
		}
	}
}

func mustGet(t *testing.T, url string, out interface{}) {
	t.Helper()
	if err := rpcGet(url, out); err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
}

func rpcGet(url string, out interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

func submitTxRPC(t *testing.T, rpcBase string, tx *types.Transaction) string {
	t.Helper()
	req := rpc.SubmitTxRequest{
		Sender:    tx.Sender.String(),
		Amount:    tx.Amount,
		Nonce:     tx.Nonce,
		Signature: hex.EncodeToString(tx.Signature),
		PubKey:    hex.EncodeToString(tx.PubKey[:]),
	}
	switch tx.Type {
	case types.TxTransfer:
		req.Type = "transfer"
		req.Recipient = tx.Recipient.String()
	case types.TxCreatePost:
		req.Type = "create_post"
		req.Text = tx.Text
	case types.TxBoostPost:
		req.Type = "boost_post"
		req.PostID = hex.EncodeToString(tx.PostID[:])
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(rpcBase+"/v1/transactions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("submit tx: %v", err)
	}
	defer resp.Body.Close()
	var result rpc.SubmitTxResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.Accepted {
		t.Fatalf("tx rejected: %s", result.Error)
	}
	return result.TxHash
}

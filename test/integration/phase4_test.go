package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/indexer"
	"github.com/drana-chain/drana/internal/node"
	"github.com/drana-chain/drana/internal/rpc"
	"github.com/drana-chain/drana/internal/types"
)

func TestPhase4Indexer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping indexer integration test in short mode")
	}

	// --- Setup: 3 validators + 2 funded users ---
	type valID struct {
		pub     crypto.PublicKey
		priv    crypto.PrivateKey
		addr    crypto.Address
		privHex string
	}
	vals := make([]valID, 3)
	for i := range vals {
		pub, priv, _ := crypto.GenerateKeyPair()
		vals[i] = valID{pub: pub, priv: priv, addr: crypto.AddressFromPublicKey(pub), privHex: hex.EncodeToString(priv[:])}
	}

	_, user1Priv, _ := crypto.GenerateKeyPair()
	var user1Pub crypto.PublicKey
	copy(user1Pub[:], user1Priv[32:])
	user1Addr := crypto.AddressFromPublicKey(user1Pub)

	_, user2Priv, _ := crypto.GenerateKeyPair()
	var user2Pub crypto.PublicKey
	copy(user2Pub[:], user2Priv[32:])
	user2Addr := crypto.AddressFromPublicKey(user2Pub)

	tmpDir := t.TempDir()
	genesisPath := filepath.Join(tmpDir, "genesis.json")
	genesisData := map[string]interface{}{
		"chainId": "drana-test-phase4", "genesisTime": time.Now().Unix(),
		"maxPostLength": 280, "maxPostBytes": 1024,
		"minPostCommitment": 1000000, "minBoostCommitment": 100000,
		"maxTxPerBlock": 100, "maxBlockBytes": 1048576,
		"blockIntervalSec": 2, "blockReward": 10000000,
		"accounts": []map[string]interface{}{
			{"address": user1Addr.String(), "balance": 1000000000},
			{"address": user2Addr.String(), "balance": 500000000},
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

	ports := []string{"127.0.0.1:26901", "127.0.0.1:26902", "127.0.0.1:26903"}
	rpcPorts := []string{"127.0.0.1:26951", "127.0.0.1:26952", "127.0.0.1:26953"}
	peerEndpoints := map[string]string{"val-0": ports[0], "val-1": ports[1], "val-2": ports[2]}

	// Start 3-node network.
	nodes := make([]*node.Node, 3)
	cancels := make([]context.CancelFunc, 3)
	for i := 0; i < 3; i++ {
		dataDir := filepath.Join(tmpDir, fmt.Sprintf("node%d", i))
		os.MkdirAll(dataDir, 0755)
		n, err := node.NewNode(&node.Config{
			GenesisPath: genesisPath, DataDir: dataDir, PrivKeyHex: vals[i].privHex,
			ListenAddr: ports[i], RPCListenAddr: rpcPorts[i], PeerEndpoints: peerEndpoints,
		})
		if err != nil {
			t.Fatalf("NewNode %d: %v", i, err)
		}
		n.Engine.BlockInterval = 2 * time.Second
		nodes[i] = n
	}

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

	// --- Submit transactions ---
	t.Log("Submitting transactions...")

	// User1 creates post A with 10M.
	postATx := &types.Transaction{
		Type: types.TxCreatePost, Sender: user1Addr,
		Text: "Post A: the attention market is live", Amount: 10_000_000, Nonce: 1,
	}
	types.SignTransaction(postATx, user1Priv)
	submitTxRPC(t, rpcBase, postATx)
	postAID := types.DerivePostID(user1Addr, 1)
	postAIDHex := hex.EncodeToString(postAID[:])

	// Wait for post A to be included.
	waitForHeight(t, rpcBase, nodes[0].Engine.CurrentHeight()+2, 30*time.Second)

	// User1 creates post B with 5M (in a later block so it has a higher height).
	postBTx := &types.Transaction{
		Type: types.TxCreatePost, Sender: user1Addr,
		Text: "Post B: a quieter entry", Amount: 5_000_000, Nonce: 2,
	}
	types.SignTransaction(postBTx, user1Priv)
	submitTxRPC(t, rpcBase, postBTx)
	postBID := types.DerivePostID(user1Addr, 2)

	// Wait for post B to be included.
	waitForHeight(t, rpcBase, nodes[0].Engine.CurrentHeight()+2, 30*time.Second)

	// User2 boosts post A with 3M (third-party boost).
	boostTx := &types.Transaction{
		Type: types.TxBoostPost, Sender: user2Addr,
		PostID: postAID, Amount: 3_000_000, Nonce: 1,
	}
	types.SignTransaction(boostTx, user2Priv)
	submitTxRPC(t, rpcBase, boostTx)

	// User1 self-boosts post A with 2M.
	selfBoostTx := &types.Transaction{
		Type: types.TxBoostPost, Sender: user1Addr,
		PostID: postAID, Amount: 2_000_000, Nonce: 3,
	}
	types.SignTransaction(selfBoostTx, user1Priv)
	submitTxRPC(t, rpcBase, selfBoostTx)

	// Wait for boosts to be included.
	waitForHeight(t, rpcBase, nodes[0].Engine.CurrentHeight()+2, 30*time.Second)

	// --- Start the indexer ---
	t.Log("Starting indexer...")
	idxDBPath := filepath.Join(tmpDir, "indexer.db")
	idxDB, err := indexer.OpenDB(idxDBPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	idxDB.Migrate()

	follower := indexer.NewFollower(rpcBase, idxDB, 500*time.Millisecond, 6, 3, 2, 1)
	idxAPI := indexer.NewAPIServer("127.0.0.1:26980", idxDB, rpcBase)
	idxAPI.Start()

	idxCtx, idxCancel := context.WithCancel(context.Background())
	go follower.Run(idxCtx)

	// Wait for indexer to catch up.
	idxBase := "http://127.0.0.1:26980"
	time.Sleep(3 * time.Second) // give it time to index

	// --- Query the indexer API ---
	t.Log("Querying indexer API...")

	// Stats
	var stats indexer.StatsResponse
	mustGet(t, idxBase+"/v1/stats", &stats)
	t.Logf("  Stats: posts=%d boosts=%d transfers=%d burned=%d issued=%d",
		stats.TotalPosts, stats.TotalBoosts, stats.TotalTransfers, stats.TotalBurned, stats.TotalIssued)
	if stats.TotalPosts != 2 {
		t.Fatalf("totalPosts: %d, want 2", stats.TotalPosts)
	}
	if stats.TotalBoosts != 2 {
		t.Fatalf("totalBoosts: %d, want 2", stats.TotalBoosts)
	}

	// Feed: top all time — post A (15M) > post B (5M)
	var feedTop indexer.FeedResponse
	mustGet(t, idxBase+"/v1/feed?strategy=top", &feedTop)
	if len(feedTop.Posts) < 2 {
		t.Fatalf("feed top: %d posts", len(feedTop.Posts))
	}
	if feedTop.Posts[0].PostID != postAIDHex {
		t.Fatalf("feed top: first should be post A, got %s", feedTop.Posts[0].PostID)
	}

	// Feed: new — post B (higher nonce → higher height) > post A
	var feedNew indexer.FeedResponse
	mustGet(t, idxBase+"/v1/feed?strategy=new", &feedNew)
	postBIDHex := hex.EncodeToString(postBID[:])
	if feedNew.Posts[0].PostID != postBIDHex {
		t.Fatalf("feed new: first should be post B (%s), got %s", postBIDHex, feedNew.Posts[0].PostID)
	}

	// Post A derived fields.
	var postA indexer.IndexedPost
	mustGet(t, idxBase+"/v1/posts/"+postAIDHex, &postA)
	// authorStaked = 10M*94% (creation) + 2M*94% (self-boost) = 9.4M + 1.88M = 11.28M
	if postA.AuthorStaked != 11_280_000 {
		t.Fatalf("postA authorStaked: %d, want 11280000", postA.AuthorStaked)
	}
	// thirdPartyStaked = 3M*94% = 2.82M
	if postA.ThirdPartyStaked != 2_820_000 {
		t.Fatalf("postA thirdPartyStaked: %d, want 2820000", postA.ThirdPartyStaked)
	}
	// totalStaked = 11.28M + 2.82M = 14.1M
	if postA.TotalStaked != 14_100_000 {
		t.Fatalf("postA totalStaked: %d, want 14100000", postA.TotalStaked)
	}
	// uniqueBoosterCount = 2 (user1 + user2)
	if postA.UniqueBoosterCount != 2 {
		t.Fatalf("postA uniqueBoosterCount: %d, want 2", postA.UniqueBoosterCount)
	}
	// stakerCount = 3 (1 from creation + 1 self-boost + 1 third-party boost)
	if postA.StakerCount != 3 {
		t.Fatalf("postA stakerCount: %d, want 3", postA.StakerCount)
	}

	// Boost history.
	var boostHistory indexer.BoostHistoryResponse
	mustGet(t, idxBase+"/v1/posts/"+postAIDHex+"/boosts", &boostHistory)
	if boostHistory.TotalCount != 2 {
		t.Fatalf("boost history: %d, want 2", boostHistory.TotalCount)
	}

	// Author profile.
	var authorProfile indexer.AuthorProfile
	mustGet(t, idxBase+"/v1/authors/"+user1Addr.String(), &authorProfile)
	if authorProfile.PostCount != 2 {
		t.Fatalf("author postCount: %d, want 2", authorProfile.PostCount)
	}

	// Leaderboard.
	var leaderboard indexer.LeaderboardResponse
	mustGet(t, idxBase+"/v1/leaderboard", &leaderboard)
	if len(leaderboard.Authors) < 1 {
		t.Fatal("leaderboard empty")
	}
	if leaderboard.Authors[0].Address != user1Addr.String() {
		t.Fatalf("leaderboard first: %s, want %s", leaderboard.Authors[0].Address, user1Addr.String())
	}

	// Feed by author.
	var feedAuthor indexer.FeedResponse
	mustGet(t, idxBase+"/v1/feed/author/"+user1Addr.String(), &feedAuthor)
	if feedAuthor.TotalCount != 2 {
		t.Fatalf("feed by author: %d, want 2", feedAuthor.TotalCount)
	}

	// --- Shutdown ---
	t.Log("Shutting down...")
	idxCancel()
	idxAPI.Stop(context.Background())
	idxDB.Close()
	for i, c := range cancels {
		c()
		nodes[i].Stop()
	}

	t.Log("Phase 4 integration test passed.")
	t.Logf("  Posts indexed: %d", stats.TotalPosts)
	t.Logf("  Boosts indexed: %d", stats.TotalBoosts)
	t.Logf("  Post A: total=%d, author=%d, thirdParty=%d, uniqueBoosters=%d",
		postA.TotalStaked, postA.AuthorStaked, postA.ThirdPartyStaked, postA.UniqueBoosterCount)
}

// submitTxRPC submits a transaction via the node RPC (reused from phase3_test).
func submitTxRPC4(t *testing.T, rpcBase string, tx *types.Transaction) string {
	t.Helper()
	req := rpc.SubmitTxRequest{
		Sender: tx.Sender.String(), Amount: tx.Amount, Nonce: tx.Nonce,
		Signature: hex.EncodeToString(tx.Signature), PubKey: hex.EncodeToString(tx.PubKey[:]),
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

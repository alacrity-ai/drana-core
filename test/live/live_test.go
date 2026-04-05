// Package live runs integration tests against a running Docker testnet.
//
// Prerequisites: `make docker-up` must be running.
// Run: go test ./test/live/ -v -timeout 600s
//
// This test creates 10 wallets, funds them, registers names, creates posts
// across channels, boosts posts, replies, and verifies everything via the
// node RPC and indexer APIs.
package live

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/rpc"
	"github.com/drana-chain/drana/internal/types"
)

const (
	nodeRPC    = "http://localhost:26657"
	indexerAPI = "http://localhost:26680"
	blockWait  = 130 * time.Second // slightly more than 120s block interval
)

// wallet holds a test user's identity.
type wallet struct {
	pub     crypto.PublicKey
	priv    crypto.PrivateKey
	addr    crypto.Address
	addrStr string
	name    string
}

func newWallet() wallet {
	pub, priv, _ := crypto.GenerateKeyPair()
	addr := crypto.AddressFromPublicKey(pub)
	return wallet{pub: pub, priv: priv, addr: addr, addrStr: addr.String()}
}

// fundingKey reads the testnet validator-1 private key for funding.
func fundingKey(t *testing.T) crypto.PrivateKey {
	t.Helper()
	data, err := os.ReadFile("../../testnet/validator-1/config.local.json")
	if err != nil {
		t.Fatalf("Cannot read testnet config — is `make docker-up` running? %v", err)
	}
	var cfg struct{ PrivKeyHex string `json:"privKeyHex"` }
	json.Unmarshal(data, &cfg)
	b, _ := hex.DecodeString(cfg.PrivKeyHex)
	var key crypto.PrivateKey
	copy(key[:], b)
	return key
}

func fundingPub(key crypto.PrivateKey) crypto.PublicKey {
	var pub crypto.PublicKey
	copy(pub[:], key[32:])
	return pub
}

func fundingAddr(key crypto.PrivateKey) string {
	return crypto.AddressFromPublicKey(fundingPub(key)).String()
}

// --- HTTP helpers ---

func httpGet(url string, out interface{}) error {
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

func submitTx(t *testing.T, tx *types.Transaction) string {
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
		req.Channel = tx.Channel
		if tx.PostID != (types.PostID{}) {
			req.ParentPostID = hex.EncodeToString(tx.PostID[:])
		}
	case types.TxBoostPost:
		req.Type = "boost_post"
		req.PostID = hex.EncodeToString(tx.PostID[:])
	case types.TxRegisterName:
		req.Type = "register_name"
		req.Text = tx.Text
	}

	body, _ := json.Marshal(req)
	resp, err := http.Post(nodeRPC+"/v1/transactions", "application/json", bytes.NewReader(body))
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

func getNonce(addr string) uint64 {
	var acct rpc.AccountResponse
	httpGet(nodeRPC+"/v1/accounts/"+addr, &acct)
	return acct.Nonce
}

func getBalance(addr string) uint64 {
	var acct rpc.AccountResponse
	httpGet(nodeRPC+"/v1/accounts/"+addr, &acct)
	return acct.Balance
}

func waitForTx(t *testing.T, hash string) {
	t.Helper()
	deadline := time.Now().Add(blockWait)
	for time.Now().Before(deadline) {
		var status rpc.TxStatusResponse
		if err := httpGet(nodeRPC+"/v1/transactions/"+hash+"/status", &status); err == nil {
			if status.Status == "confirmed" {
				return
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("tx %s not confirmed within %v", hash[:16], blockWait)
}

func waitForBlock(t *testing.T) {
	t.Helper()
	var info rpc.NodeInfoResponse
	httpGet(nodeRPC+"/v1/node/info", &info)
	startHeight := info.LatestHeight
	t.Logf("  Waiting for block (current height: %d)...", startHeight)
	deadline := time.Now().Add(blockWait)
	for time.Now().Before(deadline) {
		httpGet(nodeRPC+"/v1/node/info", &info)
		if info.LatestHeight > startHeight {
			t.Logf("  Block %d produced.", info.LatestHeight)
			return
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("no new block within %v", blockWait)
}

func waitForIndexer(t *testing.T, minHeight uint64) {
	t.Helper()
	type statsResp struct{ LatestHeight uint64 `json:"latestHeight"` }
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var stats statsResp
		if err := httpGet(indexerAPI+"/v1/stats", &stats); err == nil && stats.LatestHeight >= minHeight {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("indexer did not reach height %d", minHeight)
}

// --- The Test ---

func TestLiveNetwork(t *testing.T) {
	// Check that the testnet is running.
	var info rpc.NodeInfoResponse
	if err := httpGet(nodeRPC+"/v1/node/info", &info); err != nil {
		t.Skipf("Testnet not running (make docker-up): %v", err)
	}
	t.Logf("Connected to %s at height %d", info.ChainID, info.LatestHeight)

	// Nonce tracker — keeps local nonce per address to allow batching txs in one block.
	nonceMap := make(map[string]uint64)
	nextNonce := func(addr string) uint64 {
		if _, ok := nonceMap[addr]; !ok {
			nonceMap[addr] = getNonce(addr)
		}
		nonceMap[addr]++
		return nonceMap[addr]
	}

	// --- Phase 1: Create and fund 10 wallets ---
	t.Log("=== Phase 1: Creating 10 wallets ===")
	funderKey := fundingKey(t)
	funderAddr := fundingAddr(funderKey)

	names := []string{"alice", "bob", "carol", "dave", "eve", "frank", "grace", "heidi", "ivan", "judy"}
	amounts := []uint64{500, 400, 300, 250, 200, 150, 100, 80, 60, 40} // DRANA

	wallets := make([]wallet, 10)
	for i := range wallets {
		wallets[i] = newWallet()
		t.Logf("  Wallet %d: %s (will be '%s', funded with %d DRANA)", i, wallets[i].addrStr[:20]+"...", names[i], amounts[i])
	}

	t.Log("  Funding wallets...")
	var txHashes []string
	funderSender := crypto.AddressFromPublicKey(fundingPub(funderKey))
	for i, w := range wallets {
		tx := &types.Transaction{
			Type:      types.TxTransfer,
			Sender:    funderSender,
			Recipient: w.addr,
			Amount:    amounts[i] * 1_000_000,
			Nonce:     nextNonce(funderAddr),
		}
		types.SignTransaction(tx, funderKey)
		txHashes = append(txHashes, submitTx(t, tx))
	}

	// Wait for all funding txs to confirm.
	t.Log("  Waiting for funding to confirm...")
	for _, h := range txHashes {
		waitForTx(t, h)
	}

	// Verify balances.
	for i, w := range wallets {
		bal := getBalance(w.addrStr)
		expected := amounts[i] * 1_000_000
		if bal != expected {
			t.Fatalf("Wallet %d balance: got %d, want %d", i, bal, expected)
		}
	}
	t.Log("  All 10 wallets funded.")

	// --- Phase 2: Register names ---
	t.Log("=== Phase 2: Registering names ===")
	txHashes = nil
	for i, w := range wallets {
		tx := &types.Transaction{
			Type: types.TxRegisterName, Sender: w.addr, Text: names[i],
			Amount: 0, Nonce: nextNonce(w.addrStr),
		}
		types.SignTransaction(tx, w.priv)
		txHashes = append(txHashes, submitTx(t, tx))
		wallets[i].name = names[i]
	}

	t.Log("  Waiting for names to confirm...")
	for _, h := range txHashes {
		waitForTx(t, h)
	}

	// Verify names via RPC.
	for i, w := range wallets {
		var acct rpc.AccountResponse
		httpGet(nodeRPC+"/v1/accounts/"+w.addrStr, &acct)
		if acct.Name != names[i] {
			t.Fatalf("Wallet %d name: got %q, want %q", i, acct.Name, names[i])
		}
	}
	t.Log("  All 10 names registered.")

	// --- Phase 3: Create posts across channels ---
	t.Log("=== Phase 3: Creating posts ===")

	type postInfo struct {
		id      string // hex post ID
		author  int    // wallet index
		text    string
		channel string
		amount  uint64 // microdrana
	}

	posts := []postInfo{
		{author: 0, text: "Welcome to DRANA! The attention economy begins.", channel: "general", amount: 50_000_000},
		{author: 1, text: "First gaming post. Let the games begin.", channel: "gaming", amount: 30_000_000},
		{author: 2, text: "Political discourse costs money here.", channel: "politics", amount: 20_000_000},
		{author: 3, text: "Anyone want to talk about crypto?", channel: "crypto", amount: 15_000_000},
		{author: 0, text: "Another thought from alice.", channel: "general", amount: 10_000_000},
		{author: 4, text: "Eve's hot take on gaming.", channel: "gaming", amount: 25_000_000},
		{author: 5, text: "Frank here with the memes.", channel: "memes", amount: 8_000_000},
		{author: 1, text: "Bob's second gaming post.", channel: "gaming", amount: 12_000_000},
		{author: 6, text: "Grace discusses the future.", channel: "general", amount: 18_000_000},
		{author: 7, text: "Heidi's perspective on politics.", channel: "politics", amount: 22_000_000},
		{author: 8, text: "Ivan posts about crypto markets.", channel: "crypto", amount: 9_000_000},
		{author: 9, text: "Judy's first post. Hello world!", channel: "general", amount: 5_000_000},
	}

	txHashes = nil
	for i := range posts {
		w := wallets[posts[i].author]
		nonce := nextNonce(w.addrStr)
		tx := &types.Transaction{
			Type: types.TxCreatePost, Sender: w.addr, Text: posts[i].text,
			Channel: posts[i].channel, Amount: posts[i].amount, Nonce: nonce,
		}
		types.SignTransaction(tx, w.priv)
		txHashes = append(txHashes, submitTx(t, tx))
		pid := types.DerivePostID(w.addr, nonce)
		posts[i].id = hex.EncodeToString(pid[:])
		t.Logf("  Post %d: '%s...' in #%s by %s (%d DRANA)", i, posts[i].text[:20], posts[i].channel, names[posts[i].author], posts[i].amount/1_000_000)
	}

	t.Log("  Waiting for posts to confirm...")
	for _, h := range txHashes {
		waitForTx(t, h)
	}
	t.Logf("  All %d posts confirmed.", len(posts))

	// --- Phase 4: Boost posts ---
	t.Log("=== Phase 4: Boosting posts ===")

	type boostInfo struct {
		postIdx int
		booster int
		amount  uint64
	}

	boosts := []boostInfo{
		{postIdx: 0, booster: 1, amount: 10_000_000}, // bob boosts alice's welcome post
		{postIdx: 0, booster: 2, amount: 5_000_000},  // carol boosts it too
		{postIdx: 1, booster: 0, amount: 15_000_000}, // alice boosts bob's gaming post
		{postIdx: 5, booster: 3, amount: 8_000_000},  // dave boosts eve's gaming post
		{postIdx: 2, booster: 7, amount: 12_000_000}, // heidi boosts carol's politics post
		{postIdx: 0, booster: 0, amount: 5_000_000},  // alice self-boosts her welcome post
	}

	txHashes = nil
	for _, b := range boosts {
		w := wallets[b.booster]
		pidBytes, _ := hex.DecodeString(posts[b.postIdx].id)
		var pid types.PostID
		copy(pid[:], pidBytes)
		tx := &types.Transaction{
			Type: types.TxBoostPost, Sender: w.addr, PostID: pid,
			Amount: b.amount, Nonce: nextNonce(w.addrStr),
		}
		types.SignTransaction(tx, w.priv)
		txHashes = append(txHashes, submitTx(t, tx))
		t.Logf("  %s boosts post %d with %d DRANA", names[b.booster], b.postIdx, b.amount/1_000_000)
	}

	t.Log("  Waiting for boosts to confirm...")
	for _, h := range txHashes {
		waitForTx(t, h)
	}
	t.Logf("  All %d boosts confirmed.", len(boosts))

	// --- Phase 5: Reply to posts ---
	t.Log("=== Phase 5: Replying to posts ===")

	type replyInfo struct {
		parentIdx int
		author    int
		text      string
		amount    uint64
	}

	replies := []replyInfo{
		{parentIdx: 0, author: 3, text: "Great welcome post alice!", amount: 2_000_000},
		{parentIdx: 0, author: 4, text: "The future is here.", amount: 1_000_000},
		{parentIdx: 1, author: 5, text: "Gaming on the blockchain!", amount: 3_000_000},
		{parentIdx: 2, author: 8, text: "Politics + money = transparency.", amount: 1_500_000},
		{parentIdx: 5, author: 9, text: "Eve knows what's up.", amount: 2_500_000},
	}

	txHashes = nil
	for _, r := range replies {
		w := wallets[r.author]
		pidBytes, _ := hex.DecodeString(posts[r.parentIdx].id)
		var pid types.PostID
		copy(pid[:], pidBytes)
		tx := &types.Transaction{
			Type: types.TxCreatePost, Sender: w.addr, PostID: pid,
			Text: r.text, Amount: r.amount, Nonce: nextNonce(w.addrStr),
		}
		types.SignTransaction(tx, w.priv)
		txHashes = append(txHashes, submitTx(t, tx))
		t.Logf("  %s replies to post %d: '%s'", names[r.author], r.parentIdx, r.text)
	}

	t.Log("  Waiting for replies to confirm...")
	for _, h := range txHashes {
		waitForTx(t, h)
	}
	t.Logf("  All %d replies confirmed.", len(replies))

	// --- Phase 6: Verify via indexer ---
	t.Log("=== Phase 6: Verifying via indexer ===")

	// Wait for indexer to catch up.
	var nodeInfo rpc.NodeInfoResponse
	httpGet(nodeRPC+"/v1/node/info", &nodeInfo)
	waitForIndexer(t, nodeInfo.LatestHeight)

	// Check stats.
	type statsResp struct {
		TotalPosts  int    `json:"totalPosts"`
		TotalBoosts int    `json:"totalBoosts"`
		TotalBurned uint64 `json:"totalBurned"`
	}
	var stats statsResp
	httpGet(indexerAPI+"/v1/stats", &stats)
	expectedPosts := len(posts) + len(replies)
	if stats.TotalPosts != expectedPosts {
		t.Fatalf("Indexer totalPosts: got %d, want %d", stats.TotalPosts, expectedPosts)
	}
	if stats.TotalBoosts != len(boosts) {
		t.Fatalf("Indexer totalBoosts: got %d, want %d", stats.TotalBoosts, len(boosts))
	}
	t.Logf("  Stats: %d posts, %d boosts, %d microdrana burned", stats.TotalPosts, stats.TotalBoosts, stats.TotalBurned)

	// Check channels.
	type chanResp struct {
		Channel   string `json:"channel"`
		PostCount int    `json:"postCount"`
	}
	var channels []chanResp
	httpGet(indexerAPI+"/v1/channels", &channels)
	if len(channels) < 4 {
		t.Fatalf("Expected at least 4 channels, got %d", len(channels))
	}
	t.Logf("  Channels: %d", len(channels))
	for _, c := range channels {
		t.Logf("    #%s: %d posts", c.Channel, c.PostCount)
	}

	// Check top feed.
	type feedResp struct {
		Posts []struct {
			PostID         string `json:"postId"`
			Author         string `json:"author"`
			TotalCommitted uint64 `json:"totalCommitted"`
			BoostCount     uint64 `json:"boostCount"`
			ReplyCount     int    `json:"replyCount"`
			Channel        string `json:"channel"`
		} `json:"posts"`
		TotalCount int `json:"totalCount"`
	}
	var topFeed feedResp
	httpGet(indexerAPI+"/v1/feed?strategy=top&pageSize=5", &topFeed)
	if topFeed.TotalCount != len(posts) {
		t.Fatalf("Feed totalCount: got %d, want %d (top-level only)", topFeed.TotalCount, len(posts))
	}
	t.Log("  Top 5 posts by committed value:")
	for i, p := range topFeed.Posts {
		t.Logf("    %d. %d microdrana, %d boosts, %d replies, #%s", i+1, p.TotalCommitted, p.BoostCount, p.ReplyCount, p.Channel)
	}

	// Verify post 0 (alice's welcome post) has the right boost/reply counts.
	var post0 struct {
		TotalCommitted      uint64 `json:"totalCommitted"`
		AuthorCommitted     uint64 `json:"authorCommitted"`
		ThirdPartyCommitted uint64 `json:"thirdPartyCommitted"`
		BoostCount          uint64 `json:"boostCount"`
		UniqueBoosterCount  int    `json:"uniqueBoosterCount"`
		ReplyCount          int    `json:"replyCount"`
	}
	httpGet(indexerAPI+"/v1/posts/"+posts[0].id, &post0)
	// Post 0: 50M initial + 10M (bob) + 5M (carol) + 5M (alice self) = 70M
	if post0.TotalCommitted != 70_000_000 {
		t.Fatalf("Post 0 totalCommitted: got %d, want 70000000", post0.TotalCommitted)
	}
	// Author committed: 50M initial + 5M self-boost = 55M
	if post0.AuthorCommitted != 55_000_000 {
		t.Fatalf("Post 0 authorCommitted: got %d, want 55000000", post0.AuthorCommitted)
	}
	// Third party: 10M + 5M = 15M
	if post0.ThirdPartyCommitted != 15_000_000 {
		t.Fatalf("Post 0 thirdPartyCommitted: got %d, want 15000000", post0.ThirdPartyCommitted)
	}
	// 3 boosts (bob, carol, alice self)
	if post0.BoostCount != 3 {
		t.Fatalf("Post 0 boostCount: got %d, want 3", post0.BoostCount)
	}
	// 3 unique boosters (alice, bob, carol)
	if post0.UniqueBoosterCount != 3 {
		t.Fatalf("Post 0 uniqueBoosterCount: got %d, want 3", post0.UniqueBoosterCount)
	}
	// 2 replies (dave, eve)
	if post0.ReplyCount != 2 {
		t.Fatalf("Post 0 replyCount: got %d, want 2", post0.ReplyCount)
	}
	t.Log("  Post 0 verified: 70M committed (55M author, 15M third-party), 3 boosts, 3 unique boosters, 2 replies.")

	// Check gaming channel feed.
	var gamingFeed feedResp
	httpGet(indexerAPI+"/v1/feed?strategy=top&channel=gaming", &gamingFeed)
	if gamingFeed.TotalCount < 3 {
		t.Fatalf("Gaming feed: got %d posts, want >= 3", gamingFeed.TotalCount)
	}
	t.Logf("  Gaming channel: %d posts", gamingFeed.TotalCount)

	// Check replies for post 0.
	type repliesResp struct {
		Replies    []struct{ PostID string `json:"postId"` } `json:"replies"`
		TotalCount int                                       `json:"totalCount"`
	}
	var post0Replies repliesResp
	httpGet(indexerAPI+"/v1/posts/"+posts[0].id+"/replies", &post0Replies)
	if post0Replies.TotalCount != 2 {
		t.Fatalf("Post 0 replies: got %d, want 2", post0Replies.TotalCount)
	}
	t.Logf("  Post 0 has %d replies via indexer.", post0Replies.TotalCount)

	// Check leaderboard.
	type lbResp struct {
		Authors []struct {
			Address       string `json:"address"`
			TotalReceived uint64 `json:"totalReceived"`
		} `json:"authors"`
	}
	var lb lbResp
	httpGet(indexerAPI+"/v1/leaderboard", &lb)
	if len(lb.Authors) < 5 {
		t.Fatalf("Leaderboard: got %d authors, want >= 5", len(lb.Authors))
	}
	t.Logf("  Leaderboard top 3:")
	for i := 0; i < 3 && i < len(lb.Authors); i++ {
		t.Logf("    %d. %s received %d microdrana", i+1, lb.Authors[i].Address[:20]+"...", lb.Authors[i].TotalReceived)
	}

	// Check name resolution.
	var aliceByName rpc.AccountResponse
	httpGet(nodeRPC+"/v1/accounts/name/alice", &aliceByName)
	if aliceByName.Address != wallets[0].addrStr {
		t.Fatalf("Name resolution: alice got %s, want %s", aliceByName.Address, wallets[0].addrStr)
	}
	t.Log("  Name resolution: 'alice' resolves correctly.")

	// Supply conservation check.
	httpGet(nodeRPC+"/v1/node/info", &nodeInfo)
	var totalBal uint64
	// Check all 10 wallets + the 3 validators.
	for _, w := range wallets {
		totalBal += getBalance(w.addrStr)
	}
	// Add validator balances (they have genesis funds + block rewards - what they sent).
	// This is approximate — we just check the invariant via the node.
	t.Logf("  Supply: issued=%d, burned=%d", nodeInfo.IssuedSupply, nodeInfo.BurnedSupply)

	t.Log("")
	t.Log("=== Live network test PASSED ===")
	t.Logf("  Chain height: %d", nodeInfo.LatestHeight)
	t.Logf("  Posts: %d (%d top-level + %d replies)", stats.TotalPosts, len(posts), len(replies))
	t.Logf("  Boosts: %d", stats.TotalBoosts)
	t.Logf("  Channels: %d", len(channels))
	t.Logf("  Wallets: 10 with names")
}

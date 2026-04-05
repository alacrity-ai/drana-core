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

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/node"
	"github.com/drana-chain/drana/internal/rpc"
)

func TestErgonomicsEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ergonomics integration test in short mode")
	}

	// --- Setup: 3 validators with explicit stake ---
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

	userPub, _, _ := crypto.GenerateKeyPair()
	userAddr := crypto.AddressFromPublicKey(userPub)

	tmpDir := t.TempDir()
	genesisPath := filepath.Join(tmpDir, "genesis.json")

	genesisData := map[string]interface{}{
		"chainId": "drana-ergo-test", "genesisTime": time.Now().Unix(),
		"maxPostLength": 280, "maxPostBytes": 1024,
		"minPostCommitment": 1000000, "minBoostCommitment": 100000,
		"maxTxPerBlock": 100, "maxBlockBytes": 1048576,
		"blockIntervalSec": 2, "blockReward": 10000000,
		"minStake": 1000000000, "epochLength": 30, "unbondingPeriod": 5, "slashFractionDoubleSign": 5,
		"accounts": []map[string]interface{}{
			{"address": userAddr.String(), "balance": 5000000000},
			{"address": vals[0].addr.String(), "balance": 2000000000},
			{"address": vals[1].addr.String(), "balance": 2000000000},
			{"address": vals[2].addr.String(), "balance": 2000000000},
		},
		"validators": []map[string]interface{}{
			{"address": vals[0].addr.String(), "pubKey": hex.EncodeToString(vals[0].pub[:]), "name": "val-0", "stake": 1000000000},
			{"address": vals[1].addr.String(), "pubKey": hex.EncodeToString(vals[1].pub[:]), "name": "val-1", "stake": 1000000000},
			{"address": vals[2].addr.String(), "pubKey": hex.EncodeToString(vals[2].pub[:]), "name": "val-2", "stake": 1000000000},
		},
	}
	gdata, _ := json.MarshalIndent(genesisData, "", "  ")
	os.WriteFile(genesisPath, gdata, 0644)

	ports := []string{"127.0.0.1:27001", "127.0.0.1:27002", "127.0.0.1:27003"}
	rpcPorts := []string{"127.0.0.1:27051", "127.0.0.1:27052", "127.0.0.1:27053"}
	peerEndpoints := map[string]string{"val-0": ports[0], "val-1": ports[1], "val-2": ports[2]}

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
	waitForHeight(t, rpcBase, 3, 30*time.Second)

	// --- Test: NodeInfo includes epoch data ---
	t.Log("Testing epoch info in /v1/node/info")
	var info rpc.NodeInfoResponse
	mustGet(t, rpcBase+"/v1/node/info", &info)
	if info.EpochLength != 30 {
		t.Fatalf("epochLength: %d, want 30", info.EpochLength)
	}
	t.Logf("  CurrentEpoch=%d, BlocksUntilNext=%d", info.CurrentEpoch, info.BlocksUntilNextEpoch)

	// --- Test: Validators include stake ---
	t.Log("Testing /v1/network/validators includes stake")
	var valList []rpc.ValidatorResponse
	mustGet(t, rpcBase+"/v1/network/validators", &valList)
	if len(valList) != 3 {
		t.Fatalf("validator count: %d", len(valList))
	}
	for _, v := range valList {
		if v.StakedBalance != 1000000000 {
			t.Fatalf("validator %s stake: %d, want 1000000000", v.Address, v.StakedBalance)
		}
	}
	t.Logf("  All 3 validators have 1000 DRANA staked")

	// --- Test: Account shows staked balance ---
	t.Log("Testing /v1/accounts/{addr} shows staked balance")
	var acct rpc.AccountResponse
	mustGet(t, rpcBase+"/v1/accounts/"+vals[0].addr.String(), &acct)
	if acct.StakedBalance != 1000000000 {
		t.Fatalf("account staked: %d, want 1000000000", acct.StakedBalance)
	}
	// Balance should be >= genesis - stake (1B) due to block rewards earned.
	if acct.Balance < 1000000000 {
		t.Fatalf("account balance: %d, want >= 1000000000", acct.Balance)
	}
	t.Logf("  Validator balance: %d (includes block rewards)", acct.Balance)

	// --- Test: Unbonding endpoint (empty) ---
	t.Log("Testing /v1/accounts/{addr}/unbonding (empty)")
	var unbonding rpc.UnbondingResponse
	mustGet(t, rpcBase+"/v1/accounts/"+vals[0].addr.String()+"/unbonding", &unbonding)
	if len(unbonding.Entries) != 0 {
		t.Fatalf("expected 0 unbonding entries, got %d", len(unbonding.Entries))
	}
	if unbonding.Total != 0 {
		t.Fatalf("expected 0 total unbonding, got %d", unbonding.Total)
	}

	// --- Test: ValidatorCount reflects active set ---
	if info.ValidatorCount != 3 {
		t.Fatalf("validatorCount: %d, want 3", info.ValidatorCount)
	}

	// --- Shutdown ---
	for i, c := range cancels {
		c()
		nodes[i].Stop()
	}
	t.Log("Ergonomics test passed.")
}

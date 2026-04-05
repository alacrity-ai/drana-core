package genesis

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
)

// helper: generate a keypair and return address string + pubkey hex
func genIdentity(t *testing.T) (string, string, crypto.PrivateKey) {
	t.Helper()
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := crypto.AddressFromPublicKey(pub)
	return addr.String(), hex.EncodeToString(pub[:]), priv
}

func writeGenesisFile(t *testing.T, content interface{}) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoadGenesisValid(t *testing.T) {
	addr1, pub1, _ := genIdentity(t)
	addr2, _, _ := genIdentity(t)
	addr3, pub3, _ := genIdentity(t)

	genesis := map[string]interface{}{
		"chainId":           "drana-testnet-1",
		"genesisTime":       1700000000,
		"maxPostLength":     280,
		"maxPostBytes":      1024,
		"minPostCommitment": 1000000,
		"minBoostCommitment": 100000,
		"maxTxPerBlock":     100,
		"maxBlockBytes":     1048576,
		"blockIntervalSec":  120,
		"blockReward":       10000000,
		"accounts": []map[string]interface{}{
			{"address": addr1, "balance": 1000000000000},
			{"address": addr2, "balance": 1000000000000},
		},
		"validators": []map[string]interface{}{
			{"address": addr1, "pubKey": pub1, "name": "val-1"},
			{"address": addr3, "pubKey": pub3, "name": "val-2"},
		},
	}

	path := writeGenesisFile(t, genesis)
	cfg, err := LoadGenesis(path)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}

	if cfg.ChainID != "drana-testnet-1" {
		t.Fatalf("chainId: %q", cfg.ChainID)
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("accounts: %d", len(cfg.Accounts))
	}
	if len(cfg.Validators) != 2 {
		t.Fatalf("validators: %d", len(cfg.Validators))
	}
	if cfg.MaxPostLength != 280 {
		t.Fatalf("maxPostLength: %d", cfg.MaxPostLength)
	}
	if cfg.BlockReward != 10000000 {
		t.Fatalf("blockReward: %d", cfg.BlockReward)
	}
	if cfg.MinPostCommitment != 1000000 {
		t.Fatalf("minPostCommitment: %d", cfg.MinPostCommitment)
	}
}

func TestInitializeState(t *testing.T) {
	addr1, _, _ := genIdentity(t)
	addr2, _, _ := genIdentity(t)

	genesis := map[string]interface{}{
		"chainId":           "test",
		"genesisTime":       1700000000,
		"maxPostLength":     280,
		"maxPostBytes":      1024,
		"minPostCommitment": 1000000,
		"minBoostCommitment": 100000,
		"maxTxPerBlock":     100,
		"maxBlockBytes":     1048576,
		"blockIntervalSec":  120,
		"accounts": []map[string]interface{}{
			{"address": addr1, "balance": 500000000},
			{"address": addr2, "balance": 300000000},
		},
		"validators": []map[string]interface{}{},
	}

	path := writeGenesisFile(t, genesis)
	cfg, err := LoadGenesis(path)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}

	ws, err := InitializeState(cfg)
	if err != nil {
		t.Fatalf("InitializeState: %v", err)
	}

	// Verify accounts.
	accts := ws.AllAccounts()
	if len(accts) != 2 {
		t.Fatalf("account count: %d", len(accts))
	}

	var total uint64
	for _, a := range accts {
		total += a.Balance
		if a.Nonce != 0 {
			t.Fatalf("nonce should be 0, got %d", a.Nonce)
		}
	}
	if total != 800000000 {
		t.Fatalf("total supply: %d", total)
	}
	if ws.GetIssuedSupply() != 0 {
		t.Fatalf("initial issued supply: %d", ws.GetIssuedSupply())
	}
	if ws.GetBurnedSupply() != 0 {
		t.Fatalf("initial burned supply: %d", ws.GetBurnedSupply())
	}
	if ws.GetChainHeight() != 0 {
		t.Fatalf("initial chain height: %d", ws.GetChainHeight())
	}
}

func TestLoadGenesisDuplicateAddress(t *testing.T) {
	addr1, _, _ := genIdentity(t)

	genesis := map[string]interface{}{
		"chainId":     "test",
		"genesisTime": 1700000000,
		"accounts": []map[string]interface{}{
			{"address": addr1, "balance": 100},
			{"address": addr1, "balance": 200},
		},
		"validators": []map[string]interface{}{},
	}

	path := writeGenesisFile(t, genesis)
	if _, err := LoadGenesis(path); err == nil {
		t.Fatal("duplicate address should fail")
	}
}

func TestLoadGenesisBadAddress(t *testing.T) {
	genesis := map[string]interface{}{
		"chainId":     "test",
		"genesisTime": 1700000000,
		"accounts": []map[string]interface{}{
			{"address": "drana1badaddress", "balance": 100},
		},
		"validators": []map[string]interface{}{},
	}

	path := writeGenesisFile(t, genesis)
	if _, err := LoadGenesis(path); err == nil {
		t.Fatal("bad address should fail")
	}
}

func TestInitializeStateOverflow(t *testing.T) {
	addr1, _, _ := genIdentity(t)
	addr2, _, _ := genIdentity(t)

	path := writeGenesisFile(t, map[string]interface{}{
		"chainId":     "test",
		"genesisTime": 1700000000,
		"accounts": []map[string]interface{}{
			{"address": addr1, "balance": uint64(1<<64 - 1)},
			{"address": addr2, "balance": uint64(1)},
		},
		"validators": []map[string]interface{}{},
	})
	cfg, err := LoadGenesis(path)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}
	if _, err := InitializeState(cfg); err == nil {
		t.Fatal("overflow supply should fail")
	}
}

// AllAccounts helper for testing
func allAccounts(ws *state.WorldState) int {
	return len(ws.AllAccounts())
}

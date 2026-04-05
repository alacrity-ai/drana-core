package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/types"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "drana-test-kv-"+t.Name())
	os.RemoveAll(dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func makeAddr(t *testing.T) crypto.Address {
	t.Helper()
	pub, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return crypto.AddressFromPublicKey(pub)
}

func TestKVStoreRoundTrip(t *testing.T) {
	dir := tempDir(t)
	kv, err := OpenKVStore(dir)
	if err != nil {
		t.Fatalf("OpenKVStore: %v", err)
	}

	addr1 := makeAddr(t)
	addr2 := makeAddr(t)
	postID := types.DerivePostID(addr1, 1)

	ws := state.NewWorldState()
	ws.SetAccount(&types.Account{Address: addr1, Balance: 1_000_000, Nonce: 5})
	ws.SetAccount(&types.Account{Address: addr2, Balance: 2_000_000, Nonce: 3})
	ws.SetPost(&types.Post{
		PostID:          postID,
		Author:          addr1,
		Text:            "hello world",
		CreatedAtHeight: 10,
		CreatedAtTime:   1700000000,
		TotalStaked:  500_000,
		StakerCount:      2,
	})
	ws.SetBurnedSupply(500_000)
	ws.SetIssuedSupply(1_000_000)
	ws.SetChainHeight(10)

	if err := kv.SaveState(ws); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and load.
	kv2, err := OpenKVStore(dir)
	if err != nil {
		t.Fatalf("OpenKVStore (reopen): %v", err)
	}
	defer kv2.Close()

	loaded, err := kv2.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	acct1, ok := loaded.GetAccount(addr1)
	if !ok {
		t.Fatal("account 1 not found after reload")
	}
	if acct1.Balance != 1_000_000 || acct1.Nonce != 5 {
		t.Fatalf("account 1: balance=%d nonce=%d", acct1.Balance, acct1.Nonce)
	}

	acct2, ok := loaded.GetAccount(addr2)
	if !ok {
		t.Fatal("account 2 not found after reload")
	}
	if acct2.Balance != 2_000_000 || acct2.Nonce != 3 {
		t.Fatalf("account 2: balance=%d nonce=%d", acct2.Balance, acct2.Nonce)
	}

	post, ok := loaded.GetPost(postID)
	if !ok {
		t.Fatal("post not found after reload")
	}
	if post.Text != "hello world" {
		t.Fatalf("post text: %q", post.Text)
	}
	if post.TotalStaked != 500_000 || post.StakerCount != 2 {
		t.Fatalf("post: committed=%d boosts=%d", post.TotalStaked, post.StakerCount)
	}

	if loaded.GetBurnedSupply() != 500_000 {
		t.Fatalf("burned supply: %d", loaded.GetBurnedSupply())
	}
	if loaded.GetIssuedSupply() != 1_000_000 {
		t.Fatalf("issued supply: %d", loaded.GetIssuedSupply())
	}
	if loaded.GetChainHeight() != 10 {
		t.Fatalf("chain height: %d", loaded.GetChainHeight())
	}
}

func TestKVStoreOverwrite(t *testing.T) {
	dir := tempDir(t)
	kv, err := OpenKVStore(dir)
	if err != nil {
		t.Fatalf("OpenKVStore: %v", err)
	}
	defer kv.Close()

	addr := makeAddr(t)
	ws := state.NewWorldState()
	ws.SetAccount(&types.Account{Address: addr, Balance: 100, Nonce: 0})
	ws.SetBurnedSupply(0)

	if err := kv.SaveState(ws); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Update and save again.
	acct, _ := ws.GetAccount(addr)
	acct.Balance = 999
	acct.Nonce = 7
	ws.SetAccount(acct)
	ws.SetBurnedSupply(50)

	if err := kv.SaveState(ws); err != nil {
		t.Fatalf("SaveState (overwrite): %v", err)
	}

	loaded, err := kv.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	got, _ := loaded.GetAccount(addr)
	if got.Balance != 999 || got.Nonce != 7 {
		t.Fatalf("overwrite failed: balance=%d nonce=%d", got.Balance, got.Nonce)
	}
	if loaded.GetBurnedSupply() != 50 {
		t.Fatalf("overwrite burned supply: %d", loaded.GetBurnedSupply())
	}
}

func TestKVStoreEmptyLoad(t *testing.T) {
	dir := tempDir(t)
	kv, err := OpenKVStore(dir)
	if err != nil {
		t.Fatalf("OpenKVStore: %v", err)
	}
	defer kv.Close()

	ws, err := kv.LoadState()
	if err != nil {
		t.Fatalf("LoadState (empty): %v", err)
	}
	if ws.GetBurnedSupply() != 0 {
		t.Fatal("empty state should have zero burned supply")
	}
	if ws.GetIssuedSupply() != 0 {
		t.Fatal("empty state should have zero issued supply")
	}
	if ws.GetChainHeight() != 0 {
		t.Fatal("empty state should have zero chain height")
	}
}

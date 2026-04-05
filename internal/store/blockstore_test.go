package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func tempBlockDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "drana-test-block-"+t.Name())
	os.RemoveAll(dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func makeTestBlock(t *testing.T, height uint64) *types.Block {
	t.Helper()
	_, priv, addr := makeKeypair(t)

	tx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addr,
		Text:   "test post",
		Amount: 1_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, priv)

	txRoot := types.ComputeTxRoot([]*types.Transaction{tx})
	return &types.Block{
		Header: types.BlockHeader{
			Height:    height,
			Timestamp: 1700000000 + int64(height)*120,
			TxRoot:    txRoot,
		},
		Transactions: []*types.Transaction{tx},
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

func TestBlockStoreByHeight(t *testing.T) {
	dir := tempBlockDir(t)
	bs, err := OpenBlockStore(dir)
	if err != nil {
		t.Fatalf("OpenBlockStore: %v", err)
	}
	defer bs.Close()

	block := makeTestBlock(t, 1)
	if err := bs.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock: %v", err)
	}

	loaded, err := bs.GetBlockByHeight(1)
	if err != nil {
		t.Fatalf("GetBlockByHeight: %v", err)
	}
	if loaded.Header.Height != 1 {
		t.Fatalf("height: got %d, want 1", loaded.Header.Height)
	}
	if len(loaded.Transactions) != 1 {
		t.Fatalf("tx count: got %d, want 1", len(loaded.Transactions))
	}
}

func TestBlockStoreByHash(t *testing.T) {
	dir := tempBlockDir(t)
	bs, err := OpenBlockStore(dir)
	if err != nil {
		t.Fatalf("OpenBlockStore: %v", err)
	}
	defer bs.Close()

	block := makeTestBlock(t, 1)
	if err := bs.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock: %v", err)
	}

	hash := block.Header.Hash()
	loaded, err := bs.GetBlockByHash(hash)
	if err != nil {
		t.Fatalf("GetBlockByHash: %v", err)
	}
	if loaded.Header.Height != 1 {
		t.Fatalf("height: got %d, want 1", loaded.Header.Height)
	}
}

func TestBlockStoreLatest(t *testing.T) {
	dir := tempBlockDir(t)
	bs, err := OpenBlockStore(dir)
	if err != nil {
		t.Fatalf("OpenBlockStore: %v", err)
	}
	defer bs.Close()

	b1 := makeTestBlock(t, 1)
	b2 := makeTestBlock(t, 2)
	b3 := makeTestBlock(t, 3)

	for _, b := range []*types.Block{b1, b2, b3} {
		if err := bs.SaveBlock(b); err != nil {
			t.Fatalf("SaveBlock: %v", err)
		}
	}

	latest, err := bs.GetLatestBlock()
	if err != nil {
		t.Fatalf("GetLatestBlock: %v", err)
	}
	if latest.Header.Height != 3 {
		t.Fatalf("latest height: got %d, want 3", latest.Header.Height)
	}
}

func TestBlockStoreGetTransaction(t *testing.T) {
	dir := tempBlockDir(t)
	bs, err := OpenBlockStore(dir)
	if err != nil {
		t.Fatalf("OpenBlockStore: %v", err)
	}
	defer bs.Close()

	block := makeTestBlock(t, 1)
	if err := bs.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock: %v", err)
	}

	txHash := block.Transactions[0].Hash()
	tx, height, err := bs.GetTransaction(txHash)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if height != 1 {
		t.Fatalf("tx block height: got %d, want 1", height)
	}
	if tx.Amount != 1_000_000 {
		t.Fatalf("tx amount: got %d, want 1000000", tx.Amount)
	}
}

func TestBlockStoreNotFound(t *testing.T) {
	dir := tempBlockDir(t)
	bs, err := OpenBlockStore(dir)
	if err != nil {
		t.Fatalf("OpenBlockStore: %v", err)
	}
	defer bs.Close()

	if _, err := bs.GetBlockByHeight(999); err == nil {
		t.Fatal("should error on nonexistent height")
	}
	var fakeHash [32]byte
	if _, err := bs.GetBlockByHash(fakeHash); err == nil {
		t.Fatal("should error on nonexistent hash")
	}
	if _, err := bs.GetLatestBlock(); err == nil {
		t.Fatal("should error on empty store")
	}
}

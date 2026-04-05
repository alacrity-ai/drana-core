package mempool

import (
	"sync"
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/types"
)

func makeKeypair(t *testing.T) (crypto.PublicKey, crypto.PrivateKey, crypto.Address) {
	t.Helper()
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return pub, priv, crypto.AddressFromPublicKey(pub)
}

func makeSignedTx(t *testing.T, priv crypto.PrivateKey, sender crypto.Address, nonce uint64) *types.Transaction {
	t.Helper()
	_, _, addrB := makeKeypair(t)
	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    sender,
		Recipient: addrB,
		Amount:    100,
		Nonce:     nonce,
	}
	types.SignTransaction(tx, priv)
	return tx
}

func TestMempoolAddAndSize(t *testing.T) {
	m := New(100)
	_, priv, addr := makeKeypair(t)
	tx := makeSignedTx(t, priv, addr, 1)

	if err := m.Add(tx); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if m.Size() != 1 {
		t.Fatalf("size: %d", m.Size())
	}
	if !m.Has(tx.Hash()) {
		t.Fatal("Has should return true")
	}
}

func TestMempoolRejectDuplicate(t *testing.T) {
	m := New(100)
	_, priv, addr := makeKeypair(t)
	tx := makeSignedTx(t, priv, addr, 1)

	m.Add(tx)
	if err := m.Add(tx); err == nil {
		t.Fatal("duplicate should be rejected")
	}
}

func TestMempoolRejectBadSignature(t *testing.T) {
	m := New(100)
	_, priv, addr := makeKeypair(t)
	tx := makeSignedTx(t, priv, addr, 1)
	tx.Signature[0] ^= 0xff

	if err := m.Add(tx); err == nil {
		t.Fatal("bad signature should be rejected")
	}
}

func TestMempoolFull(t *testing.T) {
	m := New(2)
	_, priv, addr := makeKeypair(t)

	tx1 := makeSignedTx(t, priv, addr, 1)
	tx2 := makeSignedTx(t, priv, addr, 2)
	tx3 := makeSignedTx(t, priv, addr, 3)

	m.Add(tx1)
	m.Add(tx2)
	if err := m.Add(tx3); err == nil {
		t.Fatal("full mempool should reject")
	}
}

func TestMempoolRemove(t *testing.T) {
	m := New(100)
	_, priv, addr := makeKeypair(t)
	tx := makeSignedTx(t, priv, addr, 1)
	m.Add(tx)

	m.Remove([][32]byte{tx.Hash()})
	if m.Size() != 0 {
		t.Fatal("should be empty after remove")
	}
}

func TestMempoolFlush(t *testing.T) {
	m := New(100)
	_, priv, addr := makeKeypair(t)
	m.Add(makeSignedTx(t, priv, addr, 1))
	m.Add(makeSignedTx(t, priv, addr, 2))
	m.Flush()
	if m.Size() != 0 {
		t.Fatal("should be empty after flush")
	}
}

func TestMempoolReapForBlock(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)

	ws := state.NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0})

	params := &types.GenesisConfig{
		MaxPostLength:      280,
		MaxPostBytes:       1024,
		MinPostCommitment:  1000,
		MinBoostCommitment: 100,
	}

	m := New(100)
	// Add nonce 1 and 2 in order.
	tx1 := &types.Transaction{
		Type: types.TxTransfer, Sender: addrA, Recipient: addrB,
		Amount: 100, Nonce: 1,
	}
	types.SignTransaction(tx1, privA)

	tx2 := &types.Transaction{
		Type: types.TxTransfer, Sender: addrA, Recipient: addrB,
		Amount: 200, Nonce: 2,
	}
	types.SignTransaction(tx2, privA)

	m.Add(tx1)
	m.Add(tx2)

	reaped := m.ReapForBlock(ws, params, 10)
	if len(reaped) != 2 {
		t.Fatalf("reaped %d txs, want 2", len(reaped))
	}
	if reaped[0].Nonce != 1 || reaped[1].Nonce != 2 {
		t.Fatal("reaped txs should be nonce-ordered")
	}
}

func TestMempoolReapSkipsNonceGap(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)

	ws := state.NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0})

	params := &types.GenesisConfig{}

	m := New(100)
	// Add nonce 2 only (gap — nonce 1 is missing).
	tx := &types.Transaction{
		Type: types.TxTransfer, Sender: addrA, Recipient: addrB,
		Amount: 100, Nonce: 2,
	}
	types.SignTransaction(tx, privA)
	m.Add(tx)

	reaped := m.ReapForBlock(ws, params, 10)
	if len(reaped) != 0 {
		t.Fatalf("reaped %d txs, want 0 (nonce gap)", len(reaped))
	}
}

func TestMempoolReapSkipsInsufficientBalance(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)

	ws := state.NewWorldState()
	ws.SetAccount(&types.Account{Address: addrA, Balance: 50, Nonce: 0})

	params := &types.GenesisConfig{}

	m := New(100)
	tx := &types.Transaction{
		Type: types.TxTransfer, Sender: addrA, Recipient: addrB,
		Amount: 100, Nonce: 1,
	}
	types.SignTransaction(tx, privA)
	m.Add(tx)

	reaped := m.ReapForBlock(ws, params, 10)
	if len(reaped) != 0 {
		t.Fatalf("reaped %d txs, want 0 (insufficient balance)", len(reaped))
	}
}

func TestMempoolConcurrentAdd(t *testing.T) {
	m := New(1000)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, priv, addr := makeKeypair(t)
			for n := uint64(1); n <= 10; n++ {
				tx := makeSignedTx(t, priv, addr, n)
				m.Add(tx) // ignore errors
			}
		}()
	}
	wg.Wait()
	if m.Size() == 0 {
		t.Fatal("expected some txs after concurrent adds")
	}
}

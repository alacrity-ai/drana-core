package state

import (
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func makeAddr(t *testing.T) crypto.Address {
	t.Helper()
	pub, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return crypto.AddressFromPublicKey(pub)
}

func TestWorldStateAccountRoundTrip(t *testing.T) {
	ws := NewWorldState()
	addr := makeAddr(t)

	if _, ok := ws.GetAccount(addr); ok {
		t.Fatal("account should not exist yet")
	}

	acct := &types.Account{Address: addr, Balance: 1000, Nonce: 5}
	ws.SetAccount(acct)

	got, ok := ws.GetAccount(addr)
	if !ok {
		t.Fatal("account should exist")
	}
	if got.Balance != 1000 || got.Nonce != 5 {
		t.Fatalf("got balance=%d nonce=%d", got.Balance, got.Nonce)
	}
}

func TestWorldStatePostRoundTrip(t *testing.T) {
	ws := NewWorldState()
	addr := makeAddr(t)
	postID := types.DerivePostID(addr, 1)

	if _, ok := ws.GetPost(postID); ok {
		t.Fatal("post should not exist yet")
	}

	post := &types.Post{PostID: postID, Author: addr, Text: "hello", TotalStaked: 500}
	ws.SetPost(post)

	got, ok := ws.GetPost(postID)
	if !ok {
		t.Fatal("post should exist")
	}
	if got.Text != "hello" || got.TotalStaked != 500 {
		t.Fatalf("got text=%q committed=%d", got.Text, got.TotalStaked)
	}
}

func TestWorldStateBurnedSupply(t *testing.T) {
	ws := NewWorldState()
	if ws.GetBurnedSupply() != 0 {
		t.Fatal("initial burned supply should be 0")
	}
	ws.SetBurnedSupply(999)
	if ws.GetBurnedSupply() != 999 {
		t.Fatal("burned supply mismatch")
	}
}

func TestWorldStateCloneIsIndependent(t *testing.T) {
	ws := NewWorldState()
	addr := makeAddr(t)
	acct := &types.Account{Address: addr, Balance: 1000, Nonce: 0}
	ws.SetAccount(acct)
	ws.SetBurnedSupply(100)

	clone := ws.Clone()

	// Mutate clone.
	cloneAcct, _ := clone.GetAccount(addr)
	cloneAcct.Balance = 500
	clone.SetAccount(cloneAcct)
	clone.SetBurnedSupply(200)

	// Original should be unchanged.
	origAcct, _ := ws.GetAccount(addr)
	if origAcct.Balance != 1000 {
		t.Fatalf("original balance changed: %d", origAcct.Balance)
	}
	if ws.GetBurnedSupply() != 100 {
		t.Fatalf("original burned supply changed: %d", ws.GetBurnedSupply())
	}
}

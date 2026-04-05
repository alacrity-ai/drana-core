package state

import (
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func TestStateRootDeterministic(t *testing.T) {
	ws := NewWorldState()
	addr := makeAddr(t)
	ws.SetAccount(&types.Account{Address: addr, Balance: 1000, Nonce: 1})
	ws.SetBurnedSupply(500)
	ws.SetChainHeight(10)

	r1 := ComputeStateRoot(ws)
	r2 := ComputeStateRoot(ws)
	if r1 != r2 {
		t.Fatal("state root is not deterministic")
	}
}

func TestStateRootDifferentStates(t *testing.T) {
	addr := makeAddr(t)

	ws1 := NewWorldState()
	ws1.SetAccount(&types.Account{Address: addr, Balance: 1000, Nonce: 1})

	ws2 := NewWorldState()
	ws2.SetAccount(&types.Account{Address: addr, Balance: 2000, Nonce: 1})

	r1 := ComputeStateRoot(ws1)
	r2 := ComputeStateRoot(ws2)
	if r1 == r2 {
		t.Fatal("different states should produce different roots")
	}
}

func TestStateRootInsertionOrderIndependent(t *testing.T) {
	addr1 := makeAddr(t)
	addr2 := makeAddr(t)

	// Insert in order 1, 2
	ws1 := NewWorldState()
	ws1.SetAccount(&types.Account{Address: addr1, Balance: 100, Nonce: 0})
	ws1.SetAccount(&types.Account{Address: addr2, Balance: 200, Nonce: 0})

	// Insert in order 2, 1
	ws2 := NewWorldState()
	ws2.SetAccount(&types.Account{Address: addr2, Balance: 200, Nonce: 0})
	ws2.SetAccount(&types.Account{Address: addr1, Balance: 100, Nonce: 0})

	r1 := ComputeStateRoot(ws1)
	r2 := ComputeStateRoot(ws2)
	if r1 != r2 {
		t.Fatal("insertion order should not affect state root")
	}
}

func TestStateRootWithPosts(t *testing.T) {
	addr := makeAddr(t)
	ws := NewWorldState()

	pid1 := types.DerivePostID(addr, 1)
	pid2 := types.DerivePostID(addr, 2)

	// Insert posts in order 2, 1
	ws.SetPost(&types.Post{PostID: pid2, Author: addr, Text: "two", TotalStaked: 200})
	ws.SetPost(&types.Post{PostID: pid1, Author: addr, Text: "one", TotalStaked: 100})

	r1 := ComputeStateRoot(ws)

	// Fresh state, insert in order 1, 2
	ws2 := NewWorldState()
	ws2.SetPost(&types.Post{PostID: pid1, Author: addr, Text: "one", TotalStaked: 100})
	ws2.SetPost(&types.Post{PostID: pid2, Author: addr, Text: "two", TotalStaked: 200})

	r2 := ComputeStateRoot(ws2)
	if r1 != r2 {
		t.Fatal("post insertion order should not affect state root")
	}
}

func makeAddr2(t *testing.T) crypto.Address {
	return makeAddr(t)
}

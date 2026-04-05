package types

import (
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
)

func generateTestKeypair(t *testing.T) (crypto.PublicKey, crypto.PrivateKey, crypto.Address) {
	t.Helper()
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := crypto.AddressFromPublicKey(pub)
	return pub, priv, addr
}

func TestSignableBytesIsDeterministic(t *testing.T) {
	_, _, addr := generateTestKeypair(t)
	tx := &Transaction{
		Type:   TxTransfer,
		Sender: addr,
		Amount: 1000,
		Nonce:  1,
	}
	b1 := tx.SignableBytes()
	b2 := tx.SignableBytes()
	if len(b1) != len(b2) {
		t.Fatal("SignableBytes length mismatch")
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("SignableBytes differ at byte %d", i)
		}
	}
}

func TestTransactionSignAndVerify(t *testing.T) {
	pubA, privA, addrA := generateTestKeypair(t)
	_, _, addrB := generateTestKeypair(t)

	tx := &Transaction{
		Type:      TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    500,
		Nonce:     1,
	}
	SignTransaction(tx, privA)

	if tx.PubKey != pubA {
		t.Fatal("SignTransaction did not set PubKey correctly")
	}
	if !crypto.Verify(tx.PubKey, tx.SignableBytes(), tx.Signature) {
		t.Fatal("signed transaction failed verification")
	}
}

func TestTransactionSignAllTypes(t *testing.T) {
	_, privA, addrA := generateTestKeypair(t)
	_, _, addrB := generateTestKeypair(t)

	tests := []struct {
		name string
		tx   *Transaction
	}{
		{
			name: "Transfer",
			tx: &Transaction{
				Type:      TxTransfer,
				Sender:    addrA,
				Recipient: addrB,
				Amount:    100,
				Nonce:     1,
			},
		},
		{
			name: "CreatePost",
			tx: &Transaction{
				Type:   TxCreatePost,
				Sender: addrA,
				Text:   "Hello DRANA",
				Amount: 1000000,
				Nonce:  2,
			},
		},
		{
			name: "BoostPost",
			tx: &Transaction{
				Type:   TxBoostPost,
				Sender: addrA,
				PostID: DerivePostID(addrA, 2),
				Amount: 500000,
				Nonce:  3,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SignTransaction(tc.tx, privA)
			if !crypto.Verify(tc.tx.PubKey, tc.tx.SignableBytes(), tc.tx.Signature) {
				t.Fatalf("%s: signed tx failed verification", tc.name)
			}
		})
	}
}

func TestTransactionHashDeterministic(t *testing.T) {
	_, privA, addrA := generateTestKeypair(t)
	tx := &Transaction{
		Type:   TxCreatePost,
		Sender: addrA,
		Text:   "test",
		Amount: 1000000,
		Nonce:  1,
	}
	SignTransaction(tx, privA)
	h1 := tx.Hash()
	h2 := tx.Hash()
	if h1 != h2 {
		t.Fatal("Transaction.Hash is not deterministic")
	}
}

func TestPostIDDeterministic(t *testing.T) {
	_, _, addr := generateTestKeypair(t)
	id1 := DerivePostID(addr, 5)
	id2 := DerivePostID(addr, 5)
	if id1 != id2 {
		t.Fatal("DerivePostID is not deterministic")
	}
	// Different nonce produces different ID
	id3 := DerivePostID(addr, 6)
	if id1 == id3 {
		t.Fatal("different nonces should produce different PostIDs")
	}
}

func TestBlockHeaderHashDeterministic(t *testing.T) {
	h := &BlockHeader{
		Height:    1,
		Timestamp: 1700000000,
	}
	hash1 := h.Hash()
	hash2 := h.Hash()
	if hash1 != hash2 {
		t.Fatal("BlockHeader.Hash is not deterministic")
	}
}

func TestComputeTxRoot(t *testing.T) {
	_, privA, addrA := generateTestKeypair(t)
	tx1 := &Transaction{Type: TxTransfer, Sender: addrA, Amount: 100, Nonce: 1}
	tx2 := &Transaction{Type: TxTransfer, Sender: addrA, Amount: 200, Nonce: 2}
	SignTransaction(tx1, privA)
	SignTransaction(tx2, privA)

	root1 := ComputeTxRoot([]*Transaction{tx1, tx2})
	root2 := ComputeTxRoot([]*Transaction{tx1, tx2})
	if root1 != root2 {
		t.Fatal("ComputeTxRoot is not deterministic")
	}

	// Different order should produce different root
	root3 := ComputeTxRoot([]*Transaction{tx2, tx1})
	if root1 == root3 {
		t.Fatal("different tx order should produce different root")
	}

	// Empty tx list
	emptyRoot := ComputeTxRoot(nil)
	var zero [32]byte
	if emptyRoot == zero {
		t.Fatal("empty tx root should not be zero (it hashes the count)")
	}
}

func TestBlockVoteSignAndVerify(t *testing.T) {
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := crypto.AddressFromPublicKey(pub)

	vote := &BlockVote{
		Height:    5,
		BlockHash: [32]byte{1, 2, 3},
		VoterAddr: addr,
	}
	SignBlockVote(vote, priv)

	if !VerifyBlockVote(vote) {
		t.Fatal("valid vote rejected")
	}
}

func TestBlockVoteRejectsWrongSig(t *testing.T) {
	_, priv, _ := generateTestKeypair(t)
	_, _, addr2 := generateTestKeypair(t)

	vote := &BlockVote{
		Height:    1,
		BlockHash: [32]byte{1},
		VoterAddr: addr2, // wrong address for this key
	}
	SignBlockVote(vote, priv)
	// Signature is valid for priv's pubkey, but VoterAddr doesn't match
	// Actually SignBlockVote sets VoterPubKey from priv, so addr won't match addr2
	if VerifyBlockVote(vote) {
		t.Fatal("vote with mismatched address should fail")
	}
}

func TestBlockVoteRejectsCorruptedSig(t *testing.T) {
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	addr := crypto.AddressFromPublicKey(pub)

	vote := &BlockVote{
		Height:    1,
		BlockHash: [32]byte{1},
		VoterAddr: addr,
	}
	SignBlockVote(vote, priv)
	vote.Signature[0] ^= 0xff

	if VerifyBlockVote(vote) {
		t.Fatal("corrupted vote should fail")
	}
}

func TestBlockHeaderHashUnchangedByQC(t *testing.T) {
	h := &BlockHeader{Height: 1, Timestamp: 1700000000}
	hash1 := h.Hash()

	// Adding a QC to the block should not change the header hash.
	block := &Block{Header: *h, QC: &QuorumCertificate{Height: 1}}
	hash2 := block.Header.Hash()

	if hash1 != hash2 {
		t.Fatal("QC should not affect header hash")
	}
}

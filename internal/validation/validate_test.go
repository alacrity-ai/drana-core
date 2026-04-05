package validation

import (
	"testing"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

// mockStateReader is a simple in-memory StateReader for testing.
type mockStateReader struct {
	accounts map[crypto.Address]*types.Account
	posts    map[types.PostID]*types.Post
}

func newMockStateReader() *mockStateReader {
	return &mockStateReader{
		accounts: make(map[crypto.Address]*types.Account),
		posts:    make(map[types.PostID]*types.Post),
	}
}

func (m *mockStateReader) GetAccount(addr crypto.Address) (*types.Account, bool) {
	a, ok := m.accounts[addr]
	return a, ok
}

func (m *mockStateReader) GetPost(id types.PostID) (*types.Post, bool) {
	p, ok := m.posts[id]
	return p, ok
}

func (m *mockStateReader) GetPostStake(postID types.PostID, staker crypto.Address) (*types.PostStake, bool) {
	return nil, false
}

func (m *mockStateReader) GetAccountByName(name string) (*types.Account, bool) {
	for _, a := range m.accounts {
		if a.Name == name {
			return a, true
		}
	}
	return nil, false
}

func defaultParams() *types.GenesisConfig {
	return &types.GenesisConfig{
		MaxPostLength:      280,
		MaxPostBytes:       1024,
		MinPostCommitment:  1_000_000,
		MinBoostCommitment: 100_000,
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

// --- Transfer tests ---

func TestValidTransfer(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    1_000_000,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err != nil {
		t.Fatalf("valid transfer rejected: %v", err)
	}
}

func TestTransferZeroAmount(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    0,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("zero amount transfer should fail")
	}
}

func TestTransferSelfSend(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrA,
		Amount:    100,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("self-send should fail")
	}
}

func TestTransferBadSignature(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)
	tx.Signature[0] ^= 0xff // corrupt

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("bad signature should fail")
	}
}

func TestTransferWrongPubKey(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	pubB, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)
	tx.PubKey = pubB // wrong pubkey for sender address

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("wrong pubkey should fail")
	}
}

func TestTransferWrongNonce(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 5}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100,
		Nonce:     3, // should be 6
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("wrong nonce should fail")
	}
}

func TestTransferInsufficientBalance(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 50, Nonce: 0}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("insufficient balance should fail")
	}
}

func TestTransferSenderDoesNotExist(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	// no account for addrA

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    addrA,
		Recipient: addrB,
		Amount:    100,
		Nonce:     1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("nonexistent sender should fail")
	}
}

// --- CreatePost tests ---

func TestValidCreatePost(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "The empire of relevance belongs to the highest bidder.",
		Amount: 1_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err != nil {
		t.Fatalf("valid CreatePost rejected: %v", err)
	}
}

func TestCreatePostBelowMinCommitment(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "too cheap",
		Amount: 100, // below 1_000_000 min
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("below minimum commitment should fail")
	}
}

func TestCreatePostTextTooLong(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	longText := make([]byte, 281)
	for i := range longText {
		longText[i] = 'a'
	}
	tx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   string(longText),
		Amount: 1_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("text over 280 code points should fail")
	}
}

func TestCreatePostEmptyText(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:   types.TxCreatePost,
		Sender: addrA,
		Text:   "",
		Amount: 1_000_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("empty text should fail")
	}
}

// --- BoostPost tests ---

func TestValidBoostPost(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	postID := types.DerivePostID(addrA, 1)
	sr.posts[postID] = &types.Post{PostID: postID, Author: addrA}

	tx := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrA,
		PostID: postID,
		Amount: 100_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err != nil {
		t.Fatalf("valid BoostPost rejected: %v", err)
	}
}

func TestBoostPostBelowMinimum(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	postID := types.DerivePostID(addrA, 1)
	sr.posts[postID] = &types.Post{PostID: postID, Author: addrA}

	tx := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrA,
		PostID: postID,
		Amount: 10, // below 100_000 min
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("below minimum boost should fail")
	}
}

func TestBoostPostDoesNotExist(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	fakePostID := types.DerivePostID(addrA, 999)
	tx := &types.Transaction{
		Type:   types.TxBoostPost,
		Sender: addrA,
		PostID: fakePostID,
		Amount: 100_000,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("boost to nonexistent post should fail")
	}
}

// --- RegisterName tests ---

func TestValidRegisterName(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:   types.TxRegisterName,
		Sender: addrA,
		Text:   "alice",
		Amount: 0,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err != nil {
		t.Fatalf("valid RegisterName rejected: %v", err)
	}
}

func TestRegisterNameAlreadyHasName(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0, Name: "existing"}

	tx := &types.Transaction{
		Type: types.TxRegisterName, Sender: addrA, Text: "newname", Amount: 0, Nonce: 1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("account with existing name should fail")
	}
}

func TestRegisterNameAlreadyTaken(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	_, _, addrB := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0}
	sr.accounts[addrB] = &types.Account{Address: addrB, Balance: 1_000_000, Nonce: 0, Name: "alice"}

	tx := &types.Transaction{
		Type: types.TxRegisterName, Sender: addrA, Text: "alice", Amount: 0, Nonce: 1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("taken name should fail")
	}
}

func TestRegisterNameNonZeroAmount(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type: types.TxRegisterName, Sender: addrA, Text: "alice", Amount: 100, Nonce: 1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("non-zero amount should fail")
	}
}

func TestRegisterNameInvalidName(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 1_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type: types.TxRegisterName, Sender: addrA, Text: "A!", Amount: 0, Nonce: 1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("invalid name should fail")
	}
}

func TestUnknownTxType(t *testing.T) {
	_, privA, addrA := makeKeypair(t)
	sr := newMockStateReader()
	sr.accounts[addrA] = &types.Account{Address: addrA, Balance: 10_000_000, Nonce: 0}

	tx := &types.Transaction{
		Type:   99,
		Sender: addrA,
		Amount: 100,
		Nonce:  1,
	}
	types.SignTransaction(tx, privA)

	if err := ValidateTransaction(tx, sr, defaultParams()); err == nil {
		t.Fatal("unknown tx type should fail")
	}
}

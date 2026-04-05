package types

import (
	"github.com/drana-chain/drana/internal/crypto"
)

// TxType identifies the transaction variant.
type TxType uint8

const (
	TxTransfer     TxType = 1
	TxCreatePost   TxType = 2
	TxBoostPost    TxType = 3
	TxRegisterName TxType = 4
	TxStake        TxType = 5
	TxUnstake      TxType = 6
	TxUnstakePost  TxType = 7
)

// Transaction is the universal transaction envelope for all three tx types.
type Transaction struct {
	Type      TxType
	Sender    crypto.Address
	Recipient crypto.Address   // Transfer only
	PostID    PostID           // BoostPost: target; CreatePost: parent (reply)
	Text      string           // CreatePost, RegisterName
	Channel   string           // CreatePost only
	Amount    uint64           // microdrana
	Nonce     uint64
	Signature []byte
	PubKey    crypto.PublicKey
}

// SignableBytes returns the canonical byte representation of all fields
// except Signature. This is the message that gets signed.
func (tx *Transaction) SignableBytes() []byte {
	hw := NewHashWriter()
	hw.WriteUint32(uint32(tx.Type))
	hw.WriteBytes(tx.Sender[:])
	hw.WriteBytes(tx.Recipient[:])
	hw.WriteBytes(tx.PostID[:])
	hw.WriteString(tx.Text)
	hw.WriteString(tx.Channel)
	hw.WriteUint64(tx.Amount)
	hw.WriteUint64(tx.Nonce)
	hw.WriteBytes(tx.PubKey[:])
	return append([]byte(nil), hw.buf...)
}

// Hash returns the SHA-256 hash of the transaction's signable bytes.
func (tx *Transaction) Hash() [32]byte {
	hw := NewHashWriter()
	// Include signable bytes and signature to make the hash unique per signed tx.
	hw.WriteBytes(tx.SignableBytes())
	hw.WriteBytes(tx.Signature)
	return hw.Sum256()
}

// SignTransaction signs a transaction with the given private key,
// setting both the Signature and PubKey fields.
func SignTransaction(tx *Transaction, privKey crypto.PrivateKey) {
	// Derive public key from private key (Ed25519: bytes 32..64).
	copy(tx.PubKey[:], privKey[32:])
	tx.Signature = crypto.Sign(privKey, tx.SignableBytes())
}

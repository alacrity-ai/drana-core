package types

import (
	"github.com/drana-chain/drana/internal/crypto"
)

// BlockHeader contains the metadata for a block.
type BlockHeader struct {
	Height       uint64
	PrevHash     [32]byte
	ProposerAddr crypto.Address
	Timestamp    int64 // unix seconds
	StateRoot    [32]byte
	TxRoot       [32]byte
}

// Hash returns the SHA-256 hash of the deterministically serialized header.
func (h *BlockHeader) Hash() [32]byte {
	hw := NewHashWriter()
	hw.WriteUint64(h.Height)
	hw.WriteBytes(h.PrevHash[:])
	hw.WriteBytes(h.ProposerAddr[:])
	hw.WriteInt64(h.Timestamp)
	hw.WriteBytes(h.StateRoot[:])
	hw.WriteBytes(h.TxRoot[:])
	return hw.Sum256()
}

// Block is a complete block: header plus ordered transactions.
// QC is attached after finalization and is NOT part of the header hash.
type Block struct {
	Header       BlockHeader
	Transactions []*Transaction
	Evidence     []SlashEvidence    `json:"evidence,omitempty"`
	QC           *QuorumCertificate `json:"qc,omitempty"`
}

// BlockVote is a validator's signed approval of a proposed block.
type BlockVote struct {
	Height      uint64
	BlockHash   [32]byte
	VoterAddr   crypto.Address
	VoterPubKey crypto.PublicKey
	Signature   []byte
}

// SignableBytes returns the canonical bytes that get signed: height || block_hash.
func (v *BlockVote) SignableBytes() []byte {
	hw := NewHashWriter()
	hw.WriteUint64(v.Height)
	hw.WriteBytes(v.BlockHash[:])
	return append([]byte(nil), hw.buf...)
}

// SignBlockVote signs a vote with the given private key.
func SignBlockVote(vote *BlockVote, privKey crypto.PrivateKey) {
	copy(vote.VoterPubKey[:], privKey[32:])
	vote.Signature = crypto.Sign(privKey, vote.SignableBytes())
}

// VerifyBlockVote checks the signature on a vote.
func VerifyBlockVote(vote *BlockVote) bool {
	if len(vote.Signature) == 0 {
		return false
	}
	derived := crypto.AddressFromPublicKey(vote.VoterPubKey)
	if derived != vote.VoterAddr {
		return false
	}
	return crypto.Verify(vote.VoterPubKey, vote.SignableBytes(), vote.Signature)
}

// QuorumCertificate proves that a block was accepted by >= 2/3 of validators.
type QuorumCertificate struct {
	Height    uint64
	BlockHash [32]byte
	Votes     []BlockVote
}

// ComputeTxRoot computes an ordered hash of transaction hashes.
func ComputeTxRoot(txs []*Transaction) [32]byte {
	hw := NewHashWriter()
	hw.WriteUint64(uint64(len(txs)))
	for _, tx := range txs {
		txHash := tx.Hash()
		hw.WriteBytes(txHash[:])
	}
	return hw.Sum256()
}

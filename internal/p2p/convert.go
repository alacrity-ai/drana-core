package p2p

import (
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
	pb "github.com/drana-chain/drana/internal/proto/pb"
)

// --- Domain -> Protobuf ---

func BlockToProto(b *types.Block) *pb.Block {
	txs := make([]*pb.Transaction, len(b.Transactions))
	for i, tx := range b.Transactions {
		txs[i] = TxToProto(tx)
	}
	pb := &pb.Block{
		Header: &pb.BlockHeader{
			Height:       b.Header.Height,
			PrevHash:     b.Header.PrevHash[:],
			ProposerAddr: b.Header.ProposerAddr[:],
			Timestamp:    b.Header.Timestamp,
			StateRoot:    b.Header.StateRoot[:],
			TxRoot:       b.Header.TxRoot[:],
		},
		Transactions: txs,
	}
	if b.QC != nil {
		pb.Qc = QCToProto(b.QC)
	}
	// Evidence is serialized via JSON for now; protobuf SlashEvidence message
	// will be generated in a future proto regen pass.
	return pb
}

func TxToProto(tx *types.Transaction) *pb.Transaction {
	return &pb.Transaction{
		Type:      uint32(tx.Type),
		Sender:    tx.Sender[:],
		Recipient: tx.Recipient[:],
		PostId:    tx.PostID[:],
		Text:      tx.Text,
		Channel:   tx.Channel,
		Amount:    tx.Amount,
		Nonce:     tx.Nonce,
		Signature: tx.Signature,
		PubKey:    tx.PubKey[:],
	}
}

func VoteToProto(v *types.BlockVote) *pb.BlockVote {
	return &pb.BlockVote{
		Height:       v.Height,
		BlockHash:    v.BlockHash[:],
		VoterAddress: v.VoterAddr[:],
		VoterPubKey:  v.VoterPubKey[:],
		Signature:    v.Signature,
	}
}

func QCToProto(qc *types.QuorumCertificate) *pb.QuorumCertificate {
	votes := make([]*pb.BlockVote, len(qc.Votes))
	for i, v := range qc.Votes {
		votes[i] = VoteToProto(&v)
	}
	return &pb.QuorumCertificate{
		Height:    qc.Height,
		BlockHash: qc.BlockHash[:],
		Votes:     votes,
	}
}

// --- Protobuf -> Domain ---

func BlockFromProto(pb *pb.Block) *types.Block {
	txs := make([]*types.Transaction, len(pb.Transactions))
	for i, ptx := range pb.Transactions {
		txs[i] = TxFromProto(ptx)
	}
	b := &types.Block{
		Header: types.BlockHeader{
			Height:    pb.Header.Height,
			Timestamp: pb.Header.Timestamp,
		},
		Transactions: txs,
	}
	copy(b.Header.PrevHash[:], pb.Header.PrevHash)
	copy(b.Header.ProposerAddr[:], pb.Header.ProposerAddr)
	copy(b.Header.StateRoot[:], pb.Header.StateRoot)
	copy(b.Header.TxRoot[:], pb.Header.TxRoot)
	if pb.Qc != nil {
		b.QC = QCFromProto(pb.Qc)
	}
	return b
}

func TxFromProto(ptx *pb.Transaction) *types.Transaction {
	tx := &types.Transaction{
		Type:      types.TxType(ptx.Type),
		Text:      ptx.Text,
		Channel:   ptx.Channel,
		Amount:    ptx.Amount,
		Nonce:     ptx.Nonce,
		Signature: ptx.Signature,
	}
	copy(tx.Sender[:], ptx.Sender)
	copy(tx.Recipient[:], ptx.Recipient)
	copy(tx.PostID[:], ptx.PostId)
	copy(tx.PubKey[:], ptx.PubKey)
	return tx
}

func VoteFromProto(pv *pb.BlockVote) types.BlockVote {
	v := types.BlockVote{
		Height:    pv.Height,
		Signature: pv.Signature,
	}
	copy(v.BlockHash[:], pv.BlockHash)
	copy(v.VoterAddr[:], pv.VoterAddress)
	copy(v.VoterPubKey[:], pv.VoterPubKey)
	return v
}

func QCFromProto(pqc *pb.QuorumCertificate) *types.QuorumCertificate {
	qc := &types.QuorumCertificate{
		Height: pqc.Height,
	}
	copy(qc.BlockHash[:], pqc.BlockHash)
	for _, pv := range pqc.Votes {
		qc.Votes = append(qc.Votes, VoteFromProto(pv))
	}
	return qc
}

// Helper: copy address bytes into crypto.Address
func bytesToAddress(b []byte) crypto.Address {
	var addr crypto.Address
	copy(addr[:], b)
	return addr
}

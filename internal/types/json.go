package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// JSON marshaling for types with fixed-size byte arrays.

// --- Transaction ---

type transactionJSON struct {
	Type      TxType `json:"type"`
	Sender    string `json:"sender"`
	Recipient string `json:"recipient,omitempty"`
	PostID    string `json:"postId,omitempty"`
	Text      string `json:"text,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Amount    uint64 `json:"amount,string"`
	Nonce     uint64 `json:"nonce,string"`
	Signature string `json:"signature"`
	PubKey    string `json:"pubKey"`
}

func (tx *Transaction) MarshalJSON() ([]byte, error) {
	return json.Marshal(transactionJSON{
		Type:      tx.Type,
		Sender:    hex.EncodeToString(tx.Sender[:]),
		Recipient: hex.EncodeToString(tx.Recipient[:]),
		PostID:    hex.EncodeToString(tx.PostID[:]),
		Text:      tx.Text,
		Channel:   tx.Channel,
		Amount:    tx.Amount,
		Nonce:     tx.Nonce,
		Signature: hex.EncodeToString(tx.Signature),
		PubKey:    hex.EncodeToString(tx.PubKey[:]),
	})
}

func (tx *Transaction) UnmarshalJSON(data []byte) error {
	var j transactionJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	tx.Type = j.Type
	tx.Text = j.Text
	tx.Channel = j.Channel
	tx.Amount = j.Amount
	tx.Nonce = j.Nonce

	if err := decodeHexInto(tx.Sender[:], j.Sender); err != nil {
		return fmt.Errorf("sender: %w", err)
	}
	if err := decodeHexInto(tx.Recipient[:], j.Recipient); err != nil {
		return fmt.Errorf("recipient: %w", err)
	}
	if err := decodeHexInto(tx.PostID[:], j.PostID); err != nil {
		return fmt.Errorf("postId: %w", err)
	}
	if err := decodeHexInto(tx.PubKey[:], j.PubKey); err != nil {
		return fmt.Errorf("pubKey: %w", err)
	}
	sig, err := hex.DecodeString(j.Signature)
	if err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	tx.Signature = sig
	return nil
}

// --- BlockHeader ---

type blockHeaderJSON struct {
	Height       uint64 `json:"height,string"`
	PrevHash     string `json:"prevHash"`
	ProposerAddr string `json:"proposerAddr"`
	Timestamp    int64  `json:"timestamp"`
	StateRoot    string `json:"stateRoot"`
	TxRoot       string `json:"txRoot"`
}

func (h *BlockHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal(blockHeaderJSON{
		Height:       h.Height,
		PrevHash:     hex.EncodeToString(h.PrevHash[:]),
		ProposerAddr: hex.EncodeToString(h.ProposerAddr[:]),
		Timestamp:    h.Timestamp,
		StateRoot:    hex.EncodeToString(h.StateRoot[:]),
		TxRoot:       hex.EncodeToString(h.TxRoot[:]),
	})
}

func (h *BlockHeader) UnmarshalJSON(data []byte) error {
	var j blockHeaderJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	h.Height = j.Height
	h.Timestamp = j.Timestamp
	if err := decodeHexInto(h.PrevHash[:], j.PrevHash); err != nil {
		return fmt.Errorf("prevHash: %w", err)
	}
	if err := decodeHexInto(h.ProposerAddr[:], j.ProposerAddr); err != nil {
		return fmt.Errorf("proposerAddr: %w", err)
	}
	if err := decodeHexInto(h.StateRoot[:], j.StateRoot); err != nil {
		return fmt.Errorf("stateRoot: %w", err)
	}
	if err := decodeHexInto(h.TxRoot[:], j.TxRoot); err != nil {
		return fmt.Errorf("txRoot: %w", err)
	}
	return nil
}

// --- helper ---

func decodeHexInto(dst []byte, s string) error {
	if s == "" {
		// Leave as zero bytes.
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) != len(dst) {
		return fmt.Errorf("expected %d bytes, got %d", len(dst), len(b))
	}
	copy(dst, b)
	return nil
}

// MarshalJSON/UnmarshalJSON for crypto.Address is not needed here
// because Address is embedded in the above types as raw bytes.
// The Address type itself uses its fixed [24]byte representation.

// We also need PostID JSON support for any direct marshaling:

func (p PostID) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(p[:]))
}

func (p *PostID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return decodeHexInto(p[:], s)
}


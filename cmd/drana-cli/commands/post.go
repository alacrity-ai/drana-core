package commands

import (
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
	"github.com/drana-chain/drana/internal/validation"
)

func RunPost(args []string) error {
	fs := flag.NewFlagSet("post", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	text := fs.String("text", "", "post text")
	channel := fs.String("channel", "", "post channel (e.g., gaming, politics)")
	replyTo := fs.String("reply-to", "", "parent post ID (hex) to reply to")
	amount := fs.Uint64("amount", 0, "amount in microdrana")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *text == "" || *amount == 0 {
		return fmt.Errorf("--text and --amount are required")
	}

	privKey, err := loadPrivateKey(*key, *keyfile)
	if err != nil {
		return err
	}
	var pub crypto.PublicKey
	copy(pub[:], privKey[32:])
	sender := crypto.AddressFromPublicKey(pub)

	normalized, err := validation.NormalizePostText(*text)
	if err != nil {
		return fmt.Errorf("text normalization: %w", err)
	}

	nonce, err := queryNonce(*rpcAddr, sender.String())
	if err != nil {
		return err
	}

	tx := &types.Transaction{
		Type:    types.TxCreatePost,
		Sender:  sender,
		Text:    normalized,
		Channel: *channel,
		Amount:  *amount,
		Nonce:   nonce + 1,
	}

	// Set parent post ID if this is a reply.
	if *replyTo != "" {
		pidBytes, err := hex.DecodeString(*replyTo)
		if err != nil || len(pidBytes) != 32 {
			return fmt.Errorf("invalid --reply-to: must be 32-byte hex")
		}
		copy(tx.PostID[:], pidBytes)
	}

	types.SignTransaction(tx, privKey)

	postID := types.DerivePostID(sender, nonce+1)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Transaction submitted: %s\n", txHash)
	fmt.Printf("Post ID:              %s\n", hex.EncodeToString(postID[:]))
	if *channel != "" {
		fmt.Printf("Channel:              %s\n", *channel)
	}
	if *replyTo != "" {
		fmt.Printf("Reply to:             %s\n", *replyTo)
	}
	return nil
}

package commands

import (
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func RunUnstakePost(args []string) error {
	fs := flag.NewFlagSet("unstake-post", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	postIDHex := fs.String("post", "", "post ID (hex)")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *postIDHex == "" {
		return fmt.Errorf("--post is required")
	}

	privKey, err := loadPrivateKey(*key, *keyfile)
	if err != nil {
		return err
	}
	var pub crypto.PublicKey
	copy(pub[:], privKey[32:])
	sender := crypto.AddressFromPublicKey(pub)

	pidBytes, err := hex.DecodeString(*postIDHex)
	if err != nil || len(pidBytes) != 32 {
		return fmt.Errorf("invalid post ID: must be 32-byte hex")
	}
	var postID types.PostID
	copy(postID[:], pidBytes)

	nonce, err := queryNonce(*rpcAddr, sender.String())
	if err != nil {
		return err
	}

	tx := &types.Transaction{
		Type:   types.TxUnstakePost,
		Sender: sender,
		PostID: postID,
		Amount: 0,
		Nonce:  nonce + 1,
	}
	types.SignTransaction(tx, privKey)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Unstake post submitted: %s\n", txHash)
	return nil
}

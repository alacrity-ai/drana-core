package commands

import (
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func RunBoost(args []string) error {
	fs := flag.NewFlagSet("boost", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	postIDHex := fs.String("post", "", "post ID (hex)")
	amount := fs.Uint64("amount", 0, "amount in microdrana")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *postIDHex == "" || *amount == 0 {
		return fmt.Errorf("--post and --amount are required")
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
		Type:   types.TxBoostPost,
		Sender: sender,
		PostID: postID,
		Amount: *amount,
		Nonce:  nonce + 1,
	}
	types.SignTransaction(tx, privKey)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Transaction submitted: %s\n", txHash)
	return nil
}

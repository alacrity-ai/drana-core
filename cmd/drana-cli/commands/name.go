package commands

import (
	"flag"
	"fmt"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
	"github.com/drana-chain/drana/internal/validation"
)

func RunRegisterName(args []string) error {
	fs := flag.NewFlagSet("register-name", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	name := fs.String("name", "", "name to register (3-20 chars, a-z0-9_)")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if err := validation.ValidateName(*name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}

	privKey, err := loadPrivateKey(*key, *keyfile)
	if err != nil {
		return err
	}
	var pub crypto.PublicKey
	copy(pub[:], privKey[32:])
	sender := crypto.AddressFromPublicKey(pub)

	nonce, err := queryNonce(*rpcAddr, sender.String())
	if err != nil {
		return err
	}

	tx := &types.Transaction{
		Type:   types.TxRegisterName,
		Sender: sender,
		Text:   *name,
		Amount: 0,
		Nonce:  nonce + 1,
	}
	types.SignTransaction(tx, privKey)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Name registration submitted: %s\n", txHash)
	fmt.Printf("Name: %s -> %s\n", *name, sender.String())
	return nil
}

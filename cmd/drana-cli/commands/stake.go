package commands

import (
	"flag"
	"fmt"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

func RunStake(args []string) error {
	fs := flag.NewFlagSet("stake", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	amount := fs.Uint64("amount", 0, "amount to stake in microdrana")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *amount == 0 {
		return fmt.Errorf("--amount is required")
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
		Type:   types.TxStake,
		Sender: sender,
		Amount: *amount,
		Nonce:  nonce + 1,
	}
	types.SignTransaction(tx, privKey)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Stake transaction submitted: %s\n", txHash)
	fmt.Printf("Staked %d microdrana (%.6f DRANA)\n", *amount, float64(*amount)/1_000_000)
	return nil
}

func RunUnstake(args []string) error {
	fs := flag.NewFlagSet("unstake", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	amount := fs.Uint64("amount", 0, "amount to unstake in microdrana")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *amount == 0 {
		return fmt.Errorf("--amount is required")
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
		Type:   types.TxUnstake,
		Sender: sender,
		Amount: *amount,
		Nonce:  nonce + 1,
	}
	types.SignTransaction(tx, privKey)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Unstake transaction submitted: %s\n", txHash)
	fmt.Printf("Unstaking %d microdrana (%.6f DRANA)\n", *amount, float64(*amount)/1_000_000)
	return nil
}

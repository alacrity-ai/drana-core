package commands

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/drana-chain/drana/internal/crypto"
)

func RunKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	output := fs.String("output", "", "write private key to file")
	fs.Parse(args)

	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("keygen: %w", err)
	}
	addr := crypto.AddressFromPublicKey(pub)

	privHex := hex.EncodeToString(priv[:])
	fmt.Printf("Address:     %s\n", addr.String())
	fmt.Printf("Public Key:  %s\n", hex.EncodeToString(pub[:]))
	fmt.Printf("Private Key: %s\n", privHex)

	if *output != "" {
		if err := os.WriteFile(*output, []byte(privHex+"\n"), 0600); err != nil {
			return fmt.Errorf("write keyfile: %w", err)
		}
		fmt.Printf("Private key written to %s\n", *output)
	}
	return nil
}

func RunAddress(args []string) error {
	fs := flag.NewFlagSet("address", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	fs.Parse(args)

	privKey, err := loadPrivateKey(*key, *keyfile)
	if err != nil {
		return err
	}
	var pub crypto.PublicKey
	copy(pub[:], privKey[32:])
	addr := crypto.AddressFromPublicKey(pub)
	fmt.Println(addr.String())
	return nil
}

func loadPrivateKey(keyHex, keyfile string) (crypto.PrivateKey, error) {
	var privKey crypto.PrivateKey
	if keyHex == "" && keyfile != "" {
		data, err := os.ReadFile(keyfile)
		if err != nil {
			return privKey, fmt.Errorf("read keyfile: %w", err)
		}
		keyHex = string(data)
	}
	// Trim whitespace.
	for len(keyHex) > 0 && (keyHex[len(keyHex)-1] == '\n' || keyHex[len(keyHex)-1] == '\r' || keyHex[len(keyHex)-1] == ' ') {
		keyHex = keyHex[:len(keyHex)-1]
	}
	if keyHex == "" {
		return privKey, fmt.Errorf("no private key provided (use --key or --keyfile)")
	}
	b, err := hex.DecodeString(keyHex)
	if err != nil || len(b) != 64 {
		return privKey, fmt.Errorf("invalid private key: must be 64-byte hex")
	}
	copy(privKey[:], b)
	return privKey, nil
}

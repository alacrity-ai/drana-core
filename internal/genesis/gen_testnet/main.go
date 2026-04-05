// gen_testnet generates a testnet.json with real Ed25519 keys.
// Run: go run ./internal/genesis/gen_testnet
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/drana-chain/drana/internal/crypto"
)

type genesisJSON struct {
	ChainID            string        `json:"chainId"`
	GenesisTime        int64         `json:"genesisTime"`
	Accounts           []accountJSON `json:"accounts"`
	Validators         []validJSON   `json:"validators"`
	MaxPostLength      int           `json:"maxPostLength"`
	MaxPostBytes       int           `json:"maxPostBytes"`
	MinPostCommitment  uint64        `json:"minPostCommitment"`
	MinBoostCommitment uint64        `json:"minBoostCommitment"`
	MaxTxPerBlock      int           `json:"maxTxPerBlock"`
	MaxBlockBytes      int           `json:"maxBlockBytes"`
	BlockIntervalSec   int           `json:"blockIntervalSec"`
	BlockReward        uint64        `json:"blockReward"`
}

type accountJSON struct {
	Address string `json:"address"`
	Balance uint64 `json:"balance"`
}

type validJSON struct {
	Address string `json:"address"`
	PubKey  string `json:"pubKey"`
	Name    string `json:"name"`
}

func main() {
	g := genesisJSON{
		ChainID:            "drana-testnet-1",
		GenesisTime:        1700000000,
		MaxPostLength:      280,
		MaxPostBytes:       1024,
		MinPostCommitment:  1_000_000,
		MinBoostCommitment: 100_000,
		MaxTxPerBlock:      100,
		MaxBlockBytes:      1_048_576,
		BlockIntervalSec:   120,
		BlockReward:        10_000_000, // 10 DRANA per block
	}

	for i := 1; i <= 3; i++ {
		pub, priv, err := crypto.GenerateKeyPair()
		if err != nil {
			fmt.Fprintf(os.Stderr, "keygen: %v\n", err)
			os.Exit(1)
		}
		addr := crypto.AddressFromPublicKey(pub)
		g.Accounts = append(g.Accounts, accountJSON{
			Address: addr.String(),
			Balance: 1_000_000_000_000, // 1M DRANA
		})
		g.Validators = append(g.Validators, validJSON{
			Address: addr.String(),
			PubKey:  hex.EncodeToString(pub[:]),
			Name:    fmt.Sprintf("validator-%d", i),
		})
		fmt.Fprintf(os.Stderr, "validator-%d private key: %s\n", i, hex.EncodeToString(priv[:]))
	}

	data, _ := json.MarshalIndent(g, "", "  ")
	fmt.Println(string(data))
}

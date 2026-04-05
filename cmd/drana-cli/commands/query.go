package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"

	"github.com/drana-chain/drana/internal/rpc"
)

const defaultRPC = "http://localhost:26657"

func RunBalance(args []string) error {
	fs := flag.NewFlagSet("balance", flag.ExitOnError)
	address := fs.String("address", "", "drana1... address")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *address == "" {
		return fmt.Errorf("--address is required")
	}

	var resp rpc.AccountResponse
	if err := httpGet(*rpcAddr+"/v1/accounts/"+*address, &resp); err != nil {
		return err
	}
	fmt.Printf("Address: %s\n", resp.Address)
	if resp.Name != "" {
		fmt.Printf("Name:    %s\n", resp.Name)
	}
	fmt.Printf("Balance: %d microdrana (%.6f DRANA)\n", resp.Balance, float64(resp.Balance)/1_000_000)
	if resp.StakedBalance > 0 {
		fmt.Printf("Staked:  %d microdrana (%.6f DRANA)\n", resp.StakedBalance, float64(resp.StakedBalance)/1_000_000)
	}
	fmt.Printf("Nonce:   %d\n", resp.Nonce)
	return nil
}

func RunNonce(args []string) error {
	fs := flag.NewFlagSet("nonce", flag.ExitOnError)
	address := fs.String("address", "", "drana1... address")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *address == "" {
		return fmt.Errorf("--address is required")
	}

	var resp rpc.AccountResponse
	if err := httpGet(*rpcAddr+"/v1/accounts/"+*address, &resp); err != nil {
		return err
	}
	fmt.Println(resp.Nonce)
	return nil
}

func RunGetBlock(args []string) error {
	fs := flag.NewFlagSet("get-block", flag.ExitOnError)
	height := fs.Int("height", 0, "block height")
	latest := fs.Bool("latest", false, "get latest block")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	var url string
	if *latest {
		url = *rpcAddr + "/v1/blocks/latest"
	} else if *height > 0 {
		url = fmt.Sprintf("%s/v1/blocks/%d", *rpcAddr, *height)
	} else {
		return fmt.Errorf("specify --height or --latest")
	}

	var resp rpc.BlockResponse
	if err := httpGet(url, &resp); err != nil {
		return err
	}
	fmt.Printf("Height:   %d\n", resp.Height)
	fmt.Printf("Hash:     %s\n", resp.Hash)
	fmt.Printf("Proposer: %s\n", resp.ProposerAddr)
	fmt.Printf("Time:     %d\n", resp.Timestamp)
	fmt.Printf("Txs:      %d\n", resp.TxCount)
	return nil
}

func RunGetPost(args []string) error {
	fs := flag.NewFlagSet("get-post", flag.ExitOnError)
	id := fs.String("id", "", "post ID (hex)")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *id == "" {
		return fmt.Errorf("--id is required")
	}

	var resp rpc.PostResponse
	if err := httpGet(*rpcAddr+"/v1/posts/"+*id, &resp); err != nil {
		return err
	}
	fmt.Printf("Post ID:    %s\n", resp.PostID)
	fmt.Printf("Author:     %s\n", resp.Author)
	fmt.Printf("Text:       %s\n", resp.Text)
	fmt.Printf("Height:     %d\n", resp.CreatedAtHeight)
	fmt.Printf("Staked:     %d microdrana (%.6f DRANA)\n", resp.TotalStaked, float64(resp.TotalStaked)/1_000_000)
	fmt.Printf("Burned:     %d microdrana (%.6f DRANA)\n", resp.TotalBurned, float64(resp.TotalBurned)/1_000_000)
	fmt.Printf("Stakers:    %d\n", resp.StakerCount)
	return nil
}

func RunGetTx(args []string) error {
	fs := flag.NewFlagSet("get-tx", flag.ExitOnError)
	hash := fs.String("hash", "", "transaction hash (hex)")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *hash == "" {
		return fmt.Errorf("--hash is required")
	}

	var resp rpc.TransactionResponse
	if err := httpGet(*rpcAddr+"/v1/transactions/"+*hash, &resp); err != nil {
		return err
	}
	fmt.Printf("Hash:   %s\n", resp.Hash)
	fmt.Printf("Type:   %s\n", resp.Type)
	fmt.Printf("Sender: %s\n", resp.Sender)
	fmt.Printf("Amount: %d microdrana\n", resp.Amount)
	fmt.Printf("Nonce:  %d\n", resp.Nonce)
	fmt.Printf("Block:  %d\n", resp.BlockHeight)
	return nil
}

func RunNodeInfo(args []string) error {
	fs := flag.NewFlagSet("node-info", flag.ExitOnError)
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	var resp rpc.NodeInfoResponse
	if err := httpGet(*rpcAddr+"/v1/node/info", &resp); err != nil {
		return err
	}
	fmt.Printf("Chain ID:    %s\n", resp.ChainID)
	fmt.Printf("Height:      %d\n", resp.LatestHeight)
	fmt.Printf("Validators:  %d\n", resp.ValidatorCount)
	fmt.Printf("Block Reward: %d microdrana (%.6f DRANA)\n", resp.BlockReward, float64(resp.BlockReward)/1_000_000)
	fmt.Printf("Issued:      %d microdrana (%.6f DRANA)\n", resp.IssuedSupply, float64(resp.IssuedSupply)/1_000_000)
	fmt.Printf("Burned:      %d microdrana (%.6f DRANA)\n", resp.BurnedSupply, float64(resp.BurnedSupply)/1_000_000)
	return nil
}

func RunValidators(args []string) error {
	fs := flag.NewFlagSet("validators", flag.ExitOnError)
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	var vals []rpc.ValidatorResponse
	if err := httpGet(*rpcAddr+"/v1/network/validators", &vals); err != nil {
		return err
	}
	if len(vals) == 0 {
		fmt.Println("No active validators.")
		return nil
	}
	fmt.Printf("Active validators (%d):\n", len(vals))
	for i, v := range vals {
		name := v.Address
		if v.Name != "" {
			name = v.Name + " (" + v.Address + ")"
		}
		fmt.Printf("  %d. %s  stake: %d microdrana (%.6f DRANA)\n",
			i+1, name, v.StakedBalance, float64(v.StakedBalance)/1_000_000)
	}
	return nil
}

func RunUnstakeStatus(args []string) error {
	fs := flag.NewFlagSet("unstake-status", flag.ExitOnError)
	address := fs.String("address", "", "drana1... address")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *address == "" {
		return fmt.Errorf("--address is required")
	}

	var resp rpc.UnbondingResponse
	if err := httpGet(*rpcAddr+"/v1/accounts/"+*address+"/unbonding", &resp); err != nil {
		return err
	}
	if len(resp.Entries) == 0 {
		fmt.Println("No unbonding entries.")
		return nil
	}
	fmt.Printf("Unbonding for %s:\n", resp.Address)
	for _, e := range resp.Entries {
		fmt.Printf("  %d microdrana (%.6f DRANA) — releases at block %d\n",
			e.Amount, float64(e.Amount)/1_000_000, e.ReleaseHeight)
	}
	fmt.Printf("Total unbonding: %d microdrana (%.6f DRANA)\n", resp.Total, float64(resp.Total)/1_000_000)
	return nil
}

// httpGet makes a GET request and decodes the JSON response.
func httpGet(url string, out interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		var errResp rpc.ErrorResponse
		json.Unmarshal(body, &errResp)
		if errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

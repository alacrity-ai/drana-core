package main

import (
	"fmt"
	"os"

	"github.com/drana-chain/drana/cmd/drana-cli/commands"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "keygen":
		err = commands.RunKeygen(args)
	case "address":
		err = commands.RunAddress(args)
	case "balance":
		err = commands.RunBalance(args)
	case "nonce":
		err = commands.RunNonce(args)
	case "transfer":
		err = commands.RunTransfer(args)
	case "post":
		err = commands.RunPost(args)
	case "boost":
		err = commands.RunBoost(args)
	case "register-name":
		err = commands.RunRegisterName(args)
	case "stake":
		err = commands.RunStake(args)
	case "unstake":
		err = commands.RunUnstake(args)
	case "unstake-post":
		err = commands.RunUnstakePost(args)
	case "validators":
		err = commands.RunValidators(args)
	case "unstake-status":
		err = commands.RunUnstakeStatus(args)
	case "get-block":
		err = commands.RunGetBlock(args)
	case "get-post":
		err = commands.RunGetPost(args)
	case "get-tx":
		err = commands.RunGetTx(args)
	case "node-info":
		err = commands.RunNodeInfo(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage: drana-cli <command> [flags]

Commands:
  keygen       Generate a new keypair
  address      Show address for a private key
  balance      Query account balance
  nonce        Query account nonce
  transfer     Send DRANA to another address
  post         Create a post
  boost          Boost an existing post
  register-name    Register a name for your account
  validators       List active validators with stake
  unstake-status   Show unbonding entries for an address
  get-block        Get block by height
  get-post     Get post by ID
  get-tx       Get transaction by hash
  node-info    Query node info`)
}

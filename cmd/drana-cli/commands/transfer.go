package commands

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/rpc"
	"github.com/drana-chain/drana/internal/types"
)

func RunTransfer(args []string) error {
	fs := flag.NewFlagSet("transfer", flag.ExitOnError)
	key := fs.String("key", "", "private key hex")
	keyfile := fs.String("keyfile", "", "path to private key file")
	to := fs.String("to", "", "recipient drana1... address")
	amount := fs.Uint64("amount", 0, "amount in microdrana")
	rpcAddr := fs.String("rpc", defaultRPC, "node RPC endpoint")
	fs.Parse(args)

	if *to == "" || *amount == 0 {
		return fmt.Errorf("--to and --amount are required")
	}

	privKey, err := loadPrivateKey(*key, *keyfile)
	if err != nil {
		return err
	}
	var pub crypto.PublicKey
	copy(pub[:], privKey[32:])
	sender := crypto.AddressFromPublicKey(pub)

	recipient, err := crypto.ParseAddress(*to)
	if err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}

	nonce, err := queryNonce(*rpcAddr, sender.String())
	if err != nil {
		return err
	}

	tx := &types.Transaction{
		Type:      types.TxTransfer,
		Sender:    sender,
		Recipient: recipient,
		Amount:    *amount,
		Nonce:     nonce + 1,
	}
	types.SignTransaction(tx, privKey)

	txHash, err := submitTx(*rpcAddr, tx)
	if err != nil {
		return err
	}
	fmt.Printf("Transaction submitted: %s\n", txHash)
	return nil
}

func queryNonce(rpcAddr, address string) (uint64, error) {
	var resp rpc.AccountResponse
	if err := httpGet(rpcAddr+"/v1/accounts/"+address, &resp); err != nil {
		return 0, fmt.Errorf("query nonce: %w", err)
	}
	return resp.Nonce, nil
}

func submitTx(rpcAddr string, tx *types.Transaction) (string, error) {
	req := rpc.SubmitTxRequest{
		Type:      txTypeStr(tx.Type),
		Sender:    tx.Sender.String(),
		Amount:    tx.Amount,
		Nonce:     tx.Nonce,
		Signature: hex.EncodeToString(tx.Signature),
		PubKey:    hex.EncodeToString(tx.PubKey[:]),
	}
	switch tx.Type {
	case types.TxTransfer:
		req.Recipient = tx.Recipient.String()
	case types.TxCreatePost:
		req.Text = tx.Text
	case types.TxBoostPost:
		req.PostID = hex.EncodeToString(tx.PostID[:])
	}

	body, _ := json.Marshal(req)
	httpResp, err := http.Post(rpcAddr+"/v1/transactions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("submit: %w", err)
	}
	defer httpResp.Body.Close()

	var resp rpc.SubmitTxResponse
	json.NewDecoder(httpResp.Body).Decode(&resp)
	if !resp.Accepted {
		return "", fmt.Errorf("rejected: %s", resp.Error)
	}
	return resp.TxHash, nil
}

func txTypeStr(t types.TxType) string {
	switch t {
	case types.TxTransfer:
		return "transfer"
	case types.TxCreatePost:
		return "create_post"
	case types.TxBoostPost:
		return "boost_post"
	case types.TxRegisterName:
		return "register_name"
	case types.TxStake:
		return "stake"
	case types.TxUnstake:
		return "unstake"
	case types.TxUnstakePost:
		return "unstake_post"
	default:
		return "unknown"
	}
}

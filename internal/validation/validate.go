package validation

import (
	"fmt"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

// StateReader provides read-only access to chain state for validation.
type StateReader interface {
	GetAccount(addr crypto.Address) (*types.Account, bool)
	GetPost(id types.PostID) (*types.Post, bool)
	GetAccountByName(name string) (*types.Account, bool)
	GetPostStake(postID types.PostID, staker crypto.Address) (*types.PostStake, bool)
}

// ValidateTransaction checks all protocol rules for a transaction.
func ValidateTransaction(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
	switch tx.Type {
	case types.TxTransfer:
		return validateTransfer(tx, sr)
	case types.TxCreatePost:
		return validateCreatePost(tx, sr, params)
	case types.TxBoostPost:
		return validateBoostPost(tx, sr, params)
	case types.TxRegisterName:
		return validateRegisterName(tx, sr)
	case types.TxStake:
		return validateStake(tx, sr, params)
	case types.TxUnstake:
		return validateUnstake(tx, sr, params)
	case types.TxUnstakePost:
		return validateUnstakePost(tx, sr)
	default:
		return fmt.Errorf("unknown transaction type: %d", tx.Type)
	}
}

func validateCommon(tx *types.Transaction, sr StateReader) (*types.Account, error) {
	// PubKey must derive to Sender address.
	derived := crypto.AddressFromPublicKey(tx.PubKey)
	if derived != tx.Sender {
		return nil, fmt.Errorf("public key does not match sender address")
	}

	// Signature must be valid.
	if !crypto.Verify(tx.PubKey, tx.SignableBytes(), tx.Signature) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Sender account must exist.
	acct, ok := sr.GetAccount(tx.Sender)
	if !ok {
		return nil, fmt.Errorf("sender account does not exist")
	}

	// Nonce must be exactly account nonce + 1.
	if tx.Nonce != acct.Nonce+1 {
		return nil, fmt.Errorf("invalid nonce: expected %d, got %d", acct.Nonce+1, tx.Nonce)
	}

	// Sufficient balance.
	if acct.Balance < tx.Amount {
		return nil, fmt.Errorf("insufficient balance: have %d, need %d", acct.Balance, tx.Amount)
	}

	return acct, nil
}

func validateTransfer(tx *types.Transaction, sr StateReader) error {
	if tx.Amount == 0 {
		return fmt.Errorf("transfer amount must be greater than zero")
	}
	if tx.Sender == tx.Recipient {
		return fmt.Errorf("sender and recipient must be different")
	}
	if tx.Recipient.IsZero() {
		return fmt.Errorf("recipient address is zero")
	}
	_, err := validateCommon(tx, sr)
	return err
}

func validateCreatePost(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
	if tx.Amount < params.MinPostCommitment {
		return fmt.Errorf("post commitment %d below minimum %d", tx.Amount, params.MinPostCommitment)
	}
	if err := ValidatePostText(tx.Text, params.MaxPostLength, params.MaxPostBytes); err != nil {
		return fmt.Errorf("invalid post text: %w", err)
	}
	// Channel validation.
	if tx.Channel != "" {
		if err := ValidateName(tx.Channel); err != nil {
			return fmt.Errorf("invalid channel: %w", err)
		}
	}
	// Reply validation.
	var zeroPostID types.PostID
	if tx.PostID != zeroPostID {
		parent, ok := sr.GetPost(tx.PostID)
		if !ok {
			return fmt.Errorf("parent post %x does not exist", tx.PostID)
		}
		if parent.ParentPostID != zeroPostID {
			return fmt.Errorf("cannot reply to a reply (flat replies only)")
		}
	}
	_, err := validateCommon(tx, sr)
	return err
}

func validateBoostPost(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
	if tx.Amount < params.MinBoostCommitment {
		return fmt.Errorf("boost commitment %d below minimum %d", tx.Amount, params.MinBoostCommitment)
	}
	post, ok := sr.GetPost(tx.PostID)
	if !ok {
		return fmt.Errorf("post %x does not exist", tx.PostID)
	}
	if post.Withdrawn {
		return fmt.Errorf("cannot boost a withdrawn post")
	}
	_, err := validateCommon(tx, sr)
	return err
}

func validateUnstakePost(tx *types.Transaction, sr StateReader) error {
	_, err := validateCommonNoBalanceCheck(tx, sr)
	if err != nil {
		return err
	}
	if _, ok := sr.GetPostStake(tx.PostID, tx.Sender); !ok {
		return fmt.Errorf("no active stake on this post")
	}
	if _, ok := sr.GetPost(tx.PostID); !ok {
		return fmt.Errorf("post does not exist")
	}
	return nil
}

func validateRegisterName(tx *types.Transaction, sr StateReader) error {
	if tx.Amount != 0 {
		return fmt.Errorf("name registration must have zero amount")
	}
	if err := ValidateName(tx.Text); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}

	acct, err := validateCommon(tx, sr)
	if err != nil {
		return err
	}

	if acct.Name != "" {
		return fmt.Errorf("account already has a name: %q", acct.Name)
	}
	if existing, ok := sr.GetAccountByName(tx.Text); ok && existing != nil {
		return fmt.Errorf("name %q is already taken", tx.Text)
	}
	return nil
}

// validateCommonNoBalanceCheck validates identity and nonce but not spendable balance.
func validateCommonNoBalanceCheck(tx *types.Transaction, sr StateReader) (*types.Account, error) {
	derived := crypto.AddressFromPublicKey(tx.PubKey)
	if derived != tx.Sender {
		return nil, fmt.Errorf("public key does not match sender address")
	}
	if !crypto.Verify(tx.PubKey, tx.SignableBytes(), tx.Signature) {
		return nil, fmt.Errorf("invalid signature")
	}
	acct, ok := sr.GetAccount(tx.Sender)
	if !ok {
		return nil, fmt.Errorf("sender account does not exist")
	}
	if tx.Nonce != acct.Nonce+1 {
		return nil, fmt.Errorf("invalid nonce: expected %d, got %d", acct.Nonce+1, tx.Nonce)
	}
	return acct, nil
}

func validateStake(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
	if tx.Amount == 0 {
		return fmt.Errorf("stake amount must be greater than zero")
	}
	acct, err := validateCommon(tx, sr)
	if err != nil {
		return err
	}
	// If first stake, must meet minimum.
	if acct.StakedBalance == 0 && params.MinStake > 0 && tx.Amount < params.MinStake {
		return fmt.Errorf("initial stake %d below minimum %d", tx.Amount, params.MinStake)
	}
	return nil
}

func validateUnstake(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
	if tx.Amount == 0 {
		return fmt.Errorf("unstake amount must be greater than zero")
	}
	acct, err := validateCommonNoBalanceCheck(tx, sr)
	if err != nil {
		return err
	}
	if acct.StakedBalance < tx.Amount {
		return fmt.Errorf("insufficient staked balance: have %d, want to unstake %d", acct.StakedBalance, tx.Amount)
	}
	remaining := acct.StakedBalance - tx.Amount
	if remaining > 0 && params.MinStake > 0 && remaining < params.MinStake {
		return fmt.Errorf("remaining stake %d below minimum %d (unstake all or keep >= minimum)", remaining, params.MinStake)
	}
	return nil
}

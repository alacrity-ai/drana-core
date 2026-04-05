package state

import (
	"fmt"

	"github.com/drana-chain/drana/internal/types"
	"github.com/drana-chain/drana/internal/validation"
)

// Executor applies validated transactions to world state.
type Executor struct {
	Params *types.GenesisConfig
}

// ApplyTransaction executes a single transaction against the world state.
// The transaction must already be validated. Returns an error if state
// transition fails (e.g., balance underflow caught as safety net).
func (e *Executor) ApplyTransaction(ws *WorldState, tx *types.Transaction, blockHeight uint64, blockTime int64) error {
	switch tx.Type {
	case types.TxTransfer:
		return e.applyTransfer(ws, tx)
	case types.TxCreatePost:
		return e.applyCreatePost(ws, tx, blockHeight, blockTime)
	case types.TxBoostPost:
		return e.applyBoostPost(ws, tx, blockHeight)
	case types.TxUnstakePost:
		return e.applyUnstakePost(ws, tx)
	case types.TxRegisterName:
		return e.applyRegisterName(ws, tx)
	case types.TxStake:
		return e.applyStake(ws, tx)
	case types.TxUnstake:
		return e.applyUnstake(ws, tx, blockHeight)
	default:
		return fmt.Errorf("unknown tx type: %d", tx.Type)
	}
}

func (e *Executor) applyTransfer(ws *WorldState, tx *types.Transaction) error {
	sender, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("sender account not found")
	}
	if sender.Balance < tx.Amount {
		return fmt.Errorf("balance underflow")
	}

	// Get or create recipient account.
	recipient, ok := ws.GetAccount(tx.Recipient)
	if !ok {
		recipient = &types.Account{Address: tx.Recipient}
	}

	sender.Balance -= tx.Amount
	recipient.Balance += tx.Amount
	sender.Nonce++

	ws.SetAccount(sender)
	ws.SetAccount(recipient)
	return nil
}

func (e *Executor) applyCreatePost(ws *WorldState, tx *types.Transaction, blockHeight uint64, blockTime int64) error {
	author, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("author account not found")
	}
	if author.Balance < tx.Amount {
		return fmt.Errorf("balance underflow")
	}

	// Fee/stake split: PostFeePercent burned, rest staked.
	feePercent := e.Params.PostFeePercent
	if feePercent == 0 {
		feePercent = 6 // default
	}
	fee := types.MulDiv(tx.Amount, feePercent, 100)
	staked := tx.Amount - fee

	postID := types.DerivePostID(tx.Sender, author.Nonce+1)
	post := &types.Post{
		PostID:          postID,
		Author:          tx.Sender,
		Text:            tx.Text,
		Channel:         tx.Channel,
		ParentPostID:    tx.PostID,
		CreatedAtHeight: blockHeight,
		CreatedAtTime:   blockTime,
		TotalStaked:     staked,
		TotalBurned:     fee,
		StakerCount:     1,
	}

	author.Balance -= tx.Amount
	author.PostStakeBalance += staked
	author.Nonce++

	ws.SetAccount(author)
	ws.SetPost(post)
	ws.SetPostStake(&types.PostStake{PostID: postID, Staker: tx.Sender, Amount: staked, Height: blockHeight})
	ws.SetBurnedSupply(ws.GetBurnedSupply() + fee)
	return nil
}

func (e *Executor) applyBoostPost(ws *WorldState, tx *types.Transaction, blockHeight uint64) error {
	booster, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("booster account not found")
	}
	if booster.Balance < tx.Amount {
		return fmt.Errorf("balance underflow")
	}

	post, ok := ws.GetPost(tx.PostID)
	if !ok {
		return fmt.Errorf("post not found")
	}

	// Fee splits.
	burnPct := e.Params.BoostBurnPercent
	authorPct := e.Params.BoostAuthorPercent
	stakerPct := e.Params.BoostStakerPercent
	if burnPct == 0 && authorPct == 0 && stakerPct == 0 {
		burnPct, authorPct, stakerPct = 3, 2, 1 // defaults
	}

	burnAmount := types.MulDiv(tx.Amount, burnPct, 100)
	authorReward := types.MulDiv(tx.Amount, authorPct, 100)
	stakerReward := types.MulDiv(tx.Amount, stakerPct, 100)
	staked := tx.Amount - burnAmount - authorReward - stakerReward

	// Debit booster.
	booster.Balance -= tx.Amount
	booster.PostStakeBalance += staked
	booster.Nonce++
	ws.SetAccount(booster)

	// Burn.
	ws.SetBurnedSupply(ws.GetBurnedSupply() + burnAmount)

	// Author reward → wallet.
	if authorReward > 0 {
		authorAcct, ok := ws.GetAccount(post.Author)
		if ok {
			authorAcct.Balance += authorReward
			ws.SetAccount(authorAcct)
		}
	}

	// Staker reward → split pro-rata among existing stakers' wallets.
	if stakerReward > 0 && post.TotalStaked > 0 {
		stakers := ws.GetPostStakers(tx.PostID)
		for _, s := range stakers {
			share := types.MulDiv(stakerReward, s.Amount, post.TotalStaked)
			if share > 0 {
				stakerAcct, ok := ws.GetAccount(s.Staker)
				if ok {
					stakerAcct.Balance += share
					ws.SetAccount(stakerAcct)
				}
			}
		}
	}

	// Update post.
	post.TotalStaked += staked
	post.TotalBurned += burnAmount

	// Create or update stake position.
	existing, exists := ws.GetPostStake(tx.PostID, tx.Sender)
	if exists {
		existing.Amount += staked
		ws.SetPostStake(existing)
	} else {
		ws.SetPostStake(&types.PostStake{PostID: tx.PostID, Staker: tx.Sender, Amount: staked, Height: blockHeight})
		post.StakerCount++
	}
	ws.SetPost(post)

	return nil
}

func (e *Executor) applyUnstakePost(ws *WorldState, tx *types.Transaction) error {
	sender, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("sender account not found")
	}
	stake, ok := ws.GetPostStake(tx.PostID, tx.Sender)
	if !ok {
		return fmt.Errorf("no active stake on this post")
	}
	post, ok := ws.GetPost(tx.PostID)
	if !ok {
		return fmt.Errorf("post not found")
	}

	// Return staked amount to sender.
	sender.Balance += stake.Amount
	sender.PostStakeBalance -= stake.Amount
	sender.Nonce++
	ws.SetAccount(sender)

	post.TotalStaked -= stake.Amount
	post.StakerCount--
	ws.RemovePostStake(tx.PostID, tx.Sender)

	// If sender is the author → withdraw post, force-unstake all others.
	if tx.Sender == post.Author {
		post.Withdrawn = true
		for _, s := range ws.GetPostStakers(tx.PostID) {
			otherAcct, ok := ws.GetAccount(s.Staker)
			if ok {
				otherAcct.Balance += s.Amount
				otherAcct.PostStakeBalance -= s.Amount
				ws.SetAccount(otherAcct)
			}
			post.TotalStaked -= s.Amount
			post.StakerCount--
			ws.RemovePostStake(tx.PostID, s.Staker)
		}
	}

	ws.SetPost(post)
	return nil
}

func (e *Executor) applyStake(ws *WorldState, tx *types.Transaction) error {
	sender, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("sender account not found")
	}
	if sender.Balance < tx.Amount {
		return fmt.Errorf("balance underflow")
	}
	sender.Balance -= tx.Amount
	sender.StakedBalance += tx.Amount
	sender.Nonce++
	ws.SetAccount(sender)
	// Store pubkey for future validator set computation.
	ws.SetValidatorPubKey(tx.Sender, tx.PubKey)
	return nil
}

func (e *Executor) applyUnstake(ws *WorldState, tx *types.Transaction, blockHeight uint64) error {
	sender, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("sender account not found")
	}
	if sender.StakedBalance < tx.Amount {
		return fmt.Errorf("insufficient staked balance")
	}
	sender.StakedBalance -= tx.Amount
	sender.Nonce++
	ws.SetAccount(sender)
	ws.AddUnbondingEntry(types.UnbondingEntry{
		Address:       tx.Sender,
		Amount:        tx.Amount,
		ReleaseHeight: blockHeight + e.Params.UnbondingPeriod,
	})
	return nil
}

func (e *Executor) applySlashEvidence(ws *WorldState, ev *types.SlashEvidence) {
	if !ev.IsValid() {
		return
	}
	addr := ev.VoteA.VoterAddr
	if ws.HasBeenSlashed(addr, ev.VoteA.Height) {
		return
	}
	acct, ok := ws.GetAccount(addr)
	if !ok {
		return
	}
	totalAtRisk := acct.StakedBalance + ws.UnbondingBalanceFor(addr)
	if totalAtRisk == 0 {
		return
	}
	slashAmount := types.MulDiv(totalAtRisk, e.Params.SlashFractionDoubleSign, 100)
	if slashAmount == 0 {
		slashAmount = 1 // slash at least 1 microdrana
	}
	// Slash from staked balance first.
	fromStake := slashAmount
	if fromStake > acct.StakedBalance {
		fromStake = acct.StakedBalance
	}
	acct.StakedBalance -= fromStake
	remaining := slashAmount - fromStake
	ws.SetAccount(acct)

	if remaining > 0 {
		ws.SlashUnbonding(addr, remaining)
	}
	ws.RecordSlash(addr, ev.VoteA.Height)
	ws.SetBurnedSupply(ws.GetBurnedSupply() + slashAmount)
}

func (e *Executor) applyRegisterName(ws *WorldState, tx *types.Transaction) error {
	sender, ok := ws.GetAccount(tx.Sender)
	if !ok {
		return fmt.Errorf("sender account not found")
	}
	sender.Nonce++
	sender.Name = tx.Text
	ws.SetAccount(sender)
	ws.RegisterName(tx.Sender, tx.Text)
	return nil
}

// ApplyBlock validates and applies all transactions in a block atomically.
// On success, returns the new state. On failure, the input state is untouched.
func (e *Executor) ApplyBlock(ws *WorldState, block *types.Block) (*WorldState, error) {
	clone := ws.Clone()

	// 1. Process unbonding releases.
	matured := clone.RemoveMaturedUnbonding(block.Header.Height)
	for _, entry := range matured {
		acct, ok := clone.GetAccount(entry.Address)
		if !ok {
			acct = &types.Account{Address: entry.Address}
		}
		acct.Balance += entry.Amount
		clone.SetAccount(acct)
	}

	// 2. Process slash evidence.
	for i := range block.Evidence {
		e.applySlashEvidence(clone, &block.Evidence[i])
	}

	// 3. Apply transactions.
	for i, tx := range block.Transactions {
		if err := validation.ValidateTransaction(tx, clone, e.Params); err != nil {
			return nil, fmt.Errorf("tx %d validation failed: %w", i, err)
		}
		if err := e.ApplyTransaction(clone, tx, block.Header.Height, block.Header.Timestamp); err != nil {
			return nil, fmt.Errorf("tx %d apply failed: %w", i, err)
		}
	}

	// 4. Issue block reward to proposer.
	if e.Params.BlockReward > 0 {
		proposer, ok := clone.GetAccount(block.Header.ProposerAddr)
		if !ok {
			proposer = &types.Account{Address: block.Header.ProposerAddr}
		}
		proposer.Balance += e.Params.BlockReward
		clone.SetAccount(proposer)
		clone.SetIssuedSupply(clone.GetIssuedSupply() + e.Params.BlockReward)
	}

	// 5. Epoch transition: recompute active validator set.
	if e.Params.EpochLength > 0 && block.Header.Height > 0 && block.Header.Height%e.Params.EpochLength == 0 {
		newSet := clone.ComputeActiveValidatorSet(e.Params.MinStake)
		clone.SetActiveValidators(newSet)
		clone.SetCurrentEpoch(block.Header.Height / e.Params.EpochLength)
	}

	clone.SetChainHeight(block.Header.Height)
	return clone, nil
}

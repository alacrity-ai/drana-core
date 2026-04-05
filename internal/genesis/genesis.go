package genesis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/types"
)

// genesisJSON is the on-disk JSON representation of the genesis config.
type genesisJSON struct {
	ChainID           string               `json:"chainId"`
	GenesisTime       int64                `json:"genesisTime"`
	Accounts          []genesisAccountJSON `json:"accounts"`
	Validators        []genesisValidJSON   `json:"validators"`
	MaxPostLength     int                  `json:"maxPostLength"`
	MaxPostBytes      int                  `json:"maxPostBytes"`
	MinPostCommitment uint64               `json:"minPostCommitment"`
	MinBoostCommitment uint64              `json:"minBoostCommitment"`
	MaxTxPerBlock     int                  `json:"maxTxPerBlock"`
	MaxBlockBytes     int                  `json:"maxBlockBytes"`
	BlockIntervalSec         int                  `json:"blockIntervalSec"`
	BlockReward              uint64               `json:"blockReward"`
	MinStake                 uint64               `json:"minStake,omitempty"`
	EpochLength              uint64               `json:"epochLength,omitempty"`
	UnbondingPeriod          uint64               `json:"unbondingPeriod,omitempty"`
	SlashFractionDoubleSign  uint64               `json:"slashFractionDoubleSign,omitempty"`
	SeedNodes                []string             `json:"seedNodes,omitempty"`
	PostFeePercent           uint64               `json:"postFeePercent,omitempty"`
	BoostFeePercent          uint64               `json:"boostFeePercent,omitempty"`
	BoostBurnPercent         uint64               `json:"boostBurnPercent,omitempty"`
	BoostAuthorPercent       uint64               `json:"boostAuthorPercent,omitempty"`
	BoostStakerPercent       uint64               `json:"boostStakerPercent,omitempty"`
}

type genesisAccountJSON struct {
	Address string `json:"address"`
	Balance uint64 `json:"balance"`
}

type genesisValidJSON struct {
	Address string `json:"address"`
	PubKey  string `json:"pubKey"`
	Name    string `json:"name"`
	Stake   uint64 `json:"stake,omitempty"`
}

// LoadGenesis reads a JSON genesis file and returns a GenesisConfig.
func LoadGenesis(path string) (*types.GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis: %w", err)
	}

	var j genesisJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parse genesis: %w", err)
	}

	cfg := &types.GenesisConfig{
		ChainID:           j.ChainID,
		GenesisTime:       j.GenesisTime,
		MaxPostLength:     j.MaxPostLength,
		MaxPostBytes:      j.MaxPostBytes,
		MinPostCommitment: j.MinPostCommitment,
		MinBoostCommitment: j.MinBoostCommitment,
		MaxTxPerBlock:     j.MaxTxPerBlock,
		MaxBlockBytes:            j.MaxBlockBytes,
		BlockIntervalSec:         j.BlockIntervalSec,
		BlockReward:              j.BlockReward,
		MinStake:                 j.MinStake,
		EpochLength:              j.EpochLength,
		UnbondingPeriod:          j.UnbondingPeriod,
		SlashFractionDoubleSign:  j.SlashFractionDoubleSign,
		SeedNodes:                j.SeedNodes,
		PostFeePercent:           j.PostFeePercent,
		BoostFeePercent:          j.BoostFeePercent,
		BoostBurnPercent:         j.BoostBurnPercent,
		BoostAuthorPercent:       j.BoostAuthorPercent,
		BoostStakerPercent:       j.BoostStakerPercent,
	}

	seen := make(map[crypto.Address]bool)
	for i, a := range j.Accounts {
		addr, err := crypto.ParseAddress(a.Address)
		if err != nil {
			return nil, fmt.Errorf("account %d: %w", i, err)
		}
		if seen[addr] {
			return nil, fmt.Errorf("account %d: duplicate address %s", i, a.Address)
		}
		seen[addr] = true
		cfg.Accounts = append(cfg.Accounts, types.GenesisAccount{
			Address: addr,
			Balance: a.Balance,
		})
	}

	for i, v := range j.Validators {
		addr, err := crypto.ParseAddress(v.Address)
		if err != nil {
			return nil, fmt.Errorf("validator %d: %w", i, err)
		}
		pubBytes, err := hex.DecodeString(v.PubKey)
		if err != nil {
			return nil, fmt.Errorf("validator %d pubkey: %w", i, err)
		}
		if len(pubBytes) != 32 {
			return nil, fmt.Errorf("validator %d pubkey: expected 32 bytes, got %d", i, len(pubBytes))
		}
		var pubKey crypto.PublicKey
		copy(pubKey[:], pubBytes)
		cfg.Validators = append(cfg.Validators, types.GenesisValidator{
			Address: addr,
			PubKey:  pubKey,
			Name:    v.Name,
			Stake:   v.Stake,
		})
	}

	return cfg, nil
}

// InitializeState creates a WorldState from a GenesisConfig.
func InitializeState(cfg *types.GenesisConfig) (*state.WorldState, error) {
	ws := state.NewWorldState()

	var totalSupply uint64
	for i, a := range cfg.Accounts {
		if err := a.Address.Validate(); err != nil {
			return nil, fmt.Errorf("account %d: invalid address: %w", i, err)
		}
		// Check for overflow.
		if totalSupply > math.MaxUint64-a.Balance {
			return nil, fmt.Errorf("total genesis supply overflows uint64")
		}
		totalSupply += a.Balance
		ws.SetAccount(&types.Account{
			Address: a.Address,
			Balance: a.Balance,
			Nonce:   0,
		})
	}

	// Set up initial stakers from genesis validators.
	var initialValidators []types.ValidatorStake
	for _, v := range cfg.Validators {
		if v.Stake > 0 {
			acct, ok := ws.GetAccount(v.Address)
			if !ok {
				acct = &types.Account{Address: v.Address}
			}
			if acct.Balance < v.Stake {
				return nil, fmt.Errorf("validator %s: insufficient balance %d for stake %d", v.Address.String(), acct.Balance, v.Stake)
			}
			acct.Balance -= v.Stake
			acct.StakedBalance = v.Stake
			ws.SetAccount(acct)
		}
		stake := v.Stake
		if stake == 0 {
			stake = 1 // legacy: equal weight for genesis validators without explicit stake
		}
		initialValidators = append(initialValidators, types.ValidatorStake{
			Address:       v.Address,
			PubKey:        v.PubKey,
			StakedBalance: stake,
		})
	}
	// If any validators have stake, set up the active set.
	// If no PoS params, keep the set as-is (legacy: equal weight).
	if len(initialValidators) > 0 {
		ws.SetActiveValidators(initialValidators)
	}

	ws.SetChainHeight(0)
	ws.SetBurnedSupply(0)
	ws.SetIssuedSupply(0)
	ws.SetCurrentEpoch(0)
	return ws, nil
}

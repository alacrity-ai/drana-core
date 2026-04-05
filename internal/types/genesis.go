package types

import (
	"github.com/drana-chain/drana/internal/crypto"
)

// GenesisAccount defines an initial account in the genesis configuration.
type GenesisAccount struct {
	Address crypto.Address
	Balance uint64
}

// GenesisValidator defines a validator in the genesis configuration.
type GenesisValidator struct {
	Address crypto.Address
	PubKey  crypto.PublicKey
	Name    string
	Stake   uint64 // initial stake in microdrana
}

// GenesisConfig defines the full genesis configuration for the chain.
type GenesisConfig struct {
	ChainID           string
	GenesisTime       int64
	Accounts          []GenesisAccount
	Validators        []GenesisValidator
	MaxPostLength     int    // max code points
	MaxPostBytes      int    // max byte length
	MinPostCommitment uint64 // microdrana
	MinBoostCommitment uint64 // microdrana
	MaxTxPerBlock     int
	MaxBlockBytes     int
	BlockIntervalSec  int
	BlockReward              uint64 // microdrana minted per block, credited to proposer
	MinStake                 uint64 // microdrana required to validate (0 = PoS disabled, use genesis set)
	EpochLength              uint64 // blocks per epoch
	UnbondingPeriod          uint64 // blocks until unstaked funds are released
	SlashFractionDoubleSign  uint64   // percent of stake slashed for double-signing
	SeedNodes                []string // well-known seed node endpoints (host:port)
	PostFeePercent           uint64   // fee on post creation, percent (e.g., 6)
	BoostFeePercent          uint64   // total fee on boosting, percent (e.g., 6)
	BoostBurnPercent         uint64   // percent of boost amount burned (e.g., 3)
	BoostAuthorPercent       uint64   // percent of boost amount to author (e.g., 2)
	BoostStakerPercent       uint64   // percent of boost amount to stakers (e.g., 1)
}

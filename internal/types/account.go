package types

import (
	"github.com/drana-chain/drana/internal/crypto"
)

// Account represents an on-chain wallet.
type Account struct {
	Address          crypto.Address
	Balance          uint64 // spendable microdrana
	Nonce            uint64
	Name             string // empty if no name registered
	StakedBalance    uint64 // locked microdrana for PoS validation
	PostStakeBalance uint64 // total locked across all post stakes
}

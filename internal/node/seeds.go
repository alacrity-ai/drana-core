package node

// DefaultSeedNodes are hardcoded seed endpoints compiled into the binary.
// These are used as a fallback when no seeds are configured and none are
// listed in the genesis file.
//
// TODO: Replace these with real endpoints once genesis-validator.drana.io
// is deployed. See TODO.md for details.
var DefaultSeedNodes = []string{
	"genesis-validator.drana.io:26601",
}

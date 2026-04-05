package state

import (
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
)

// WorldState holds the current materialized chain state in memory.
type WorldState struct {
	accounts         map[crypto.Address]*types.Account
	posts            map[types.PostID]*types.Post
	nameIndex        map[string]crypto.Address
	postStakes       map[postStakeKey]*types.PostStake
	activeValidators []types.ValidatorStake
	unbondingQueue   []types.UnbondingEntry
	slashRecord      map[slashKey]bool
	burnedSupply     uint64
	issuedSupply     uint64
	chainHeight      uint64
	currentEpoch     uint64
}

type postStakeKey struct {
	postID types.PostID
	staker crypto.Address
}

type slashKey struct {
	addr   crypto.Address
	height uint64
}

func NewWorldState() *WorldState {
	return &WorldState{
		accounts:    make(map[crypto.Address]*types.Account),
		posts:       make(map[types.PostID]*types.Post),
		nameIndex:   make(map[string]crypto.Address),
		postStakes:  make(map[postStakeKey]*types.PostStake),
		slashRecord: make(map[slashKey]bool),
	}
}

func (ws *WorldState) GetAccount(addr crypto.Address) (*types.Account, bool) {
	a, ok := ws.accounts[addr]
	return a, ok
}

func (ws *WorldState) SetAccount(acct *types.Account) {
	ws.accounts[acct.Address] = acct
	// Maintain name index if account has a name.
	if acct.Name != "" {
		ws.nameIndex[acct.Name] = acct.Address
	}
}

func (ws *WorldState) GetAccountByName(name string) (*types.Account, bool) {
	addr, ok := ws.nameIndex[name]
	if !ok {
		return nil, false
	}
	return ws.GetAccount(addr)
}

func (ws *WorldState) RegisterName(addr crypto.Address, name string) {
	ws.nameIndex[name] = addr
}

func (ws *WorldState) GetPost(id types.PostID) (*types.Post, bool) {
	p, ok := ws.posts[id]
	return p, ok
}

func (ws *WorldState) SetPost(post *types.Post) {
	ws.posts[post.PostID] = post
}

// --- Post stake methods ---

func (ws *WorldState) GetPostStake(postID types.PostID, staker crypto.Address) (*types.PostStake, bool) {
	ps, ok := ws.postStakes[postStakeKey{postID, staker}]
	return ps, ok
}

func (ws *WorldState) SetPostStake(ps *types.PostStake) {
	ws.postStakes[postStakeKey{ps.PostID, ps.Staker}] = ps
}

func (ws *WorldState) RemovePostStake(postID types.PostID, staker crypto.Address) {
	delete(ws.postStakes, postStakeKey{postID, staker})
}

func (ws *WorldState) GetPostStakers(postID types.PostID) []*types.PostStake {
	var out []*types.PostStake
	for k, v := range ws.postStakes {
		if k.postID == postID {
			out = append(out, v)
		}
	}
	return out
}

func (ws *WorldState) GetStakesByAddress(addr crypto.Address) []*types.PostStake {
	var out []*types.PostStake
	for k, v := range ws.postStakes {
		if k.staker == addr {
			out = append(out, v)
		}
	}
	return out
}

func (ws *WorldState) AllPostStakes() []*types.PostStake {
	out := make([]*types.PostStake, 0, len(ws.postStakes))
	for _, v := range ws.postStakes {
		out = append(out, v)
	}
	return out
}

func (ws *WorldState) GetBurnedSupply() uint64 {
	return ws.burnedSupply
}

func (ws *WorldState) SetBurnedSupply(amount uint64) {
	ws.burnedSupply = amount
}

func (ws *WorldState) GetIssuedSupply() uint64 {
	return ws.issuedSupply
}

func (ws *WorldState) SetIssuedSupply(amount uint64) {
	ws.issuedSupply = amount
}

func (ws *WorldState) GetChainHeight() uint64 {
	return ws.chainHeight
}

func (ws *WorldState) SetChainHeight(h uint64) {
	ws.chainHeight = h
}

// AllAccounts returns all accounts. Order is non-deterministic.
func (ws *WorldState) AllAccounts() []*types.Account {
	out := make([]*types.Account, 0, len(ws.accounts))
	for _, a := range ws.accounts {
		out = append(out, a)
	}
	return out
}

// AllPosts returns all posts. Order is non-deterministic.
func (ws *WorldState) AllPosts() []*types.Post {
	out := make([]*types.Post, 0, len(ws.posts))
	for _, p := range ws.posts {
		out = append(out, p)
	}
	return out
}

// --- Staking state ---

func (ws *WorldState) GetActiveValidators() []types.ValidatorStake {
	out := make([]types.ValidatorStake, len(ws.activeValidators))
	copy(out, ws.activeValidators)
	return out
}

func (ws *WorldState) SetActiveValidators(vs []types.ValidatorStake) {
	ws.activeValidators = make([]types.ValidatorStake, len(vs))
	copy(ws.activeValidators, vs)
}

func (ws *WorldState) GetUnbondingQueue() []types.UnbondingEntry {
	out := make([]types.UnbondingEntry, len(ws.unbondingQueue))
	copy(out, ws.unbondingQueue)
	return out
}

func (ws *WorldState) AddUnbondingEntry(e types.UnbondingEntry) {
	ws.unbondingQueue = append(ws.unbondingQueue, e)
}

// RemoveMaturedUnbonding removes and returns all entries with ReleaseHeight <= currentHeight.
func (ws *WorldState) RemoveMaturedUnbonding(currentHeight uint64) []types.UnbondingEntry {
	var matured, remaining []types.UnbondingEntry
	for _, e := range ws.unbondingQueue {
		if e.ReleaseHeight <= currentHeight {
			matured = append(matured, e)
		} else {
			remaining = append(remaining, e)
		}
	}
	ws.unbondingQueue = remaining
	return matured
}

// UnbondingBalanceFor returns the total unbonding amount for an address.
func (ws *WorldState) UnbondingBalanceFor(addr crypto.Address) uint64 {
	var total uint64
	for _, e := range ws.unbondingQueue {
		if e.Address == addr {
			total += e.Amount
		}
	}
	return total
}

// SlashUnbonding reduces unbonding entries for an address by the given amount.
func (ws *WorldState) SlashUnbonding(addr crypto.Address, amount uint64) {
	remaining := amount
	for i := range ws.unbondingQueue {
		if remaining == 0 {
			break
		}
		if ws.unbondingQueue[i].Address == addr {
			if ws.unbondingQueue[i].Amount <= remaining {
				remaining -= ws.unbondingQueue[i].Amount
				ws.unbondingQueue[i].Amount = 0
			} else {
				ws.unbondingQueue[i].Amount -= remaining
				remaining = 0
			}
		}
	}
	// Remove zero entries.
	var cleaned []types.UnbondingEntry
	for _, e := range ws.unbondingQueue {
		if e.Amount > 0 {
			cleaned = append(cleaned, e)
		}
	}
	ws.unbondingQueue = cleaned
}

func (ws *WorldState) HasBeenSlashed(addr crypto.Address, height uint64) bool {
	return ws.slashRecord[slashKey{addr, height}]
}

func (ws *WorldState) RecordSlash(addr crypto.Address, height uint64) {
	ws.slashRecord[slashKey{addr, height}] = true
}

func (ws *WorldState) GetCurrentEpoch() uint64 {
	return ws.currentEpoch
}

func (ws *WorldState) SetCurrentEpoch(epoch uint64) {
	ws.currentEpoch = epoch
}

// ComputeActiveValidatorSet builds the active set from all staked accounts.
func (ws *WorldState) ComputeActiveValidatorSet(minStake uint64) []types.ValidatorStake {
	var set []types.ValidatorStake
	for _, acct := range ws.accounts {
		if acct.StakedBalance >= minStake {
			set = append(set, types.ValidatorStake{
				Address:       acct.Address,
				PubKey:        ws.pubKeyForAccount(acct.Address),
				StakedBalance: acct.StakedBalance,
			})
		}
	}
	// Sort by address for determinism.
	sortValidatorSet(set)
	return set
}

// TotalActiveStake returns the sum of all active validators' stake.
func (ws *WorldState) TotalActiveStake() uint64 {
	var total uint64
	for _, v := range ws.activeValidators {
		total += v.StakedBalance
	}
	return total
}

// pubKeyForAccount looks up the pubkey for an address from the active set.
// For new stakers, the pubkey is stored when they first stake.
func (ws *WorldState) pubKeyForAccount(addr crypto.Address) crypto.PublicKey {
	for _, v := range ws.activeValidators {
		if v.Address == addr {
			return v.PubKey
		}
	}
	return crypto.PublicKey{}
}

// SetValidatorPubKey ensures a pubkey is associated with a staker address.
func (ws *WorldState) SetValidatorPubKey(addr crypto.Address, pubKey crypto.PublicKey) {
	// Store in active set if present, or it will be picked up at next epoch.
	for i := range ws.activeValidators {
		if ws.activeValidators[i].Address == addr {
			ws.activeValidators[i].PubKey = pubKey
			return
		}
	}
}

func sortValidatorSet(vs []types.ValidatorStake) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && compareAddrs(vs[j].Address, vs[j-1].Address) < 0; j-- {
			vs[j], vs[j-1] = vs[j-1], vs[j]
		}
	}
}

func compareAddrs(a, b crypto.Address) int {
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// Clone produces an independent deep copy of the world state.
func (ws *WorldState) Clone() *WorldState {
	c := &WorldState{
		accounts:     make(map[crypto.Address]*types.Account, len(ws.accounts)),
		posts:        make(map[types.PostID]*types.Post, len(ws.posts)),
		nameIndex:    make(map[string]crypto.Address, len(ws.nameIndex)),
		postStakes:   make(map[postStakeKey]*types.PostStake, len(ws.postStakes)),
		slashRecord:  make(map[slashKey]bool, len(ws.slashRecord)),
		burnedSupply: ws.burnedSupply,
		issuedSupply: ws.issuedSupply,
		chainHeight:  ws.chainHeight,
		currentEpoch: ws.currentEpoch,
	}
	for k, v := range ws.accounts {
		acctCopy := *v
		c.accounts[k] = &acctCopy
	}
	for k, v := range ws.posts {
		postCopy := *v
		c.posts[k] = &postCopy
	}
	for k, v := range ws.nameIndex {
		c.nameIndex[k] = v
	}
	for k, v := range ws.slashRecord {
		c.slashRecord[k] = v
	}
	for k, v := range ws.postStakes {
		psCopy := *v
		c.postStakes[k] = &psCopy
	}
	c.activeValidators = make([]types.ValidatorStake, len(ws.activeValidators))
	copy(c.activeValidators, ws.activeValidators)
	c.unbondingQueue = make([]types.UnbondingEntry, len(ws.unbondingQueue))
	copy(c.unbondingQueue, ws.unbondingQueue)
	return c
}

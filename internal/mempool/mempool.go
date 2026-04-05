package mempool

import (
	"fmt"
	"sort"
	"sync"

	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/types"
	"github.com/drana-chain/drana/internal/validation"
)

// AccountChecker provides read-only account lookups for mempool validation.
type AccountChecker interface {
	GetAccount(addr crypto.Address) (*types.Account, bool)
}

// Mempool holds pending transactions awaiting inclusion in a block.
// All methods are safe for concurrent use.
type Mempool struct {
	mu           sync.RWMutex
	txs          map[[32]byte]*types.Transaction
	maxSize      int
	stateChecker AccountChecker
}

// New creates a mempool with the given maximum capacity.
func New(maxSize int) *Mempool {
	return &Mempool{
		txs:     make(map[[32]byte]*types.Transaction),
		maxSize: maxSize,
	}
}

// SetStateChecker sets the state reader used for balance/nonce checks on Add.
func (m *Mempool) SetStateChecker(sc AccountChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateChecker = sc
}

// Add accepts a transaction after basic validation (signature, pubkey match, no duplicate).
func (m *Mempool) Add(tx *types.Transaction) error {
	// Verify signature.
	derived := crypto.AddressFromPublicKey(tx.PubKey)
	if derived != tx.Sender {
		return fmt.Errorf("pubkey does not match sender")
	}
	if !crypto.Verify(tx.PubKey, tx.SignableBytes(), tx.Signature) {
		return fmt.Errorf("invalid signature")
	}

	// Check sender account exists and has sufficient balance.
	if m.stateChecker != nil {
		acct, exists := m.stateChecker.GetAccount(tx.Sender)
		if !exists {
			return fmt.Errorf("sender account does not exist")
		}
		if tx.Amount > 0 && acct.Balance < tx.Amount {
			return fmt.Errorf("insufficient balance: have %d, need %d", acct.Balance, tx.Amount)
		}
		if tx.Nonce <= acct.Nonce {
			return fmt.Errorf("nonce too low: account nonce is %d, tx nonce is %d", acct.Nonce, tx.Nonce)
		}
	}

	txHash := tx.Hash()
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.txs[txHash]; exists {
		return fmt.Errorf("duplicate transaction")
	}
	if len(m.txs) >= m.maxSize {
		return fmt.Errorf("mempool full")
	}
	m.txs[txHash] = tx
	return nil
}

// Remove evicts transactions by hash (called after block finalization).
func (m *Mempool) Remove(txHashes [][32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range txHashes {
		delete(m.txs, h)
	}
}

// EvictStale removes transactions whose nonce is <= the sender's current on-chain nonce,
// or whose sender account does not exist. Called after each block commit.
func (m *Mempool) EvictStale(getAccountNonce func(crypto.Address) (uint64, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for hash, tx := range m.txs {
		nonce, exists := getAccountNonce(tx.Sender)
		if !exists {
			// Sender doesn't exist on-chain — can't process this tx.
			delete(m.txs, hash)
			continue
		}
		if tx.Nonce <= nonce {
			// Nonce already used — stale.
			delete(m.txs, hash)
		}
	}
}

// Has checks if a transaction hash is in the pool.
func (m *Mempool) Has(txHash [32]byte) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.txs[txHash]
	return ok
}

// Pending returns all pending transactions (unordered).
func (m *Mempool) Pending() []*types.Transaction {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*types.Transaction, 0, len(m.txs))
	for _, tx := range m.txs {
		out = append(out, tx)
	}
	return out
}

// Size returns the number of pending transactions.
func (m *Mempool) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.txs)
}

// Flush removes all transactions.
func (m *Mempool) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.txs = make(map[[32]byte]*types.Transaction)
}

// SpeculativeState wraps a StateReader and tracks nonce/balance changes
// as transactions are speculatively accepted during block assembly.
type speculativeState struct {
	base     validation.StateReader
	accounts map[crypto.Address]*types.Account // overrides
}

func newSpeculativeState(base validation.StateReader) *speculativeState {
	return &speculativeState{
		base:     base,
		accounts: make(map[crypto.Address]*types.Account),
	}
}

func (s *speculativeState) GetAccount(addr crypto.Address) (*types.Account, bool) {
	if a, ok := s.accounts[addr]; ok {
		return a, true
	}
	a, ok := s.base.GetAccount(addr)
	if ok {
		// Copy so we can mutate without affecting base.
		acctCopy := *a
		s.accounts[addr] = &acctCopy
		return &acctCopy, true
	}
	return nil, false
}

func (s *speculativeState) GetPost(id types.PostID) (*types.Post, bool) {
	return s.base.GetPost(id)
}

func (s *speculativeState) GetPostStake(postID types.PostID, staker crypto.Address) (*types.PostStake, bool) {
	return s.base.GetPostStake(postID, staker)
}

func (s *speculativeState) GetAccountByName(name string) (*types.Account, bool) {
	// Check overrides first for recently registered names.
	for _, a := range s.accounts {
		if a.Name == name {
			return a, true
		}
	}
	return s.base.GetAccountByName(name)
}

func (s *speculativeState) applyTx(tx *types.Transaction) {
	acct, _ := s.accounts[tx.Sender]
	acct.Nonce++
	acct.Balance -= tx.Amount
	if tx.Type == types.TxTransfer {
		recip, ok := s.accounts[tx.Recipient]
		if !ok {
			if r, ok2 := s.base.GetAccount(tx.Recipient); ok2 {
				recipCopy := *r
				recip = &recipCopy
			} else {
				recip = &types.Account{Address: tx.Recipient}
			}
			s.accounts[tx.Recipient] = recip
		}
		recip.Balance += tx.Amount
	}
}

// ReapForBlock returns an ordered list of transactions suitable for block inclusion.
// It groups by sender, orders by nonce, and validates each against evolving speculative state.
func (m *Mempool) ReapForBlock(sr validation.StateReader, params *types.GenesisConfig, maxTx int) []*types.Transaction {
	m.mu.RLock()
	pending := make([]*types.Transaction, 0, len(m.txs))
	for _, tx := range m.txs {
		pending = append(pending, tx)
	}
	m.mu.RUnlock()

	// Group by sender, sort by nonce within each group.
	bySender := make(map[crypto.Address][]*types.Transaction)
	for _, tx := range pending {
		bySender[tx.Sender] = append(bySender[tx.Sender], tx)
	}
	for _, txs := range bySender {
		sort.Slice(txs, func(i, j int) bool {
			return txs[i].Nonce < txs[j].Nonce
		})
	}

	spec := newSpeculativeState(sr)

	type senderQueue struct {
		addr crypto.Address
		txs  []*types.Transaction
		idx  int
	}
	queues := make([]senderQueue, 0, len(bySender))
	for addr, txs := range bySender {
		queues = append(queues, senderQueue{addr: addr, txs: txs})
	}

	var result []*types.Transaction
	changed := true
	for changed && len(result) < maxTx {
		changed = false
		for i := range queues {
			if queues[i].idx >= len(queues[i].txs) {
				continue
			}
			tx := queues[i].txs[queues[i].idx]
			if err := validation.ValidateTransaction(tx, spec, params); err != nil {
				continue
			}
			spec.applyTx(tx)
			result = append(result, tx)
			queues[i].idx++
			changed = true
			if len(result) >= maxTx {
				break
			}
		}
	}
	return result
}

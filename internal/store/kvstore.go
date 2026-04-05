package store

import (
	"encoding/binary"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/drana-chain/drana/internal/crypto"
	"github.com/drana-chain/drana/internal/state"
	"github.com/drana-chain/drana/internal/types"
)

// Key prefixes for the KV store.
var (
	prefixAccount      = []byte("account:")
	prefixPost         = []byte("post:")
	prefixPostStake    = []byte("poststake:")
	keyBurnedSupply    = []byte("meta:burned_supply")
	keyIssuedSupply    = []byte("meta:issued_supply")
	keyChainHeight     = []byte("meta:chain_height")
	keyCurrentEpoch    = []byte("meta:current_epoch")
	keyActiveValidators = []byte("meta:active_validators")
	keyUnbondingQueue  = []byte("meta:unbonding_queue")
)

// KVStore persists materialized chain state using BadgerDB.
type KVStore struct {
	db *badger.DB
}

// OpenKVStore opens or creates a BadgerDB at the given path.
func OpenKVStore(path string) (*KVStore, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // suppress badger logs
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open kvstore: %w", err)
	}
	return &KVStore{db: db}, nil
}

// Close closes the underlying database.
func (kv *KVStore) Close() error {
	return kv.db.Close()
}

// SaveState persists the entire world state atomically.
func (kv *KVStore) SaveState(ws *state.WorldState) error {
	return kv.db.Update(func(txn *badger.Txn) error {
		// Accounts
		for _, acct := range ws.AllAccounts() {
			key := append(append([]byte(nil), prefixAccount...), acct.Address[:]...)
			val := encodeAccount(acct)
			if err := txn.Set(key, val); err != nil {
				return err
			}
		}
		// Posts
		for _, post := range ws.AllPosts() {
			key := append(append([]byte(nil), prefixPost...), post.PostID[:]...)
			val := encodePost(post)
			if err := txn.Set(key, val); err != nil {
				return err
			}
		}
		// Burned supply
		if err := txn.Set(keyBurnedSupply, encodeUint64(ws.GetBurnedSupply())); err != nil {
			return err
		}
		// Issued supply
		if err := txn.Set(keyIssuedSupply, encodeUint64(ws.GetIssuedSupply())); err != nil {
			return err
		}
		// Chain height
		if err := txn.Set(keyChainHeight, encodeUint64(ws.GetChainHeight())); err != nil {
			return err
		}
		// Current epoch
		if err := txn.Set(keyCurrentEpoch, encodeUint64(ws.GetCurrentEpoch())); err != nil {
			return err
		}
		// Active validators
		if data := encodeValidatorSet(ws.GetActiveValidators()); data != nil {
			if err := txn.Set(keyActiveValidators, data); err != nil {
				return err
			}
		}
		// Unbonding queue
		if data := encodeUnbondingQueue(ws.GetUnbondingQueue()); data != nil {
			if err := txn.Set(keyUnbondingQueue, data); err != nil {
				return err
			}
		}
		// Post stakes
		for _, ps := range ws.AllPostStakes() {
			key := make([]byte, len(prefixPostStake)+32+crypto.AddressLen)
			copy(key, prefixPostStake)
			copy(key[len(prefixPostStake):], ps.PostID[:])
			copy(key[len(prefixPostStake)+32:], ps.Staker[:])
			if err := txn.Set(key, encodePostStake(ps)); err != nil {
				return err
			}
		}
		return nil
	})
}

// LoadState reads the entire world state from the store.
func (kv *KVStore) LoadState() (*state.WorldState, error) {
	ws := state.NewWorldState()

	err := kv.db.View(func(txn *badger.Txn) error {
		// Accounts
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefixAccount); it.ValidForPrefix(prefixAccount); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				acct, err := decodeAccount(val)
				if err != nil {
					return err
				}
				ws.SetAccount(acct)
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Posts
		for it.Seek(prefixPost); it.ValidForPrefix(prefixPost); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				post, err := decodePost(val)
				if err != nil {
					return err
				}
				ws.SetPost(post)
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Burned supply
		item, err := txn.Get(keyBurnedSupply)
		if err == badger.ErrKeyNotFound {
			// fresh db
		} else if err != nil {
			return err
		} else {
			if err := item.Value(func(val []byte) error {
				ws.SetBurnedSupply(decodeUint64(val))
				return nil
			}); err != nil {
				return err
			}
		}

		// Issued supply
		item, err = txn.Get(keyIssuedSupply)
		if err == badger.ErrKeyNotFound {
			// fresh db
		} else if err != nil {
			return err
		} else {
			if err := item.Value(func(val []byte) error {
				ws.SetIssuedSupply(decodeUint64(val))
				return nil
			}); err != nil {
				return err
			}
		}

		// Chain height
		item, err = txn.Get(keyChainHeight)
		if err == badger.ErrKeyNotFound {
			// fresh db
		} else if err != nil {
			return err
		} else {
			if err := item.Value(func(val []byte) error {
				ws.SetChainHeight(decodeUint64(val))
				return nil
			}); err != nil {
				return err
			}
		}

		// Current epoch
		item, err = txn.Get(keyCurrentEpoch)
		if err == nil {
			item.Value(func(val []byte) error { ws.SetCurrentEpoch(decodeUint64(val)); return nil })
		}

		// Active validators
		item, err = txn.Get(keyActiveValidators)
		if err == nil {
			item.Value(func(val []byte) error {
				ws.SetActiveValidators(decodeValidatorSet(val))
				return nil
			})
		}

		// Unbonding queue
		item, err = txn.Get(keyUnbondingQueue)
		if err == nil {
			item.Value(func(val []byte) error {
				for _, u := range decodeUnbondingQueue(val) {
					ws.AddUnbondingEntry(u)
				}
				return nil
			})
		}

		// Post stakes
		for it.Seek(prefixPostStake); it.ValidForPrefix(prefixPostStake); it.Next() {
			item := it.Item()
			item.Value(func(val []byte) error {
				ps, err := decodePostStake(val)
				if err == nil {
					ws.SetPostStake(ps)
				}
				return nil
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	return ws, nil
}

// --- Encoding helpers ---
// Simple fixed-layout binary encoding. No protobuf dependency here for now;
// we can migrate to protobuf later if needed.

func encodeUint64(v uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return buf
}

func decodeUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func encodeAccount(a *types.Account) []byte {
	// address (24) + balance (8) + nonce (8) + stakedBalance (8) + nameLen (4) + name (variable)
	nameBytes := []byte(a.Name)
	size := crypto.AddressLen + 8 + 8 + 8 + 8 + 4 + len(nameBytes)
	buf := make([]byte, size)
	off := 0
	copy(buf[off:], a.Address[:])
	off += crypto.AddressLen
	binary.BigEndian.PutUint64(buf[off:], a.Balance)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], a.Nonce)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], a.StakedBalance)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], a.PostStakeBalance)
	off += 8
	binary.BigEndian.PutUint32(buf[off:], uint32(len(nameBytes)))
	off += 4
	copy(buf[off:], nameBytes)
	return buf
}

func decodeAccount(b []byte) (*types.Account, error) {
	minSize := crypto.AddressLen + 8 + 8
	if len(b) < minSize {
		return nil, fmt.Errorf("account data too short: %d bytes", len(b))
	}
	a := &types.Account{}
	off := 0
	copy(a.Address[:], b[off:off+crypto.AddressLen])
	off += crypto.AddressLen
	a.Balance = binary.BigEndian.Uint64(b[off:])
	off += 8
	a.Nonce = binary.BigEndian.Uint64(b[off:])
	off += 8
	// StakedBalance + PostStakeBalance + Name fields — may not be present in old data.
	if len(b) > off+8 {
		a.StakedBalance = binary.BigEndian.Uint64(b[off:])
		off += 8
		if len(b) > off+8 {
			a.PostStakeBalance = binary.BigEndian.Uint64(b[off:])
			off += 8
			if len(b) > off+4 {
				nameLen := binary.BigEndian.Uint32(b[off:])
				off += 4
				if len(b) >= off+int(nameLen) {
					a.Name = string(b[off : off+int(nameLen)])
				}
			}
		}
	}
	return a, nil
}

func encodePost(p *types.Post) []byte {
	// postID(32) + author(24) + createdAtHeight(8) + createdAtTime(8)
	// + totalStaked(8) + totalBurned(8) + stakerCount(8) + withdrawn(1)
	// + textLen(4) + text + channelLen(4) + channel + parentPostID(32)
	textBytes := []byte(p.Text)
	channelBytes := []byte(p.Channel)
	size := 32 + crypto.AddressLen + 8 + 8 + 8 + 8 + 8 + 1 + 4 + len(textBytes) + 4 + len(channelBytes) + 32
	buf := make([]byte, size)
	off := 0
	copy(buf[off:], p.PostID[:])
	off += 32
	copy(buf[off:], p.Author[:])
	off += crypto.AddressLen
	binary.BigEndian.PutUint64(buf[off:], p.CreatedAtHeight)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(p.CreatedAtTime))
	off += 8
	binary.BigEndian.PutUint64(buf[off:], p.TotalStaked)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], p.TotalBurned)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], p.StakerCount)
	off += 8
	if p.Withdrawn {
		buf[off] = 1
	}
	off += 1
	binary.BigEndian.PutUint32(buf[off:], uint32(len(textBytes)))
	off += 4
	copy(buf[off:], textBytes)
	off += len(textBytes)
	binary.BigEndian.PutUint32(buf[off:], uint32(len(channelBytes)))
	off += 4
	copy(buf[off:], channelBytes)
	off += len(channelBytes)
	copy(buf[off:], p.ParentPostID[:])
	return buf
}

func decodePost(b []byte) (*types.Post, error) {
	minSize := 32 + crypto.AddressLen + 8 + 8 + 8 + 8 + 8 + 1 + 4
	if len(b) < minSize {
		return nil, fmt.Errorf("post data too short: %d bytes", len(b))
	}
	p := &types.Post{}
	off := 0
	copy(p.PostID[:], b[off:off+32])
	off += 32
	copy(p.Author[:], b[off:off+crypto.AddressLen])
	off += crypto.AddressLen
	p.CreatedAtHeight = binary.BigEndian.Uint64(b[off:])
	off += 8
	p.CreatedAtTime = int64(binary.BigEndian.Uint64(b[off:]))
	off += 8
	p.TotalStaked = binary.BigEndian.Uint64(b[off:])
	off += 8
	p.TotalBurned = binary.BigEndian.Uint64(b[off:])
	off += 8
	p.StakerCount = binary.BigEndian.Uint64(b[off:])
	off += 8
	if len(b) > off {
		p.Withdrawn = b[off] == 1
		off += 1
	}
	textLen := binary.BigEndian.Uint32(b[off:])
	off += 4
	if len(b) < off+int(textLen) {
		return nil, fmt.Errorf("post text truncated")
	}
	p.Text = string(b[off : off+int(textLen)])
	off += int(textLen)
	// Channel and ParentPostID — may not be present in old data.
	if len(b) >= off+4 {
		chanLen := binary.BigEndian.Uint32(b[off:])
		off += 4
		if len(b) >= off+int(chanLen) {
			p.Channel = string(b[off : off+int(chanLen)])
			off += int(chanLen)
		}
		if len(b) >= off+32 {
			copy(p.ParentPostID[:], b[off:off+32])
		}
	}
	return p, nil
}

// --- Validator set encoding ---
// Format: count(4) + [address(24) + pubkey(32) + stake(8)] * count

func encodeValidatorSet(vs []types.ValidatorStake) []byte {
	buf := make([]byte, 4+len(vs)*(crypto.AddressLen+32+8))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(vs)))
	off := 4
	for _, v := range vs {
		copy(buf[off:], v.Address[:])
		off += crypto.AddressLen
		copy(buf[off:], v.PubKey[:])
		off += 32
		binary.BigEndian.PutUint64(buf[off:], v.StakedBalance)
		off += 8
	}
	return buf
}

func decodeValidatorSet(b []byte) []types.ValidatorStake {
	if len(b) < 4 {
		return nil
	}
	count := binary.BigEndian.Uint32(b[:4])
	off := 4
	entrySize := crypto.AddressLen + 32 + 8
	var vs []types.ValidatorStake
	for i := 0; i < int(count) && off+entrySize <= len(b); i++ {
		var v types.ValidatorStake
		copy(v.Address[:], b[off:off+crypto.AddressLen])
		off += crypto.AddressLen
		copy(v.PubKey[:], b[off:off+32])
		off += 32
		v.StakedBalance = binary.BigEndian.Uint64(b[off:])
		off += 8
		vs = append(vs, v)
	}
	return vs
}

// --- Unbonding queue encoding ---
// Format: count(4) + [address(24) + amount(8) + releaseHeight(8)] * count

func encodeUnbondingQueue(entries []types.UnbondingEntry) []byte {
	buf := make([]byte, 4+len(entries)*(crypto.AddressLen+8+8))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(entries)))
	off := 4
	for _, e := range entries {
		copy(buf[off:], e.Address[:])
		off += crypto.AddressLen
		binary.BigEndian.PutUint64(buf[off:], e.Amount)
		off += 8
		binary.BigEndian.PutUint64(buf[off:], e.ReleaseHeight)
		off += 8
	}
	return buf
}

func decodeUnbondingQueue(b []byte) []types.UnbondingEntry {
	if len(b) < 4 {
		return nil
	}
	count := binary.BigEndian.Uint32(b[:4])
	off := 4
	entrySize := crypto.AddressLen + 8 + 8
	var entries []types.UnbondingEntry
	for i := 0; i < int(count) && off+entrySize <= len(b); i++ {
		var e types.UnbondingEntry
		copy(e.Address[:], b[off:off+crypto.AddressLen])
		off += crypto.AddressLen
		e.Amount = binary.BigEndian.Uint64(b[off:])
		off += 8
		e.ReleaseHeight = binary.BigEndian.Uint64(b[off:])
		off += 8
		entries = append(entries, e)
	}
	return entries
}

// --- Post stake encoding ---
// Fixed layout: postID(32) + staker(24) + amount(8) + height(8) = 72 bytes

func encodePostStake(ps *types.PostStake) []byte {
	buf := make([]byte, 32+crypto.AddressLen+8+8)
	off := 0
	copy(buf[off:], ps.PostID[:])
	off += 32
	copy(buf[off:], ps.Staker[:])
	off += crypto.AddressLen
	binary.BigEndian.PutUint64(buf[off:], ps.Amount)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], ps.Height)
	return buf
}

func decodePostStake(b []byte) (*types.PostStake, error) {
	size := 32 + crypto.AddressLen + 8 + 8
	if len(b) < size {
		return nil, fmt.Errorf("post stake data too short")
	}
	ps := &types.PostStake{}
	off := 0
	copy(ps.PostID[:], b[off:off+32])
	off += 32
	copy(ps.Staker[:], b[off:off+crypto.AddressLen])
	off += crypto.AddressLen
	ps.Amount = binary.BigEndian.Uint64(b[off:])
	off += 8
	ps.Height = binary.BigEndian.Uint64(b[off:])
	return ps, nil
}

package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/drana-chain/drana/internal/types"
)

// Key prefixes for the block store.
var (
	prefixBlockHeight = []byte("block:height:")
	prefixBlockHash   = []byte("block:hash:")
	prefixTx          = []byte("tx:")
	keyLatestHeight   = []byte("meta:latest_height")
)

// BlockStore persists canonical block history using BadgerDB.
type BlockStore struct {
	db *badger.DB
}

// OpenBlockStore opens or creates a BadgerDB block store at the given path.
func OpenBlockStore(path string) (*BlockStore, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open blockstore: %w", err)
	}
	return &BlockStore{db: db}, nil
}

// Close closes the underlying database.
func (bs *BlockStore) Close() error {
	return bs.db.Close()
}

// SaveBlock persists a block and indexes it by height, hash, and tx hashes.
func (bs *BlockStore) SaveBlock(block *types.Block) error {
	blockBytes, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}

	blockHash := block.Header.Hash()
	heightKey := makeHeightKey(block.Header.Height)
	hashKey := append(append([]byte(nil), prefixBlockHash...), blockHash[:]...)

	return bs.db.Update(func(txn *badger.Txn) error {
		// Block by height
		if err := txn.Set(heightKey, blockBytes); err != nil {
			return err
		}
		// Hash -> height index
		if err := txn.Set(hashKey, encodeUint64(block.Header.Height)); err != nil {
			return err
		}
		// Tx hash -> height index
		for _, tx := range block.Transactions {
			txHash := tx.Hash()
			txKey := append(append([]byte(nil), prefixTx...), txHash[:]...)
			if err := txn.Set(txKey, encodeUint64(block.Header.Height)); err != nil {
				return err
			}
		}
		// Update latest height
		if err := txn.Set(keyLatestHeight, encodeUint64(block.Header.Height)); err != nil {
			return err
		}
		return nil
	})
}

// GetBlockByHeight retrieves a block by its height.
func (bs *BlockStore) GetBlockByHeight(height uint64) (*types.Block, error) {
	var block types.Block
	err := bs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(makeHeightKey(height))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("block at height %d not found", height)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &block)
		})
	})
	if err != nil {
		return nil, err
	}
	return &block, nil
}

// GetBlockByHash retrieves a block by its header hash.
func (bs *BlockStore) GetBlockByHash(hash [32]byte) (*types.Block, error) {
	var height uint64
	err := bs.db.View(func(txn *badger.Txn) error {
		hashKey := append(append([]byte(nil), prefixBlockHash...), hash[:]...)
		item, err := txn.Get(hashKey)
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("block with hash %x not found", hash)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			height = decodeUint64(val)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return bs.GetBlockByHeight(height)
}

// GetLatestBlock returns the most recently saved block.
func (bs *BlockStore) GetLatestBlock() (*types.Block, error) {
	var height uint64
	err := bs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(keyLatestHeight)
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("no blocks stored")
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			height = decodeUint64(val)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return bs.GetBlockByHeight(height)
}

// GetTransaction finds a transaction by its hash and returns the tx plus the block height.
func (bs *BlockStore) GetTransaction(txHash [32]byte) (*types.Transaction, uint64, error) {
	var height uint64
	err := bs.db.View(func(txn *badger.Txn) error {
		txKey := append(append([]byte(nil), prefixTx...), txHash[:]...)
		item, err := txn.Get(txKey)
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("transaction %x not found", txHash)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			height = decodeUint64(val)
			return nil
		})
	})
	if err != nil {
		return nil, 0, err
	}

	block, err := bs.GetBlockByHeight(height)
	if err != nil {
		return nil, 0, err
	}
	for _, tx := range block.Transactions {
		if tx.Hash() == txHash {
			return tx, height, nil
		}
	}
	return nil, 0, fmt.Errorf("transaction %x indexed but not found in block %d", txHash, height)
}

func makeHeightKey(height uint64) []byte {
	key := make([]byte, len(prefixBlockHeight)+8)
	copy(key, prefixBlockHeight)
	binary.BigEndian.PutUint64(key[len(prefixBlockHeight):], height)
	return key
}

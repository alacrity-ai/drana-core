# IMPLEMENTATION_PHASE_1.md

## Phase 1 — Primitives and Local State Machine

### Purpose

This phase builds everything needed to deterministically execute DRANA transactions against world state on a single machine, with no networking. It produces a testable, persist-capable state machine that Phase 2 will wrap in consensus and networking.

### Go Module

```
module github.com/drana-chain/drana
go 1.22
```

### Directory Layout After Phase 1

```
drana/
  go.mod
  go.sum
  cmd/
    drana-node/
      main.go
  internal/
    crypto/
      keys.go
      keys_test.go
      address.go
      address_test.go
    types/
      transaction.go
      block.go
      account.go
      post.go
      genesis.go
      hash.go
    validation/
      validate.go
      validate_test.go
      text.go
      text_test.go
    state/
      state.go
      state_test.go
      executor.go
      executor_test.go
      stateroot.go
      stateroot_test.go
    store/
      kvstore.go
      kvstore_test.go
      blockstore.go
      blockstore_test.go
    genesis/
      genesis.go
      genesis_test.go
      testnet.json
    proto/
      types.proto
      gen.go
  test/
    integration/
      phase1_test.go
```

---

## Step 1 — Project Scaffold

### Files

**`go.mod`**

Initialize the Go module. Add dependencies as needed in later steps (protobuf libraries, BadgerDB or Pebble, testify if desired).

**`cmd/drana-node/main.go`**

Stub entrypoint. In Phase 1 this will only be used for manual integration testing — it loads genesis, executes a hardcoded block sequence, and prints resulting state. No daemon loop, no networking.

### Acceptance Criteria

- `go build ./...` succeeds with zero errors.
- `go test ./...` runs (and trivially passes) with no test files yet.

---

## Step 2 — Cryptography

### Files

**`internal/crypto/keys.go`**

- `GenerateKeyPair() (PublicKey, PrivateKey, error)` — generates an Ed25519 keypair using `crypto/ed25519`.
- `type PublicKey [32]byte`
- `type PrivateKey [64]byte`
- `Sign(privKey PrivateKey, message []byte) []byte` — produces a 64-byte Ed25519 signature.
- `Verify(pubKey PublicKey, message []byte, sig []byte) bool` — verifies an Ed25519 signature.

**`internal/crypto/address.go`**

- `type Address [24]byte` — 20-byte body + 4-byte checksum.
- `AddressFromPublicKey(pubKey PublicKey) Address` — derives address:
  1. `pubKeyHash = SHA-256(pubKey)`
  2. `body = pubKeyHash[:20]`
  3. `checksum = SHA-256(body)[:4]`
  4. `Address = body || checksum`
- `func (a Address) String() string` — returns `drana1` + hex(address bytes). Total display length: 6 prefix chars + 48 hex chars = 54 characters.
- `ParseAddress(s string) (Address, error)` — parses a `drana1`-prefixed hex string, validates checksum.
- `func (a Address) Validate() error` — verifies internal checksum consistency.

**`internal/crypto/keys_test.go`**

- Generate keypair, sign a message, verify succeeds.
- Verify fails with wrong message.
- Verify fails with wrong public key.
- Verify fails with corrupted signature.

**`internal/crypto/address_test.go`**

- Derive address from known public key, confirm deterministic output.
- Round-trip: `ParseAddress(addr.String())` recovers original address.
- `ParseAddress` rejects bad checksum.
- `ParseAddress` rejects wrong prefix.
- `ParseAddress` rejects wrong length.

### Acceptance Criteria

- All tests pass.
- Address derivation is deterministic: same pubkey always produces same address.
- No external crypto dependencies — uses only Go standard library `crypto/ed25519` and `crypto/sha256`.

---

## Step 3 — Core Types

### Files

**`internal/types/account.go`**

```go
type Account struct {
    Address crypto.Address
    Balance uint64  // in microdrana
    Nonce   uint64
}
```

**`internal/types/post.go`**

```go
type PostID [32]byte // SHA-256 of (author address || author nonce at creation)

type Post struct {
    PostID         PostID
    Author         crypto.Address
    Text           string
    CreatedAtHeight uint64
    CreatedAtTime   int64  // unix seconds
    TotalCommitted uint64  // microdrana
    BoostCount     uint64
}
```

**`internal/types/transaction.go`**

```go
type TxType uint8

const (
    TxTransfer   TxType = 1
    TxCreatePost TxType = 2
    TxBoostPost  TxType = 3
)

type Transaction struct {
    Type      TxType
    Sender    crypto.Address
    Recipient crypto.Address  // Transfer only
    PostID    PostID          // BoostPost only
    Text      string          // CreatePost only
    Amount    uint64          // microdrana
    Nonce     uint64
    Signature []byte
    PubKey    crypto.PublicKey
}
```

- `func (tx *Transaction) SignableBytes() []byte` — returns the canonical byte representation of all fields except `Signature`. This is what gets signed. Must be deterministic: fields serialized in fixed order with fixed encoding (length-prefixed, big-endian integers).
- `func (tx *Transaction) Hash() [32]byte` — SHA-256 of `SignableBytes()`.
- `func SignTransaction(tx *Transaction, privKey crypto.PrivateKey)` — sets `tx.Signature` and `tx.PubKey`.

**`internal/types/block.go`**

```go
type BlockHeader struct {
    Height       uint64
    PrevHash     [32]byte
    ProposerAddr crypto.Address
    Timestamp    int64     // unix seconds
    StateRoot    [32]byte
    TxRoot       [32]byte  // merkle root or hash of ordered tx hashes
}

type Block struct {
    Header       BlockHeader
    Transactions []*Transaction
    // Validator signatures omitted in Phase 1 — added in Phase 2
}
```

- `func (h *BlockHeader) Hash() [32]byte` — SHA-256 of deterministically serialized header.
- `func ComputeTxRoot(txs []*Transaction) [32]byte` — ordered hash of transaction hashes.

**`internal/types/hash.go`**

- Utility functions for deterministic serialization of integers and byte slices into hash inputs. Used by `SignableBytes`, `BlockHeader.Hash`, `ComputeTxRoot`, and `PostID` derivation.

**`internal/types/genesis.go`**

```go
type GenesisAccount struct {
    Address crypto.Address
    Balance uint64
}

type GenesisValidator struct {
    Address crypto.Address
    PubKey  crypto.PublicKey
    Name    string
}

type GenesisConfig struct {
    ChainID             string
    GenesisTime         int64
    Accounts            []GenesisAccount
    Validators          []GenesisValidator
    MaxPostLength       int     // code points
    MaxPostBytes        int     // byte cap
    MinPostCommitment   uint64  // microdrana
    MinBoostCommitment  uint64  // microdrana
    MaxTxPerBlock       int
    MaxBlockBytes       int
    BlockIntervalSec    int
    BlockReward         uint64  // microdrana minted per block, credited to proposer
}
```

### Acceptance Criteria

- `Transaction.SignableBytes()` is deterministic: identical fields always produce identical bytes.
- `PostID` derivation is deterministic from author address + author nonce.
- `BlockHeader.Hash()` is deterministic.
- `SignTransaction` + `Verify` round-trip succeeds for all three tx types.
- Types compile cleanly with no unused fields.

---

## Step 4 — Text Validation

### Files

**`internal/validation/text.go`**

- `NormalizePostText(raw string) (string, error)` — applies Unicode NFC normalization, trims leading/trailing whitespace, collapses internal whitespace runs. Returns error if result is empty.
- `ValidatePostText(text string, maxCodePoints int, maxBytes int) error` — checks:
  1. Valid UTF-8.
  2. Text has been normalized (caller must normalize first, or this function detects non-NFC input).
  3. Code point count <= `maxCodePoints` (280 default).
  4. Byte length <= `maxBytes` (1024 default).
  5. Non-empty after normalization.

**`internal/validation/text_test.go`**

- ASCII text within limits passes.
- Multi-byte UTF-8 (CJK, emoji) within 280 code points but over 280 bytes passes (under byte cap).
- Text at exactly 280 code points passes.
- Text at 281 code points fails.
- Text over 1024 bytes fails even if under 280 code points.
- Empty string fails.
- Whitespace-only string fails after normalization.
- Invalid UTF-8 byte sequence fails.
- Unnormalized Unicode (NFD when NFC expected) is handled correctly.

### Acceptance Criteria

- All tests pass.
- Validation uses `unicode/utf8` and `golang.org/x/text/unicode/norm` (the one external dependency here, or vendored).

---

## Step 5 — Transaction Validation

### Files

**`internal/validation/validate.go`**

- `ValidateTransaction(tx *types.Transaction, stateReader StateReader, params *types.GenesisConfig) error`

  `StateReader` is an interface:
  ```go
  type StateReader interface {
      GetAccount(addr crypto.Address) (*types.Account, error)
      GetPost(id types.PostID) (*types.Post, error)
  }
  ```

  Validation logic per tx type:

  **Transfer:**
  1. Amount > 0.
  2. Sender != Recipient.
  3. PubKey derives to Sender address.
  4. Signature valid over `tx.SignableBytes()`.
  5. Sender account exists.
  6. Nonce == account.Nonce + 1.
  7. Balance >= Amount.

  **CreatePost:**
  1. Amount >= `MinPostCommitment`.
  2. PubKey derives to Sender address.
  3. Signature valid.
  4. Sender account exists.
  5. Nonce == account.Nonce + 1.
  6. Balance >= Amount.
  7. `ValidatePostText(tx.Text, params.MaxPostLength, params.MaxPostBytes)` passes.

  **BoostPost:**
  1. Amount >= `MinBoostCommitment`.
  2. PubKey derives to Sender address.
  3. Signature valid.
  4. Sender account exists.
  5. Nonce == account.Nonce + 1.
  6. Balance >= Amount.
  7. Post referenced by `tx.PostID` exists.

**`internal/validation/validate_test.go`**

For each of the three tx types, test every validation rule:
- Valid transaction passes.
- Each individual rule violation produces the correct error (bad sig, wrong nonce, insufficient balance, post doesn't exist, text too long, amount below minimum, etc.).
- Tests use an in-memory mock `StateReader`.

### Acceptance Criteria

- All tests pass.
- Every validation rule from DESIGN.md sections 8.1–8.3 is enforced.
- Validation returns typed errors that identify which rule failed.

---

## Step 6 — State Machine and Executor

### Files

**`internal/state/state.go`**

In-memory world state that implements `StateReader` and provides mutation:

```go
type WorldState struct { ... }

func NewWorldState() *WorldState
func (ws *WorldState) GetAccount(addr crypto.Address) (*types.Account, error)
func (ws *WorldState) SetAccount(acct *types.Account)
func (ws *WorldState) GetPost(id types.PostID) (*types.Post, error)
func (ws *WorldState) SetPost(post *types.Post)
func (ws *WorldState) GetBurnedSupply() uint64
func (ws *WorldState) SetBurnedSupply(amount uint64)
func (ws *WorldState) GetIssuedSupply() uint64
func (ws *WorldState) SetIssuedSupply(amount uint64)
func (ws *WorldState) GetChainHeight() uint64
func (ws *WorldState) SetChainHeight(h uint64)
func (ws *WorldState) NextPostID(author crypto.Address, nonce uint64) types.PostID
func (ws *WorldState) Clone() *WorldState  // for speculative execution / rollback
```

**`internal/state/state_test.go`**

- Set and get accounts, posts, burned supply, issued supply.
- Clone produces independent copy — mutations to clone do not affect original.

**`internal/state/executor.go`**

```go
type Executor struct {
    Params *types.GenesisConfig
}

func (e *Executor) ApplyTransaction(ws *WorldState, tx *types.Transaction, blockHeight uint64, blockTime int64) error
func (e *Executor) ApplyBlock(ws *WorldState, block *types.Block) (*WorldState, error)
```

`ApplyTransaction` — assumes tx is already validated. Executes the state transition:

- **Transfer:** debit sender, credit recipient, increment sender nonce.
- **CreatePost:** debit author, increment author nonce, create Post with `TotalCommitted = Amount`, increment burned supply.
- **BoostPost:** debit booster, increment booster nonce, increment post's `TotalCommitted` and `BoostCount`, increment burned supply.

`ApplyBlock` — clones state, validates and applies each transaction in order against the clone. If any tx fails, returns error and original state is untouched. On success:
- credits `block_reward` to the proposer's account (creating the account if it does not exist),
- increments `issuedSupply` by `block_reward`,
- sets chain height,
- returns the new state.

Block reward issuance happens **after** all transactions are applied, so minted coins are not spendable within the same block they are issued.

**`internal/state/executor_test.go`**

- **Transfer:** Apply a transfer, verify sender debited, recipient credited, nonces updated.
- **CreatePost:** Apply a create post, verify author debited, post exists with correct fields, burned supply incremented.
- **BoostPost:** Apply a boost, verify booster debited, post's `TotalCommitted` and `BoostCount` updated, burned supply incremented.
- **Self-boost:** Author boosts own post — must succeed.
- **Sequential nonces:** Two transactions from same sender in one block — both apply correctly with incrementing nonces.
- **Balance underflow:** Second tx that would overdraw fails, first tx's effects are rolled back (entire block fails).
- **Full block apply:** Block with mixed tx types produces correct final state.
- **Block reward:** ApplyBlock credits proposer with block reward, issued supply incremented.
- **Block reward creates account:** If proposer has no prior account, one is created with balance = block reward.
- **Supply conservation:** After ApplyBlock, `sum(balances) == genesis_supply + issuedSupply - burnedSupply`.

**`internal/state/stateroot.go`**

```go
func ComputeStateRoot(ws *WorldState) [32]byte
```

Produces a deterministic hash of the entire world state. Implementation: sort all accounts by address, sort all posts by PostID, serialize each deterministically, hash the concatenation along with burned supply, issued supply, and chain height.

**`internal/state/stateroot_test.go`**

- Same state always produces same root.
- Different states produce different roots.
- Order of insertion does not affect root (because we sort).

### Acceptance Criteria

- All tests pass.
- `ApplyBlock` is atomic: either all transactions succeed or state is unchanged.
- `ComputeStateRoot` is deterministic regardless of insertion order.
- All state transitions match DESIGN.md sections 8.1–8.3 exactly.
- Balance can never go negative (uint64 underflow is caught before subtraction).
- Burned supply accounting is exact: sum of all post `TotalCommitted` values equals burned supply.
- Supply conservation holds: `sum(balances) == genesis_supply + issuedSupply - burnedSupply`.

---

## Step 7 — Persistence

### Files

**`internal/store/kvstore.go`**

State persistence using an embedded KV store (BadgerDB or Pebble — choose one at implementation time).

```go
type KVStore struct { ... }

func OpenKVStore(path string) (*KVStore, error)
func (kv *KVStore) Close() error
func (kv *KVStore) SaveState(ws *state.WorldState) error
func (kv *KVStore) LoadState() (*state.WorldState, error)
```

Key layout:
- `account:<address_hex>` → serialized Account
- `post:<postid_hex>` → serialized Post
- `meta:burned_supply` → uint64
- `meta:issued_supply` → uint64
- `meta:chain_height` → uint64

**`internal/store/kvstore_test.go`**

- Save state, close, reopen, load — all accounts, posts, and counters match.
- Overwrite state with updated values, reload — new values present.
- Empty state loads cleanly as zero-valued world state.

**`internal/store/blockstore.go`**

Append-only block history.

```go
type BlockStore struct { ... }

func OpenBlockStore(path string) (*BlockStore, error)
func (bs *BlockStore) Close() error
func (bs *BlockStore) SaveBlock(block *types.Block) error
func (bs *BlockStore) GetBlockByHeight(height uint64) (*types.Block, error)
func (bs *BlockStore) GetBlockByHash(hash [32]byte) (*types.Block, error)
func (bs *BlockStore) GetLatestBlock() (*types.Block, error)
func (bs *BlockStore) GetTransaction(txHash [32]byte) (*types.Transaction, uint64, error) // tx, block height, error
```

Key layout:
- `block:height:<height_be8>` → serialized Block
- `block:hash:<hash_hex>` → height (for hash-to-height index)
- `tx:<txhash_hex>` → height (for tx lookup)
- `meta:latest_height` → uint64

**`internal/store/blockstore_test.go`**

- Save block at height 1, retrieve by height and by hash — identical.
- Save multiple blocks, `GetLatestBlock` returns the most recent.
- `GetTransaction` returns correct tx and block height.
- Request nonexistent height/hash returns appropriate error.

### Acceptance Criteria

- All tests pass.
- Data survives process restart (close + reopen).
- Block store is append-only: no update or delete operations on blocks.
- Serialization format is deterministic (same data always produces same bytes on disk).

---

## Step 8 — Genesis Loading

### Files

**`internal/genesis/genesis.go`**

```go
func LoadGenesis(path string) (*types.GenesisConfig, error)
func InitializeState(cfg *types.GenesisConfig) (*state.WorldState, error)
```

- `LoadGenesis` reads a JSON genesis file and returns a `GenesisConfig`.
- `InitializeState` creates a `WorldState` with:
  - One `Account` per genesis account entry, with the specified balance and nonce 0.
  - Chain height 0.
  - Burned supply 0.
  - Validates that total genesis supply does not overflow uint64.
  - Validates that all genesis addresses have valid checksums.

**`internal/genesis/testnet.json`**

A sample genesis file for local testing:

```json
{
  "chainId": "drana-testnet-1",
  "genesisTime": 1700000000,
  "accounts": [
    { "address": "drana1...", "balance": 1000000000000 },
    { "address": "drana1...", "balance": 1000000000000 },
    { "address": "drana1...", "balance": 1000000000000 }
  ],
  "validators": [
    { "address": "drana1...", "pubKey": "...", "name": "validator-1" },
    { "address": "drana1...", "pubKey": "...", "name": "validator-2" },
    { "address": "drana1...", "pubKey": "...", "name": "validator-3" }
  ],
  "maxPostLength": 280,
  "maxPostBytes": 1024,
  "minPostCommitment": 1000000,
  "minBoostCommitment": 100000,
  "maxTxPerBlock": 100,
  "maxBlockBytes": 1048576,
  "blockIntervalSec": 120,
  "blockReward": 10000000
}
```

Values above: min post = 1 DRANA, min boost = 0.1 DRANA, each test account gets 1,000,000 DRANA, block reward = 10 DRANA per block.

**`internal/genesis/genesis_test.go`**

- Load valid genesis file, verify all fields parsed correctly.
- `InitializeState` produces world state with correct account count and balances.
- Total supply across genesis accounts is correct.
- Invalid genesis (duplicate address, bad checksum, overflow supply) returns error.

### Acceptance Criteria

- All tests pass.
- Genesis loading is the only way to create initial state — there is no hardcoded bootstrap path.
- `testnet.json` is a working genesis file that the integration test uses.

---

## Step 9 — Protobuf Definitions

### Files

**`internal/proto/types.proto`**

Canonical Protobuf definitions for all serializable structures. These are used for:
- deterministic on-disk serialization in the KV store and block store,
- wire format foundation for Phase 2 gRPC,
- `SignableBytes` encoding for transactions.

```protobuf
syntax = "proto3";
package drana.v1;
option go_package = "github.com/drana-chain/drana/internal/proto/pb";

message Account {
  bytes address = 1;   // 24 bytes
  uint64 balance = 2;
  uint64 nonce = 3;
}

message Post {
  bytes post_id = 1;   // 32 bytes
  bytes author = 2;    // 24 bytes
  string text = 3;
  uint64 created_at_height = 4;
  int64 created_at_time = 5;
  uint64 total_committed = 6;
  uint64 boost_count = 7;
}

message Transaction {
  uint32 type = 1;
  bytes sender = 2;
  bytes recipient = 3;
  bytes post_id = 4;
  string text = 5;
  uint64 amount = 6;
  uint64 nonce = 7;
  bytes signature = 8;
  bytes pub_key = 9;
}

message BlockHeader {
  uint64 height = 1;
  bytes prev_hash = 2;
  bytes proposer_addr = 3;
  int64 timestamp = 4;
  bytes state_root = 5;
  bytes tx_root = 6;
}

message Block {
  BlockHeader header = 1;
  repeated Transaction transactions = 2;
}

message GenesisConfig {
  string chain_id = 1;
  int64 genesis_time = 2;
  repeated GenesisAccount accounts = 3;
  repeated GenesisValidator validators = 4;
  int32 max_post_length = 5;
  int32 max_post_bytes = 6;
  uint64 min_post_commitment = 7;
  uint64 min_boost_commitment = 8;
  int32 max_tx_per_block = 9;
  int32 max_block_bytes = 10;
  int32 block_interval_sec = 11;
  uint64 block_reward = 12;
}

message GenesisAccount {
  bytes address = 1;
  uint64 balance = 2;
}

message GenesisValidator {
  bytes address = 1;
  bytes pub_key = 2;
  string name = 3;
}
```

**`internal/proto/gen.go`**

```go
//go:generate protoc --go_out=. --go_opt=paths=source_relative types.proto
```

Conversion functions between Go domain types (`internal/types/`) and protobuf-generated types live alongside each domain type file (e.g., `account.go` gets `func (a *Account) ToProto()` and `func AccountFromProto()`). This keeps serialization close to the types without polluting the proto package.

### Acceptance Criteria

- `go generate ./internal/proto/` succeeds and produces Go code.
- Round-trip: domain type -> proto -> bytes -> proto -> domain type produces identical values for all types.
- Serialization is deterministic: same domain object always produces same bytes.

---

## Step 10 — Integration Test

### Files

**`test/integration/phase1_test.go`**

A single end-to-end test that exercises the full Phase 1 stack:

1. Load genesis from `testnet.json`.
2. Initialize world state from genesis.
3. Persist initial state to disk (KV store).
4. Construct and sign a `Transfer` transaction (Alice sends 100 DRANA to Bob).
5. Construct and sign a `CreatePost` transaction (Alice posts "The empire of relevance belongs to the highest bidder." with 200 DRANA).
6. Construct and sign a `BoostPost` transaction (Bob boosts Alice's post with 75 DRANA).
7. Assemble these three transactions into a `Block` at height 1.
8. Compute `TxRoot` and `StateRoot` (pre-compute by applying to cloned state).
9. Apply the block via `Executor.ApplyBlock`.
10. Verify resulting state matches DESIGN.md section 23 examples (adjusted for block reward):
    - Alice balance = genesis - 100 (transfer) - 200 (post) = starting - 300.
    - Bob balance = genesis + 100 (received) - 75 (boost) = starting + 25.
    - Proposer balance = block_reward (created if not already an account).
    - Post `TotalCommitted` = 275.
    - Post `BoostCount` = 1.
    - Burned supply = 275.
    - Issued supply = block_reward.
    - Supply conservation: `sum(balances) == genesis_supply + issuedSupply - burnedSupply`.
11. Compute state root, set it in block header.
12. Persist block to block store.
13. Persist final state to KV store.
14. Close all stores.
15. Reopen stores, load state and latest block.
16. Verify loaded state matches pre-close state exactly (state root comparison).
17. Verify loaded block matches saved block exactly.

**`cmd/drana-node/main.go`** (updated)

Wired up to run the same sequence as the integration test but with log output, for manual verification. Reads genesis path from a command-line flag.

### Acceptance Criteria

- Integration test passes.
- State root computed before persistence matches state root computed after reload.
- All numeric values match the worked examples in DESIGN.md section 23.
- No panics, no negative balances.
- Supply conservation invariant holds: `sum(balances) == genesis_supply + issuedSupply - burnedSupply`.

---

## Phase 1 Exit Criteria (Summary)

All of the following must be true before moving to Phase 2:

1. `go build ./...` succeeds.
2. `go test ./...` passes with all unit and integration tests green.
3. A single process can: load genesis, execute a multi-transaction block, persist state and blocks, restart, and reload identical state.
4. State root computation is deterministic regardless of operation order.
5. Every validation rule and invariant from DESIGN.md sections 8 and 19 is tested and enforced.
6. Burned supply accounting is exact at all times.
6b. Issued supply accounting is exact at all times.
6c. Supply conservation invariant holds at every block height.
7. No transaction can produce a negative balance.
8. Post text immutability is enforced — no mutation path exists in the API.

---

## Dependency Summary

| Dependency | Purpose |
|---|---|
| Go standard library (`crypto/ed25519`, `crypto/sha256`, `encoding/hex`, `unicode/utf8`) | Crypto, hashing, encoding |
| `golang.org/x/text/unicode/norm` | Unicode NFC normalization for post text |
| `google.golang.org/protobuf` | Protobuf serialization |
| BadgerDB or Pebble (choose one) | Embedded KV store for state and block persistence |
| `github.com/stretchr/testify` (optional) | Test assertions |

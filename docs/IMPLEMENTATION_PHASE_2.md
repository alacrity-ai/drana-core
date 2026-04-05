# IMPLEMENTATION_PHASE_2.md

## Phase 2 — Consensus and Networking

### Purpose

This phase turns the single-process state machine from Phase 1 into a replicated multi-validator network. After Phase 2, multiple independent nodes will propose blocks, vote on them, finalize them with a 2/3 quorum, and converge on identical chain state.

### Prerequisites

All Phase 1 exit criteria must be met. In particular, the following Phase 1 interfaces are consumed directly by Phase 2:

- `state.Executor.ApplyBlock(ws, block) (*WorldState, error)` — atomic block execution
- `state.ComputeStateRoot(ws) [32]byte` — deterministic state hashing
- `validation.ValidateTransaction(tx, stateReader, params) error` — tx-level validation
- `store.KVStore.SaveState / LoadState` — state persistence
- `store.BlockStore.SaveBlock / GetBlockByHeight / GetLatestBlock` — block persistence
- `genesis.LoadGenesis / InitializeState` — chain bootstrapping
- `types.Block`, `types.BlockHeader`, `types.Transaction` — canonical data structures
- `crypto.Sign / Verify / GenerateKeyPair` — Ed25519 operations

### What This Phase Adds to the Directory Layout

```
internal/
  consensus/
    proposer.go
    proposer_test.go
    validator.go
    validator_test.go
    engine.go
    engine_test.go
  mempool/
    mempool.go
    mempool_test.go
  p2p/
    server.go
    client.go
    peer.go
    peer_test.go
  node/
    node.go
    node_test.go
    config.go
  proto/
    consensus.proto       (new)
    types.proto            (updated — add validator signatures to Block)
    pb/                    (generated code)
test/
  integration/
    phase2_test.go
```

---

## Step 1 — Protobuf Code Generation and Wire Types

### Files

**`internal/proto/consensus.proto`**

gRPC service and message definitions for validator-to-validator communication.

```protobuf
syntax = "proto3";
package drana.v1;
option go_package = "github.com/drana-chain/drana/internal/proto/pb";

import "types.proto";

// --- Consensus messages ---

message BlockProposal {
  Block block = 1;
}

message BlockVote {
  uint64 height = 1;
  bytes block_hash = 2;        // 32 bytes — hash of the proposed block header
  bytes voter_address = 3;     // 24 bytes
  bytes voter_pub_key = 4;     // 32 bytes
  bytes signature = 5;         // 64 bytes — Ed25519 sig over (height || block_hash)
}

message QuorumCertificate {
  uint64 height = 1;
  bytes block_hash = 2;
  repeated BlockVote votes = 3;
}

message FinalizedBlock {
  Block block = 1;
  QuorumCertificate qc = 2;
}

// --- Sync messages ---

message SyncRequest {
  uint64 from_height = 1;
  uint64 to_height = 2;       // 0 = "up to your latest"
}

message SyncResponse {
  repeated FinalizedBlock blocks = 1;
}

message PeerStatus {
  bytes address = 1;
  uint64 latest_height = 2;
  bytes latest_block_hash = 3;
  string chain_id = 4;
}

// --- Transaction relay ---

message TxSubmission {
  Transaction tx = 1;
}

message TxSubmissionResponse {
  bool accepted = 1;
  string error = 2;
}

// --- gRPC Service ---

service ConsensusService {
  // Block proposal: proposer sends to each validator.
  rpc ProposeBlock(BlockProposal) returns (BlockVote);

  // Vote collection is implicit: proposer calls ProposeBlock on each peer
  // and collects the returned BlockVotes.

  // Finalized block broadcast.
  rpc NotifyFinalizedBlock(FinalizedBlock) returns (PeerStatus);

  // Chain sync: a catching-up node requests a range of finalized blocks.
  rpc SyncBlocks(SyncRequest) returns (SyncResponse);

  // Peer status exchange.
  rpc GetStatus(PeerStatus) returns (PeerStatus);

  // Transaction relay: any node can forward a tx to peers.
  rpc SubmitTx(TxSubmission) returns (TxSubmissionResponse);
}
```

**`internal/proto/types.proto`** (updated)

Add `QuorumCertificate` reference to `Block` message:

```protobuf
message Block {
  BlockHeader header = 1;
  repeated Transaction transactions = 2;
  QuorumCertificate qc = 3;   // added in Phase 2
}
```

**`internal/proto/gen.go`**

```go
//go:generate protoc --go_out=. --go_opt=paths=source_relative \
//   --go-grpc_out=. --go-grpc_opt=paths=source_relative \
//   types.proto consensus.proto
```

**Go domain type updates:**

Add to `internal/types/block.go`:

```go
type BlockVote struct {
    Height     uint64
    BlockHash  [32]byte
    VoterAddr  crypto.Address
    VoterPubKey crypto.PublicKey
    Signature  []byte
}

type QuorumCertificate struct {
    Height    uint64
    BlockHash [32]byte
    Votes     []BlockVote
}
```

Add `QC *QuorumCertificate` field to `Block` struct. The QC is **not** included in `BlockHeader.Hash()` (the header hash is what gets voted on — the QC is attached afterward).

Signing and verification helpers:

```go
func (v *BlockVote) SignableBytes() []byte   // height || block_hash
func SignBlockVote(vote *BlockVote, privKey crypto.PrivateKey)
func VerifyBlockVote(vote *BlockVote) bool
```

### Acceptance Criteria

- `go generate ./internal/proto/` produces Go + gRPC code.
- Round-trip: domain types ↔ protobuf ↔ bytes ↔ protobuf ↔ domain types for all consensus messages.
- `BlockVote` signing/verification round-trips correctly.
- `BlockHeader.Hash()` does not change when QC is added to the block (no hash breakage from Phase 1).

---

## Step 2 — Proposer Selection

### Files

**`internal/consensus/proposer.go`**

```go
func ProposerForHeight(validators []types.GenesisValidator, height uint64) types.GenesisValidator
```

Deterministic round-robin: `validators[height % len(validators)]`.

Also:

```go
func IsProposer(validators []types.GenesisValidator, height uint64, addr crypto.Address) bool
```

**`internal/consensus/proposer_test.go`**

- 3 validators, heights 1–9: verify exact round-robin rotation.
- Single validator: always selected.
- `IsProposer` returns true only for the correct validator at each height.

### Acceptance Criteria

- Proposer selection is deterministic and consistent across calls.
- No off-by-one: height 1 selects `validators[1 % N]`, not `validators[0]` (height 0 is genesis, no block is proposed at height 0).

---

## Step 3 — Mempool

### Files

**`internal/mempool/mempool.go`**

```go
type Mempool struct { ... }

func New(maxSize int) *Mempool
func (m *Mempool) Add(tx *types.Transaction) error
func (m *Mempool) Remove(txHashes [][32]byte)
func (m *Mempool) Has(txHash [32]byte) bool
func (m *Mempool) Pending() []*types.Transaction
func (m *Mempool) ReapForBlock(stateReader validation.StateReader, params *types.GenesisConfig, maxTx int) []*types.Transaction
func (m *Mempool) Size() int
func (m *Mempool) Flush()
```

Behavior:

- `Add` — accepts a transaction if:
  1. Signature is valid.
  2. Sender pubkey matches sender address.
  3. Transaction hash is not already in the pool.
  4. Pool is not at capacity.
  Does **not** check nonce or balance (those are checked at proposal time against current state).

- `ReapForBlock` — returns an ordered list of transactions suitable for inclusion in a block:
  1. Groups pending txs by sender.
  2. Within each sender, sorts by nonce ascending.
  3. For each sender, includes transactions sequentially starting from `account.Nonce + 1`, skipping if nonce gap exists.
  4. Validates each candidate tx against evolving speculative state (balance, nonce).
  5. Stops when `maxTx` is reached.

- `Remove` — called after a block is finalized to evict included transactions.

- Thread-safe: all methods are safe for concurrent use (the mempool is accessed by the gRPC server, the proposer goroutine, and the finalization path).

**`internal/mempool/mempool_test.go`**

- Add valid tx, verify `Size() == 1` and `Has()` returns true.
- Add duplicate tx hash, verify rejected.
- `ReapForBlock` returns nonce-ordered txs that pass validation.
- `ReapForBlock` skips txs with nonce gaps.
- `ReapForBlock` skips txs with insufficient balance.
- `Remove` evicts txs after block finalization.
- Concurrent `Add` from multiple goroutines does not panic or corrupt state.
- Pool at capacity rejects new adds.

### Acceptance Criteria

- All tests pass.
- `ReapForBlock` never produces a set that would fail `Executor.ApplyBlock`.
- Thread-safe under concurrent access.

---

## Step 4 — Block Validation (Consensus-Level)

### Files

**`internal/consensus/validator.go`**

Full block validation that a non-proposer node performs upon receiving a `BlockProposal`:

```go
func ValidateProposedBlock(
    block *types.Block,
    currentState *state.WorldState,
    lastBlock *types.Block,       // nil for height 1
    validators []types.GenesisValidator,
    params *types.GenesisConfig,
) error
```

Checks:

1. **Height continuity:** `block.Height == lastBlock.Height + 1` (or `== 1` if no previous block).
2. **Parent hash:** `block.PrevHash == lastBlock.Header.Hash()` (or zero hash if height 1).
3. **Proposer identity:** `block.ProposerAddr` matches `ProposerForHeight(validators, block.Height).Address`.
4. **Timestamp rules:**
   - `block.Timestamp > lastBlock.Timestamp` (or `>= genesisTime` for height 1).
   - `block.Timestamp <= time.Now().Unix() + AllowedFutureDrift` (15 seconds).
5. **Transaction root:** `block.TxRoot == ComputeTxRoot(block.Transactions)`.
6. **Transaction count:** `len(block.Transactions) <= params.MaxTxPerBlock`.
7. **State execution:** Apply block via `Executor.ApplyBlock` — if it fails, block is invalid.
8. **State root:** `block.StateRoot == ComputeStateRoot(resultState)`.

Returns nil if valid, typed error otherwise.

**`internal/consensus/validator_test.go`**

- Valid block passes.
- Wrong height fails.
- Wrong parent hash fails.
- Wrong proposer fails.
- Timestamp in the past (before parent) fails.
- Timestamp too far in the future fails.
- Wrong tx root fails.
- Too many transactions fails.
- Invalid transaction inside block fails.
- Wrong state root fails.

### Acceptance Criteria

- All tests pass.
- A block that passes `ValidateProposedBlock` will always succeed in `Executor.ApplyBlock`.
- Validation is deterministic: same inputs always produce same result.

---

## Step 5 — Consensus Engine

### Files

**`internal/consensus/engine.go`**

The consensus engine orchestrates the block lifecycle for a single node. It runs as a long-lived goroutine.

```go
type Engine struct {
    config       *node.Config
    params       *types.GenesisConfig
    validators   []types.GenesisValidator
    privKey      crypto.PrivateKey
    address      crypto.Address
    state        *state.WorldState
    executor     *state.Executor
    mempool      *mempool.Mempool
    blockStore   *store.BlockStore
    kvStore      *store.KVStore
    peers        *p2p.PeerManager
    currentHeight uint64
    lastBlock    *types.Block
}

func NewEngine(...) *Engine
func (e *Engine) Start(ctx context.Context) error
func (e *Engine) Stop()
```

**Proposer path** (when this node is proposer for the current height):

1. Wait until the block interval has elapsed since the last block timestamp.
2. Call `mempool.ReapForBlock(...)` to collect candidate transactions.
3. Build `BlockHeader`:
   - Height = currentHeight + 1
   - PrevHash = lastBlock.Header.Hash()
   - ProposerAddr = self address
   - Timestamp = time.Now().Unix()
   - TxRoot = ComputeTxRoot(txs)
   - StateRoot = (computed after trial execution)
4. Trial-execute the block via `Executor.ApplyBlock` to compute the state root.
5. Set `block.Header.StateRoot`.
6. Send `BlockProposal` to each peer via gRPC.
7. Collect `BlockVote` responses.
8. Once >= 2/3 votes collected (including own vote), assemble `QuorumCertificate`.
9. Attach QC to block.
10. Call `NotifyFinalizedBlock` on each peer.
11. Commit: persist block and new state, advance `currentHeight`, remove txs from mempool.

**Voter path** (when this node is not the proposer):

1. Receive `BlockProposal` via gRPC.
2. Run `ValidateProposedBlock(...)`.
3. If valid, sign a `BlockVote` and return it.
4. If invalid, return an error (no vote).
5. Wait for `FinalizedBlock` notification.
6. Verify QC: check that >= 2/3 votes are present and all signatures are valid.
7. Commit: persist block and new state, advance `currentHeight`, remove txs from mempool.

**Timeout handling:**

- If the proposer does not receive enough votes within a timeout (e.g., 2x block interval), the round fails. The next height's proposer will propose.
- If a voter does not receive a proposal within a timeout, it does nothing — the next proposer will advance the chain.
- Empty blocks are valid: if the mempool is empty, the proposer still proposes an empty block to keep the chain advancing and collecting block rewards.

**`internal/consensus/engine_test.go`**

- Single-validator engine proposes and self-finalizes a block.
- Engine correctly identifies when it is / is not the proposer.
- Engine rejects a proposal from the wrong proposer.
- Engine produces valid QC with enough votes.
- Engine rejects QC with insufficient votes.
- Engine rejects QC with invalid vote signatures.

### Acceptance Criteria

- All tests pass.
- Engine cleanly starts and stops via context cancellation.
- Block production rate approximates the configured interval.
- All blocks produced satisfy `ValidateProposedBlock`.

---

## Step 6 — P2P Layer

### Files

**`internal/p2p/server.go`**

Implements the `ConsensusService` gRPC server:

```go
type Server struct {
    engine  *consensus.Engine
    mempool *mempool.Mempool
    // ... references to state, stores as needed
}

func NewServer(...) *Server
func (s *Server) Start(listenAddr string) error
func (s *Server) Stop()
```

gRPC method implementations:
- `ProposeBlock` — delegates to `ValidateProposedBlock`, returns signed vote or error.
- `NotifyFinalizedBlock` — verifies QC, commits block, returns own status.
- `SyncBlocks` — reads requested range from block store, returns finalized blocks.
- `GetStatus` — returns this node's current height, latest block hash, chain ID.
- `SubmitTx` — adds transaction to mempool, optionally relays to peers.

**`internal/p2p/client.go`**

gRPC client wrapper for outbound calls to a single peer:

```go
type Client struct {
    conn   *grpc.ClientConn
    client pb.ConsensusServiceClient
    addr   string
}

func Dial(addr string) (*Client, error)
func (c *Client) Close() error
func (c *Client) ProposeBlock(ctx context.Context, block *types.Block) (*types.BlockVote, error)
func (c *Client) NotifyFinalized(ctx context.Context, block *types.Block, qc *types.QuorumCertificate) (*PeerStatus, error)
func (c *Client) SyncBlocks(ctx context.Context, from, to uint64) ([]*FinalizedBlock, error)
func (c *Client) GetStatus(ctx context.Context) (*PeerStatus, error)
func (c *Client) SubmitTx(ctx context.Context, tx *types.Transaction) (bool, error)
```

**`internal/p2p/peer.go`**

Manages the set of known peers and their connections:

```go
type PeerManager struct {
    peers map[crypto.Address]*Client
}

func NewPeerManager(validators []types.GenesisValidator, selfAddr crypto.Address, peerEndpoints map[crypto.Address]string) *PeerManager
func (pm *PeerManager) Connect(ctx context.Context) error
func (pm *PeerManager) Close()
func (pm *PeerManager) Peers() []*Client
func (pm *PeerManager) BroadcastTx(ctx context.Context, tx *types.Transaction)
```

Peer endpoints are provided via node config (static peer configuration per DESIGN.md section 21.2). The `PeerManager` dials all peers at startup and maintains the connections.

**`internal/p2p/peer_test.go`**

- `PeerManager` correctly excludes self from peer list.
- `PeerManager` connects to all configured peers (using a local test gRPC server).

### Acceptance Criteria

- gRPC server starts, accepts connections, and responds to all RPC methods.
- Client correctly serializes/deserializes all message types.
- `PeerManager` maintains connections and correctly identifies peers.

---

## Step 7 — Node Configuration

### Files

**`internal/node/config.go`**

```go
type Config struct {
    GenesisPath    string                         // path to genesis.json
    DataDir        string                         // root directory for state + blocks
    PrivKeyHex     string                         // this validator's Ed25519 private key (hex)
    ListenAddr     string                         // gRPC listen address (e.g., "0.0.0.0:26600")
    PeerEndpoints  map[string]string              // validator name -> "host:port"
}

func LoadConfig(path string) (*Config, error)     // JSON config file
```

**`internal/node/node.go`**

Top-level node wiring that connects all components:

```go
type Node struct {
    config    *Config
    genesis   *types.GenesisConfig
    state     *state.WorldState
    executor  *state.Executor
    mempool   *mempool.Mempool
    blockStore *store.BlockStore
    kvStore    *store.KVStore
    engine    *consensus.Engine
    p2pServer *p2p.Server
    peers     *p2p.PeerManager
}

func NewNode(cfg *Config) (*Node, error)
func (n *Node) Start(ctx context.Context) error
func (n *Node) Stop() error
```

`NewNode`:
1. Loads genesis config.
2. Opens (or creates) KV store and block store.
3. If state exists on disk, loads it; otherwise, initializes from genesis.
4. Derives this node's address from the private key.
5. Verifies this node is in the genesis validator set.
6. Creates mempool, executor, peer manager, consensus engine, gRPC server.

`Start`:
1. Connects to peers.
2. Runs chain sync if behind (requests missing blocks from peers).
3. Starts gRPC server.
4. Starts consensus engine main loop.

`Stop`:
1. Cancels context → engine and server shut down.
2. Closes peer connections.
3. Persists final state.
4. Closes stores.

**`internal/node/node_test.go`**

- `NewNode` with valid config initializes all components.
- `NewNode` fails if private key does not correspond to a genesis validator.
- `Start` and `Stop` complete without error in a single-node configuration.

### Acceptance Criteria

- A single node starts, produces blocks (as sole proposer in a 1-validator setup), persists them, and shuts down cleanly.
- State survives restart: node loads persisted state on second start and continues from where it left off.

---

## Step 8 — Chain Sync

### Files

Chain sync logic lives within the consensus engine but is described separately for clarity.

**In `internal/consensus/engine.go`:**

```go
func (e *Engine) syncToNetwork(ctx context.Context) error
```

On startup (or after detecting that peers are ahead):

1. Query each peer via `GetStatus` to find the highest known height.
2. If this node is behind, call `SyncBlocks(from=myHeight+1, to=peerHeight)` on the most-ahead peer.
3. For each received `FinalizedBlock`:
   a. Verify the QC (>= 2/3 valid vote signatures).
   b. Run `ValidateProposedBlock` against current state.
   c. Commit: apply block, persist state and block, advance height.
4. After catching up, enter normal consensus loop.

**Edge cases:**

- If the peer returns an invalid block, try another peer.
- If no peers are available, wait and retry.
- Sync is bounded: request at most 100 blocks per `SyncBlocks` call to limit memory usage.

### Acceptance Criteria

- A node started late can catch up to the network by syncing blocks from peers.
- Synced blocks pass full validation — a malicious peer cannot feed invalid blocks.
- After sync, the node participates in consensus normally.

---

## Step 9 — Update `cmd/drana-node/main.go`

### Files

**`cmd/drana-node/main.go`**

Replace the Phase 1 stub with a full node entrypoint:

```go
func main() {
    configPath := flag.String("config", "", "path to node config JSON")
    flag.Parse()

    cfg, err := node.LoadConfig(*configPath)
    // ...
    n, err := node.NewNode(cfg)
    // ...
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    if err := n.Start(ctx); err != nil {
        log.Fatalf("node stopped: %v", err)
    }
    n.Stop()
}
```

Behavior:
- Reads config from a JSON file.
- Starts the full node (genesis load, state recovery, peer connection, sync, consensus loop).
- Shuts down cleanly on SIGINT/SIGTERM.

### Acceptance Criteria

- `drana-node -config node1.json` starts a validator node.
- CTRL+C triggers clean shutdown with state persisted.
- Logs show block production or voting activity.

---

## Step 10 — Multi-Node Integration Test

### Files

**`test/integration/phase2_test.go`**

End-to-end test that runs 3 validator nodes in a single test process:

1. Generate 3 validator keypairs.
2. Write a genesis file with all 3 validators and 2 funded accounts.
3. Start 3 `Node` instances on different ports (e.g., `localhost:26601`, `:26602`, `:26603`).
4. Wait for each node to connect to its peers.
5. Wait for at least 5 blocks to be produced (block interval can be shortened to 2–5 seconds for testing).
6. Verify:
   - All 3 nodes report the same latest height.
   - All 3 nodes have identical state roots at every height.
   - Block proposer rotates correctly across the 3 validators.
   - Block reward has been credited to each proposer for their respective blocks.
7. Submit a `Transfer` transaction to one node's mempool via `SubmitTx`.
8. Submit a `CreatePost` transaction.
9. Wait for the next block(s) to include these transactions.
10. Verify:
    - All 3 nodes have the transfer reflected in balances.
    - All 3 nodes have the post materialized in state.
    - Supply conservation holds on all 3 nodes.
11. Stop node 3.
12. Wait for 3 more blocks to be produced by nodes 1 and 2 (2/3 quorum still met with 2 of 3).
13. Restart node 3.
14. Verify node 3 syncs to the latest height and has identical state root.
15. Shut down all nodes cleanly.

### Acceptance Criteria

- Test passes end-to-end.
- 3 validators converge on identical state over a sustained block sequence.
- Proposer rotation is correct and observable.
- Transaction submitted to one node appears in all nodes' state after finalization.
- A restarted node catches up via chain sync and rejoins consensus.
- All nodes' state roots match at every height.
- No panics, no data races (test runs with `-race`).

---

## Phase 2 Exit Criteria (Summary)

All of the following must be true before moving to Phase 3:

1. `go build ./...` succeeds.
2. `go test ./... -race` passes with all unit and integration tests green.
3. Three validator nodes on localhost converge on identical chain state over a sustained block sequence.
4. Proposer rotation follows deterministic round-robin.
5. Block finalization requires >= 2/3 validator votes with valid signatures.
6. A restarted validator catches up via chain sync and re-joins consensus.
7. Transactions submitted to any node propagate and finalize in the next block.
8. Invalid blocks (wrong proposer, bad state root, bad parent hash, future timestamp) are rejected.
9. Empty blocks are produced to keep the chain advancing when the mempool is empty.
10. Supply conservation invariant holds across all nodes at every height.
11. Clean shutdown persists state; restart resumes without re-executing the full chain.

---

## Dependency Summary

| Dependency | Purpose |
|---|---|
| Everything from Phase 1 | State machine, persistence, types, crypto |
| `google.golang.org/grpc` | gRPC server and client |
| `google.golang.org/protobuf` | Protobuf serialization (already present) |
| `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` | Code generation (build-time only) |

---

## Key Design Decisions

**Why the proposer collects votes (not a gossip protocol):**
In a small permissioned validator set (3–7 nodes), the proposer can directly RPC each validator and collect votes. This is simpler than a gossip layer and avoids the complexity of vote aggregation across an unstructured network. It is appropriate for v1.

**Why QC is attached to the block, not the header:**
The header hash is what gets voted on. If the QC were in the header, you'd need the votes to compute the hash, which creates a circular dependency. The QC sits alongside the block as proof of finalization.

**Why empty blocks are produced:**
Block rewards must flow to keep the economy circulating. Stopping block production when the mempool is empty would halt issuance and break the economic loop described in DESIGN.md section 6.5. Empty blocks also provide consistent timestamp progression for client-side ranking.

**Why chain sync requests finalized blocks (not raw blocks):**
A syncing node needs proof that each block was accepted by quorum. Receiving `FinalizedBlock` (block + QC) lets it verify this locally without trusting the peer.

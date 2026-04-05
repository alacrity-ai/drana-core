# IMPLEMENTATION_PHASE_3.md

## Phase 3 — Client RPC and CLI Wallet

### Purpose

This phase adds the external interface layer that clients, wallets, explorers, and operators use to interact with the chain. After Phase 3, an operator can manage wallets, submit all three transaction types, and query any chain state — entirely from the command line. Third-party developers can build against the JSON HTTP API.

### Prerequisites

All Phase 2 exit criteria must be met. In particular, Phase 3 consumes:

- `node.Node` — top-level wiring with access to engine, stores, genesis config
- `consensus.Engine.CurrentState()` — live world state for queries
- `consensus.Engine.CurrentHeight()` — current chain height
- `consensus.Engine.OnSubmitTx()` — transaction submission into consensus
- `store.BlockStore.GetBlockByHeight / GetBlockByHash / GetLatestBlock / GetTransaction` — block and tx queries
- `state.WorldState.GetAccount / GetPost / AllPosts / GetBurnedSupply / GetIssuedSupply` — state queries
- `crypto.GenerateKeyPair / Sign / Verify / AddressFromPublicKey / ParseAddress` — wallet operations
- `types.Transaction / SignTransaction / Block / Post / Account` — domain types
- `p2p.BlockToProto / TxToProto` — serialization for tx submission

### What This Phase Adds to the Directory Layout

```
internal/
  rpc/
    server.go
    server_test.go
    handlers.go
    types.go
cmd/
  drana-cli/
    main.go
    commands/
      wallet.go
      transfer.go
      post.go
      boost.go
      query.go
test/
  integration/
    phase3_test.go
```

---

## Step 1 — RPC JSON Response Types

### Files

**`internal/rpc/types.go`**

JSON-serializable response types for all RPC endpoints. These are the wire format that clients see.

```go
// --- Chain / Node ---

type NodeInfoResponse struct {
    ChainID       string `json:"chainId"`
    LatestHeight  uint64 `json:"latestHeight"`
    LatestHash    string `json:"latestHash"`     // hex
    GenesisTime   int64  `json:"genesisTime"`
    BlockInterval int    `json:"blockIntervalSec"`
    BlockReward   uint64 `json:"blockReward"`    // microdrana
    BurnedSupply  uint64 `json:"burnedSupply"`   // microdrana
    IssuedSupply  uint64 `json:"issuedSupply"`   // microdrana
    ValidatorCount int   `json:"validatorCount"`
}

type BlockResponse struct {
    Height       uint64              `json:"height"`
    Hash         string              `json:"hash"`         // hex
    PrevHash     string              `json:"prevHash"`     // hex
    ProposerAddr string              `json:"proposerAddr"` // drana1...
    Timestamp    int64               `json:"timestamp"`
    StateRoot    string              `json:"stateRoot"`    // hex
    TxRoot       string              `json:"txRoot"`       // hex
    TxCount      int                 `json:"txCount"`
    Transactions []TransactionResponse `json:"transactions,omitempty"`
}

// --- Accounts ---

type AccountResponse struct {
    Address string `json:"address"` // drana1...
    Balance uint64 `json:"balance"` // microdrana
    Nonce   uint64 `json:"nonce"`
}

// --- Posts ---

type PostResponse struct {
    PostID         string `json:"postId"`         // hex
    Author         string `json:"author"`         // drana1...
    Text           string `json:"text"`
    CreatedAtHeight uint64 `json:"createdAtHeight"`
    CreatedAtTime   int64  `json:"createdAtTime"`
    TotalCommitted uint64 `json:"totalCommitted"` // microdrana
    BoostCount     uint64 `json:"boostCount"`
}

type PostListResponse struct {
    Posts      []PostResponse `json:"posts"`
    TotalCount int            `json:"totalCount"`
    Page       int            `json:"page"`
    PageSize   int            `json:"pageSize"`
}

// --- Transactions ---

type TransactionResponse struct {
    Hash      string `json:"hash"`      // hex
    Type      string `json:"type"`      // "transfer", "create_post", "boost_post"
    Sender    string `json:"sender"`    // drana1...
    Recipient string `json:"recipient,omitempty"` // drana1...
    PostID    string `json:"postId,omitempty"`    // hex
    Text      string `json:"text,omitempty"`
    Amount    uint64 `json:"amount"`    // microdrana
    Nonce     uint64 `json:"nonce"`
    BlockHeight uint64 `json:"blockHeight,omitempty"`
}

type SubmitTxRequest struct {
    Type      string `json:"type"`      // "transfer", "create_post", "boost_post"
    Sender    string `json:"sender"`    // drana1...
    Recipient string `json:"recipient,omitempty"`
    PostID    string `json:"postId,omitempty"`    // hex
    Text      string `json:"text,omitempty"`
    Amount    uint64 `json:"amount"`
    Nonce     uint64 `json:"nonce"`
    Signature string `json:"signature"` // hex
    PubKey    string `json:"pubKey"`    // hex
}

type SubmitTxResponse struct {
    Accepted bool   `json:"accepted"`
    TxHash   string `json:"txHash,omitempty"` // hex
    Error    string `json:"error,omitempty"`
}

type TxStatusResponse struct {
    Hash        string `json:"hash"`
    Status      string `json:"status"` // "pending", "confirmed", "unknown"
    BlockHeight uint64 `json:"blockHeight,omitempty"`
}

// --- Network ---

type ValidatorResponse struct {
    Address string `json:"address"` // drana1...
    Name    string `json:"name"`
    PubKey  string `json:"pubKey"` // hex
}

type PeerResponse struct {
    Address string `json:"address"`
    Endpoint string `json:"endpoint"`
}

// --- Common ---

type ErrorResponse struct {
    Error string `json:"error"`
}
```

### Acceptance Criteria

- All types compile and can be marshaled/unmarshaled to/from JSON.
- Field naming follows camelCase JSON convention consistently.
- Hex fields use lowercase hex without `0x` prefix.
- Address fields use the `drana1` display format.

---

## Step 2 — RPC HTTP Server and Handlers

### Files

**`internal/rpc/server.go`**

```go
type Server struct {
    engine     *consensus.Engine
    blockStore *store.BlockStore
    genesis    *types.GenesisConfig
    httpServer *http.Server
}

func NewServer(
    listenAddr string,
    engine *consensus.Engine,
    blockStore *store.BlockStore,
    genesis *types.GenesisConfig,
) *Server

func (s *Server) Start() error
func (s *Server) Stop(ctx context.Context) error
```

The server uses `net/http` with a simple mux. No external router framework — the endpoint count is small enough.

All responses are `Content-Type: application/json`. Errors return appropriate HTTP status codes (400 for bad requests, 404 for not found, 500 for internal errors) with an `ErrorResponse` body.

**`internal/rpc/handlers.go`**

Handler implementations for each endpoint. Each is a method on `*Server`.

### Endpoint Table

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/v1/node/info` | `handleGetNodeInfo` | Chain ID, height, supply stats |
| GET | `/v1/blocks/latest` | `handleGetLatestBlock` | Most recent finalized block |
| GET | `/v1/blocks/{height}` | `handleGetBlockByHeight` | Block at specific height |
| GET | `/v1/blocks/hash/{hash}` | `handleGetBlockByHash` | Block by header hash |
| GET | `/v1/accounts/{address}` | `handleGetAccount` | Balance and nonce |
| GET | `/v1/posts/{postId}` | `handleGetPost` | Single post by ID |
| GET | `/v1/posts` | `handleListPosts` | Paginated post list (query params: `page`, `pageSize`, `author`) |
| POST | `/v1/transactions` | `handleSubmitTransaction` | Submit a signed transaction |
| GET | `/v1/transactions/{hash}` | `handleGetTransaction` | Transaction by hash |
| GET | `/v1/transactions/{hash}/status` | `handleGetTransactionStatus` | Pending/confirmed/unknown |
| GET | `/v1/network/validators` | `handleListValidators` | Genesis validator set |
| GET | `/v1/network/peers` | `handleListPeers` | Connected peers |

### Handler Details

**`handleGetNodeInfo`:**
Reads from engine state and genesis config. Returns `NodeInfoResponse`.

**`handleGetLatestBlock`:**
Calls `blockStore.GetLatestBlock()`. Converts to `BlockResponse`. Includes transaction summaries (no full tx list) unless `?full=true` query param is set.

**`handleGetBlockByHeight`:**
Parses height from URL path. Calls `blockStore.GetBlockByHeight(height)`. Returns 404 if not found.

**`handleGetBlockByHash`:**
Parses hex hash from URL path. Calls `blockStore.GetBlockByHash(hash)`. Returns 404 if not found.

**`handleGetAccount`:**
Parses `drana1...` address from URL path. Looks up in `engine.CurrentState()`. If the account does not exist, returns `AccountResponse` with zero balance and zero nonce (not a 404 — any address can receive funds).

**`handleGetPost`:**
Parses hex post ID from URL path. Looks up in `engine.CurrentState()`. Returns 404 if post does not exist.

**`handleListPosts`:**
Reads all posts from `engine.CurrentState().AllPosts()`. Supports query params:
- `page` (default 1)
- `pageSize` (default 20, max 100)
- `author` (optional `drana1...` address filter)

Sorts by `CreatedAtHeight` descending (newest first). Returns `PostListResponse`.

**`handleSubmitTransaction`:**
Accepts a `SubmitTxRequest` JSON body. Parses and reconstructs a `types.Transaction`. Submits via `engine.OnSubmitTx()`. Returns `SubmitTxResponse` with the tx hash if accepted.

**`handleGetTransaction`:**
Parses hex tx hash from URL path. Calls `blockStore.GetTransaction(hash)`. Returns 404 if not found. Returns `TransactionResponse` with block height.

**`handleGetTransactionStatus`:**
Checks `blockStore.GetTransaction(hash)` — if found, status is `"confirmed"` with block height. Then checks `engine.Mempool.Has(hash)` — if found, status is `"pending"`. Otherwise `"unknown"`.

**`handleListValidators`:**
Returns genesis validator list as `[]ValidatorResponse`.

**`handleListPeers`:**
Returns connected peer list from `engine.Peers.Peers()`.

**`internal/rpc/server_test.go`**

Tests all endpoints using `httptest.NewServer`:
- `GetNodeInfo` returns valid JSON with correct chain ID and height.
- `GetLatestBlock` returns the most recent block.
- `GetBlockByHeight` returns correct block; 404 for nonexistent height.
- `GetAccount` returns balance for existing account; zero for unknown address.
- `GetPost` returns post for existing post ID; 404 for nonexistent.
- `ListPosts` returns paginated results; filters by author.
- `SubmitTransaction` accepts a valid signed tx; rejects bad signature.
- `GetTransaction` returns confirmed tx with block height; 404 for unknown.
- `GetTransactionStatus` returns "confirmed", "pending", or "unknown" correctly.
- `ListValidators` returns all genesis validators.

### Acceptance Criteria

- All tests pass.
- Every endpoint from DESIGN.md section 17.1 is implemented.
- Error responses are consistent JSON with appropriate HTTP status codes.
- No endpoint panics on malformed input.

---

## Step 3 — Wire RPC Server into Node

### Files

**`internal/node/node.go`** (updated)

Add `RPCServer *rpc.Server` to the `Node` struct.

**`internal/node/config.go`** (updated)

Add `RPCListenAddr string` field (e.g., `"0.0.0.0:26657"`).

**`Node.Start()`** (updated):
After starting the P2P server, also start the RPC server:
```go
if cfg.RPCListenAddr != "" {
    n.RPCServer = rpc.NewServer(cfg.RPCListenAddr, n.Engine, n.BlockStore, n.Genesis)
    n.RPCServer.Start()
}
```

**`Node.Stop()`** (updated):
Stop the RPC server before closing stores.

### Acceptance Criteria

- A running node exposes both the gRPC P2P port and the HTTP RPC port.
- `curl http://localhost:26657/v1/node/info` returns valid JSON from a running node.

---

## Step 4 — CLI Wallet: Key Management

### Files

**`cmd/drana-cli/main.go`**

Entrypoint that dispatches to subcommands. Uses Go's standard `flag` package or a minimal subcommand pattern (no external CLI framework).

```
drana-cli <command> [flags]

Commands:
  keygen        Generate a new keypair
  address       Show address for a private key
  balance       Query account balance
  nonce         Query account nonce
  transfer      Send DRANA to another address
  post          Create a post
  boost         Boost an existing post
  get-block     Get block by height
  get-post      Get post by ID
  get-tx        Get transaction by hash
  node-info     Query node info
```

**`cmd/drana-cli/commands/wallet.go`**

`keygen` subcommand:
1. Calls `crypto.GenerateKeyPair()`.
2. Prints private key (hex), public key (hex), and address (`drana1...`).
3. Optionally writes private key to a file path (`--output`).

`address` subcommand:
1. Reads private key hex from `--key` flag or `--keyfile` path.
2. Derives and prints the `drana1...` address.

### Acceptance Criteria

- `drana-cli keygen` prints a valid keypair.
- `drana-cli address --key <hex>` prints the correct address.
- Generated keys can be used to sign transactions that the chain accepts.

---

## Step 5 — CLI Wallet: Query Commands

### Files

**`cmd/drana-cli/commands/query.go`**

All query commands take `--rpc` flag (default `http://localhost:26657`) to specify the node RPC endpoint.

`balance` subcommand:
```
drana-cli balance --address drana1... [--rpc http://...]
```
Calls `GET /v1/accounts/{address}`, prints balance in both microdrana and DRANA.

`nonce` subcommand:
```
drana-cli nonce --address drana1... [--rpc http://...]
```
Calls `GET /v1/accounts/{address}`, prints nonce.

`get-block` subcommand:
```
drana-cli get-block --height 5 [--rpc http://...]
drana-cli get-block --latest [--rpc http://...]
```
Calls `GET /v1/blocks/{height}` or `GET /v1/blocks/latest`. Prints block summary.

`get-post` subcommand:
```
drana-cli get-post --id <hex> [--rpc http://...]
```
Calls `GET /v1/posts/{postId}`. Prints post details.

`get-tx` subcommand:
```
drana-cli get-tx --hash <hex> [--rpc http://...]
```
Calls `GET /v1/transactions/{hash}`. Prints transaction details and block height.

`node-info` subcommand:
```
drana-cli node-info [--rpc http://...]
```
Calls `GET /v1/node/info`. Prints chain ID, height, supply stats.

### Acceptance Criteria

- All query commands print human-readable output.
- All commands exit 0 on success, non-zero on error.
- Balance display shows both microdrana and DRANA (e.g., `1,000,000 microdrana (1.000000 DRANA)`).

---

## Step 6 — CLI Wallet: Transaction Commands

### Files

**`cmd/drana-cli/commands/transfer.go`**

```
drana-cli transfer \
  --key <hex> \
  --to drana1... \
  --amount 1000000 \
  [--rpc http://...]
```

1. Reads private key from `--key` or `--keyfile`.
2. Derives sender address.
3. Queries current nonce via `GET /v1/accounts/{address}`.
4. Constructs `Transaction` with `nonce = queriedNonce + 1`.
5. Signs transaction.
6. Submits via `POST /v1/transactions`.
7. Prints tx hash if accepted.

**`cmd/drana-cli/commands/post.go`**

```
drana-cli post \
  --key <hex> \
  --text "The empire of relevance belongs to the highest bidder." \
  --amount 1000000 \
  [--rpc http://...]
```

1. Same nonce-query and signing flow as transfer.
2. Normalizes text via `validation.NormalizePostText` before signing.
3. Submits `CreatePost` transaction.
4. Prints tx hash and derived post ID.

**`cmd/drana-cli/commands/boost.go`**

```
drana-cli boost \
  --key <hex> \
  --post <hex postId> \
  --amount 500000 \
  [--rpc http://...]
```

1. Same nonce-query and signing flow.
2. Submits `BoostPost` transaction.
3. Prints tx hash.

### Acceptance Criteria

- All three tx commands construct valid transactions that the chain accepts.
- Nonce is automatically queried — the user does not need to specify it.
- Text normalization happens client-side before signing.
- Errors (insufficient balance, post not found, etc.) are reported clearly.

---

## Step 7 — Integration Test

### Files

**`test/integration/phase3_test.go`**

End-to-end test that exercises the full RPC and CLI surface against a live network:

1. Start a 3-validator network (reuse Phase 2 setup).
2. Start RPC servers on each node.
3. Wait for 3 blocks to establish some chain state.
4. **RPC tests** (using `net/http` client against the RPC endpoint):
   a. `GET /v1/node/info` — verify chain ID, height > 0, block reward matches genesis.
   b. `GET /v1/blocks/latest` — verify height matches node info.
   c. `GET /v1/blocks/1` — verify block at height 1 exists with correct proposer.
   d. `GET /v1/accounts/{validatorAddr}` — verify block reward has accumulated.
   e. `POST /v1/transactions` — submit a Transfer tx (fund user -> recipient).
   f. Wait for next block.
   g. `GET /v1/transactions/{txHash}` — verify tx is confirmed with correct block height.
   h. `GET /v1/transactions/{txHash}/status` — verify status is "confirmed".
   i. `GET /v1/accounts/{recipientAddr}` — verify balance matches transfer amount.
   j. `POST /v1/transactions` — submit a CreatePost tx.
   k. Wait for next block.
   l. `GET /v1/posts/{postId}` — verify post text and committed amount.
   m. `GET /v1/posts?author={addr}` — verify post appears in author's list.
   n. `POST /v1/transactions` — submit a BoostPost tx.
   o. Wait for next block.
   p. `GET /v1/posts/{postId}` — verify `totalCommitted` increased and `boostCount` is 1.
   q. `GET /v1/network/validators` — verify all 3 validators listed.
   r. `GET /v1/accounts/{unknownAddr}` — verify returns zero balance (not 404).
   s. `GET /v1/posts/{fakeId}` — verify returns 404.
   t. Supply conservation: `sum(balances) == genesis + issued - burned` via RPC queries.
5. Shut down all nodes cleanly.

### Acceptance Criteria

- Test passes end-to-end.
- All RPC endpoints return correct data.
- Transaction lifecycle works via RPC: submit -> confirm -> query.
- Error cases return appropriate status codes and messages.
- Supply conservation holds after all operations.

---

## Phase 3 Exit Criteria (Summary)

All of the following must be true before moving to Phase 4:

1. `go build ./...` succeeds.
2. `go test ./... -race` passes with all unit and integration tests green.
3. JSON HTTP RPC server exposes all endpoints from DESIGN.md section 17.1.
4. `drana-cli keygen` generates valid keypairs.
5. `drana-cli transfer / post / boost` construct, sign, and submit valid transactions.
6. `drana-cli balance / nonce / get-block / get-post / get-tx / node-info` return correct data from a live node.
7. An operator can, using only `drana-cli`, create a wallet, query a balance, transfer funds, create a post, boost a post, and inspect all resulting state.
8. Third-party HTTP clients can interact with the RPC API using only the documented JSON format.
9. All Phase 1 and Phase 2 tests continue to pass.

---

## Dependency Summary

| Dependency | Purpose |
|---|---|
| Everything from Phase 1 and Phase 2 | State machine, consensus, networking |
| Go standard library (`net/http`, `encoding/json`, `flag`) | HTTP server, JSON, CLI |
| No new external dependencies | Phase 3 is pure Go stdlib on top of existing packages |

---

## Key Design Decisions

**Why `net/http` with manual routing instead of a framework:**
The endpoint count is ~12. A framework (chi, gin, echo) adds a dependency and learning curve for zero benefit at this scale. Path parsing can be done with `strings.TrimPrefix` or a simple helper. If the endpoint count grows significantly in Phase 4, a lightweight router can be introduced then.

**Why the CLI auto-queries nonce:**
Requiring users to manually track and specify nonces is error-prone and painful. The CLI queries the current nonce from the node and increments by 1. This means two rapid-fire transactions from the same CLI instance could race, but that's acceptable for a v1 operator tool — the user simply retries.

**Why unknown addresses return zero balance instead of 404:**
Any address can receive funds at any time. Returning 404 would imply the address is "invalid" or "doesn't exist," which is misleading. Returning zero balance with zero nonce is the correct semantic — the account simply hasn't been credited yet.

**Why text normalization happens client-side:**
The transaction signature covers the exact text bytes. If the client sends unnormalized text and the node normalizes it before inclusion, the signature won't match. The CLI must normalize before signing so the signed text matches what the chain stores.

**Why the RPC server is separate from the gRPC P2P server:**
The gRPC server handles validator-to-validator protocol traffic (proposals, votes, sync). The HTTP RPC server handles client/wallet/explorer traffic. Different audiences, different transports, different security postures. They run on different ports.

# NAME_ENHANCEMENT.md

## On-Chain Name Registration

### Overview

This enhancement adds a fourth transaction type, `RegisterName`, which allows an account to associate a unique human-readable name with its address. Names are stored in consensus state and are immutable once set — an address can register a name exactly once, and no two addresses may share the same name.

Names give posts a human face. The attention market is about public identity and signaling; hex addresses undermine that. A name turns `drana1a3f8...` into `satoshi` at the protocol level.

### Design Rules

- A name is **3–20 characters**, lowercase alphanumeric plus underscores (`[a-z0-9_]`).
- A name is **unique** across the entire chain. No two addresses may hold the same name.
- A name is **immutable**. Once registered, it cannot be changed, released, or transferred.
- An address may register **at most one name**. Attempting a second registration fails.
- Name registration is **free** (no burn, no cost beyond the transaction itself). The cost is the nonce — you need an account to register.
- Names are **not required**. An account without a name functions identically to one with a name.
- Names are stored in **consensus state** and included in the **state root hash**.

### Why These Rules

**Immutable and unique:** Prevents squatting-and-reselling, impersonation via name recycling, and confusion from name changes. If you see a post from `alice`, it was always `alice` and will always be `alice`.

**Free to register:** The name system exists to make the attention market legible, not to be a market itself. Charging for names would create a speculative namespace economy that distracts from the core product.

**Lowercase alphanumeric + underscore:** Simple, unambiguous, URL-safe, no Unicode confusability attacks.

**3–20 characters:** Short enough to display in feeds, long enough to be expressive.

---

## Implementation Steps

### Step 1 — Types

**`internal/types/transaction.go`**

Add the new tx type constant:

```go
const (
    TxTransfer     TxType = 1
    TxCreatePost   TxType = 2
    TxBoostPost    TxType = 3
    TxRegisterName TxType = 4
)
```

The `Text` field on `Transaction` is reused to carry the name string for `RegisterName` (no new field needed — `Text` is already present and included in `SignableBytes`). `Amount` is 0 for name registration.

**`internal/types/account.go`**

Add a `Name` field to `Account`:

```go
type Account struct {
    Address crypto.Address
    Balance uint64
    Nonce   uint64
    Name    string  // empty if no name registered
}
```

**`internal/types/genesis.go`**

No changes needed. Genesis accounts start with empty names.

### Step 2 — Name Validation

**`internal/validation/text.go`** (or a new `name.go`)

Add name validation:

```go
func ValidateName(name string) error
```

Rules:
1. Length: 3–20 characters.
2. Characters: only `[a-z0-9_]`.
3. Must not start or end with underscore.
4. Must not contain consecutive underscores.

**`internal/validation/validate.go`**

Add `StateReader` method:

```go
type StateReader interface {
    GetAccount(addr crypto.Address) (*types.Account, bool)
    GetPost(id types.PostID) (*types.Post, bool)
    GetAccountByName(name string) (*types.Account, bool)  // new
}
```

Add `RegisterName` validation in `ValidateTransaction`:

```go
func validateRegisterName(tx *types.Transaction, sr StateReader) error
```

Checks:
1. PubKey derives to Sender address.
2. Signature valid.
3. Sender account exists.
4. Nonce == account.Nonce + 1.
5. `ValidateName(tx.Text)` passes.
6. Sender does not already have a name (`account.Name == ""`).
7. Name is not already taken (`sr.GetAccountByName(tx.Text)` returns nil).
8. Amount == 0 (registration is free).

### Step 3 — State

**`internal/state/state.go`**

Add a name-to-address index:

```go
type WorldState struct {
    accounts     map[crypto.Address]*types.Account
    posts        map[types.PostID]*types.Post
    nameIndex    map[string]crypto.Address           // name -> address
    // ... existing fields
}
```

Add methods:

```go
func (ws *WorldState) GetAccountByName(name string) (*types.Account, bool)
func (ws *WorldState) RegisterName(addr crypto.Address, name string)
```

`RegisterName` sets `account.Name = name` and adds to `nameIndex`.

Update `Clone()` to deep-copy `nameIndex`.

**`internal/state/stateroot.go`**

The state root already hashes all accounts. Since `Account.Name` is now a field, the hash changes when a name is set. No explicit new hashing is needed, but the account serialization in the hash writer must include the Name field:

```go
// In ComputeStateRoot, for each account:
hw.WriteBytes(a.Address[:])
hw.WriteUint64(a.Balance)
hw.WriteUint64(a.Nonce)
hw.WriteString(a.Name)  // add this
```

### Step 4 — Executor

**`internal/state/executor.go`**

Add `applyRegisterName`:

```go
func (e *Executor) applyRegisterName(ws *WorldState, tx *types.Transaction) error {
    sender, ok := ws.GetAccount(tx.Sender)
    if !ok {
        return fmt.Errorf("sender account not found")
    }
    sender.Nonce++
    sender.Name = tx.Text
    ws.SetAccount(sender)
    ws.RegisterName(tx.Sender, tx.Text)
    return nil
}
```

No balance change. No burn. Just nonce increment and name assignment.

### Step 5 — Persistence

**`internal/store/kvstore.go`**

Update `encodeAccount` / `decodeAccount` to include the `Name` field. Add a name-length prefix followed by name bytes to the binary encoding.

Update `SaveState` / `LoadState` to rebuild the `nameIndex` from loaded accounts (iterate all accounts, add those with non-empty names to the index).

### Step 6 — Protobuf and JSON

**`internal/proto/types.proto`**

Add `name` field to `Account` message:

```protobuf
message Account {
    bytes address = 1;
    uint64 balance = 2;
    uint64 nonce = 3;
    string name = 4;
}
```

Add `TxRegisterName = 4` to the transaction type documentation.

**`internal/types/json.go`**

No changes needed — the `Transaction` JSON marshaling already handles `Text`.

**`internal/p2p/convert.go`**

No changes needed — the conversion already passes `Text` through.

### Step 7 — RPC

**`internal/rpc/server.go`**

Update `AccountResponse` to include `Name`:

```go
type AccountResponse struct {
    Address string `json:"address"`
    Balance uint64 `json:"balance"`
    Nonce   uint64 `json:"nonce"`
    Name    string `json:"name,omitempty"`
}
```

Add a new endpoint:

```
GET /v1/accounts/name/{name}
```

Resolves a name to an account. Returns the same `AccountResponse` if found, 404 if the name is not registered.

Update `txTypeString` to handle `"register_name"`.

Update `handleSubmitTransaction` to accept type `"register_name"`.

### Step 8 — CLI

**`cmd/drana-cli/commands/`**

Add a `name.go` command:

```
drana-cli register-name --key <hex> --name <name> [--rpc http://...]
```

1. Reads private key, derives address.
2. Queries nonce.
3. Constructs `RegisterName` transaction with `Text = name`, `Amount = 0`.
4. Signs and submits.
5. Prints tx hash.

Add `--name` to the `balance` command output so it displays the registered name alongside the address.

Update `main.go` to add the `register-name` subcommand.

### Step 9 — Indexer

**`internal/indexer/`**

Update the follower to handle `register_name` transaction type:
- Store the name in a `names` table or simply note it (the indexer can also query `GET /v1/accounts/name/{name}` from the node RPC).
- Update `IndexedPost.Author` display: the indexer API can optionally resolve author addresses to names. This is a presentation concern — the indexer stores the address as the canonical author, but can include the name in responses.

Add a `name` field to `AuthorProfile` and API responses where author addresses appear. This enrichment can be done by joining against a local names table or by querying the node.

### Step 10 — Tests

Add tests at each layer:

**Validation tests:**
- Valid name passes (`alice`, `user_42`, `abc`).
- Too short (2 chars) fails.
- Too long (21 chars) fails.
- Invalid characters (`Alice`, `user!`, `user name`) fail.
- Leading/trailing underscore fails.
- Consecutive underscores fail.
- Registering when account already has a name fails.
- Registering an already-taken name fails.
- Amount > 0 fails.

**State tests:**
- `RegisterName` sets account name and indexes it.
- `GetAccountByName` returns the correct account.
- `Clone` preserves name index.
- State root changes when a name is registered.

**Executor tests:**
- Apply `RegisterName`, verify account name is set and nonce incremented.
- Block with RegisterName + other tx types processes correctly.

**Integration test:**
- Register a name via RPC.
- Query `GET /v1/accounts/name/{name}` — returns the correct account.
- Create a post after registering a name — post author has the name.
- Attempt duplicate name registration — rejected.
- Attempt second name on same account — rejected.

**Persistence test:**
- Save state with named accounts, reload, verify names survive restart.

### Acceptance Criteria

1. `go build ./...` succeeds.
2. `go test ./...` passes with all existing and new tests green.
3. `RegisterName` transaction is validated and executed correctly.
4. Name uniqueness is enforced at the consensus level.
5. Name immutability is enforced — no path to change or release a name.
6. State root includes account names (name registration changes the state root).
7. Persistence survives restart — names are loaded from disk.
8. RPC exposes name resolution endpoint and includes names in account responses.
9. CLI `register-name` command works end-to-end.
10. All prior Phase 1–4 tests continue to pass (name field defaults to empty string, no breakage).

---

## Files Modified (Summary)

| File | Change |
|------|--------|
| `internal/types/transaction.go` | Add `TxRegisterName = 4` |
| `internal/types/account.go` | Add `Name string` field |
| `internal/validation/text.go` (or `name.go`) | Add `ValidateName()` |
| `internal/validation/validate.go` | Add `GetAccountByName` to `StateReader`, add `validateRegisterName` |
| `internal/state/state.go` | Add `nameIndex`, `GetAccountByName`, `RegisterName`, update `Clone` |
| `internal/state/executor.go` | Add `applyRegisterName` |
| `internal/state/stateroot.go` | Add `hw.WriteString(a.Name)` to account hash |
| `internal/store/kvstore.go` | Update account encode/decode for Name, rebuild index on load |
| `internal/proto/types.proto` | Add `name` field to Account |
| `internal/rpc/server.go` + `types.go` | Add name to AccountResponse, add `/v1/accounts/name/{name}` endpoint |
| `internal/p2p/convert.go` | No changes (Text already passes through) |
| `cmd/drana-cli/commands/name.go` | New `register-name` command |
| `cmd/drana-cli/main.go` | Add `register-name` to dispatch |
| `internal/indexer/follower.go` | Handle `register_name` tx type |
| `internal/indexer/api.go` | Include name in author-related responses |
| Various `*_test.go` | New tests per Step 10 |

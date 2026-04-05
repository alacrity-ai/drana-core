# POST_STAKING_BACKEND_IMPLEMENTATION.md

## Post Staking — Backend Implementation Steps

Reference: `POST_STAKING_DESIGN.md`

---

## Step 1 — Types: Post Stake Record, Updated Post, Updated Account

### Files

**`internal/types/staking.go`** (add to existing file)

Add the post stake record:

```go
type PostStake struct {
    PostID  PostID
    Staker  crypto.Address
    Amount  uint64 // microdrana locked
    Height  uint64 // block height when staked
}
```

**`internal/types/post.go`**

Replace `TotalCommitted` and `BoostCount` with post-staking fields:

```go
type Post struct {
    PostID          PostID
    Author          crypto.Address
    Text            string
    Channel         string
    ParentPostID    PostID
    CreatedAtHeight uint64
    CreatedAtTime   int64
    TotalStaked     uint64  // sum of active stake positions (was TotalCommitted)
    TotalBurned     uint64  // cumulative fees burned on this post
    StakerCount     uint64  // count of unique active stakers (was BoostCount)
    Withdrawn       bool    // true if author unstaked
}
```

**`internal/types/account.go`**

Add `PostStakeBalance`:

```go
type Account struct {
    Address          crypto.Address
    Balance          uint64 // spendable
    Nonce            uint64
    Name             string
    StakedBalance    uint64 // validator staking
    PostStakeBalance uint64 // total locked across all post stakes
}
```

**`internal/types/transaction.go`**

Add `TxUnstakePost`:

```go
const (
    TxTransfer     TxType = 1
    TxCreatePost   TxType = 2
    TxBoostPost    TxType = 3
    TxRegisterName TxType = 4
    TxStake        TxType = 5
    TxUnstake      TxType = 6
    TxUnstakePost  TxType = 7
)
```

No new fields on `Transaction` — `TxUnstakePost` uses `PostID` to identify which post to unstake from. `Amount` is ignored (all-or-nothing).

**`internal/types/genesis.go`**

Add fee parameters:

```go
type GenesisConfig struct {
    // ... existing fields ...
    PostFeePercent      uint64 // total fee on post creation, percent (e.g., 6)
    BoostFeePercent     uint64 // total fee on boosting, percent (e.g., 6)
    BoostBurnPercent    uint64 // percent of boost amount burned (e.g., 3)
    BoostAuthorPercent  uint64 // percent of boost amount to author (e.g., 2)
    BoostStakerPercent  uint64 // percent of boost amount to existing stakers (e.g., 1)
}
```

### Acceptance Criteria

- All types compile.
- `PostStake` struct exists with PostID, Staker, Amount, Height fields.
- `Post` has `TotalStaked`, `TotalBurned`, `StakerCount`, `Withdrawn` instead of `TotalCommitted`, `BoostCount`.
- `Account` has `PostStakeBalance`.
- `TxUnstakePost = 7` exists.
- Genesis config has the five fee parameters.

---

## Step 2 — State: Post Stake Tracking in WorldState

### Files

**`internal/state/state.go`**

Add to `WorldState`:

```go
type WorldState struct {
    // ... existing fields ...
    postStakes map[postStakeKey]*types.PostStake
}

type postStakeKey struct {
    postID types.PostID
    staker crypto.Address
}
```

Add methods:

```go
func (ws *WorldState) GetPostStake(postID types.PostID, staker crypto.Address) (*types.PostStake, bool)
func (ws *WorldState) SetPostStake(ps *types.PostStake)
func (ws *WorldState) RemovePostStake(postID types.PostID, staker crypto.Address)
func (ws *WorldState) GetPostStakers(postID types.PostID) []*types.PostStake
func (ws *WorldState) GetStakesByAddress(addr crypto.Address) []*types.PostStake
```

`GetPostStakers` returns all stake positions on a post (needed for staker reward distribution and force-unstaking on withdrawal).

`GetStakesByAddress` returns all posts a user is staked on (needed for the "My Stakes" queries).

Update `NewWorldState()` to initialize `postStakes` map.

Update `Clone()` to deep-copy `postStakes`.

**`internal/state/stateroot.go`**

Add post stakes to the state root hash. After the posts section:

```go
// Sort post stakes by (postID, staker) for determinism.
var allStakes []*types.PostStake
for _, ps := range ws.postStakes {
    allStakes = append(allStakes, ps)
}
sort.Slice(allStakes, func(i, j int) bool { ... })
hw.WriteUint64(uint64(len(allStakes)))
for _, ps := range allStakes {
    hw.WriteBytes(ps.PostID[:])
    hw.WriteBytes(ps.Staker[:])
    hw.WriteUint64(ps.Amount)
    hw.WriteUint64(ps.Height)
}
```

Also update the post hash fields from `TotalCommitted`/`BoostCount` to `TotalStaked`/`TotalBurned`/`StakerCount`/`Withdrawn`:

```go
for _, p := range posts {
    // ... existing fields ...
    hw.WriteUint64(p.TotalStaked)
    hw.WriteUint64(p.TotalBurned)
    hw.WriteUint64(p.StakerCount)
    if p.Withdrawn { hw.WriteUint64(1) } else { hw.WriteUint64(0) }
}
```

### Acceptance Criteria

- `GetPostStake` / `SetPostStake` / `RemovePostStake` work correctly.
- `GetPostStakers` returns all stakers for a given post.
- `GetStakesByAddress` returns all posts a user is staked on.
- `Clone` produces an independent copy of post stakes.
- State root changes when post stakes change.

---

## Step 3 — Validation: Updated CreatePost, BoostPost, New UnstakePost

### Files

**`internal/validation/validate.go`**

Update `StateReader` interface:

```go
type StateReader interface {
    GetAccount(addr crypto.Address) (*types.Account, bool)
    GetPost(id types.PostID) (*types.Post, bool)
    GetAccountByName(name string) (*types.Account, bool)
    GetPostStake(postID types.PostID, staker crypto.Address) (*types.PostStake, bool)
}
```

Add `TxUnstakePost` to the switch in `ValidateTransaction`.

**`validateCreatePost`** — no change to validation logic. The fee computation happens in the executor, not validation. Amount still must be >= `MinPostCommitment`.

**`validateBoostPost`** — add check: post must not be withdrawn.

```go
func validateBoostPost(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
    // ... existing checks ...
    post, ok := sr.GetPost(tx.PostID)
    if !ok { return error }
    if post.Withdrawn { return fmt.Errorf("cannot boost a withdrawn post") }
    _, err := validateCommon(tx, sr)
    return err
}
```

**`validateUnstakePost`** (new):

```go
func validateUnstakePost(tx *types.Transaction, sr StateReader) error {
    // Standard identity/nonce checks (no balance check — unstaking doesn't spend).
    _, err := validateCommonNoBalanceCheck(tx, sr)
    if err != nil { return err }
    // Must have an active stake on this post.
    _, ok := sr.GetPostStake(tx.PostID, tx.Sender)
    if !ok { return fmt.Errorf("no active stake on this post") }
    // Post must exist.
    _, ok = sr.GetPost(tx.PostID)
    if !ok { return fmt.Errorf("post does not exist") }
    return nil
}
```

### Acceptance Criteria

- `TxUnstakePost` validation passes when staker has an active position.
- `TxUnstakePost` fails when no position exists.
- `TxBoostPost` fails on a withdrawn post.
- All existing validation tests still pass.

---

## Step 4 — Executor: Fee Splitting, Staking, Unstaking

### Files

**`internal/state/executor.go`**

**Rewrite `applyCreatePost`:**

```go
func (e *Executor) applyCreatePost(ws *WorldState, tx *types.Transaction, blockHeight uint64, blockTime int64) error {
    author := getAccount(...)
    feePercent := e.Params.PostFeePercent  // e.g., 6
    fee := tx.Amount * feePercent / 100
    staked := tx.Amount - fee

    // Debit full amount from spendable balance.
    author.Balance -= tx.Amount

    // Fee is burned (entire fee on post creation — no author/staker split).
    ws.SetBurnedSupply(ws.GetBurnedSupply() + fee)

    // Staked portion goes to post stake.
    author.PostStakeBalance += staked
    author.Nonce++
    ws.SetAccount(author)

    // Create post.
    post := &types.Post{
        TotalStaked: staked,
        TotalBurned: fee,
        StakerCount: 1,
        // ... other fields ...
    }
    ws.SetPost(post)

    // Create stake position.
    ws.SetPostStake(&types.PostStake{
        PostID: postID, Staker: tx.Sender, Amount: staked, Height: blockHeight,
    })
    return nil
}
```

**Rewrite `applyBoostPost`:**

```go
func (e *Executor) applyBoostPost(ws *WorldState, tx *types.Transaction) error {
    booster := getAccount(...)
    post := getPost(...)

    totalFeePercent := e.Params.BoostFeePercent  // e.g., 6
    burnPercent := e.Params.BoostBurnPercent      // e.g., 3
    authorPercent := e.Params.BoostAuthorPercent   // e.g., 2
    stakerPercent := e.Params.BoostStakerPercent   // e.g., 1

    burnAmount := tx.Amount * burnPercent / 100
    authorReward := tx.Amount * authorPercent / 100
    stakerReward := tx.Amount * stakerPercent / 100
    staked := tx.Amount - burnAmount - authorReward - stakerReward

    // Debit full amount from booster.
    booster.Balance -= tx.Amount
    booster.PostStakeBalance += staked
    booster.Nonce++
    ws.SetAccount(booster)

    // Burn portion.
    ws.SetBurnedSupply(ws.GetBurnedSupply() + burnAmount)

    // Author reward → straight to author's wallet.
    authorAcct := getAccount(post.Author)
    authorAcct.Balance += authorReward
    ws.SetAccount(authorAcct)

    // Staker reward → split pro-rata among existing stakers, to their wallets.
    if stakerReward > 0 {
        stakers := ws.GetPostStakers(tx.PostID)
        totalExistingStake := post.TotalStaked
        if totalExistingStake > 0 {
            for _, s := range stakers {
                share := stakerReward * s.Amount / totalExistingStake
                if share > 0 {
                    stakerAcct, _ := ws.GetAccount(s.Staker)
                    if stakerAcct == nil { continue }
                    stakerAcct.Balance += share
                    ws.SetAccount(stakerAcct)
                }
            }
        }
    }

    // Update post.
    post.TotalStaked += staked
    post.TotalBurned += burnAmount
    post.StakerCount++  // will be corrected below if staker already exists
    ws.SetPost(post)

    // Create or update stake position.
    existing, exists := ws.GetPostStake(tx.PostID, tx.Sender)
    if exists {
        existing.Amount += staked
        ws.SetPostStake(existing)
        // StakerCount was incremented but shouldn't be — this is a re-stake.
        post.StakerCount--
        ws.SetPost(post)
    } else {
        ws.SetPostStake(&types.PostStake{
            PostID: tx.PostID, Staker: tx.Sender, Amount: staked, Height: blockHeight,
        })
    }
    return nil
}
```

Note: `applyBoostPost` now needs `blockHeight` parameter for the stake record. Update the `ApplyTransaction` dispatch to pass it.

**New `applyUnstakePost`:**

```go
func (e *Executor) applyUnstakePost(ws *WorldState, tx *types.Transaction) error {
    sender := getAccount(tx.Sender)
    post := getPost(tx.PostID)
    stake := getPostStake(tx.PostID, tx.Sender)

    // Return staked amount to sender.
    sender.Balance += stake.Amount
    sender.PostStakeBalance -= stake.Amount
    sender.Nonce++
    ws.SetAccount(sender)

    // Update post.
    post.TotalStaked -= stake.Amount
    post.StakerCount--

    // Remove stake position.
    ws.RemovePostStake(tx.PostID, tx.Sender)

    // If sender is the author → withdraw the post, force-unstake all others.
    if tx.Sender == post.Author {
        post.Withdrawn = true
        // Force-unstake all other stakers.
        for _, s := range ws.GetPostStakers(tx.PostID) {
            otherAcct, _ := ws.GetAccount(s.Staker)
            if otherAcct != nil {
                otherAcct.Balance += s.Amount
                otherAcct.PostStakeBalance -= s.Amount
                ws.SetAccount(otherAcct)
            }
            post.TotalStaked -= s.Amount
            post.StakerCount--
            ws.RemovePostStake(tx.PostID, s.Staker)
        }
    }

    ws.SetPost(post)
    return nil
}
```

**Update `ApplyTransaction` dispatch** to include `TxUnstakePost` and pass `blockHeight` to `applyBoostPost`:

```go
case types.TxBoostPost:
    return e.applyBoostPost(ws, tx, blockHeight)
case types.TxUnstakePost:
    return e.applyUnstakePost(ws, tx)
```

### Acceptance Criteria

- `applyCreatePost` deducts full amount, burns 6%, stakes 94%, creates PostStake record.
- `applyBoostPost` deducts full amount, burns 3%, sends 2% to author wallet, sends 1% split to stakers' wallets, stakes 94%, creates/updates PostStake record.
- `applyUnstakePost` returns staked amount to wallet, removes PostStake record, decrements post totals.
- Author unstake: post marked withdrawn, all other stakers force-refunded.
- Supply conservation: `sum(spendable) + sum(validatorStaked) + sum(postStaked) + sum(unbonding) = genesis + issued - burned`.

---

## Step 5 — Persistence: Encode/Decode Post Stakes, Updated Post Format

### Files

**`internal/store/kvstore.go`**

Add new key prefix:

```go
prefixPostStake = []byte("poststake:")
```

Key format: `poststake:<postID_hex>:<staker_hex>` → encoded PostStake.

**Add encode/decode for PostStake:**

```go
func encodePostStake(ps *types.PostStake) []byte
func decodePostStake(b []byte) (*types.PostStake, error)
```

Binary layout: `postID(32) + staker(24) + amount(8) + height(8)` = 72 bytes fixed.

**Update `encodePost` / `decodePost`:**

Replace `TotalCommitted(8) + BoostCount(8)` with `TotalStaked(8) + TotalBurned(8) + StakerCount(8) + Withdrawn(1)`. Backward-compatible: old format has `TotalCommitted` where `TotalStaked` now is — acceptable for a pre-mainnet reset.

**Update `SaveState`:** Also iterate and save all post stakes.

**Update `LoadState`:** Also scan `prefixPostStake` and populate `ws.postStakes`.

**Update `encodeAccount` / `decodeAccount`:** Add `PostStakeBalance` field (8 bytes, after `StakedBalance`).

### Acceptance Criteria

- Post stakes survive restart (save + reload).
- Updated post format encodes/decodes correctly.
- Account PostStakeBalance persists.

---

## Step 6 — Genesis: Fee Parameters

### Files

**`internal/genesis/genesis.go`**

Add fee fields to `genesisJSON`:

```go
PostFeePercent      uint64 `json:"postFeePercent,omitempty"`
BoostFeePercent     uint64 `json:"boostFeePercent,omitempty"`
BoostBurnPercent    uint64 `json:"boostBurnPercent,omitempty"`
BoostAuthorPercent  uint64 `json:"boostAuthorPercent,omitempty"`
BoostStakerPercent  uint64 `json:"boostStakerPercent,omitempty"`
```

Update `LoadGenesis` to parse them into `GenesisConfig`.

**`scripts/gen-testnet.sh`** and **`scripts/init-network.sh`**

Add to generated genesis:

```json
"postFeePercent": 6,
"boostFeePercent": 6,
"boostBurnPercent": 3,
"boostAuthorPercent": 2,
"boostStakerPercent": 1
```

### Acceptance Criteria

- Genesis file includes fee parameters.
- `LoadGenesis` parses them correctly.
- Testnet genesis includes the default 6% fee configuration.

---

## Step 7 — RPC: Updated Responses, New Endpoints, UnstakePost Support

### Files

**`internal/rpc/types.go`**

Update `PostResponse`:

```go
type PostResponse struct {
    PostID          string `json:"postId"`
    Author          string `json:"author"`
    Text            string `json:"text"`
    Channel         string `json:"channel,omitempty"`
    ParentPostID    string `json:"parentPostId,omitempty"`
    CreatedAtHeight uint64 `json:"createdAtHeight"`
    CreatedAtTime   int64  `json:"createdAtTime"`
    TotalStaked     uint64 `json:"totalStaked"`       // was TotalCommitted
    TotalBurned     uint64 `json:"totalBurned"`       // new
    StakerCount     uint64 `json:"stakerCount"`       // was BoostCount
    Withdrawn       bool   `json:"withdrawn,omitempty"` // new
}
```

Update `AccountResponse`:

```go
type AccountResponse struct {
    Address          string `json:"address"`
    Balance          uint64 `json:"balance"`
    Nonce            uint64 `json:"nonce"`
    Name             string `json:"name,omitempty"`
    StakedBalance    uint64 `json:"stakedBalance"`
    PostStakeBalance uint64 `json:"postStakeBalance"` // new
}
```

Add new response types:

```go
type PostStakeResponse struct {
    PostID string `json:"postId"`
    Amount uint64 `json:"amount"`
    Height uint64 `json:"height"`
}

type MyStakesResponse struct {
    Stakes     []PostStakeResponse `json:"stakes"`
    TotalCount int                 `json:"totalCount"`
    TotalStaked uint64             `json:"totalStaked"`
}

type StakerResponse struct {
    Address string `json:"address"`
    Amount  uint64 `json:"amount"`
    Height  uint64 `json:"height"`
}
```

**`internal/rpc/server.go`**

- Update `postToResponse` to use new field names.
- Update `txTypeString` / `parseTxType` for `"unstake_post"`.
- Update `handleGetAccount` to include `PostStakeBalance`.
- Add `GET /v1/posts/{id}/stakers` — returns active staker list for a post.
- Add `GET /v1/accounts/{address}/post-stakes` — returns all posts the user is staked on.
- Update `handleListPosts` to exclude withdrawn posts by default (add `?includeWithdrawn=true` override).

### Acceptance Criteria

- `PostResponse` uses `totalStaked` / `stakerCount` / `withdrawn` instead of `totalCommitted` / `boostCount`.
- `AccountResponse` includes `postStakeBalance`.
- `GET /v1/posts/{id}/stakers` returns active stakers with amounts.
- `GET /v1/accounts/{addr}/post-stakes` returns the user's stake positions.
- `unstake_post` tx type is accepted via `POST /v1/transactions`.
- Withdrawn posts hidden from default list.

---

## Step 8 — P2P: No Structural Changes

### Files

**`internal/p2p/convert.go`** — No changes needed. `TxUnstakePost` uses the existing `PostID` field on `Transaction`, which is already serialized. The `Amount` field is present but ignored for unstake.

**`internal/proto/types.proto`** — No structural changes needed. Document `TxUnstakePost = 7` in comments.

### Acceptance Criteria

- `TxUnstakePost` transactions serialize/deserialize correctly over gRPC.

---

## Step 9 — Indexer: Updated Schema, Stake Tracking, Reward Events

### Files

**`internal/indexer/types.go`**

Update `IndexedPost`:

```go
type IndexedPost struct {
    // ... existing fields ...
    TotalStaked         uint64 `json:"totalStaked"`          // was TotalCommitted
    TotalBurned         uint64 `json:"totalBurned"`          // new
    AuthorStaked        uint64 `json:"authorStaked"`         // was AuthorCommitted
    ThirdPartyStaked    uint64 `json:"thirdPartyStaked"`     // was ThirdPartyCommitted
    StakerCount         uint64 `json:"stakerCount"`          // was BoostCount
    Withdrawn           bool   `json:"withdrawn"`            // new
    // ... keep UniqueBoosterCount, LastBoostAtHeight, ReplyCount ...
}
```

Add new types:

```go
type IndexedRewardEvent struct {
    PostID    string `json:"postId"`
    Recipient string `json:"recipient"`
    Amount    uint64 `json:"amount"`
    Height    uint64 `json:"blockHeight"`
    BlockTime int64  `json:"blockTime"`
    Trigger   string `json:"trigger"`
    Type      string `json:"type"` // "author" or "staker"
}
```

**`internal/indexer/db.go`**

Update `posts` table schema: rename columns `total_committed` → `total_staked`, `author_committed` → `author_staked`, `third_party_committed` → `third_party_staked`, `boost_count` → `staker_count`, add `total_burned INTEGER NOT NULL DEFAULT 0`, add `withdrawn INTEGER NOT NULL DEFAULT 0`.

Add new `post_stakes` table:

```sql
CREATE TABLE IF NOT EXISTS post_stakes (
    post_id TEXT NOT NULL,
    staker TEXT NOT NULL,
    amount INTEGER NOT NULL,
    block_height INTEGER NOT NULL,
    PRIMARY KEY (post_id, staker)
);
CREATE INDEX IF NOT EXISTS idx_post_stakes_staker ON post_stakes(staker);
```

Add new `reward_events` table:

```sql
CREATE TABLE IF NOT EXISTS reward_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id TEXT NOT NULL,
    recipient TEXT NOT NULL,
    amount INTEGER NOT NULL,
    block_height INTEGER NOT NULL,
    block_time INTEGER NOT NULL,
    trigger_address TEXT NOT NULL,
    reward_type TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rewards_recipient ON reward_events(recipient);
CREATE INDEX IF NOT EXISTS idx_rewards_height ON reward_events(block_height);
```

Add DB methods:

```go
func (d *DB) InsertPostStake(postID, staker string, amount, height uint64) error
func (d *DB) RemovePostStake(postID, staker string) error
func (d *DB) GetPostStakers(postID string) ([]StakerResponse, error)
func (d *DB) GetStakesByAddress(address string) ([]PostStakeResponse, error)
func (d *DB) InsertRewardEvent(ev *IndexedRewardEvent) error
func (d *DB) GetRewardsByAddress(address string, sinceHeight uint64, page, pageSize int) ([]IndexedRewardEvent, int, uint64, error)
func (d *DB) GetRewardSummary(address string) (last24h, last7d, allTime uint64, error)
```

**`internal/indexer/follower.go`**

Update `create_post` case: the tx amount is no longer the full staked amount — compute `staked = amount * 94 / 100` (or fetch from the chain's post state). Index the post stake position.

Update `boost_post` case: compute fee splits, record the staker reward events, update stake positions.

Add `unstake_post` case: remove the stake position, mark post withdrawn if author.

**`internal/indexer/api.go`**

Add endpoints:

```
GET /v1/rewards/{address}?since={height}&page=1&pageSize=20
GET /v1/rewards/{address}/summary
GET /v1/posts/{id}/stakers
```

Update existing `handlePost` to dispatch `/stakers` sub-path.

### Acceptance Criteria

- Indexer tracks post stake positions in `post_stakes` table.
- Reward events are recorded with type, amount, trigger, and recipient.
- `GET /v1/rewards/{address}` returns paginated reward history.
- `GET /v1/rewards/{address}/summary` returns 24h/7d/all-time aggregates.
- `GET /v1/posts/{id}/stakers` returns active staker list.
- Withdrawn posts are flagged in the indexer.

---

## Step 10 — CLI: UnstakePost Command

### Files

**`cmd/drana-cli/commands/`** — add `unstake_post.go`:

```
drana-cli unstake-post --key <hex> --post <post-id-hex> [--rpc http://...]
```

Constructs a `TxUnstakePost` transaction with the PostID, signs, submits. No amount needed — it's all-or-nothing.

**`cmd/drana-cli/main.go`** — add dispatch.

**`cmd/drana-cli/commands/transfer.go`** — update `txTypeStr` for `unstake_post`.

### Acceptance Criteria

- `drana-cli unstake-post --key <hex> --post <hex>` works.
- Returns the staked DRANA to the sender's wallet.

---

## Files Modified Summary

| Step | Files |
|------|-------|
| 1 | `types/staking.go`, `types/post.go`, `types/account.go`, `types/transaction.go`, `types/genesis.go` |
| 2 | `state/state.go`, `state/stateroot.go` |
| 3 | `validation/validate.go` |
| 4 | `state/executor.go` |
| 5 | `store/kvstore.go` |
| 6 | `genesis/genesis.go`, `scripts/gen-testnet.sh`, `scripts/init-network.sh` |
| 7 | `rpc/types.go`, `rpc/server.go` |
| 8 | `p2p/convert.go` (comments only), `proto/types.proto` (comments only) |
| 9 | `indexer/types.go`, `indexer/db.go`, `indexer/follower.go`, `indexer/api.go` |
| 10 | `cmd/drana-cli/commands/unstake_post.go`, `cmd/drana-cli/main.go` |

---

## Supply Conservation Invariant

After every block:

```
sum(account.Balance) + sum(account.StakedBalance) + sum(account.PostStakeBalance) + sum(unbondingQueue.Amount)
  = genesisSupply + issuedSupply - burnedSupply
```

Burns come from: post creation fees (6%), boost burn portion (3%), and validator slashing.

Author rewards (2%) and staker rewards (1%) are **not** burns — they are transfers from the booster to other accounts. They appear as balance increases on the recipients and are already deducted from the booster's amount before staking.

---

## Backward Compatibility

This is a **breaking change** to the consensus state model. The `Post` struct changes field names and semantics, `Account` gains a new field, and a new state structure (`postStakes`) is added.

For pre-mainnet: wipe and restart with the new genesis. No migration.

For a live chain: coordinated upgrade at an epoch boundary with a state migration tool that converts `TotalCommitted` → `TotalStaked` and creates initial `PostStake` records with the author as sole staker.

# REWARD_TRACKING_IMPLEMENTATION.md

## Reward Event Tracking in the Indexer

### Problem

The executor distributes rewards correctly (author gets 2%, stakers get 1% pro-rata on each boost), and the rewards show up in wallet balances. But the indexer doesn't record *which* rewards were paid, *to whom*, *for which post*, or *when*. The Rewards page has no data to display.

### Root Cause

The executor computes rewards in `applyBoostPost` and credits wallets directly. This happens inside consensus state transitions — the indexer only sees the raw `boost_post` transaction with its total amount, not the derived reward payments. The reward breakdown is computed on-chain but never exposed to the indexer.

### Solution

Two approaches, use both:

1. **Indexer computes reward breakdown from the boost amount** using the same fee percentages as the executor. This gives us the per-boost breakdown (how much burned, how much to author, how much to stakers, how much staked).

2. **Indexer records per-recipient reward events** by computing the pro-rata staker split at index time using the post's staker state at the time of the boost. This gives us the individual reward feed ("you earned 0.47 DRANA from bob's boost on post X").

---

## Step 1 — Add Fee Config to Indexer

### File: `internal/indexer/follower.go`

The follower needs access to the fee percentages to compute reward breakdowns. Currently it only has the node RPC URL.

Add fee config fields to the `Follower` struct:

```go
type Follower struct {
    nodeRPC        string
    db             *DB
    pollInterval   time.Duration
    postFeePercent uint64  // from genesis, e.g., 6
    boostBurnPct   uint64  // e.g., 3
    boostAuthorPct uint64  // e.g., 2
    boostStakerPct uint64  // e.g., 1
}
```

The follower can fetch these from `GET /v1/node/info` on startup (we'd need to add them to the NodeInfo response), or they can be passed as CLI flags to `drana-indexer`. The simplest approach: pass as CLI flags with defaults matching the genesis.

### File: `cmd/drana-indexer/main.go`

Add flags:

```go
postFeePct := flag.Uint64("post-fee", 6, "post fee percent")
boostBurnPct := flag.Uint64("boost-burn", 3, "boost burn percent")
boostAuthorPct := flag.Uint64("boost-author", 2, "boost author percent")
boostStakerPct := flag.Uint64("boost-staker", 1, "boost staker percent")
```

Pass to `NewFollower`.

### Acceptance Criteria

- Follower has access to fee percentages.
- Defaults match genesis config.

---

## Step 2 — New `reward_events` Table

### File: `internal/indexer/db.go`

Add to `Migrate()`:

```sql
CREATE TABLE IF NOT EXISTS reward_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id TEXT NOT NULL,
    recipient TEXT NOT NULL,
    amount INTEGER NOT NULL,
    block_height INTEGER NOT NULL,
    block_time INTEGER NOT NULL,
    trigger_tx TEXT NOT NULL,
    trigger_address TEXT NOT NULL,
    reward_type TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rewards_recipient ON reward_events(recipient);
CREATE INDEX IF NOT EXISTS idx_rewards_post ON reward_events(post_id);
CREATE INDEX IF NOT EXISTS idx_rewards_height ON reward_events(block_height);
```

Note: For Postgres compatibility, `AUTOINCREMENT` becomes `SERIAL`. Use dialect-aware SQL:
- SQLite: `id INTEGER PRIMARY KEY AUTOINCREMENT`
- Postgres: `id SERIAL PRIMARY KEY`

Add `InsertRewardEvent` method:

```go
func (d *DB) InsertRewardEvent(postID, recipient, triggerTx, triggerAddr, rewardType string, amount, height uint64, blockTime int64) error
```

### Acceptance Criteria

- Table is created on migration.
- `InsertRewardEvent` works on both SQLite and Postgres.

---

## Step 3 — Update `boosts` Table with Breakdown

### File: `internal/indexer/db.go`

Add columns to `boosts` table in `Migrate()`:

```sql
CREATE TABLE IF NOT EXISTS boosts (
    tx_hash TEXT PRIMARY KEY,
    post_id TEXT NOT NULL,
    booster TEXT NOT NULL,
    amount INTEGER NOT NULL,
    author_reward INTEGER NOT NULL DEFAULT 0,
    staker_reward INTEGER NOT NULL DEFAULT 0,
    burn_amount INTEGER NOT NULL DEFAULT 0,
    staked_amount INTEGER NOT NULL DEFAULT 0,
    block_height INTEGER NOT NULL,
    block_time INTEGER NOT NULL
);
```

Update `IndexedBoost` type to include the breakdown:

```go
type IndexedBoost struct {
    PostID       string `json:"postId"`
    Booster      string `json:"booster"`
    Amount       uint64 `json:"amount"`
    AuthorReward uint64 `json:"authorReward"`
    StakerReward uint64 `json:"stakerReward"`
    BurnAmount   uint64 `json:"burnAmount"`
    StakedAmount uint64 `json:"stakedAmount"`
    BlockHeight  uint64 `json:"blockHeight"`
    BlockTime    int64  `json:"blockTime"`
    TxHash       string `json:"txHash"`
}
```

Update `InsertBoost` to write the new columns. Update `GetBoostsForPost` to read them.

### Acceptance Criteria

- Boost records include the full fee breakdown.
- Existing boost queries return the new fields.

---

## Step 4 — Compute Rewards in Follower

### File: `internal/indexer/follower.go`

Update the `boost_post` case in `IndexBlock`:

```go
case "boost_post":
    if tx.PostID == "" { continue }
    
    post, _ := f.db.GetPost(tx.PostID)
    author := ""
    if post != nil { author = post.Author }
    
    // Compute fee breakdown.
    burnAmount := tx.Amount * f.boostBurnPct / 100
    authorReward := tx.Amount * f.boostAuthorPct / 100
    stakerReward := tx.Amount * f.boostStakerPct / 100
    stakedAmount := tx.Amount - burnAmount - authorReward - stakerReward
    
    // Insert boost with breakdown.
    f.db.InsertBoost(&IndexedBoost{
        PostID: tx.PostID, Booster: tx.Sender, Amount: tx.Amount,
        AuthorReward: authorReward, StakerReward: stakerReward,
        BurnAmount: burnAmount, StakedAmount: stakedAmount,
        BlockHeight: height, BlockTime: block.Timestamp, TxHash: tx.Hash,
    }, author)
    
    // Record author reward event.
    if authorReward > 0 && author != "" {
        f.db.InsertRewardEvent(tx.PostID, author, tx.Hash, tx.Sender, "author",
            authorReward, height, block.Timestamp)
    }
    
    // Record staker reward events (pro-rata split).
    if stakerReward > 0 && post != nil && post.TotalStaked > 0 {
        // Need staker positions to compute pro-rata shares.
        // Option A: Query the node RPC for post stakers.
        // Option B: Track post_stakes in the indexer (from Step 5).
        stakers := f.db.GetPostStakePositions(tx.PostID)
        for _, s := range stakers {
            share := stakerReward * s.Amount / post.TotalStaked
            if share > 0 {
                f.db.InsertRewardEvent(tx.PostID, s.Staker, tx.Hash, tx.Sender, "staker",
                    share, height, block.Timestamp)
            }
        }
    }
```

### Acceptance Criteria

- Each boost creates 1 author reward event + N staker reward events.
- Reward amounts match the executor's math (same percentages, same pro-rata formula).

---

## Step 5 — Track Post Stake Positions in Indexer

### File: `internal/indexer/db.go`

The indexer needs to know who is staked on each post (and how much) to compute pro-rata staker rewards. Add a `post_stakes` table:

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

Add methods:

```go
func (d *DB) UpsertPostStake(postID, staker string, amount, height uint64) error
func (d *DB) RemovePostStake(postID, staker string) error
func (d *DB) GetPostStakePositions(postID string) []PostStakeRecord
```

### File: `internal/indexer/follower.go`

Update `create_post` case: insert a post stake for the author (staked amount = 94% of tx amount).

Update `boost_post` case: upsert the booster's post stake.

Add `unstake_post` case: remove the stake position. If the unstaker is the author, remove all positions for that post and mark the post as withdrawn.

### Acceptance Criteria

- Indexer tracks who is staked on each post.
- Stake positions update on create/boost/unstake.
- Pro-rata reward computation uses these positions.

---

## Step 6 — Reward Query Endpoints

### File: `internal/indexer/db.go`

Add query methods:

```go
// GetRewardsByAddress returns paginated reward events for an address.
func (d *DB) GetRewardsByAddress(address string, sinceHeight uint64, page, pageSize int) ([]RewardEvent, int, uint64, error)

// GetRewardSummary returns aggregate reward stats for an address.
func (d *DB) GetRewardSummary(address string, nowUnix int64) (last24h, last7d, allTime uint64, err error)

// GetRewardsForPost returns total rewards earned by an address from a specific post.
func (d *DB) GetRewardsForPost(address, postID string) (uint64, error)
```

`GetRewardSummary` SQL:

```sql
-- All time
SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ?

-- Last 24h
SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND block_time > ?

-- Last 7d
SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND block_time > ?
```

`GetRewardsForPost` SQL:

```sql
SELECT COALESCE(SUM(amount), 0) FROM reward_events WHERE recipient = ? AND post_id = ?
```

### File: `internal/indexer/types.go`

Add type:

```go
type RewardEvent struct {
    PostID         string `json:"postId"`
    Recipient      string `json:"recipient"`
    Amount         uint64 `json:"amount"`
    BlockHeight    uint64 `json:"blockHeight"`
    BlockTime      int64  `json:"blockTime"`
    TriggerTx      string `json:"triggerTx"`
    TriggerAddress string `json:"triggerAddress"`
    Type           string `json:"type"`
}
```

### Acceptance Criteria

- `GetRewardsByAddress` returns paginated reward history.
- `GetRewardSummary` returns correct 24h/7d/allTime aggregates.
- `GetRewardsForPost` returns per-post lifetime rewards.

---

## Step 7 — Reward API Endpoints

### File: `internal/indexer/api.go`

Add routes:

```go
mux.HandleFunc("/v1/rewards/", a.handleRewards)
```

Handlers:

**`GET /v1/rewards/{address}`** — paginated reward event list.

Query params: `since` (block height), `page`, `pageSize`.

Response:

```json
{
  "events": [
    { "postId": "...", "recipient": "...", "amount": 200000, "blockHeight": 42,
      "blockTime": 1700000000, "triggerAddress": "drana1...", "type": "author" }
  ],
  "totalCount": 15,
  "totalAmount": 3500000
}
```

**`GET /v1/rewards/{address}/summary`** — aggregate stats.

Response:

```json
{
  "last24h": 500000,
  "last7d": 2500000,
  "allTime": 12000000,
  "postCount": 3,
  "totalStaked": 9400000
}
```

The `postCount` and `totalStaked` come from the `post_stakes` table.

### Acceptance Criteria

- Both endpoints return correct data.
- Pagination works.
- Summary aggregates match individual events.

---

## Step 8 — Update Rewards Frontend Page

### File: `drana-app/src/pages/Rewards.tsx`

The page already queries `getRewardSummary` and `getRewards` — once the indexer endpoints exist, the data will flow through. No frontend code changes needed beyond what's already implemented (the queries have `retry: false` and gracefully handle missing endpoints).

The only addition: show per-post lifetime rewards on each stake row. Add a call to fetch rewards per post:

```typescript
// For each stake, show lifetime rewards from that post.
const rewardsPerPost = useQuery({
  queryKey: ['rewards-per-post', address, stakes.data?.stakes],
  queryFn: async () => {
    if (!stakes.data?.stakes || !address) return {};
    const map: Record<string, number> = {};
    for (const s of stakes.data.stakes) {
      try {
        const r = await getRewardsForPost(address, s.postId);
        map[s.postId] = r;
      } catch { map[s.postId] = 0; }
    }
    return map;
  },
  enabled: !!address && !!stakes.data?.stakes?.length,
});
```

Then on each stake row: `Earned: +X.XX DRANA`.

### File: `drana-app/src/api/indexerApi.ts`

Add:

```typescript
export const getRewardsForPost = (address: string, postId: string) =>
  get<{ totalReward: number }>(`/v1/rewards/${address}/post/${postId}`);
```

### Acceptance Criteria

- Rewards page shows 24h/7d/allTime summary when indexer data is available.
- Reward feed shows individual events.
- Each stake row shows lifetime earnings from that post.

---

## Files Modified Summary

| Step | Files | Change |
|------|-------|--------|
| 1 | `indexer/follower.go`, `cmd/drana-indexer/main.go` | Fee config on follower, CLI flags |
| 2 | `indexer/db.go` | `reward_events` table, `InsertRewardEvent` |
| 3 | `indexer/db.go`, `indexer/types.go` | Boost table breakdown columns, `IndexedBoost` fields |
| 4 | `indexer/follower.go` | Compute rewards on boost, insert reward events |
| 5 | `indexer/db.go`, `indexer/follower.go` | `post_stakes` table, track positions on create/boost/unstake |
| 6 | `indexer/db.go`, `indexer/types.go` | Reward query methods |
| 7 | `indexer/api.go` | `GET /v1/rewards/{addr}`, `GET /v1/rewards/{addr}/summary` |
| 8 | `drana-app/src/pages/Rewards.tsx`, `drana-app/src/api/indexerApi.ts` | Per-post reward display, `getRewardsForPost` |

---

## Data Flow After Implementation

```
Boost tx submitted
  ↓
Executor: distributes rewards to wallets (on-chain, consensus)
  ↓
Block finalized
  ↓
Indexer polls block via RPC
  ↓
Indexer sees boost_post tx
  ├── Computes: burnAmount, authorReward, stakerReward, stakedAmount
  ├── Inserts boost record with breakdown
  ├── Inserts author reward event
  ├── Looks up post stakers from post_stakes table
  ├── Computes pro-rata shares
  ├── Inserts staker reward events (one per staker)
  └── Updates post_stakes (adds booster's position)
  ↓
Frontend queries /v1/rewards/{addr}/summary
  → Shows: "Last 24h: +12.47 DRANA"
  
Frontend queries /v1/rewards/{addr}
  → Shows: "+0.47 DRANA · bob staked on 'Welcome to DRANA' · 2 min ago"
```

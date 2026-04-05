# POST_STAKING_DESIGN.md

## Post Staking: Replacing Burn with Stake-to-Curate

### Summary

Replace the burn-on-post model with a stake-on-post model. When you create a post or boost someone's post, your DRANA is **locked (staked)**, not burned. A small fee is taken on each action. You can unstake at any time and get your principal back (minus the fee). Posts are ranked by total currently staked, not total lifetime burned.

This transforms the attention market from "pay to be seen" into "invest in content you believe in."

---

## 1. Core Mechanics

### Creating a Post

User commits 100 DRANA to create a post:

```
Fee:      6 DRANA burned (6%)          ← gone forever, deflationary
Staked:  94 DRANA locked on the post   ← recoverable via unstake
```

The author is now the first staker on their own post. Their 94 DRANA determines the post's initial ranking. The full 6% is burned because there are no existing stakers or separate author to pay.

### Boosting a Post

User commits 100 DRANA to boost someone's post:

```
Fee:      6 DRANA total (6%)
  ├── 3 DRANA burned (3%)              ← deflationary
  ├── 2 DRANA to author's wallet (2%)  ← instant, spendable
  └── 1 DRANA split among existing stakers' wallets (1%)  ← instant, spendable
Staked:  94 DRANA locked on the post   ← recoverable via unstake
```

The booster is now a staker on this post. They will earn a share of future boosters' 1% staker rewards.

### Fee Breakdown (6% Total)

| Recipient | Post Creation | Boost |
|-----------|--------------|-------|
| Burned | 6% | 3% |
| Author | — (is the author) | 2% → author's wallet |
| Existing stakers | — (no stakers yet) | 1% → split pro-rata to wallets |
| Staked | 94% | 94% |

### How Staker Rewards Work

When a new booster stakes on a post, the 1% staker reward is distributed **immediately** to all existing stakers' spendable balances, proportional to their share of total staked.

Example: A post has two stakers — Alice (60 DRANA staked) and Bob (40 DRANA staked). Carol boosts with 100 DRANA:

```
Carol pays 100 DRANA:
  3 DRANA burned
  2 DRANA → author's wallet
  1 DRANA staker reward:
    └── Alice gets 0.60 DRANA (60/100 share) → wallet
    └── Bob gets 0.40 DRANA (40/100 share)   → wallet
  94 DRANA staked by Carol on the post
```

Alice and Bob see their spendable balance increase immediately. No claiming, no compounding, no lockup. The reward goes straight to the wallet.

### Reward Events

Each staker reward payment is recorded as a **RewardEvent** on-chain:

```go
type RewardEvent struct {
    PostID    PostID
    Recipient crypto.Address
    Amount    uint64
    Height    uint64
    Trigger   crypto.Address // the booster who triggered this reward
}
```

These events are indexed and power the rewards dashboard on the frontend.

---

## 2. Unstaking

### All-or-Nothing

Unstaking from a post is **all-or-nothing**. You either have your full stake on a post or you don't. No partial withdrawals.

- If you staked 95 DRANA on a post, you unstake all 95.
- This simplifies accounting: each staker has exactly one position per post.

### Author Unstakes (Post Withdrawal)

When the **author** unstakes from their own post:

1. The post is marked `withdrawn: true` on-chain.
2. **All other stakers are force-unstaked** — their staked DRANA returns to their spendable balance automatically.
3. The post is hidden from feeds and rankings.
4. The post still exists on-chain (immutable history) but is no longer active.

Rationale: the author's stake is the foundational commitment. If they pull out, the post has no sponsor. Other stakers shouldn't be trapped on an abandoned post.

### Non-Author Unstakes

When a **non-author staker** unstakes:

1. Their staked DRANA returns to their spendable balance.
2. The post's total staked decreases by that amount.
3. The post drops in ranking accordingly.
4. The post remains active (the author is still committed).

### Unstake Cooldown

Unstaking is **instant** — no unbonding period for post stakes. The unbonding period exists for validator staking (consensus security). Post staking is a content market, not a security mechanism. Instant unstaking means the ranking is always honest: it reflects current conviction, not historical lockup.

---

## 3. Ranking

A post's ranking score is based on **total currently staked** (not lifetime total):

```
totalStaked = sum of all active stake positions on the post
```

This replaces `TotalCommitted` (which was lifetime burned). The key difference: ranking is dynamic. If stakers leave, the post drops. A post must maintain ongoing support to stay visible.

The indexer ranking strategies adapt:

- **Top:** `ORDER BY total_staked DESC`
- **New:** `ORDER BY created_at_height DESC` (unchanged)
- **Trending:** `log(1 + total_staked) / (1 + ageHours)^1.5`
- **Controversial:** `staker_count * log(1 + total_staked)`

---

## 4. Curation Rewards

Existing stakers earn a 1% cut of every new boost on posts they're staked on. Rewards are paid **instantly to the staker's spendable wallet balance** — no claiming, no compounding, no lockup.

### The Incentive Loop

1. You find a post you think will attract boosts.
2. You stake on it early, getting a large share of total staked.
3. Others see the post rising, boost it too.
4. Each new boost sends 1% of the boost amount to existing stakers, split by share.
5. Your wallet balance increases with every new booster.
6. You can unstake your principal at any time — the rewards you already earned are yours.

### Why This Works

- **Early stakers earn more per-boost** because their share of the pool is larger before others join.
- **Rewards decrease per-boost as more stakers join** — natural dilution prevents infinite returns.
- **No ponzi dynamics** — rewards come from the entry fee of new participants, but the principal is always recoverable. Nobody loses money by staking (they only lose the 6% entry fee they already paid).
- **Authors are incentivized to create quality content** — the 2% author reward is ongoing income whenever someone boosts their post.

### Edge Cases

**Self-boosting:** Alice creates a post and then boosts it herself. She pays 6% fee (3% burned, 2% to herself as author, 1% to herself as sole staker). Net: she pays 3% to the burn. Not profitable unless others follow.

**No existing stakers (first boost on an author-only post):** The 1% staker reward goes to the author (they're the only staker). So the author gets 2% author fee + 1% staker fee = 3% of every boost when they're the sole staker.

**Post withdrawn:** No new boosts possible on a withdrawn post. Stakers keep all rewards they already earned.

---

## 5. On-Chain Data Model

### New: Stake Positions

A new state structure tracks who has staked what on which post:

```go
type PostStake struct {
    PostID  PostID
    Staker  crypto.Address
    Amount  uint64 // microdrana locked
    Height  uint64 // block height when staked
}
```

The world state needs a mapping: `(PostID, Address) → PostStake`. This is how we know:
- Who is staked on a post (for unstaking and force-unstaking)
- How much each staker has locked (for ranking and fee splits)
- When they staked (for potential future time-weighted features)

### Modified: Post

```go
type Post struct {
    PostID          PostID
    Author          crypto.Address
    Text            string
    Channel         string
    ParentPostID    PostID
    CreatedAtHeight uint64
    CreatedAtTime   int64
    TotalStaked     uint64  // was TotalCommitted — now sum of active stakes
    TotalBurned     uint64  // cumulative fees burned on this post
    StakerCount     uint64  // was BoostCount — now count of active stakers
    Withdrawn       bool    // true if author unstaked
}
```

Key changes:
- `TotalCommitted` → `TotalStaked` (reflects current locked amount, not lifetime)
- New `TotalBurned` (tracks the fee portion that was burned, for analytics)
- New `Withdrawn` flag
- `BoostCount` → `StakerCount` (number of unique current stakers, not lifetime boosts)

### Modified: Account

```go
type Account struct {
    Address         crypto.Address
    Balance         uint64 // spendable
    Nonce           uint64
    Name            string
    StakedBalance   uint64 // validator staking (unchanged)
    PostStakeBalance uint64 // total DRANA locked across all post stakes
}
```

New `PostStakeBalance` tracks the total DRANA an account has locked in post stakes. This is separate from `StakedBalance` (validator staking).

### Genesis Parameters

```go
type GenesisConfig struct {
    // ... existing fields ...
    PostFeePercent      uint64 // total fee on post creation, percent (e.g., 6)
    BoostFeePercent     uint64 // total fee on boosting, percent (e.g., 6)
    BoostBurnPercent    uint64 // portion of boost fee burned, percent of total amount (e.g., 3)
    BoostAuthorPercent  uint64 // portion of boost fee to author, percent of total amount (e.g., 2)
    BoostStakerPercent  uint64 // portion of boost fee to existing stakers, percent of total amount (e.g., 1)
}
```

Default configuration:
- `PostFeePercent: 6` — 6% of post amount is burned (entire fee, since no author/stakers to pay)
- `BoostFeePercent: 6` — 6% of boost amount is the total fee
- `BoostBurnPercent: 3` — 3% of boost amount is burned
- `BoostAuthorPercent: 2` — 2% of boost amount goes to the post author's wallet
- `BoostStakerPercent: 1` — 1% of boost amount is split among existing stakers' wallets

Invariant: `BoostBurnPercent + BoostAuthorPercent + BoostStakerPercent == BoostFeePercent`

### Reward Event Record

```go
type RewardEvent struct {
    PostID    PostID
    Recipient crypto.Address
    Amount    uint64 // microdrana
    Height    uint64 // block height
    Trigger   crypto.Address // the booster whose action triggered this reward
    Type      string // "author" or "staker"
}
```

Reward events are stored in the indexer (not in consensus state — they're derivable from boost transactions). The indexer records every reward payment for dashboard queries.

---

## 6. New Transaction Type: UnstakePost

```
TxUnstakePost = 7
```

Fields:
- `sender` — the staker's address
- `postId` — the post to unstake from
- `nonce`, `signature`

No `amount` field — it's all-or-nothing. The chain looks up the staker's position and returns the full amount.

Validation:
- Sender has an active stake on the referenced post
- Post exists
- Standard signature/nonce checks

State transition:
- Return staked amount to sender's spendable balance
- Decrease sender's `PostStakeBalance`
- Decrease post's `TotalStaked` and `StakerCount`
- Remove the `PostStake` record
- If sender is the post author:
  - Mark post as `Withdrawn`
  - Force-unstake all other stakers (return their DRANA)

---

## 7. Supply Accounting

The conservation invariant becomes:

```
sum(spendable) + sum(validatorStaked) + sum(postStaked) + sum(unbonding)
  = genesis_supply + total_issued - total_burned
```

Burns now come from:
- Post creation fees (6% of post amount)
- Boost burn portion (3% of boost amount)
- Slashing (validator double-sign)

Non-burn fee flows (not burned, redistributed):
- Author reward (2% of boost amount → author's spendable balance)
- Staker reward (1% of boost amount → existing stakers' spendable balances)

Burns do NOT come from the staked principal — that's always recoverable.

---

## 8. Frontend Changes

### Balance Display (TopBar)

```
Before:  42.50 DRANA
After:   42.50 DRANA  |  Staked: 95.00  |  Posts: 150.00
```

Three categories: spendable, validator-staked, post-staked.

### New Post Modal

```
Before:
  Commit: [100] DRANA
  "This burns 100 DRANA permanently."

After:
  Amount: [100] DRANA
  ├── Fee (6%):  6.00 DRANA burned
  └── Staked:   94.00 DRANA (recoverable)
  "You can unstake and recover 94 DRANA at any time."
```

### Boost Modal

```
Before:
  Amount: [50] DRANA
  "This burns 50 DRANA permanently."

After:
  Amount: [50] DRANA
  ├── Fee (6%):    3.00 DRANA
  │   ├── Burned:  1.50 DRANA
  │   ├── Author:  1.00 DRANA → their wallet
  │   └── Stakers: 0.50 DRANA → split to current stakers
  └── Staked:     47.00 DRANA (recoverable)
  "You can unstake and recover 47.00 DRANA at any time."
```

### Post Card

```
Before:
  🔥 42.50 DRANA                              2h ago
  satoshi · #general
  "The empire of relevance..."
  💬 12    👥 8                        [Boost] [Reply]

After:
  📌 42.50 DRANA staked (8 stakers)           2h ago
  satoshi · #general
  "The empire of relevance..."
  💬 12    Your stake: 5.00           [Boost] [Unstake]
```

If the user has a stake on a post, they see their position and an Unstake button instead of Boost.

### New: My Stakes Page/Section

Accessible from the wallet dropdown. Shows all posts the user is staked on:

```
My Stakes (150.00 DRANA across 12 posts)
─────────────────────────────────────────
📌 95.00 DRANA  "Welcome to DRANA..."     by you · #general    [Unstake]
📌 47.50 DRANA  "Gaming is the future"    by bob · #gaming     [Unstake]
📌  7.50 DRANA  "Hot take on crypto"      by eve · #crypto     [Unstake]
```

### Rewards Dashboard

A new page or section accessible from the wallet dropdown: **"Rewards"**.

#### Rewards Summary (top of page)

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  💰 Rewards                                                         │
│                                                                     │
│  Last 24 hours    +12.47 DRANA                                      │
│  Last 7 days     +284.30 DRANA                                      │
│  All time       +1,847.50 DRANA                                     │
│                                                                     │
│  Earning from 12 posts · 94.00 DRANA staked across posts            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

The summary shows aggregate rewards over time windows. "Earning from 12 posts" = number of posts you're staked on.

#### Reward Feed (below summary)

A chronological list of reward events, newest first:

```
┌─────────────────────────────────────────────────────────────────────┐
│  +0.47 DRANA   bob boosted "Welcome to DRANA..."        2 min ago   │
│  +0.23 DRANA   carol boosted "Gaming is the future"    15 min ago   │
│  +1.20 DRANA   dave boosted "Welcome to DRANA..."       1 hour ago  │
│  +0.08 DRANA   eve boosted "Hot take on crypto"         3 hours ago │
│  [Load more]                                                        │
└─────────────────────────────────────────────────────────────────────┘
```

Each row shows: reward amount, who triggered it (the booster), which post, and when.

#### Cha-Ching Notification

When the user opens the app after being away, if rewards have accumulated, show a toast notification:

```
┌──────────────────────────────┐
│  💰 +2,768.00 DRANA          │
│  Rewards since last visit    │
│  from 47 boosts on 8 posts   │
└──────────────────────────────┘
```

Auto-dismiss after 5 seconds, or click to navigate to the Rewards dashboard.

Implementation: the frontend stores `lastSeenRewardHeight` in localStorage. On load, query the indexer for reward events since that height. If total > 0, show the toast. Update the stored height.

#### My Stakes (integrated into Rewards page)

Below the reward feed, show the staked positions:

```
My Stakes (150.00 DRANA across 12 posts)
─────────────────────────────────────────
📌 94.00 DRANA  "Welcome to DRANA..."     by you · #general
   Earned: +47.20 DRANA all time                     [Unstake]

📌 47.00 DRANA  "Gaming is the future"    by bob · #gaming
   Earned: +12.80 DRANA all time                     [Unstake]

📌  7.50 DRANA  "Hot take on crypto"      by eve · #crypto
   Earned: +0.90 DRANA all time                      [Unstake]
```

Each stake shows the cumulative rewards earned from that specific post.

#### Indexer Support

The indexer needs a new table and endpoints:

```sql
CREATE TABLE reward_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id TEXT NOT NULL,
    recipient TEXT NOT NULL,
    amount INTEGER NOT NULL,
    block_height INTEGER NOT NULL,
    block_time INTEGER NOT NULL,
    trigger_address TEXT NOT NULL,
    reward_type TEXT NOT NULL  -- 'author' or 'staker'
);
CREATE INDEX idx_rewards_recipient ON reward_events(recipient);
CREATE INDEX idx_rewards_post ON reward_events(post_id);
CREATE INDEX idx_rewards_height ON reward_events(block_height);
```

New indexer API endpoints:

```
GET /v1/rewards/{address}?since={height}&page=1&pageSize=20
  → { events: [...], totalCount, totalAmount, page, pageSize }

GET /v1/rewards/{address}/summary
  → { last24h: amount, last7d: amount, allTime: amount, postCount: N, totalStaked: amount }
```

### Post Detail

The boost history becomes a **staker list** — active positions, not historical events:

```
Stakers (8) — 42.50 DRANA total
  satoshi (author)    10.00 DRANA    since block 42
  bob                  8.00 DRANA    since block 55
  carol                6.50 DRANA    since block 58
  ...
```

### Withdrawn Posts

Posts where the author unstaked show a visual indicator:

```
⚠ WITHDRAWN — This post's author has unstaked. Content preserved for history.
```

Withdrawn posts don't appear in feeds. They're only visible via direct link.

---

## 9. What Doesn't Change

- **Validator staking** — unchanged. `TxStake`, `TxUnstake`, epoch transitions, slashing, block rewards all work exactly as before.
- **Transfers** — unchanged.
- **Name registration** — unchanged.
- **Post text immutability** — unchanged. Text is permanent even if the post is withdrawn.
- **Channels and replies** — unchanged.
- **Consensus** — unchanged. Validator voting power comes from validator staking, not post staking.

---

## 10. Migration from Burn Model

For a pre-mainnet chain: hard reset. The testnet can be wiped and restarted with the new model.

For a live chain (if applicable): a coordinated upgrade at an epoch boundary. Existing `TotalCommitted` on posts becomes `TotalStaked` with the author as the sole staker. No DRANA is returned (it was already burned). Going forward, new posts and boosts use the staking model.

---

## 11. Summary

| Aspect | Before (Burn) | After (Post Staking) |
|--------|--------------|---------------------|
| Post creation | 100% burned | 6% burned, 94% staked |
| Boosting | 100% burned | 3% burned, 2% author, 1% stakers, 94% staked |
| Ranking basis | Lifetime burned | Currently staked |
| Reversibility | Permanent | Unstake anytime (minus fee) |
| Author exit | Impossible (burned) | Unstake → post withdrawn, stakers refunded |
| Author income | None | 2% of every boost → wallet |
| Staker income | None | 1% of future boosts → wallet (proportional) |
| User psychology | "I'm paying for attention" | "I'm investing in content I believe in" |
| Deflationary pressure | High (100% burn) | Moderate (3-6% fee burn) |
| Participation barrier | High (permanent loss) | Low (recoverable principal + passive income) |
| Dashboard | None | Rewards summary, reward feed, cha-ching notifications |

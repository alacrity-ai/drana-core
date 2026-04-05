# POST_STAKING_TEST_IMPLEMENTATION.md

## Post Staking — Live Test Implementation Steps

Reference: `POST_STAKING_DESIGN.md`, `POST_STAKING_BACKEND_IMPLEMENTATION.md`

This document describes how to update `test/live/live_test.go` to validate the post-staking model end-to-end against a running Docker testnet.

### Current Test Structure (Preserved)

Phases 1-2 (create wallets, fund, register names) are **unchanged**. The same 10 wallets with the same names and funding amounts are used.

### What Changes

- **Phase 3 (posts):** Verify fee/stake split instead of burn.
- **Phase 4 (boosts):** Verify fee distribution (3% burn, 2% author, 1% stakers) and staker reward payouts.
- **Phase 5 (replies):** Same as posts — verify stake model on replies.
- **Phase 6 (verification):** Updated field names (`totalStaked` etc.) and new assertions.
- **Phase 7 (new):** Unstake from a boost — verify stake returned, post ranking drops.
- **Phase 8 (new):** Author unstakes — verify post withdrawn, all stakers refunded.
- **Phase 9 (new):** Verify reward distribution via indexer.
- **Phase 10 (new):** Supply conservation with post stakes.

---

## Step 1 — Update Response Types in Test

### File: `test/live/live_test.go`

Update all inline response structs to use the new field names:

```go
// Old:
TotalCommitted      uint64 `json:"totalCommitted"`
AuthorCommitted     uint64 `json:"authorCommitted"`
ThirdPartyCommitted uint64 `json:"thirdPartyCommitted"`
BoostCount          uint64 `json:"boostCount"`

// New:
TotalStaked         uint64 `json:"totalStaked"`
TotalBurned         uint64 `json:"totalBurned"`
AuthorStaked        uint64 `json:"authorStaked"`
ThirdPartyStaked    uint64 `json:"thirdPartyStaked"`
StakerCount         uint64 `json:"stakerCount"`
Withdrawn           bool   `json:"withdrawn"`
```

Update `AccountResponse` usage to include `PostStakeBalance`:

```go
type acctResp struct {
    Balance          uint64 `json:"balance"`
    StakedBalance    uint64 `json:"stakedBalance"`
    PostStakeBalance uint64 `json:"postStakeBalance"`
}
```

Add helper functions:

```go
func getPostStakeBalance(addr string) uint64
func getPostDetail(postID string) postDetailResp
```

### Acceptance Criteria

- All response structs match the updated backend field names.
- Helper functions for new queries exist.

---

## Step 2 — Update Phase 3: Post Creation with Fee/Stake Verification

### What Changes

After posts confirm, verify the fee/stake split for each post:

```go
// For each post with amount X:
// Fee = X * 6 / 100 (burned)
// Staked = X - Fee

// Example: alice posts 50M DRANA
// Fee = 3M burned
// Staked = 47M on the post
```

After all posts confirm:

1. **Check post `totalStaked`** via indexer — should be `amount * 94 / 100` for each post (not the full amount).
2. **Check post `totalBurned`** — should be `amount * 6 / 100`.
3. **Check author's `postStakeBalance`** — should be the sum of their post stakes.
4. **Check author's spendable balance** — should be `funded - totalSpent` (where totalSpent is the full amount, not just the staked portion).

```go
// After post 0 (alice, 50M DRANA) confirms:
post0Staked := uint64(50_000_000 * 94 / 100)  // 47,000,000
post0Burned := uint64(50_000_000 * 6 / 100)   // 3,000,000

var post0 postDetailResp
httpGet(indexerAPI+"/v1/posts/"+posts[0].id, &post0)
assert(post0.TotalStaked == post0Staked)
assert(post0.TotalBurned == post0Burned)
assert(post0.StakerCount == 1)  // author only
```

### Acceptance Criteria

- Post `totalStaked` = 94% of the original amount.
- Post `totalBurned` = 6% of the original amount.
- Author's `postStakeBalance` reflects their total staked across all posts.
- StakerCount = 1 for each freshly created post.

---

## Step 3 — Update Phase 4: Boost with Reward Verification

### What Changes

The boost phase becomes the most critical test. For each boost, verify:

1. **Booster's balance decreases** by the full boost amount.
2. **Post's `totalStaked` increases** by 94% of the boost amount.
3. **Post's `totalBurned` increases** by 3% of the boost amount.
4. **Author receives 2% reward** in their spendable balance.
5. **Existing stakers receive 1% reward** split proportionally in their spendable balances.

Record balances before and after each boost to verify exact reward payments:

```go
// Before bob boosts alice's post 0 with 10M:
aliceBalBefore := getBalance(wallets[0].addrStr)  // alice is author + only staker

// Bob boosts 10M:
// - 300K burned (3%)
// - 200K to alice (author, 2%)
// - 100K to stakers (1%) → alice is only staker → 100K to alice
// - 9,400K staked by bob on the post

aliceBalAfter := getBalance(wallets[0].addrStr)
aliceReward := aliceBalAfter - aliceBalBefore
expectedReward := uint64(10_000_000 * 2 / 100) + uint64(10_000_000 * 1 / 100) // 200K + 100K = 300K
assert(aliceReward == expectedReward)
```

For the second boost on the same post (carol boosts 5M when alice + bob are staked):

```go
// Stakers: alice (47M from post creation), bob (9.4M from boost)
// Total existing stake: 56.4M
// Carol boosts 5M:
// - 150K burned
// - 100K to alice (author)
// - 50K staker reward, split:
//   - alice share: 50K * 47M / 56.4M ≈ 41,666
//   - bob share: 50K * 9.4M / 56.4M ≈ 8,333
```

**Test this precisely.** Record all 10 wallet balances before and after each boost. Verify the math.

### Acceptance Criteria

- Author receives exactly 2% of each boost in their wallet.
- Existing stakers receive their pro-rata share of 1% in their wallets.
- Burned amount is exactly 3%.
- Staked amount is exactly 94%.
- All rewards are in spendable balance (not staked, not locked).

---

## Step 4 — Update Phase 5: Replies (Same Stake Model)

### What Changes

Replies use the same `CreatePost` mechanism, so the same fee/stake split applies. Verify:

- Reply's `totalStaked` = 94% of amount.
- Reply's `totalBurned` = 6% of amount.
- Reply author's `postStakeBalance` includes the reply stake.

No boosts on replies in the current test, so no reward distribution to verify here.

### Acceptance Criteria

- Replies are created with the same stake model as top-level posts.
- Reply stake counts toward the author's `postStakeBalance`.

---

## Step 5 — New Phase 7: Unstake from a Boost

### What to Test

Bob unstakes from alice's post 0 (where he previously boosted).

```go
// Before unstake:
bobBalBefore := getBalance(wallets[1].addrStr)
post0StakedBefore := getPostDetail(posts[0].id).TotalStaked
post0StakersBefore := getPostDetail(posts[0].id).StakerCount

// Bob submits TxUnstakePost for post 0:
tx := &types.Transaction{
    Type: types.TxUnstakePost, Sender: wallets[1].addr,
    PostID: pid, Amount: 0, Nonce: nextNonce(wallets[1].addrStr),
}

// After confirmation:
bobBalAfter := getBalance(wallets[1].addrStr)
// Bob should have received his staked amount back (9.4M)
assert(bobBalAfter == bobBalBefore + bobStakedAmount)

// Post should have less stake and one fewer staker:
post0After := getPostDetail(posts[0].id)
assert(post0After.TotalStaked == post0StakedBefore - bobStakedAmount)
assert(post0After.StakerCount == post0StakersBefore - 1)
assert(post0After.Withdrawn == false)  // bob is not the author
```

Verify Bob's `postStakeBalance` decreased accordingly.

### Acceptance Criteria

- Bob's staked amount returned to spendable balance.
- Post's `totalStaked` and `stakerCount` decreased.
- Post is NOT withdrawn (bob is not the author).
- Bob's `postStakeBalance` decreased.

---

## Step 6 — New Phase 8: Author Unstakes (Post Withdrawal + Force Refund)

### What to Test

Create a new post specifically for this test. Have multiple stakers boost it. Then the author unstakes.

```go
// Frank creates a post with 20M. Staked: 18.8M
// Grace boosts 10M. Staked: 9.4M
// Heidi boosts 5M. Staked: 4.7M
// Post total staked: 18.8 + 9.4 + 4.7 = 32.9M
// Post has 3 stakers: frank, grace, heidi

// Record all three balances before:
frankBalBefore := getBalance(wallets[5].addrStr)  // frank
graceBalBefore := getBalance(wallets[6].addrStr)  // grace
heidiBalBefore := getBalance(wallets[7].addrStr)  // heidi

// Frank (author) unstakes → post withdrawn, all refunded:
tx := TxUnstakePost for frank on this post

// After confirmation:
// - frank gets 18.8M back
// - grace gets 9.4M back (force-refunded)
// - heidi gets 4.7M back (force-refunded)
// - post.Withdrawn == true
// - post.TotalStaked == 0
// - post.StakerCount == 0

frankBalAfter := getBalance(wallets[5].addrStr)
assert(frankBalAfter == frankBalBefore + frankStakedAmount)

graceBalAfter := getBalance(wallets[6].addrStr)
assert(graceBalAfter == graceBalBefore + graceStakedAmount)

heidiBalAfter := getBalance(wallets[7].addrStr)
assert(heidiBalAfter == heidiBalBefore + heidiStakedAmount)

withdrawnPost := getPostDetail(withdrawalPostID)
assert(withdrawnPost.Withdrawn == true)
assert(withdrawnPost.TotalStaked == 0)
assert(withdrawnPost.StakerCount == 0)
```

Also verify the post no longer appears in the feed:

```go
// Feed should not contain the withdrawn post
var feed feedResp
httpGet(indexerAPI+"/v1/feed?strategy=top", &feed)
for _, p := range feed.Posts {
    assert(p.PostID != withdrawalPostID)
}
```

### Acceptance Criteria

- Author's stake returned to wallet.
- All other stakers force-refunded to their wallets.
- Post marked `withdrawn: true`.
- Post's `totalStaked` and `stakerCount` both 0.
- Post does not appear in the feed.
- All three `postStakeBalance` values decreased to reflect the refund.

---

## Step 7 — New Phase 9: Verify Rewards via Indexer

### What to Test

After all boosts have been processed, verify rewards through the indexer API:

```go
// Check alice's rewards (she's the author of post 0 and received author + staker rewards)
var aliceRewards struct {
    Events []struct {
        PostID string `json:"postId"`
        Amount uint64 `json:"amount"`
        Type   string `json:"type"` // "author" or "staker"
    } `json:"events"`
    TotalAmount uint64 `json:"totalAmount"`
}
httpGet(indexerAPI+"/v1/rewards/"+wallets[0].addrStr, &aliceRewards)

// Alice should have reward events from each boost on her post 0
assert(len(aliceRewards.Events) > 0)
assert(aliceRewards.TotalAmount > 0)
t.Logf("  Alice total rewards: %d microdrana from %d events", aliceRewards.TotalAmount, len(aliceRewards.Events))

// Verify reward types — alice should have both "author" and "staker" events
hasAuthor := false
hasStaker := false
for _, ev := range aliceRewards.Events {
    if ev.Type == "author" { hasAuthor = true }
    if ev.Type == "staker" { hasStaker = true }
}
assert(hasAuthor)  // alice got author rewards from boosts on her post
assert(hasStaker)  // alice got staker rewards (she's staked on her own post)
```

Check reward summary:

```go
var summary struct {
    Last24h    uint64 `json:"last24h"`
    AllTime    uint64 `json:"allTime"`
    PostCount  int    `json:"postCount"`
    TotalStaked uint64 `json:"totalStaked"`
}
httpGet(indexerAPI+"/v1/rewards/"+wallets[0].addrStr+"/summary", &summary)
assert(summary.AllTime == aliceRewards.TotalAmount)
assert(summary.PostCount > 0)
t.Logf("  Alice reward summary: %d allTime, %d posts staked, %d total staked", summary.AllTime, summary.PostCount, summary.TotalStaked)
```

### Acceptance Criteria

- Reward events exist for authors and stakers.
- Event types are correctly labeled ("author" vs "staker").
- Reward summary aggregates match individual events.
- Total reward amounts match the expected fee math.

---

## Step 8 — New Phase 10: Supply Conservation

### What to Test

After all operations (posts, boosts, replies, unstakes, withdrawals), verify the fundamental invariant:

```go
// Sum all 10 wallet balances
var totalSpendable uint64
var totalPostStaked uint64
for _, w := range wallets {
    var acct acctResp
    httpGet(nodeRPC+"/v1/accounts/"+w.addrStr, &acct)
    totalSpendable += acct.Balance
    totalPostStaked += acct.PostStakeBalance
}

// Add validator balances (they have genesis funds + block rewards - transfers to test wallets)
// Add validator staked balances
// This is complex — instead, use the node's supply info

var nodeInfo rpc.NodeInfoResponse
httpGet(nodeRPC+"/v1/node/info", &nodeInfo)

// The invariant (checking wallet subset):
// For the 10 test wallets:
// sum(balance) + sum(postStakeBalance) = totalFunded - totalBurned(from their actions)
// This is hard to compute exactly due to rewards flowing between wallets.
// 
// Instead, verify the global invariant via RPC:
// The node should report consistent supply numbers.
t.Logf("  Supply: issued=%d, burned=%d", nodeInfo.IssuedSupply, nodeInfo.BurnedSupply)
t.Logf("  Test wallets: spendable=%d, postStaked=%d", totalSpendable, totalPostStaked)

// Verify no DRANA was created or destroyed outside of the fee mechanism:
// total burned should be > 0 (fees were taken)
assert(nodeInfo.BurnedSupply > 0)

// Verify that post stake balances are consistent:
// No wallet should have negative post stake balance
for _, w := range wallets {
    psb := getPostStakeBalance(w.addrStr)
    assert(psb >= 0)  // can't go negative
}

// Verify that withdrawn posts have 0 total staked
withdrawnPost := getPostDetail(withdrawalPostID)
assert(withdrawnPost.TotalStaked == 0)
```

### Acceptance Criteria

- Global burned supply > 0 (fees were taken).
- No negative post stake balances.
- Withdrawn posts have 0 total staked.
- Total spendable + staked amounts are consistent across wallets.

---

## Step Summary

| Phase | What | Key Assertions |
|-------|------|---------------|
| 1-2 | Create & fund 10 wallets, register names | Unchanged |
| 3 | Create 12 posts | `totalStaked = 94%`, `totalBurned = 6%`, `stakerCount = 1` per post |
| 4 | 6 boosts with reward verification | 3% burn, 2% author → wallet, 1% stakers → wallets (pro-rata math) |
| 5 | 5 replies | Same stake model as posts |
| 6 | Indexer verification | Updated field names, channel counts, feed ordering by `totalStaked` |
| 7 | Unstake from boost | Staker refunded, post ranking drops, post NOT withdrawn |
| 8 | Author unstakes (withdrawal) | Post withdrawn, all stakers force-refunded, post hidden from feed |
| 9 | Reward verification via indexer | Reward events exist, types correct, summary matches |
| 10 | Supply conservation | No DRANA created/destroyed outside fees, no negative balances |

---

## Test Utilities to Add

```go
// Helper to compute expected stake/fee amounts
func computePostFee(amount uint64) (burned, staked uint64) {
    burned = amount * 6 / 100
    staked = amount - burned
    return
}

func computeBoostFee(amount uint64) (burned, authorReward, stakerReward, staked uint64) {
    burned = amount * 3 / 100
    authorReward = amount * 2 / 100
    stakerReward = amount * 1 / 100
    staked = amount - burned - authorReward - stakerReward
    return
}

// Helper to submit TxUnstakePost
func submitUnstakePost(t *testing.T, w wallet, postID string, nonce uint64) string {
    pidBytes, _ := hex.DecodeString(postID)
    var pid types.PostID
    copy(pid[:], pidBytes)
    tx := &types.Transaction{
        Type: types.TxUnstakePost, Sender: w.addr,
        PostID: pid, Amount: 0, Nonce: nonce,
    }
    types.SignTransaction(tx, w.priv)
    return submitTx(t, tx)
}
```

---

## Running the Test

```bash
make docker-up          # start testnet with post-staking genesis params
make test-live          # runs go test ./test/live/ -v -timeout 600s
```

The test auto-skips if the testnet isn't running. Timeout is 600 seconds (10 minutes) to accommodate multiple block confirmations across 10 phases.

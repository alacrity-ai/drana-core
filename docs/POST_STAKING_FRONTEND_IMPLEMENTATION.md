# POST_STAKING_FRONTEND_IMPLEMENTATION.md

## Post Staking — Frontend Implementation Steps

Reference: `POST_STAKING_DESIGN.md`, `POST_STAKING_BACKEND_IMPLEMENTATION.md`

Prerequisite: Backend Steps 1-10 must be complete. The API returns `totalStaked` / `stakerCount` / `withdrawn` / `postStakeBalance` and the new endpoints `/posts/{id}/stakers`, `/accounts/{addr}/post-stakes`, `/rewards/{addr}`, and `/rewards/{addr}/summary` are live.

---

## Step 1 — API Types: Rename Fields, Add New Interfaces

### File: `drana-app/src/api/types.ts`

**Update `Post` interface:**

```typescript
export interface Post {
  postId: string;
  author: string;
  text: string;
  channel?: string;
  parentPostId?: string;
  createdAtHeight: number;
  createdAtTime: number;
  totalStaked: number;       // was totalCommitted
  totalBurned: number;       // new
  stakerCount: number;       // was boostCount
  withdrawn?: boolean;       // new
}
```

**Update `RankedPost`:**

```typescript
export interface RankedPost extends Post {
  authorStaked: number;         // was authorCommitted
  thirdPartyStaked: number;     // was thirdPartyCommitted
  uniqueBoosterCount: number;   // keep (unique stakers)
  lastBoostAtHeight: number;    // keep
  replyCount: number;
  score: number;
}
```

**Update `Account`:**

```typescript
export interface Account {
  address: string;
  balance: number;
  nonce: number;
  name?: string;
  stakedBalance: number;       // validator staking
  postStakeBalance: number;    // new — total locked across post stakes
}
```

**Update `AuthorProfile`:**

```typescript
export interface AuthorProfile {
  address: string;
  postCount: number;
  totalStaked: number;       // was totalCommitted
  totalReceived: number;     // rewards received
  uniqueBoosterCount: number;
}
```

**Add new interfaces:**

```typescript
export interface PostStakePosition {
  postId: string;
  amount: number;
  height: number;
}

export interface MyStakesResponse {
  stakes: PostStakePosition[];
  totalCount: number;
  totalStaked: number;
}

export interface StakerInfo {
  address: string;
  amount: number;
  height: number;
}

export interface RewardEvent {
  postId: string;
  recipient: string;
  amount: number;
  blockHeight: number;
  blockTime: number;
  trigger: string;
  type: 'author' | 'staker';
}

export interface RewardSummary {
  last24h: number;
  last7d: number;
  allTime: number;
  postCount: number;
  totalStaked: number;
}
```

### Acceptance Criteria

- All type names match the backend response field names exactly.
- No references to `totalCommitted`, `boostCount`, `authorCommitted`, or `thirdPartyCommitted` remain.

---

## Step 2 — API Client: New Endpoints

### File: `drana-app/src/api/nodeRpc.ts`

Add:

```typescript
export const getPostStakers = (postId: string) =>
  get<{ stakers: StakerInfo[] }>(`/v1/posts/${postId}/stakers`);
export const getMyPostStakes = (addr: string) =>
  get<MyStakesResponse>(`/v1/accounts/${addr}/post-stakes`);
```

### File: `drana-app/src/api/indexerApi.ts`

Replace `getPostBoosts` with `getPostStakers` (the concept changes from "boost history" to "active staker list"):

```typescript
export const getPostStakers = (id: string, page = 1) =>
  get<{ stakers: StakerInfo[]; totalCount: number }>(`/v1/posts/${id}/stakers?page=${page}`);
```

Add reward endpoints:

```typescript
export function getRewards(address: string, sinceHeight = 0, page = 1) {
  return get<{ events: RewardEvent[]; totalCount: number; totalAmount: number }>(
    `/v1/rewards/${address}?since=${sinceHeight}&page=${page}`);
}
export const getRewardSummary = (address: string) =>
  get<RewardSummary>(`/v1/rewards/${address}/summary`);
```

Keep `getPostBoosts` as an alias or remove — the indexer may still expose boost history, but the primary view is now the staker list.

### Acceptance Criteria

- All new endpoints are typed and callable.
- `getPostStakers` returns active positions (not historical events).
- `getRewards` and `getRewardSummary` return reward data.

---

## Step 3 — PostCard: Staked Amount, Your Stake, Unstake Button

### File: `drana-app/src/components/PostCard.tsx`

**Changes:**

1. Replace `post.totalCommitted` with `post.totalStaked` in the DranaAmount display.
2. Replace `post.uniqueBoosterCount` with `post.stakerCount` label: `📌 8 stakers` instead of `👥 8`.
3. Add "Your stake" display if the user has a position on this post.
4. Show `[Unstake]` button instead of `[Boost]` when the user has an active stake.
5. Grey out / mark withdrawn posts.

The component needs to know the user's stake on this post. Two options:
- Pass it as a prop from the parent (requires the parent to batch-fetch stakes)
- Fetch inside the component (N+1 queries)

**Recommendation:** Add an optional `userStake` prop. The Feed page pre-fetches the user's stake positions (one API call for all of them via `getMyPostStakes`) and passes the matching stake amount to each PostCard.

```typescript
export function PostCard({ post, onBoost, onReply, onUnstake, isHighValue, userStake }: {
  post: RankedPost;
  onBoost: () => void;
  onReply: () => void;
  onUnstake?: () => void;
  isHighValue?: boolean;
  userStake?: number;  // microdrana, 0 or undefined = not staked
}) {
```

Display logic:
- If `userStake > 0`: show "Your stake: X.XX" and `[Unstake]` button
- If `userStake === 0` or undefined: show `[Boost]` button
- If `post.withdrawn`: dim the card, show "WITHDRAWN" label, no action buttons

### Acceptance Criteria

- Posts show `📌 42.50 DRANA staked (8 stakers)` instead of `🔥 42.50 DRANA`.
- User sees their own stake amount on posts they're staked on.
- `[Unstake]` button appears when user has a position.
- Withdrawn posts are visually distinct and non-interactive.

---

## Step 4 — NewPostModal: Fee/Stake Breakdown

### File: `drana-app/src/components/NewPostModal.tsx`

**Changes:**

Replace the burn warning with a fee/stake breakdown that updates live as the user types the amount:

```typescript
const fee = parseFloat(amount) * 0.06;
const staked = parseFloat(amount) - fee;
```

Display:

```
Amount: [100] DRANA
├── Fee (6%):  6.00 DRANA burned
└── Staked:   94.00 DRANA (recoverable)

You can unstake and recover 94.00 DRANA at any time.
```

Replace: `"This burns {amount} DRANA permanently."` → the breakdown above.

Color: the fee line in `var(--error)` (red), the staked line in `var(--success)` (green), the recovery message in `var(--text-secondary)`.

### Acceptance Criteria

- Fee breakdown updates live as amount changes.
- No mention of "burns permanently" for the staked portion.
- Fee percentage matches genesis config (hardcoded to 6% in frontend for v1).

---

## Step 5 — BoostModal: Fee/Stake Breakdown with Recipient Info

### File: `drana-app/src/components/BoostModal.tsx`

**Changes:**

Replace burn warning with the boost fee breakdown:

```typescript
const total = parseFloat(amount);
const burned = total * 0.03;
const authorReward = total * 0.02;
const stakerReward = total * 0.01;
const staked = total - burned - authorReward - stakerReward;
```

Display:

```
Amount: [50] DRANA
├── Fee (6%):    3.00 DRANA
│   ├── Burned:  1.50 DRANA
│   ├── Author:  1.00 DRANA → their wallet
│   └── Stakers: 0.50 DRANA → split to current stakers
└── Staked:     47.00 DRANA (recoverable)

You can unstake and recover 47.00 DRANA at any time.
```

Also update the "Currently X DRANA" line from `post.totalCommitted` to `post.totalStaked`.

Replace: `"This burns {amount} DRANA permanently."` → the breakdown above.

### Acceptance Criteria

- Fee breakdown shows burn/author/staker split.
- Staked portion clearly marked as recoverable.
- No "burns permanently" language for the principal.

---

## Step 6 — UnstakeModal: New Component

### File: `drana-app/src/components/UnstakeModal.tsx` (new)

A confirmation modal for unstaking from a post:

```
Unstake from Post

"The empire of relevance..."
by satoshi · #general

Your stake: 94.00 DRANA
This will return 94.00 DRANA to your wallet.

⚠ If you are the author, this will withdraw the post
  and refund all other stakers.

[Unstake]    [Cancel]
```

On confirm: `signAndSubmit({ type: 7, postId: post.postId, amount: 0 })`. Amount is 0 because it's all-or-nothing — the chain looks up the position.

Add to `PendingContext`: type 7 = unstake_post.

### Acceptance Criteria

- Modal shows the post text, author, and stake amount.
- Author warning appears only when the user is the post author.
- Submits `TxUnstakePost` (type 7) transaction.
- On success, stake returns to spendable balance.

---

## Step 7 — TopBar: Post Stake Balance

### File: `drana-app/src/components/TopBar.tsx`

**Changes:**

The balance display in the wallet pill currently shows only spendable balance. Add post stake balance:

```
[satoshi · 42.50 DRANA | 📌 150.00]
```

Or if no post stakes:

```
[satoshi · 42.50 DRANA]
```

The `📌 150.00` indicates DRANA locked in post stakes. Clicking the wallet dropdown shows the full breakdown.

**WalletContext changes:** Add `postStakeBalance` to the context. The `refreshBalance` function already fetches the account — just read the new field.

### Acceptance Criteria

- Post stake balance visible in the wallet pill when > 0.
- Three-part balance breakdown accessible in the dropdown or tooltip.

---

## Step 8 — Rewards Page: Dashboard, Feed, Cha-Ching

### Files

**`drana-app/src/pages/Rewards.tsx`** (new)

A new page at route `#/rewards`:

1. **Summary section:** Rewards earned in last 24h, 7d, all-time. Number of posts staked on. Total DRANA staked.
2. **Reward feed:** Chronological list of reward events (who boosted what, how much you earned).
3. **My Stakes:** All posts you're staked on with per-post cumulative earnings and `[Unstake]` buttons.

Data sources:
- `getRewardSummary(address)` for the summary
- `getRewards(address, sinceHeight, page)` for the feed
- `getMyPostStakes(address)` for the stakes list

**`drana-app/src/components/ChaChing.tsx`** (new)

A toast notification component. On app load, if the user has a wallet:
1. Read `lastSeenRewardHeight` from localStorage.
2. Fetch `getRewards(address, lastSeenRewardHeight)`.
3. If `totalAmount > 0`, show toast: `💰 +X.XX DRANA — Rewards since last visit`.
4. Update `lastSeenRewardHeight` to current chain height.
5. Auto-dismiss after 5 seconds. Click navigates to `/rewards`.

**`drana-app/src/router.ts`**

Add route: `if (parts[0] === 'rewards') return { page: 'rewards', params: {} };`

**`drana-app/src/App.tsx`**

Add `case 'rewards': content = <Rewards />;` and render `<ChaChing />`.

**`drana-app/src/components/TopBar.tsx`**

Add "Rewards" to the nav bar: `<a href="#/rewards" className="nav-link">Rewards</a>`

Add "My Stakes" to the wallet dropdown (links to `#/rewards`).

### Acceptance Criteria

- `/rewards` page shows summary, reward feed, and stake positions.
- Cha-ching toast appears on load when rewards have accumulated.
- Clicking the toast navigates to the rewards page.
- "Rewards" link in nav bar.
- "My Stakes" in wallet dropdown links to rewards page.

---

## Step 9 — PostDetail: Stakers List Replaces Boost History

### File: `drana-app/src/pages/PostDetail.tsx`

**Changes:**

1. Replace `boosts` query with `stakers` query:
   ```typescript
   const stakers = useQuery({ queryKey: ['stakers', id], queryFn: () => getPostStakers(id) });
   ```

2. Replace the "BOOST HISTORY" section with "STAKERS":
   ```
   STAKERS (8) — 42.50 DRANA total
     satoshi (author)    10.00 DRANA    since block 42
     bob                  8.00 DRANA    since block 55
     carol                6.50 DRANA    since block 58
   ```

3. Update the amount display from `p.totalCommitted` to `p.totalStaked`.

4. Update the breakdown from `authorCommitted / totalCommitted - authorCommitted` to `authorStaked / thirdPartyStaked`.

5. Change "Boost this post" button text to "Stake on this post".

6. If the user has an active stake, show `[Unstake]` instead of `[Stake on this post]`.

7. If post is withdrawn, show the `⚠ WITHDRAWN` banner and disable all action buttons.

### Acceptance Criteria

- Staker list shows active positions with amounts and block heights.
- Boost history section is gone (or renamed to "Stake History" if the indexer still provides it).
- Amount breakdown uses `authorStaked` / `thirdPartyStaked`.
- Withdrawn posts show the warning banner.

---

## Step 10 — Profile: Post Stake Balance Display

### File: `drana-app/src/pages/Profile.tsx`

**Changes:**

Add "Post Stakes" to the account stats section:

```typescript
{account.data?.postStakeBalance > 0 && (
  <div>
    <span className="label">Post Stakes</span>
    <div><DranaAmount microdrana={account.data.postStakeBalance} size={16} /></div>
  </div>
)}
```

Update "Boosts Received" label to "Rewards Earned":

```typescript
<span className="label">Rewards Earned</span>
<div><DranaAmount microdrana={profile.data.totalReceived} size={16} /></div>
```

### Acceptance Criteria

- Profile shows post stake balance when > 0.
- "Rewards Earned" label replaces "Boosts Received".

---

## Step 11 — WalletContext and PendingContext Updates

### File: `drana-app/src/wallet/WalletContext.tsx`

1. Add `postStakeBalance` to the context type and state.
2. Update `refreshBalance` to read `acct.postStakeBalance`.
3. Update the tx type map to include `'unstake_post'` at index 7:
   ```typescript
   type: ['', 'transfer', 'create_post', 'boost_post', 'register_name', 'stake', 'unstake', 'unstake_post'][fullTx.type]
   ```

### File: `drana-app/src/wallet/PendingContext.tsx`

Update the pending tx type descriptions:

```typescript
tx.type === 7 ? 'Unstake Post' : ...
```

### Acceptance Criteria

- `postStakeBalance` is accessible via `useWallet()`.
- `unstake_post` transactions can be submitted via `signAndSubmit`.
- Pending context recognizes type 7.

---

## Step 12 — Feed: Fetch User Stakes for PostCard

### File: `drana-app/src/pages/Feed.tsx`

Add a query to fetch the current user's post stake positions:

```typescript
const myStakes = useQuery({
  queryKey: ['my-post-stakes', wallet.address],
  queryFn: () => wallet.address ? getMyPostStakes(wallet.address) : null,
  enabled: !!wallet.address && wallet.isUnlocked,
});

const stakeMap = new Map<string, number>();
myStakes.data?.stakes?.forEach(s => stakeMap.set(s.postId, s.amount));
```

Pass to each PostCard:

```typescript
<PostCard ... userStake={stakeMap.get(post.postId) || 0} onUnstake={() => ...} />
```

Add an unstake modal state and handler (similar to boost modal).

### Acceptance Criteria

- User's stake positions are fetched once and distributed to PostCards.
- PostCards show "Your stake" and `[Unstake]` when applicable.
- Unstake modal opens on click.

---

## Files Summary

| Step | Files | Change |
|------|-------|--------|
| 1 | `api/types.ts` | Rename fields, add PostStakePosition, StakerInfo, RewardEvent, RewardSummary |
| 2 | `api/nodeRpc.ts`, `api/indexerApi.ts` | Add getPostStakers, getMyPostStakes, getRewards, getRewardSummary |
| 3 | `components/PostCard.tsx` | totalStaked, stakerCount, userStake prop, Unstake button, withdrawn state |
| 4 | `components/NewPostModal.tsx` | Fee/stake breakdown, remove "burns permanently" |
| 5 | `components/BoostModal.tsx` | Fee breakdown (burn/author/staker), remove "burns permanently" |
| 6 | `components/UnstakeModal.tsx` | New: confirmation modal for unstaking |
| 7 | `components/TopBar.tsx` | Post stake balance in pill, dropdown |
| 8 | `pages/Rewards.tsx`, `components/ChaChing.tsx`, `router.ts`, `App.tsx` | New: rewards dashboard, reward feed, cha-ching toast, route |
| 9 | `pages/PostDetail.tsx` | Stakers list replaces boost history, totalStaked, withdrawn banner |
| 10 | `pages/Profile.tsx` | Post stake balance, "Rewards Earned" label |
| 11 | `wallet/WalletContext.tsx`, `wallet/PendingContext.tsx` | postStakeBalance, unstake_post type support |
| 12 | `pages/Feed.tsx` | Batch-fetch user stakes, pass to PostCards, unstake handler |

# PENDING_ENHANCEMENT_DESIGN.md

## Pending Transaction UX

### Problem

After submitting a transaction (post, boost, transfer, etc.), the user sees nothing change until the next block is produced (up to 120 seconds). Their balance doesn't update, their post doesn't appear in the feed, and there's no indication that anything happened. This makes the app feel broken.

### What Already Exists

The backend has everything needed for basic pending tracking:

| Capability | Endpoint / Method | Status |
|-----------|-------------------|--------|
| Tx hash returned on submit | `POST /v1/transactions` → `{ txHash: "..." }` | Exists |
| Tx status polling | `GET /v1/transactions/{hash}/status` → `confirmed/pending/unknown` | Exists |
| Check if tx is in mempool | `mempool.Has(txHash)` (internal) | Exists but not exposed |
| List all pending txs | `mempool.Pending()` (internal) | Exists but not exposed |
| List pending txs by sender | — | Does not exist |

### What's Missing

**Backend:** One new endpoint to list pending transactions for a specific sender. This lets the frontend show "your pending transactions" without tracking them client-side across page refreshes.

**Frontend:** A pending transaction tracker that:
1. Records submitted tx hashes locally
2. Polls their status until confirmed
3. Shows optimistic UI (pending post in feed, estimated balance)

---

## Proposed Solution

### Layer 1: Backend — New Endpoint

**`GET /v1/mempool/pending?sender={address}`**

Returns pending transactions from the mempool, optionally filtered by sender.

Response:
```json
{
  "transactions": [
    {
      "hash": "f8e7...",
      "type": "create_post",
      "sender": "drana1...",
      "amount": 1000000,
      "nonce": 5,
      "text": "My pending post",
      "channel": "gaming"
    }
  ],
  "count": 1
}
```

Without `?sender=`, returns all pending txs (capped at 100). With `?sender=`, filters to that address only.

Implementation: the mempool already has `Pending()` which returns all txs. The handler filters by sender and converts to `TransactionResponse` format. ~20 lines of new code in `rpc/server.go`.

### Layer 2: Frontend — Pending Transaction Tracker

A React context that tracks submitted transactions and their lifecycle:

**`src/wallet/PendingContext.tsx`**

```typescript
type PendingTx = {
  hash: string;
  type: number;        // TxType
  text?: string;       // for posts
  channel?: string;    // for posts
  amount: number;
  submittedAt: number; // unix ms
  status: 'pending' | 'confirmed' | 'failed';
};

type PendingContextType = {
  pendingTxs: PendingTx[];
  addPending: (tx: PendingTx) => void;
  getPendingPosts: () => PendingTx[];
  estimatedBalance: number;  // balance - sum(pending outgoing amounts)
};
```

**How it works:**

1. When `signAndSubmit` succeeds (accepted: true), the wallet context calls `addPending({ hash, type, amount, text, channel, ... })`.

2. The pending context polls `GET /v1/transactions/{hash}/status` every 5 seconds for each pending tx.

3. When a tx transitions to `confirmed`, it's removed from pending and the feed/balance queries are invalidated (TanStack Query cache).

4. If a tx stays pending for > 5 minutes, mark it as `failed` (likely dropped from mempool).

5. On page load, if there are stale pending entries in localStorage, reconcile them against the mempool endpoint: `GET /v1/mempool/pending?sender={myAddress}`. If the tx is no longer in the mempool and not confirmed, it was dropped.

### Layer 3: Frontend — Optimistic UI

**Estimated balance:**

The top bar balance display changes from:
```
42.50 DRANA
```
to (when there are pending txs):
```
~41.50 DRANA (1 pending)
```

The estimated balance is: `confirmed balance - sum(pending tx amounts)`. The `~` prefix and "(N pending)" suffix make it clear this is an estimate.

When all pending txs confirm, the `~` disappears and it shows the real confirmed balance.

**Pending posts in feed:**

When the user creates a post, a "pending post" card appears at the top of the feed immediately, styled differently:

```
┌─────────────────────────────────────────────────────────────────────┐
│  ⏳ PENDING                                                    just now │
│                                                                     │
│  you · #gaming                                                      │
│  "Hello world! It's great to see you!"                              │
│                                                                     │
│  1.00 DRANA committed · Waiting for next block...                   │
└─────────────────────────────────────────────────────────────────────┘
```

Visual differences from confirmed posts:
- Left border is dashed amber instead of solid
- Amount is dimmed (not the usual bright amber)
- Shows "PENDING" label and "Waiting for next block..." instead of reply/boost counts
- No Boost or Reply buttons (can't interact with pending posts)

When the tx confirms, the pending card disappears and the real post appears in its normal ranked position (after the next feed refresh).

**Pending indicator in top bar:**

When there are pending txs, show a small amber dot on the wallet pill:

```
[satoshi · ~41.50 DRANA 🟡]
```

Clicking the wallet dropdown shows pending txs at the top:

```
┌──────────────────────────────────┐
│  Pending (2)                     │
│    ⏳ Post in #gaming  1.0 DRANA │
│    ⏳ Boost            0.5 DRANA │
│  ─────────────────────────────── │
│  Send DRANA                      │
│  Register Name                   │
│  ...                             │
└──────────────────────────────────┘
```

---

## Implementation Plan

### Step 1 — Backend: Mempool Pending Endpoint

**Files:** `internal/rpc/server.go`, `internal/rpc/types.go`

Add `GET /v1/mempool/pending` endpoint. Register route. Handler calls `engine.Mempool.Pending()`, filters by `?sender=` if present, converts to response format, caps at 100 results.

New response type:
```go
type MempoolResponse struct {
    Transactions []TransactionResponse `json:"transactions"`
    Count        int                   `json:"count"`
}
```

### Step 2 — Frontend: PendingContext

**Files:** `src/wallet/PendingContext.tsx`

React context that:
- Stores pending txs in state + localStorage (survives refresh)
- Polls tx status every 5 seconds
- Reconciles with mempool endpoint on page load
- Exposes `pendingTxs`, `addPending`, `getPendingPosts`, `estimatedBalance`
- Invalidates TanStack Query cache keys when txs confirm

### Step 3 — Frontend: Wire into WalletContext

**Files:** `src/wallet/WalletContext.tsx`

After `signAndSubmit` succeeds, call `pendingContext.addPending(...)` with the tx details. The estimated balance computation: `balance - sumOfPendingAmounts`.

### Step 4 — Frontend: Optimistic Feed

**Files:** `src/pages/Feed.tsx`, `src/components/PendingPostCard.tsx`

New `PendingPostCard` component with the dashed-border pending style. The Feed page prepends pending posts (from `getPendingPosts()`) above the real feed.

### Step 5 — Frontend: Top Bar Pending Indicator

**Files:** `src/components/TopBar.tsx`

Show `~` prefix on balance when pending txs exist. Show pending count. Show pending tx list in the dropdown.

---

## What This Does NOT Solve

- **Pending posts are not visible to other users.** Only the author sees the optimistic card. Other users see it after confirmation. This is correct — the post isn't on-chain yet.
- **No real-time push.** We poll every 5 seconds. A WebSocket would be more responsive but is significant scope. Polling is fine for 120-second block intervals.
- **Pending boosts don't update the post's committed amount** in the feed for other users. Only the booster sees their pending boost reflected in their estimated balance.
- **No mempool ordering guarantees.** The pending tx will be included in the next block proposed by a validator, but the exact order within the block is proposer-controlled.

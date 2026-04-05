# API Reference

> **Note:** All uint64 values (amounts, balances, heights, nonces, timestamps, stake amounts) are serialized as JSON strings to prevent JavaScript floating-point precision loss. Parse them as integers on the client side.

All amounts are in **microdrana** (1 DRANA = 1,000,000 microdrana). Hex values are lowercase, no `0x` prefix. Addresses use the `drana1` display format.

---

## Node RPC

Default port: **26657**

### Chain Info

#### GET /v1/node/info

Returns chain status, supply statistics, and epoch information.

**Response:**

```json
{
  "chainId": "drana-testnet-1",
  "latestHeight": "142",
  "latestHash": "a3f8...",
  "genesisTime": 1700000000,
  "blockIntervalSec": 120,
  "blockReward": "10000000",
  "burnedSupply": "50000000",
  "issuedSupply": "1420000000",
  "validatorCount": 3,
  "currentEpoch": "4",
  "epochLength": "30",
  "blocksUntilNextEpoch": "8"
}
```

### Blocks

#### GET /v1/blocks/latest

Returns the most recent finalized block.

**Query params:** `full=true` to include full transaction list.

**Response:**

```json
{
  "height": "142",
  "hash": "a3f8...",
  "prevHash": "b2e1...",
  "proposerAddr": "drana1...",
  "timestamp": 1700017040,
  "stateRoot": "c4d5...",
  "txRoot": "e6f7...",
  "txCount": 3,
  "transactions": []
}
```

#### GET /v1/blocks/{height}

Returns a block at the specified height. Returns 404 if not found.

**Query params:** `full=true` to include transactions.

#### GET /v1/blocks/hash/{hash}

Returns a block by its header hash (64-char hex). Returns 404 if not found.

**Query params:** `full=true` to include transactions.

### Accounts

#### GET /v1/accounts/{address}

Returns account balance, staked balance, nonce, and name. Returns zero values (not 404) for unknown addresses — any address can receive funds.

**Response:**

```json
{
  "address": "drana1...",
  "balance": "950000000",
  "nonce": "5",
  "name": "alice",
  "stakedBalance": "1000000000",
  "postStakeBalance": "500000000"
}
```

#### GET /v1/accounts/name/{name}

Resolves a registered name to an account. Returns 404 if the name is not registered.

**Response:** Same as `/v1/accounts/{address}`.

#### GET /v1/accounts/{address}/unbonding

Returns pending unbonding entries for an address (DRANA in the process of being unstaked).

**Response:**

```json
{
  "address": "drana1...",
  "entries": [
    { "amount": "500000000", "releaseHeight": "180" }
  ],
  "total": "500000000"
}
```

### Posts

#### GET /v1/posts/{id}

Returns a single post by its 64-char hex ID. Returns 404 if not found.

**Response:**

```json
{
  "postId": "a1b2...",
  "author": "drana1...",
  "text": "The attention economy is live.",
  "channel": "general",
  "parentPostId": "",
  "createdAtHeight": "42",
  "createdAtTime": 1700005040,
  "totalStaked": "15000000",
  "totalBurned": "900000",
  "stakerCount": "3",
  "withdrawn": false
}
```

#### GET /v1/posts

Returns a paginated list of top-level posts, sorted by creation height (newest first). Replies are excluded by default.

**Query params:**

| Param | Default | Description |
|-------|---------|-------------|
| `page` | 1 | Page number |
| `pageSize` | 20 | Results per page (max 100) |
| `author` | — | Filter by author address (drana1...) |
| `channel` | — | Filter by channel name |
| `includeReplies` | false | Set to `true` to include replies |

**Response:**

```json
{
  "posts": [...],
  "totalCount": 47,
  "page": 1,
  "pageSize": 20
}
```

### Transactions

#### POST /v1/transactions

Submits a signed transaction. The transaction must be fully constructed and signed client-side.

**Request body:**

```json
{
  "type": "transfer",
  "sender": "drana1...",
  "recipient": "drana1...",
  "amount": "1000000",
  "nonce": "6",
  "signature": "a1b2c3...",
  "pubKey": "d4e5f6..."
}
```

Supported `type` values: `transfer`, `create_post`, `boost_post`, `unstake_post`, `register_name`, `stake`, `unstake`.

Type-specific fields:
- `transfer`: `recipient` (required)
- `create_post`: `text` (required), `channel` (optional), `parentPostId` (optional, 64-char hex — makes this a reply). 6% fee burned, 94% staked.
- `boost_post`: `postId` (required, 64-char hex). 3% burned, 2% to author, 1% to stakers, 94% staked.
- `unstake_post`: `postId` (required). All-or-nothing — returns your full stake. If author, post is withdrawn and all stakers refunded.
- `register_name`: `text` (required, the name to register), `amount` must be 0
- `stake`: `amount` (required, microdrana to stake)
- `unstake`: `amount` (required, microdrana to unstake)

**Response:**

```json
{
  "accepted": true,
  "txHash": "f8e7..."
}
```

Or on rejection:

```json
{
  "accepted": false,
  "error": "insufficient balance: have 500, need 1000"
}
```

#### GET /v1/transactions/{hash}

Returns a confirmed transaction by its 64-char hex hash. Returns 404 if not found.

**Response:**

```json
{
  "hash": "f8e7...",
  "type": "transfer",
  "sender": "drana1...",
  "recipient": "drana1...",
  "amount": "1000000",
  "nonce": "6",
  "blockHeight": "42"
}
```

#### GET /v1/transactions/{hash}/status

Returns the status of a transaction: `confirmed` (with block height), `pending` (in mempool), or `unknown`.

**Response:**

```json
{
  "hash": "f8e7...",
  "status": "confirmed",
  "blockHeight": "42"
}
```

### Network

#### GET /v1/network/validators

Returns the active validator set with stake amounts.

**Response:**

```json
[
  {
    "address": "drana1...",
    "name": "alice",
    "pubKey": "a1b2...",
    "stakedBalance": "1000000000",
  "postStakeBalance": "500000000"
  }
]
```

#### GET /v1/network/peers

Returns connected peers.

**Response:**

```json
[
  { "endpoint": "192.168.1.10:26601" }
]
```

### Error Responses

All errors return an appropriate HTTP status code with a JSON body:

```json
{ "error": "block at height 999 not found" }
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request (invalid address, malformed hash, etc.) |
| 404 | Resource not found |
| 405 | Method not allowed |
| 500 | Internal server error |

---

## Indexer API

Default port: **26680**

The indexer is a separate process that follows the chain and provides rich query capabilities beyond what the node RPC offers.

### Feed

#### GET /v1/feed

Returns a ranked feed of top-level posts. Replies are excluded.

**Query params:**

| Param | Default | Description |
|-------|---------|-------------|
| `strategy` | `trending` | Ranking algorithm: `trending`, `top`, `new`, `controversial` |
| `channel` | — | Filter by channel name |
| `page` | 1 | Page number |
| `pageSize` | 20 | Results per page (max 100) |

**Ranking strategies:**

| Strategy | Formula | Best for |
|----------|---------|----------|
| `trending` | `log(1 + committed) / (1 + ageHours)^1.5` | Discovery — recent high-value posts |
| `top` | `totalStaked` | Conviction — highest total burn |
| `new` | `createdAtHeight` | Freshness — chronological |
| `controversial` | `stakerCount * log(1 + committed)` | Engagement — many boosters |

**Response:**

```json
{
  "posts": [
    {
      "postId": "a1b2...",
      "author": "drana1...",
      "text": "The attention economy is live.",
      "channel": "general",
      "parentPostId": "",
      "createdAtHeight": "42",
      "createdAtTime": 1700005040,
      "totalStaked": "15000000",
      "authorStaked": "10000000",
      "thirdPartyStaked": "5000000",
      "stakerCount": "3",
      "uniqueBoosterCount": 2,
      "lastBoostAtHeight": "98",
      "replyCount": 5,
      "score": 14.72
    }
  ],
  "totalCount": 47,
  "page": 1,
  "pageSize": 20,
  "strategy": "trending"
}
```

#### GET /v1/feed/author/{address}

Same as `/v1/feed` but filtered to a single author. Same query params and response format.

### Channels

#### GET /v1/channels

Returns all channels with post counts, sorted by count descending.

**Response:**

```json
[
  { "channel": "general", "postCount": 42 },
  { "channel": "gaming", "postCount": 15 },
  { "channel": "politics", "postCount": 8 }
]
```

### Posts (Enriched)

#### GET /v1/posts/{id}

Returns a post with all derived fields (richer than the node RPC version).

**Response:**

```json
{
  "postId": "a1b2...",
  "author": "drana1...",
  "text": "...",
  "channel": "general",
  "parentPostId": "",
  "createdAtHeight": "42",
  "createdAtTime": 1700005040,
  "totalStaked": "15000000",
  "authorStaked": "10000000",
  "thirdPartyStaked": "5000000",
  "totalBurned": "900000",
  "withdrawn": false,
  "stakerCount": "3",
  "uniqueBoosterCount": 2,
  "lastBoostAtHeight": "98",
  "replyCount": 5
}
```

#### GET /v1/posts/{id}/replies

Returns replies to a post, sorted by committed value (highest first).

**Query params:** `page`, `pageSize` (same defaults as feed).

**Response:**

```json
{
  "replies": [
    {
      "postId": "c3d4...",
      "author": "drana1...",
      "text": "Great point!",
      "parentPostId": "a1b2...",
      "totalStaked": "2000000",
      "stakerCount": "1",
      "replyCount": 0
    }
  ],
  "totalCount": 5,
  "page": 1,
  "pageSize": 20
}
```

#### GET /v1/posts/{id}/boosts

Returns the boost history for a post, including the fee breakdown for each boost.

**Query params:** `page`, `pageSize` (same defaults as feed).

**Response:**

```json
{
  "boosts": [
    {
      "postId": "a1b2...",
      "booster": "drana1...",
      "amount": "5000000",
      "authorReward": "100000",
      "stakerReward": "50000",
      "burnAmount": "150000",
      "stakedAmount": "4700000",
      "blockHeight": "55",
      "blockTime": 1700006600,
      "txHash": "c3d4..."
    }
  ],
  "totalCount": 3,
  "page": 1,
  "pageSize": 20
}
```

### Authors

#### GET /v1/authors/{address}

Returns aggregate profile stats for an author. Returns 404 if the address has no posts.

**Response:**

```json
{
  "address": "drana1...",
  "postCount": 12,
  "totalStaked": "85000000",
  "totalReceived": "23000000",
  "uniqueBoosterCount": 7
}
```

### Rewards

#### GET /v1/rewards/{address}

Returns paginated reward events for an address. Includes both author rewards (2% of boosts on your posts) and staker rewards (1% pro-rata share when someone boosts a post you're staked on).

**Query params:**

| Param | Default | Description |
|-------|---------|-------------|
| `since` | 0 | Only return events at or above this block height |
| `page` | 1 | Page number |
| `pageSize` | 20 | Results per page (max 100) |

**Response:**

```json
{
  "events": [
    {
      "postId": "a1b2...",
      "recipient": "drana1...",
      "amount": "200000",
      "blockHeight": "42",
      "blockTime": 1700000000,
      "triggerTx": "c3d4...",
      "triggerAddress": "drana1...",
      "type": "author"
    }
  ],
  "totalCount": 15,
  "totalAmount": "3500000"
}
```

#### GET /v1/rewards/{address}/summary

Returns aggregate reward stats for an address.

**Response:**

```json
{
  "last24h": "500000",
  "last7d": "2500000",
  "allTime": "12000000",
  "postCount": 3,
  "totalStaked": "9400000"
}
```

#### GET /v1/rewards/{address}/post/{postId}

Returns total lifetime rewards earned by an address from a specific post.

**Response:**

```json
{
  "totalReward": "750000"
}
```

### Analytics

#### GET /v1/stats

Returns global chain statistics.

**Response:**

```json
{
  "latestHeight": "142",
  "totalPosts": 47,
  "totalBoosts": 89,
  "totalTransfers": 23,
  "totalBurned": "50000000",
  "totalIssued": "1420000000",
  "circulatingSupply": "1370000000"
}
```

#### GET /v1/leaderboard

Returns authors ranked by total boosts received.

**Query params:** `page`, `pageSize` (same defaults as feed).

**Response:**

```json
{
  "authors": [
    {
      "address": "drana1...",
      "totalReceived": "23000000",
      "postCount": 12,
      "stakerCount": 15
    }
  ],
  "totalCount": 8,
  "page": 1,
  "pageSize": 20
}
```

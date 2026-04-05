# TODO

Items to complete before or shortly after the first public network launch.

---

## Register DNS and deploy genesis validator

**Priority:** Required before launch

The seed node address `genesis-validator.drana.io` is currently referenced in three places:

1. **Hardcoded in binary** — `internal/node/seeds.go` contains `DefaultSeedNodes` with `genesis-validator.drana.io:26601`. This is compiled into every `drana-node` binary so that new nodes can find the network without any configuration beyond the genesis file.

2. **In the genesis file** — `networks/mainnet/genesis.json` has a `seedNodes` array containing the same address. Every node on the network loads this at startup.

3. **In the README and MINERS_GUIDE** — documentation references this endpoint for RPC queries and as the seed for new validators.

**Why this matters:** Without a reachable seed node at this address, new validators cannot join the network. The genesis file and hardcoded defaults both point here. If the domain doesn't resolve, new nodes fall back to peer exchange with whatever manual `peerEndpoints` they configure — which defeats the purpose of easy onboarding.

**To complete:**
- [ ] Register `drana.io` domain
- [ ] Deploy genesis validator to a VPS (see NETWORK_LAUNCH_GUIDE.md)
- [ ] Create DNS A record: `genesis-validator.drana.io` → server IP
- [ ] Verify: `curl http://genesis-validator.drana.io:26657/v1/node/info`
- [ ] Optional: deploy indexer, create `indexer.drana.io` DNS record

---

## Add additional seed nodes

**Priority:** After launch, before the network has 10+ validators

A single seed node is a single point of failure for onboarding. If `genesis-validator.drana.io` is unreachable, new nodes can't discover the network (existing nodes are fine — they already know each other via peer exchange).

**To complete:**
- [ ] Recruit 2-3 community members to run stable seed nodes
- [ ] Add their endpoints to `DefaultSeedNodes` in `internal/node/seeds.go`
- [ ] Add their endpoints to `seedNodes` in `networks/mainnet/genesis.json`
- [ ] Release a new binary version with the updated seed list

---

## Build a faucet

**Priority:** Nice-to-have for testnet, important for onboarding

New users need DRANA to stake or post. Currently the only way to get DRANA is to receive a transfer from someone who has it. A faucet — a simple web endpoint that sends a small amount of DRANA to any address — removes this friction.

**To complete:**
- [ ] Build a simple HTTP service that signs and submits transfer txs from a funded wallet
- [ ] Rate-limit by IP or address (e.g., 10 DRANA per address per day)
- [ ] Deploy at `faucet.drana.io`

---

## TLS / HTTPS for RPC

**Priority:** Before any production use

The RPC server currently uses plain HTTP. For a public-facing endpoint, this should be TLS-terminated. Options:
- Reverse proxy (nginx, Caddy) in front of the RPC port with Let's Encrypt
- Cloudflare proxy (if using Cloudflare DNS)

The P2P gRPC port (26601) would also benefit from TLS but is lower priority since it's validator-to-validator traffic.

---

## Channels and Replies

**Priority:** After launch, before major adoption push

Two features that turn DRANA from a single billboard into something closer to a forum: **channels** (topic-scoped feeds) and **replies** (flat comment threads on posts).

### Channels

A channel is an optional string tag on a post. Posts without a channel go to `"general"`. Frontends can filter by channel — gaming, politics, memes, etc.

**Chain-level changes:**
- Add `Channel string` field to `CreatePost` transaction (reuse the same pattern as `Text` — included in `SignableBytes`, validated, stored on-chain)
- Validation: 3-20 chars, `[a-z0-9_]`, same rules as names. Empty string is valid (defaults to general)
- `Post` struct gains a `Channel` field, stored in consensus state and persistence
- State root hash includes the channel field
- No new genesis parameter needed — channels are free-form, not pre-registered

**Indexer changes:**
- `posts` table gains a `channel` column with an index
- `GET /v1/feed` gains a `?channel=gaming` query param
- `GET /v1/channels` — new endpoint returning all channels with post counts
- Ranking strategies work per-channel (trending in gaming, top in politics, etc.)

**RPC changes:**
- `PostResponse` gains `channel` field
- `GET /v1/posts` gains `?channel=` filter
- `SubmitTxRequest` for `create_post` accepts optional `channel` field

**CLI changes:**
- `drana-cli post --text "..." --amount 1000000 --channel gaming`

**Files touched:** `types/post.go`, `types/transaction.go`, `validation/validate.go`, `state/executor.go`, `state/stateroot.go`, `store/kvstore.go`, `rpc/server.go`, `rpc/types.go`, `indexer/db.go`, `indexer/api.go`, `indexer/follower.go`, `cmd/drana-cli/commands/post.go`, `proto/types.proto`

**Backward compatibility:** Channel defaults to empty string. Existing posts are channelless (general). No migration needed.

---

## Replies (flat comment threads)

**Priority:** After channels, before major adoption push

A reply is a post that references a parent post. It has text, costs DRANA to create, and can be boosted. The parent-child relationship is metadata — a reply is still a first-class post on-chain.

**Design rule: flat replies only.** A reply must reference a post that itself has no parent. Two levels max (post → replies). No replies to replies. This keeps indexer queries simple and avoids recursive tree traversal. If you want to address someone in a reply thread, you quote them — like old-school forums.

**Chain-level changes:**
- Add `ParentPostID PostID` field to `CreatePost` transaction (zero value = top-level post)
- Validation: if `ParentPostID` is set, the referenced post must exist and must itself have no parent
- `Post` struct gains `ParentPostID` field
- State root includes the field

**Indexer changes:**
- `posts` table gains `parent_post_id` column (nullable, indexed)
- `GET /v1/posts/{id}/replies` — new endpoint returning replies to a post, ranked by committed value
- Replies are excluded from the main feed by default (they have a parent)
- `PostResponse` gains `replyCount` field on top-level posts
- Author profile includes reply count

**RPC changes:**
- `PostResponse` gains `parentPostId` field (omitempty)
- `SubmitTxRequest` for `create_post` accepts optional `parentPostId`

**CLI changes:**
- `drana-cli post --text "Great post" --amount 100000 --reply-to <post-id-hex>`

**Files touched:** `types/post.go`, `types/transaction.go`, `validation/validate.go`, `state/executor.go`, `state/stateroot.go`, `store/kvstore.go`, `rpc/server.go`, `rpc/types.go`, `indexer/db.go`, `indexer/api.go`, `indexer/follower.go`, `cmd/drana-cli/commands/post.go`, `proto/types.proto`

**Backward compatibility:** ParentPostID defaults to zero. Existing posts are top-level. No migration needed.

**Implementation order:** Channels first (simpler — one new field, no cross-post references), then replies (needs "parent must exist and have no parent" validation).

---

## Indexer: Upgrade from SQLite to PostgreSQL

**Priority:** When post volume exceeds ~100K posts or concurrent read load exceeds what a single SQLite file can handle

The indexer currently uses SQLite (`modernc.org/sqlite`). This is ideal for development and small deployments — zero infrastructure, single file, fast reads. But SQLite has limitations at scale:

- **Single-writer lock:** Only one process can write at a time. The follower holds the write lock while indexing, which can briefly block API reads under heavy write load.
- **No concurrent connections from multiple processes.** You can't run two indexer replicas against the same database file.
- **No streaming replication.** You can't have a read replica in another region.

PostgreSQL removes all of these. With Docker, the migration is painless — validators already run in containers, and adding a Postgres container is one `docker-compose` service.

**What changes:**
- Replace `modernc.org/sqlite` driver with `github.com/lib/pq` (or `pgx`)
- Replace `sql.Open("sqlite", path)` with `sql.Open("postgres", connString)`
- Schema is already standard SQL — the only SQLite-isms are `INSERT OR IGNORE` (becomes `INSERT ... ON CONFLICT DO NOTHING`) and `INTEGER` primary keys (Postgres uses `BIGINT`)
- Add a `DRANA_INDEXER_DB_URL` environment variable for the connection string
- Default to SQLite if no Postgres URL is configured (keeps the zero-config dev experience)

**Docker Compose addition:**

```yaml
services:
  indexer-db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: drana_indexer
      POSTGRES_USER: drana
      POSTGRES_PASSWORD: drana
    volumes:
      - indexer-pg-data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  indexer:
    build: .
    entrypoint: ["drana-indexer"]
    command: ["-rpc", "http://validator-1:26657", "-db", "postgres://drana:drana@indexer-db:5432/drana_indexer?sslmode=disable", "-listen", "0.0.0.0:26680"]
    depends_on:
      - indexer-db
      - validator-1
```

**Files touched:** `internal/indexer/db.go` (driver swap + dialect-aware SQL), `cmd/drana-indexer/main.go` (detect postgres:// URL vs file path), `docker-compose.yml`, `go.mod`

**Migration path:** Export SQLite data with `sqlite3 indexer.db .dump`, import into Postgres. Or just let the indexer re-index from block 1 — it takes minutes for small chains, hours for large ones.

---

## Delegation

**Priority:** After PoS is proven stable

Delegation allows non-validators to stake DRANA behind a validator they trust, sharing rewards proportionally. This is deferred from v1 but designed to be addable without breaking changes. See `POS_ENHANCEMENT_DESIGN.md` section 10 for the design stub.

---

## Dynamic peer discovery (DHT)

**Priority:** After 100+ validators

The current peer exchange protocol works by asking connected peers "who do you know?" This scales to ~100 nodes. Beyond that, a structured overlay network (Kademlia DHT or similar) would be more efficient. For now, gossip-based exchange is sufficient.

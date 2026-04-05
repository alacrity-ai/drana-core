# CHANNELS_REPLIES_POSTGRES_IMPLEMENTATION.md

## Channels, Replies, and PostgreSQL Indexer Upgrade

### Overview

Three enhancements implemented in order:

1. **Channels** — topic tag on posts (Steps 1-6)
2. **Replies** — flat one-level comment threads (Steps 7-10)
3. **PostgreSQL upgrade** — dual-driver indexer (Steps 11-13)

Each enhancement is backward-compatible. Existing posts have empty channel and no parent.

---

## Channels

### Step 1 — Types: Add Channel to Post and Transaction

**`internal/types/post.go`**

Add `Channel` field to `Post`:

```go
type Post struct {
    PostID          PostID
    Author          crypto.Address
    Text            string
    Channel         string  // empty = "general"
    CreatedAtHeight uint64
    CreatedAtTime   int64
    TotalCommitted  uint64
    BoostCount      uint64
}
```

**`internal/types/transaction.go`**

The `Text` field already carries the post body for `CreatePost`. Channel needs its own field because a post has both text and a channel, and both must be signed:

```go
type Transaction struct {
    Type      TxType
    Sender    crypto.Address
    Recipient crypto.Address
    PostID    PostID
    Text      string
    Channel   string           // CreatePost only
    Amount    uint64
    Nonce     uint64
    Signature []byte
    PubKey    crypto.PublicKey
}
```

Update `SignableBytes` to include the channel:

```go
func (tx *Transaction) SignableBytes() []byte {
    hw := NewHashWriter()
    hw.WriteUint32(uint32(tx.Type))
    hw.WriteBytes(tx.Sender[:])
    hw.WriteBytes(tx.Recipient[:])
    hw.WriteBytes(tx.PostID[:])
    hw.WriteString(tx.Text)
    hw.WriteString(tx.Channel)     // add after Text
    hw.WriteUint64(tx.Amount)
    hw.WriteUint64(tx.Nonce)
    hw.WriteBytes(tx.PubKey[:])
    return append([]byte(nil), hw.buf...)
}
```

**`internal/types/json.go`**

Add `Channel string` to `transactionJSON` struct and include it in `MarshalJSON` / `UnmarshalJSON`.

**`internal/proto/types.proto`**

```protobuf
message Post {
  // ... existing fields 1-7 ...
  string channel = 8;
}

message Transaction {
  // ... existing fields 1-9 ...
  string channel = 10;
}
```

### Step 2 — Validation: Channel Name Rules

**`internal/validation/validate.go`**

Update `validateCreatePost` to validate the channel if present:

```go
func validateCreatePost(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
    // ... existing checks ...
    if tx.Channel != "" {
        if err := ValidateChannelName(tx.Channel); err != nil {
            return fmt.Errorf("invalid channel: %w", err)
        }
    }
    _, err := validateCommon(tx, sr)
    return err
}
```

**`internal/validation/name.go`** (add to existing file)

```go
func ValidateChannelName(name string) error {
    // Same rules as account names: 3-20 chars, [a-z0-9_], no edge underscores.
    return ValidateName(name)
}
```

Channel validation reuses `ValidateName` — same character set, same length bounds.

### Step 3 — State and Executor

**`internal/state/executor.go`**

Update `applyCreatePost` to set the channel on the post:

```go
post := &types.Post{
    PostID:          postID,
    Author:          tx.Sender,
    Text:            tx.Text,
    Channel:         tx.Channel,     // add this
    CreatedAtHeight: blockHeight,
    CreatedAtTime:   blockTime,
    TotalCommitted:  tx.Amount,
    BoostCount:      0,
}
```

**`internal/state/stateroot.go`**

Add channel to the post hash in `ComputeStateRoot`:

```go
for _, p := range posts {
    hw.WriteBytes(p.PostID[:])
    hw.WriteBytes(p.Author[:])
    hw.WriteString(p.Text)
    hw.WriteString(p.Channel)      // add after Text
    hw.WriteUint64(p.CreatedAtHeight)
    hw.WriteInt64(p.CreatedAtTime)
    hw.WriteUint64(p.TotalCommitted)
    hw.WriteUint64(p.BoostCount)
}
```

### Step 4 — Persistence

**`internal/store/kvstore.go`**

Update `encodePost` — append channel after text using the same length-prefix pattern:

```go
// After writing text:
channelBytes := []byte(p.Channel)
binary.BigEndian.PutUint32(buf[off:], uint32(len(channelBytes)))
off += 4
copy(buf[off:], channelBytes)
```

Update buffer size calculation to include `4 + len(channelBytes)`.

Update `decodePost` — read channel after text. If data ends before the channel field (old data), default to empty string.

### Step 5 — RPC and CLI

**`internal/rpc/types.go`**

Add `Channel` to `PostResponse`:

```go
type PostResponse struct {
    // ... existing fields ...
    Channel         string `json:"channel,omitempty"`
}
```

Add `Channel` to `SubmitTxRequest`:

```go
type SubmitTxRequest struct {
    // ... existing fields ...
    Channel   string `json:"channel,omitempty"`
}
```

**`internal/rpc/server.go`**

- `postToResponse`: include `Channel: p.Channel`
- `handleListPosts`: add `?channel=` query param filter (same pattern as `author` filter)
- `handleSubmitTransaction`: set `tx.Channel = req.Channel` for `create_post` type

**`internal/p2p/convert.go`**

- `TxToProto`: add `Channel: tx.Channel` (protobuf field 10 when regenerated, or pass via `Text` second field)
- `TxFromProto`: read `tx.Channel = ptx.Channel`

**`cmd/drana-cli/commands/post.go`**

Add `--channel` flag:

```go
channel := fs.String("channel", "", "post channel (e.g., gaming, politics)")
// ... in tx construction:
tx := &types.Transaction{
    Type:    types.TxCreatePost,
    Sender:  sender,
    Text:    normalized,
    Channel: *channel,
    Amount:  *amount,
    Nonce:   nonce + 1,
}
```

### Step 6 — Indexer

**`internal/indexer/types.go`**

Add `Channel` to `IndexedPost`:

```go
type IndexedPost struct {
    // ... existing fields ...
    Channel             string `json:"channel,omitempty"`
}
```

**`internal/indexer/db.go`**

Update schema — add `channel` column to `posts` table:

```sql
ALTER TABLE posts ADD COLUMN channel TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_posts_channel ON posts(channel);
```

(In practice, add to the `CREATE TABLE` in `Migrate()` and handle backward compat.)

Update `InsertPost`: include channel in INSERT.
Update `GetPost`: include channel in SELECT.

**`internal/indexer/ranking.go`**

Update `rankedPostsQuery` to accept optional channel filter:

```go
func (d *DB) RankedPosts(strategy RankingStrategy, page, pageSize int, nowUnix int64, channel string) ([]RankedPost, int, error)
```

When `channel != ""`, add `WHERE channel = ?` to both count and data queries.

**`internal/indexer/api.go`**

- `handleFeed`: read `?channel=` query param, pass to `RankedPosts`
- Add new endpoint: `GET /v1/channels` — returns distinct channels with post counts:

```go
func (a *APIServer) handleChannels(w http.ResponseWriter, r *http.Request) {
    // SELECT channel, COUNT(*) as post_count FROM posts GROUP BY channel ORDER BY post_count DESC
}
```

**`internal/indexer/follower.go`**

Update `create_post` case to include channel from the transaction response (requires the RPC to return channel in the block's full tx list — which it will after Step 5).

### Channels Acceptance Criteria

- Posts can be created with an optional channel.
- Channel is validated (same rules as names).
- Channel is included in state root hash and persistence.
- Node RPC `/v1/posts` supports `?channel=` filter.
- Indexer `/v1/feed` supports `?channel=` filter.
- Indexer `/v1/channels` returns channel list with post counts.
- CLI `post --channel gaming` works.
- Existing posts without channels have empty channel (backward compatible).
- All prior tests pass.

---

## Replies

### Step 7 — Types: Add ParentPostID to Post and Transaction

**`internal/types/post.go`**

Add `ParentPostID` field:

```go
type Post struct {
    PostID          PostID
    Author          crypto.Address
    Text            string
    Channel         string
    ParentPostID    PostID         // zero = top-level post
    CreatedAtHeight uint64
    CreatedAtTime   int64
    TotalCommitted  uint64
    BoostCount      uint64
}
```

**`internal/types/transaction.go`**

The `PostID` field currently carries the boost target. For `CreatePost`, it's unused (zero). We reuse it: when `Type == TxCreatePost` and `PostID` is non-zero, it's the parent post ID. No new field needed — `PostID` has dual meaning based on tx type:

- `TxBoostPost`: `PostID` = post to boost
- `TxCreatePost`: `PostID` = parent post (zero = top-level)

`SignableBytes` already includes `PostID`, so replies are covered by the signature with no change.

### Step 8 — Validation: Reply Rules

**`internal/validation/validate.go`**

Update `validateCreatePost`:

```go
func validateCreatePost(tx *types.Transaction, sr StateReader, params *types.GenesisConfig) error {
    // ... existing amount and text checks ...
    if tx.Channel != "" {
        if err := ValidateChannelName(tx.Channel); err != nil {
            return fmt.Errorf("invalid channel: %w", err)
        }
    }
    // Reply validation.
    var zeroPostID types.PostID
    if tx.PostID != zeroPostID {
        parent, ok := sr.GetPost(tx.PostID)
        if !ok {
            return fmt.Errorf("parent post %x does not exist", tx.PostID)
        }
        if parent.ParentPostID != zeroPostID {
            return fmt.Errorf("cannot reply to a reply (flat replies only)")
        }
    }
    _, err := validateCommon(tx, sr)
    return err
}
```

Three rules:
1. If `PostID` is set, the parent must exist.
2. The parent must itself be a top-level post (no reply-to-reply).
3. A reply inherits its parent's channel (or the executor sets it — see Step 9).

### Step 9 — State, Executor, Persistence, State Root

**`internal/state/executor.go`**

Update `applyCreatePost` to set `ParentPostID` on the new post:

```go
postID := types.DerivePostID(tx.Sender, author.Nonce+1)
post := &types.Post{
    PostID:          postID,
    Author:          tx.Sender,
    Text:            tx.Text,
    Channel:         tx.Channel,
    ParentPostID:    tx.PostID,       // zero for top-level, set for replies
    CreatedAtHeight: blockHeight,
    CreatedAtTime:   blockTime,
    TotalCommitted:  tx.Amount,
    BoostCount:      0,
}
```

**`internal/state/stateroot.go`**

Add `ParentPostID` to the post hash:

```go
hw.WriteBytes(p.ParentPostID[:])   // add after Channel
```

**`internal/store/kvstore.go`**

Update `encodePost` — append `ParentPostID` (32 bytes) after the channel field.
Update `decodePost` — read `ParentPostID` after channel. Default to zero for old data.

### Step 10 — RPC, CLI, Indexer, P2P

**`internal/rpc/types.go`**

Add to `PostResponse`:
```go
ParentPostID    string `json:"parentPostId,omitempty"`
```

Add to `SubmitTxRequest`:
```go
ParentPostID string `json:"parentPostId,omitempty"`  // hex, for replies
```

**`internal/rpc/server.go`**

- `postToResponse`: include `ParentPostID` if non-zero.
- `handleListPosts`: by default, exclude replies from the main list (where `ParentPostID` is zero). Add `?includeReplies=true` to override.
- `handleSubmitTransaction`: for `create_post`, if `req.ParentPostID != ""`, decode and set `tx.PostID`.
- New endpoint or sub-path: `GET /v1/posts/{id}/replies` on the node RPC (in addition to the indexer). List posts where `ParentPostID == id`, sorted by committed value descending.

**`cmd/drana-cli/commands/post.go`**

Add `--reply-to` flag:

```go
replyTo := fs.String("reply-to", "", "parent post ID (hex) to reply to")
// ... in tx construction:
if *replyTo != "" {
    pidBytes, err := hex.DecodeString(*replyTo)
    // ... copy into tx.PostID
}
```

**`internal/indexer/db.go`**

Update schema — add `parent_post_id` and `reply_count` columns:

```sql
ALTER TABLE posts ADD COLUMN parent_post_id TEXT NOT NULL DEFAULT '';
ALTER TABLE posts ADD COLUMN reply_count INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_posts_parent ON posts(parent_post_id);
```

Update `InsertPost`: include `parent_post_id`.
When a reply is inserted, increment the parent's `reply_count`:

```sql
UPDATE posts SET reply_count = reply_count + 1 WHERE post_id = ?
```

**`internal/indexer/types.go`**

Add to `IndexedPost`:
```go
ParentPostID string `json:"parentPostId,omitempty"`
ReplyCount   int    `json:"replyCount"`
```

**`internal/indexer/ranking.go`**

Update `rankedPostsQuery`: by default, only return top-level posts (`WHERE parent_post_id = ''`). Add a `parentID` parameter for fetching replies to a specific post.

**`internal/indexer/api.go`**

- Update `handlePost` to handle `/v1/posts/{id}/replies` — returns replies sorted by committed value.
- Main feed excludes replies by default.

**`internal/indexer/follower.go`**

Update `create_post` case: if the transaction response includes a `postId` field (indicating a reply), set `parent_post_id` on the indexed post and increment the parent's reply count.

Note: The node RPC `TransactionResponse` currently includes `PostID` for `boost_post` but not for `create_post`. The `txToResponse` function needs updating: for `create_post`, if `tx.PostID` is non-zero, include it as `parentPostId` in the response.

### Replies Acceptance Criteria

- Posts can be created as replies to existing top-level posts.
- Replies to replies are rejected.
- Replies are excluded from the main feed by default.
- `GET /v1/posts/{id}/replies` returns replies ranked by committed value.
- Indexer tracks `reply_count` on parent posts.
- CLI `post --reply-to <hex>` works.
- Existing posts have zero `ParentPostID` (backward compatible).
- All prior tests pass.

---

## PostgreSQL Indexer Upgrade

### Step 11 — Dual-Driver Database Layer

**`internal/indexer/db.go`**

Refactor `OpenDB` to detect the driver from the connection string:

```go
func OpenDB(dsn string) (*DB, error) {
    if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
        return openPostgres(dsn)
    }
    return openSQLite(dsn)
}

func openSQLite(path string) (*DB, error) {
    db, err := sql.Open("sqlite", path)
    // ... existing WAL pragma ...
    return &DB{db: db, driver: "sqlite"}, nil
}

func openPostgres(dsn string) (*DB, error) {
    db, err := sql.Open("pgx", dsn)
    // ...
    return &DB{db: db, driver: "postgres"}, nil
}
```

Add `driver string` field to `DB` struct for dialect-aware SQL.

### Step 12 — Dialect-Aware SQL

The only SQLite-specific SQL in the codebase is `INSERT OR IGNORE`. In PostgreSQL this becomes `INSERT ... ON CONFLICT DO NOTHING`.

Create a helper:

```go
func (d *DB) insertOrIgnore() string {
    if d.driver == "postgres" {
        return "INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING"
    }
    return "INSERT OR IGNORE INTO %s (%s) VALUES (%s)"
}
```

Or simpler — use a method that wraps the dialect difference:

```go
func (d *DB) upsertSQL(table, cols, placeholders, conflictTarget string) string {
    if d.driver == "postgres" {
        return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING", table, cols, placeholders, conflictTarget)
    }
    return fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)", table, cols, placeholders)
}
```

Update all `INSERT OR IGNORE` calls in:
- `InsertPost`
- `InsertBoost`
- `InsertTransfer`
- `SetLastIndexedHeight`

**Schema differences:**

| SQLite | PostgreSQL |
|--------|-----------|
| `INTEGER` | `BIGINT` |
| `TEXT PRIMARY KEY` | `TEXT PRIMARY KEY` (same) |
| `INSERT OR IGNORE` | `INSERT ... ON CONFLICT DO NOTHING` |
| `PRAGMA journal_mode=WAL` | (not needed) |
| Auto-increment via `ROWID` | `SERIAL` or `BIGSERIAL` |

The current schema uses only `TEXT` and `INTEGER` types with explicit primary keys — all valid in both databases. The main change is the `INSERT OR IGNORE` → `ON CONFLICT` swap.

**Migrate function:**

```go
func (d *DB) Migrate() error {
    if d.driver == "postgres" {
        return d.migratePostgres()
    }
    return d.migrateSQLite()
}
```

Both `migratePostgres` and `migrateSQLite` contain the same schema with minor syntax differences (`IF NOT EXISTS` works in both).

### Step 13 — Dependencies, Config, Docker Compose

**`go.mod`**

Add PostgreSQL driver:
```
go get github.com/jackc/pgx/v5/stdlib
```

The `pgx` driver registers as `"pgx"` with `database/sql`.

**`cmd/drana-indexer/main.go`**

The `-db` flag already accepts a path. No change needed — if the user passes `postgres://...`, `OpenDB` detects it and uses `pgx`.

```bash
# SQLite (default, zero config):
drana-indexer -db indexer.db

# PostgreSQL:
drana-indexer -db "postgres://drana:drana@localhost:5432/drana_indexer?sslmode=disable"
```

**`docker-compose.yml`**

Add Postgres service and indexer service:

```yaml
services:
  # ... existing validator services ...

  indexer-db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: drana_indexer
      POSTGRES_USER: drana
      POSTGRES_PASSWORD: drana
    volumes:
      - indexer-pg-data:/var/lib/postgresql/data

  indexer:
    build: .
    entrypoint: ["drana-indexer"]
    command:
      - "-rpc"
      - "http://validator-1:26657"
      - "-db"
      - "postgres://drana:drana@indexer-db:5432/drana_indexer?sslmode=disable"
      - "-listen"
      - "0.0.0.0:26680"
    ports:
      - "26680:26680"
    depends_on:
      - indexer-db
      - validator-1

volumes:
  # ... existing volumes ...
  indexer-pg-data:
```

### PostgreSQL Acceptance Criteria

- `OpenDB("indexer.db")` uses SQLite (existing behavior, unchanged).
- `OpenDB("postgres://...")` uses PostgreSQL via pgx.
- All indexer tests pass with SQLite (default).
- A manual test with a local Postgres instance indexes blocks correctly.
- Docker Compose starts Postgres, indexer, and validators together.
- Schema migration is idempotent on both drivers.

---

## Implementation Order

1. Steps 1-6: Channels (types → validation → executor → persistence → RPC/CLI → indexer)
2. Steps 7-10: Replies (types → validation → executor → persistence → RPC/CLI → indexer)
3. Steps 11-13: PostgreSQL (driver abstraction → dialect SQL → deps/config/docker)

Each group is independently deployable. Channels can ship without replies. Postgres can ship without channels. But channels should come before replies (replies inherit parent channel).

---

## Files Modified Summary

| Step | Files |
|------|-------|
| 1 | `types/post.go`, `types/transaction.go`, `types/json.go`, `proto/types.proto` |
| 2 | `validation/validate.go`, `validation/name.go` |
| 3 | `state/executor.go`, `state/stateroot.go` |
| 4 | `store/kvstore.go` |
| 5 | `rpc/types.go`, `rpc/server.go`, `p2p/convert.go`, `cmd/drana-cli/commands/post.go` |
| 6 | `indexer/types.go`, `indexer/db.go`, `indexer/ranking.go`, `indexer/api.go`, `indexer/follower.go` |
| 7 | `types/post.go` |
| 8 | `validation/validate.go` |
| 9 | `state/executor.go`, `state/stateroot.go`, `store/kvstore.go` |
| 10 | `rpc/types.go`, `rpc/server.go`, `cmd/drana-cli/commands/post.go`, `indexer/types.go`, `indexer/db.go`, `indexer/ranking.go`, `indexer/api.go`, `indexer/follower.go` |
| 11 | `indexer/db.go` |
| 12 | `indexer/db.go` (all INSERT OR IGNORE calls) |
| 13 | `go.mod`, `cmd/drana-indexer/main.go`, `docker-compose.yml` |

---

## Backward Compatibility

- `Channel` defaults to `""` — existing posts are channelless (general feed).
- `ParentPostID` defaults to zero — existing posts are top-level.
- `SignableBytes` changes are **breaking for transaction signatures** — any unsigned transactions in mempools will become invalid. This is acceptable for a pre-mainnet chain. For a live chain, this would require a coordinated upgrade at an epoch boundary.
- SQLite remains the default indexer backend. Postgres is opt-in via connection string.
- All existing tests must pass without modification after each step.

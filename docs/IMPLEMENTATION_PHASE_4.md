# IMPLEMENTATION_PHASE_4.md

## Phase 4 — Indexer and Feed Infrastructure

### Purpose

This phase builds a separate read-optimized service that follows the canonical chain, denormalizes all data into a relational store, computes derived fields, and serves the rich query API that feed clients, dashboards, and the future web frontend need.

The node RPC from Phase 3 deliberately supports only basic queries (get by ID, simple list). The indexer exists to serve everything beyond that: ranked feeds, boost history, per-author activity, global analytics, and time-decay ranking functions.

### Architecture

The indexer is a **separate process** that runs alongside (or on a different machine from) the chain node. It consumes data from the node's JSON HTTP RPC, not from internal Go interfaces. This means:

- The indexer can be restarted, rebuilt, or replaced independently of the chain.
- Third parties can run their own indexers with custom schemas.
- The chain node remains lean.

```
┌──────────────┐     JSON HTTP      ┌──────────────┐     JSON HTTP     ┌──────────┐
│  Chain Node   │ ◄──── polls ────── │   Indexer     │ ◄──── queries ── │  Clients  │
│  (Phase 1-3)  │                    │  (Phase 4)    │                  │  Web, etc │
└──────────────┘                    └──────────────┘                  └──────────┘
      ▲                                    │
      │                              ┌─────┴─────┐
      │                              │  SQLite    │
      │                              │  (indexed  │
      │                              │   data)    │
      │                              └───────────┘
```

### Prerequisites

All Phase 3 exit criteria must be met. The indexer consumes the following node RPC endpoints:

- `GET /v1/node/info` — current height, supply stats
- `GET /v1/blocks/{height}?full=true` — block with all transactions
- `GET /v1/blocks/latest` — latest block height for polling
- `GET /v1/accounts/{address}` — balance queries (optional, for verification)

### What This Phase Adds to the Directory Layout

```
cmd/
  drana-indexer/
    main.go
internal/
  indexer/
    follower.go
    follower_test.go
    db.go
    db_test.go
    ranking.go
    ranking_test.go
    api.go
    api_test.go
    types.go
test/
  integration/
    phase4_test.go
```

---

## Step 1 — Indexer Data Model and SQLite Schema

### Files

**`internal/indexer/types.go`**

Indexed data types — these mirror chain types but include derived fields:

```go
type IndexedPost struct {
    PostID             string  // hex
    Author             string  // drana1...
    Text               string
    CreatedAtHeight    uint64
    CreatedAtTime      int64
    TotalCommitted     uint64  // microdrana
    AuthorCommitted    uint64  // microdrana committed by the author
    ThirdPartyCommitted uint64 // microdrana committed by non-author boosters
    BoostCount         uint64
    UniqueBoosterCount int
    LastBoostAtHeight  uint64
}

type IndexedBoost struct {
    PostID      string // hex
    Booster     string // drana1...
    Amount      uint64
    BlockHeight uint64
    BlockTime   int64
    TxHash      string // hex
}

type IndexedTransfer struct {
    TxHash      string
    Sender      string
    Recipient   string
    Amount      uint64
    BlockHeight uint64
    BlockTime   int64
}

type ChainStats struct {
    LatestHeight    uint64
    TotalPosts      int
    TotalBoosts     int
    TotalTransfers  int
    TotalBurned     uint64
    TotalIssued     uint64
    CirculatingSupply uint64
}

// RankedPost extends IndexedPost with a computed score.
type RankedPost struct {
    IndexedPost
    Score float64 `json:"score"`
}
```

**`internal/indexer/db.go`**

SQLite persistence layer:

```go
type DB struct {
    db *sql.DB
}

func OpenDB(path string) (*DB, error)
func (d *DB) Close() error
func (d *DB) Migrate() error  // creates tables if not exist
```

SQLite schema:

```sql
CREATE TABLE IF NOT EXISTS posts (
    post_id TEXT PRIMARY KEY,
    author TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at_height INTEGER NOT NULL,
    created_at_time INTEGER NOT NULL,
    total_committed INTEGER NOT NULL DEFAULT 0,
    author_committed INTEGER NOT NULL DEFAULT 0,
    third_party_committed INTEGER NOT NULL DEFAULT 0,
    boost_count INTEGER NOT NULL DEFAULT 0,
    unique_booster_count INTEGER NOT NULL DEFAULT 0,
    last_boost_at_height INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_posts_author ON posts(author);
CREATE INDEX IF NOT EXISTS idx_posts_height ON posts(created_at_height);
CREATE INDEX IF NOT EXISTS idx_posts_committed ON posts(total_committed);

CREATE TABLE IF NOT EXISTS boosts (
    tx_hash TEXT PRIMARY KEY,
    post_id TEXT NOT NULL,
    booster TEXT NOT NULL,
    amount INTEGER NOT NULL,
    block_height INTEGER NOT NULL,
    block_time INTEGER NOT NULL,
    FOREIGN KEY (post_id) REFERENCES posts(post_id)
);
CREATE INDEX IF NOT EXISTS idx_boosts_post ON boosts(post_id);
CREATE INDEX IF NOT EXISTS idx_boosts_booster ON boosts(booster);

CREATE TABLE IF NOT EXISTS transfers (
    tx_hash TEXT PRIMARY KEY,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    amount INTEGER NOT NULL,
    block_height INTEGER NOT NULL,
    block_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Stores: "last_indexed_height" -> "123"
```

DB methods:

```go
func (d *DB) InsertPost(p *IndexedPost) error
func (d *DB) UpdatePostBoost(postID string, amount uint64, boosterAddr string, height uint64, isAuthor bool) error
func (d *DB) InsertBoost(b *IndexedBoost) error
func (d *DB) InsertTransfer(t *IndexedTransfer) error
func (d *DB) GetPost(postID string) (*IndexedPost, error)
func (d *DB) GetLastIndexedHeight() (uint64, error)
func (d *DB) SetLastIndexedHeight(h uint64) error
func (d *DB) GetChainStats() (*ChainStats, error)
```

**`internal/indexer/db_test.go`**

- Insert a post, retrieve it, verify all fields.
- Insert a boost, verify post derived fields update (`uniqueBoosterCount`, `thirdPartyCommitted`, `lastBoostAtHeight`).
- Author self-boost updates `authorCommitted`, not `thirdPartyCommitted`.
- Sync state round-trip.

### Acceptance Criteria

- All tests pass.
- Schema migrations are idempotent (safe to run multiple times).
- Derived fields are updated atomically with boost insertion.

---

## Step 2 — Chain Follower

### Files

**`internal/indexer/follower.go`**

The follower polls the node RPC for new blocks and indexes their contents:

```go
type Follower struct {
    nodeRPC    string    // e.g., "http://localhost:26657"
    db         *DB
    pollInterval time.Duration
}

func NewFollower(nodeRPC string, db *DB, pollInterval time.Duration) *Follower
func (f *Follower) Run(ctx context.Context) error
func (f *Follower) IndexBlock(height uint64) error
```

`Run` loop:
1. Read `lastIndexedHeight` from DB.
2. Poll `GET /v1/node/info` to get current chain height.
3. For each height from `lastIndexedHeight + 1` to `currentHeight`:
   a. Fetch `GET /v1/blocks/{height}?full=true`.
   b. For each transaction in the block:
      - **Transfer:** Insert into `transfers` table.
      - **CreatePost:** Insert into `posts` table with `authorCommitted = amount`.
      - **BoostPost:** Insert into `boosts` table. Update post derived fields: increment `boostCount`, add to `totalCommitted`, compute whether booster is the author (updates `authorCommitted` or `thirdPartyCommitted`), update `uniqueBoosterCount` via a distinct count query, set `lastBoostAtHeight`.
   c. Update `lastIndexedHeight` to this block's height.
4. Sleep for `pollInterval`, then repeat.

If the node is unreachable, the follower logs a warning, sleeps, and retries. It never skips blocks.

**`internal/indexer/follower_test.go`**

- Mock HTTP server returning known block data. Verify the follower indexes posts, boosts, and transfers correctly.
- Verify `lastIndexedHeight` advances.
- Verify the follower resumes from where it left off after restart.

### Acceptance Criteria

- All tests pass.
- The follower correctly indexes all three transaction types.
- Derived fields on posts are correct after indexing boosts.
- The follower is idempotent: re-indexing an already-indexed block is safe.
- The follower resumes from `lastIndexedHeight` after restart.

---

## Step 3 — Ranking Functions

### Files

**`internal/indexer/ranking.go`**

Implements multiple ranking strategies that clients can choose from:

```go
type RankingStrategy string

const (
    RankByTrending   RankingStrategy = "trending"    // time-decayed committed value
    RankByTopAllTime RankingStrategy = "top"          // total committed, no decay
    RankByNew        RankingStrategy = "new"          // creation height descending
    RankByControversial RankingStrategy = "controversial" // high boost count relative to committed
)
```

Query functions:

```go
func (d *DB) RankedPosts(strategy RankingStrategy, page, pageSize int, nowUnix int64) ([]RankedPost, int, error)
func (d *DB) RankedPostsByAuthor(author string, strategy RankingStrategy, page, pageSize int, nowUnix int64) ([]RankedPost, int, error)
```

**Ranking formulas:**

**Trending:** `score = log(1 + totalCommitted) / (1 + ageHours)^alpha` where `alpha = 1.5` and `ageHours = (now - createdAtTime) / 3600`. This rewards recent posts with high commitment.

**Top (all-time):** `score = totalCommitted`. Simple, no decay.

**New:** `score = createdAtHeight`. Just chronological ordering.

**Controversial:** `score = boostCount * log(1 + totalCommitted)`. High engagement relative to commitment — many small boosters signal disagreement/discourse.

The ranking computation happens in SQL where possible (sorting, pagination) with score calculation in Go for the formulas that need floating-point math.

**`internal/indexer/ranking_test.go`**

- Insert 10 posts with varying committed amounts and creation times.
- `RankByTopAllTime` returns them in descending committed order.
- `RankByNew` returns them in descending height order.
- `RankByTrending` returns recent high-commitment posts above older ones.
- Pagination works correctly (page 2 returns the next set).
- Author filter restricts results.

### Acceptance Criteria

- All tests pass.
- Each ranking strategy produces a distinct, defensible ordering.
- Pagination is correct: no duplicates, no gaps.
- Score is included in the response so clients can display it.

---

## Step 4 — Indexer HTTP API

### Files

**`internal/indexer/api.go`**

A separate HTTP server that exposes the indexer's rich query surface:

```go
type APIServer struct {
    db         *DB
    httpServer *http.Server
}

func NewAPIServer(listenAddr string, db *DB) *APIServer
func (a *APIServer) Start() error
func (a *APIServer) Stop(ctx context.Context) error
```

### Endpoint Table

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/feed` | Ranked post feed. Query params: `strategy` (trending/top/new/controversial, default trending), `page`, `pageSize` |
| GET | `/v1/feed/author/{address}` | Ranked posts by a specific author |
| GET | `/v1/posts/{postId}` | Full indexed post with derived fields |
| GET | `/v1/posts/{postId}/boosts` | Boost history for a post (paginated) |
| GET | `/v1/authors/{address}` | Author profile: post count, total committed, total received boosts |
| GET | `/v1/authors/{address}/activity` | Recent activity: posts and boosts by this author (paginated) |
| GET | `/v1/stats` | Global chain stats: total posts, total boosts, total burned, total issued, circulating supply |
| GET | `/v1/leaderboard` | Top authors by total committed received (paginated) |

### Response Types

```go
type FeedResponse struct {
    Posts      []RankedPost `json:"posts"`
    TotalCount int          `json:"totalCount"`
    Page       int          `json:"page"`
    PageSize   int          `json:"pageSize"`
    Strategy   string       `json:"strategy"`
}

type BoostHistoryResponse struct {
    Boosts     []IndexedBoost `json:"boosts"`
    TotalCount int            `json:"totalCount"`
    Page       int            `json:"page"`
    PageSize   int            `json:"pageSize"`
}

type AuthorProfileResponse struct {
    Address           string `json:"address"`
    PostCount         int    `json:"postCount"`
    TotalCommitted    uint64 `json:"totalCommitted"`    // spent by this author
    TotalReceived     uint64 `json:"totalReceived"`     // boosts received on their posts
    UniqueBoosterCount int   `json:"uniqueBoosterCount"`
}

type StatsResponse struct {
    LatestHeight      uint64 `json:"latestHeight"`
    TotalPosts        int    `json:"totalPosts"`
    TotalBoosts       int    `json:"totalBoosts"`
    TotalTransfers    int    `json:"totalTransfers"`
    TotalBurned       uint64 `json:"totalBurned"`
    TotalIssued       uint64 `json:"totalIssued"`
    CirculatingSupply uint64 `json:"circulatingSupply"`
}

type LeaderboardEntry struct {
    Address        string `json:"address"`
    TotalReceived  uint64 `json:"totalReceived"`
    PostCount      int    `json:"postCount"`
    BoostCount     int    `json:"boostCount"`
}

type LeaderboardResponse struct {
    Authors    []LeaderboardEntry `json:"authors"`
    TotalCount int                `json:"totalCount"`
    Page       int                `json:"page"`
    PageSize   int                `json:"pageSize"`
}
```

**`internal/indexer/api_test.go`**

Tests all endpoints using `httptest.NewServer` against a pre-populated SQLite database:
- `/v1/feed` with each strategy returns correctly ordered results.
- `/v1/feed?strategy=trending` places recent high-commitment posts first.
- `/v1/feed/author/{addr}` filters to a single author.
- `/v1/posts/{id}` returns full derived fields including `uniqueBoosterCount`.
- `/v1/posts/{id}/boosts` returns boost history in chronological order.
- `/v1/authors/{addr}` returns accurate aggregate stats.
- `/v1/stats` returns correct global counters.
- `/v1/leaderboard` returns authors sorted by total received boosts.
- Pagination works across all list endpoints.
- 404 for nonexistent post or author.

### Acceptance Criteria

- All tests pass.
- Every endpoint returns well-structured JSON.
- Ranking strategies produce meaningfully different orderings.
- Derived counts are accurate.

---

## Step 5 — Indexer Entrypoint

### Files

**`cmd/drana-indexer/main.go`**

```go
func main() {
    nodeRPC := flag.String("rpc", "http://localhost:26657", "node RPC endpoint")
    dbPath := flag.String("db", "indexer.db", "SQLite database path")
    listenAddr := flag.String("listen", "0.0.0.0:26680", "indexer API listen address")
    pollInterval := flag.Duration("poll", 2*time.Second, "chain poll interval")
    flag.Parse()

    db, _ := indexer.OpenDB(*dbPath)
    db.Migrate()

    follower := indexer.NewFollower(*nodeRPC, db, *pollInterval)
    api := indexer.NewAPIServer(*listenAddr, db)

    ctx, cancel := signal.NotifyContext(...)
    defer cancel()

    api.Start()
    follower.Run(ctx)  // blocks until cancelled
    api.Stop(...)
    db.Close()
}
```

Behavior:
- Starts the chain follower and the HTTP API server concurrently.
- The follower begins indexing from where it last left off (or from block 1).
- CTRL+C triggers clean shutdown.

### Acceptance Criteria

- `drana-indexer -rpc http://localhost:26657 -db indexer.db -listen :26680` starts and begins indexing.
- The API is queryable while the follower is running.
- Clean shutdown preserves `lastIndexedHeight`.

---

## Step 6 — Integration Test

### Files

**`test/integration/phase4_test.go`**

End-to-end test that exercises the indexer against a live chain:

1. Start a 3-validator network with RPC enabled (reuse Phase 3 setup).
2. Wait for 3 blocks.
3. Submit via RPC:
   - User creates post A with 10M microdrana.
   - User creates post B with 5M microdrana.
   - User2 boosts post A with 3M microdrana.
   - User boosts post A (self-boost) with 2M microdrana.
4. Wait for all txs to be included.
5. Start the indexer follower against the node RPC, pointed at a temp SQLite file.
6. Wait for the indexer to catch up to the chain height.
7. Query the indexer API:
   a. `GET /v1/stats` — verify `totalPosts == 2`, `totalBoosts == 2`, burned supply correct.
   b. `GET /v1/feed?strategy=top` — post A should be first (15M committed > 5M).
   c. `GET /v1/feed?strategy=new` — post B should be first (created at higher height).
   d. `GET /v1/posts/{postA}` — verify derived fields:
      - `authorCommitted == 12M` (10M creation + 2M self-boost)
      - `thirdPartyCommitted == 3M`
      - `uniqueBoosterCount == 2` (author + user2)
      - `lastBoostAtHeight` matches the boost block
   e. `GET /v1/posts/{postA}/boosts` — verify 2 boosts listed.
   f. `GET /v1/authors/{user}` — verify `postCount == 2`, `totalCommitted` is correct.
   g. `GET /v1/leaderboard` — user should be the top author.
   h. `GET /v1/feed/author/{user}` — verify both posts appear.
8. Shut down indexer and nodes.

### Acceptance Criteria

- Test passes end-to-end.
- All derived fields match expected values exactly.
- Ranking strategies produce correct orderings.
- The indexer correctly distinguishes author self-boosts from third-party boosts.
- Global stats are accurate.

---

## Phase 4 Exit Criteria (Summary)

All of the following must be true before the Go chain implementation is considered feature-complete:

1. `go build ./...` succeeds.
2. `go test ./... -race` passes with all unit and integration tests green.
3. The indexer follows a live network in real time, never skipping blocks.
4. All derived fields (`authorCommitted`, `thirdPartyCommitted`, `uniqueBoosterCount`, `lastBoostAtHeight`) are computed correctly.
5. Four ranking strategies (trending, top, new, controversial) produce meaningful, distinct orderings.
6. The indexer API serves paginated feeds, boost history, author profiles, global stats, and leaderboard.
7. The indexer resumes from where it left off after restart.
8. Derived counters match canonical chain state exactly.
9. All Phase 1, 2, and 3 tests continue to pass.

---

## Dependency Summary

| Dependency | Purpose |
|---|---|
| Everything from Phases 1–3 | Chain, consensus, RPC |
| `modernc.org/sqlite` (or `github.com/mattn/go-sqlite3`) | Pure-Go SQLite driver (no CGO) or CGO-based driver |
| Go standard library (`database/sql`, `net/http`, `math`) | SQL, HTTP API, ranking math |

The choice between `modernc.org/sqlite` (pure Go, no CGO, easier cross-compilation) and `mattn/go-sqlite3` (CGO, faster) is an implementation detail. For v1, `modernc.org/sqlite` is recommended for simplicity.

---

## Key Design Decisions

**Why a separate process instead of an in-node indexer:**
Coupling the indexer to the node creates a liability: indexer bugs could crash the validator, schema migrations require node restarts, and the node's memory footprint grows with index size. A separate process with a simple RPC polling interface is operationally cleaner. The node stays lean. The indexer can be rebuilt from scratch by re-reading the chain.

**Why SQLite instead of PostgreSQL:**
For v1, SQLite is the right choice: zero-infrastructure, single-file database, excellent read performance for this query pattern, easy to deploy alongside the node. PostgreSQL can replace it later if the indexer needs to scale horizontally or support concurrent write-heavy workloads, but for a single-indexer setup, SQLite is simpler and faster to ship.

**Why polling instead of streaming/websockets:**
Polling `GET /v1/blocks/{height}` is the simplest correct approach. The indexer tracks its own cursor (`lastIndexedHeight`) and fetches one block at a time. This is robust against network interruptions, indexer restarts, and node restarts. A streaming interface can be added later as an optimization but is not needed for v1.

**Why the indexer computes derived fields instead of the node:**
Fields like `uniqueBoosterCount` and `authorCommitted` require cross-referencing boost transactions against post authorship. This is a relational query concern, not a consensus concern. Putting it in the node would bloat the state machine and slow block execution. The indexer has SQL and can compute these efficiently.

**Why four ranking strategies:**
The design doc explicitly calls for multiple client interpretations. Shipping four strategies demonstrates the principle and gives the future web frontend real choices to present. Each strategy serves a different user intent: discovery (trending), conviction (top), freshness (new), engagement (controversial).

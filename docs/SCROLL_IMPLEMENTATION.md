# SCROLL_IMPLEMENTATION.md

## Feed Pagination Fix — Implementation Steps

### Overview

Two changes: backend moves from fetch-all-then-slice to SQL-level pagination, frontend moves from page-replacement to infinite scroll with accumulated pages.

---

## Step 1 — Backend: SQL-Level Pagination in ranking.go

### File: `internal/indexer/ranking.go`

Replace the single `rankedPostsQuery` function (which loads all posts into Go memory) with strategy-specific SQL queries.

**Current code (lines 30-79):** Fetches all posts, scores in Go, sorts in Go, slices in Go.

**New code:** Three paths:

**`top` and `new`** — pure SQL:
```go
func (d *DB) sqlPaginatedQuery(orderByCol, author, channel string, page, pageSize int) ([]RankedPost, int, error)
```
Builds:
```sql
SELECT {cols}, {orderByCol} as score FROM posts
WHERE parent_post_id = '' [AND author = ?] [AND channel = ?]
ORDER BY {orderByCol} DESC
LIMIT ? OFFSET ?
```
The `score` column is just the ORDER BY value cast to float. Count query uses the same WHERE without LIMIT.

For `top`: `orderByCol = "total_committed"`
For `new`: `orderByCol = "created_at_height"`

**`controversial`** — SQL with computed expression:
```go
orderByExpr = "(boost_count * 1.0) * total_committed"
```
Both SQLite and Postgres handle this arithmetic. Wrap in the same `sqlPaginatedQuery` with the expression as the ORDER BY.

**`trending`** — Go-side scoring on a reduced set:
```go
func (d *DB) trendingQuery(author, channel string, page, pageSize int, nowUnix int64) ([]RankedPost, int, error)
```
Adds a time filter to the WHERE clause:
```sql
AND created_at_time > ?
```
Where the cutoff is `nowUnix - 72*3600` (72 hours). Fetches only recent posts, scores in Go with the `log(1+committed)/(1+ageHours)^1.5` formula, sorts, slices. Falls back to `top` ranking if the recent window yields fewer results than requested (handles the case where the chain is young or a channel has sparse recent activity).

**The `RankedPosts` and `RankedPostsByAuthor` dispatch functions** stay the same — they just call `rankedPostsQuery` which now routes to the right method.

**Delete `sortRankedPosts`** (lines 100-106) — no longer needed since SQL does the sorting for 3 of 4 strategies, and `trending` uses Go's `slices.SortFunc` or the existing insertion sort on a small set.

**Scan columns:** The SELECT now includes a `score` column, so the scan adds one more field. The `cols` constant gains `, {scoreExpr} as score` and the Scan adds `&p.Score` at the end (for SQL-paginated paths) or the score is computed in Go (for trending).

### Acceptance Criteria

- `GET /v1/feed?strategy=top&page=5&pageSize=20` returns posts 81-100 by committed value, without loading posts 1-80 into memory.
- `GET /v1/feed?strategy=new` returns chronologically, SQL-paginated.
- `GET /v1/feed?strategy=trending` only loads posts from the last 72 hours.
- `GET /v1/feed?strategy=controversial` sorts by SQL expression.
- `totalCount` is accurate for all strategies.
- Existing indexer unit tests still pass (they use small datasets where both approaches produce the same results).

---

## Step 2 — Backend: Add Indexes for Sort Columns

### File: `internal/indexer/db.go`

The `Migrate` function already has indexes on `created_at_height` and `total_committed`. For controversial, the sort expression `(boost_count * total_committed)` can't be directly indexed, but `boost_count` should be indexed for the WHERE filter. Add:

```sql
CREATE INDEX IF NOT EXISTS idx_posts_boost_count ON posts(boost_count);
```

This is a one-line addition to the `Migrate()` schema string. No data migration needed — SQLite and Postgres create the index on existing data.

### Acceptance Criteria

- Schema migration is idempotent.
- Existing indexer tests pass.

---

## Step 3 — Frontend: Switch Feed to useInfiniteQuery

### File: `drana-app/src/pages/Feed.tsx`

**Remove:**
- `const [page, setPage] = useState(1)` — TanStack manages page state internally
- The `useQuery` call for the feed
- The "Load more" button and its `page * 20` check

**Add:**
- Import `useInfiniteQuery` from `@tanstack/react-query`
- Import `useRef, useEffect` (already imported)

**Replace the feed query:**

```typescript
const feed = useInfiniteQuery({
  queryKey: ['feed', strategy, channel],
  queryFn: ({ pageParam = 1 }) => getFeed({ strategy, channel, page: pageParam, pageSize: 20 }),
  getNextPageParam: (lastPage, allPages) => {
    const loaded = allPages.length * 20;
    return loaded < lastPage.totalCount ? allPages.length + 1 : undefined;
  },
  initialPageParam: 1,
});
```

**Replace the post rendering:**

```typescript
const allPosts = feed.data?.pages.flatMap(page => page.posts) ?? [];
const topCommitted = allPosts[0]?.totalCommitted || 0;
const highValueThreshold = topCommitted * 0.5;

{allPosts.map(post => (
  <PostCard key={post.postId} post={post}
    isHighValue={post.totalCommitted > highValueThreshold && post.totalCommitted > 0}
    onBoost={...} onReply={...} />
))}
```

**Add the scroll sentinel:**

```typescript
const sentinelRef = useRef<HTMLDivElement>(null);

useEffect(() => {
  if (!sentinelRef.current || !feed.hasNextPage) return;
  const observer = new IntersectionObserver(([entry]) => {
    if (entry.isIntersecting && !feed.isFetchingNextPage) {
      feed.fetchNextPage();
    }
  }, { rootMargin: '200px' });
  observer.observe(sentinelRef.current);
  return () => observer.disconnect();
}, [feed.hasNextPage, feed.isFetchingNextPage, feed.fetchNextPage]);
```

At the bottom of the post list:

```tsx
{feed.hasNextPage && <div ref={sentinelRef} style={{ height: 1 }} />}
{feed.isFetchingNextPage && <LoadingDots />}
```

**Reset behavior:** When `strategy` or `channel` changes, the query key changes, which automatically resets the infinite query. The `setChannel` function no longer needs to call `setPage(1)` since page state is gone.

### Acceptance Criteria

- Scrolling to the bottom loads the next 20 posts automatically.
- Posts from previous pages remain visible (accumulated).
- Changing strategy or channel resets to page 1.
- The loading indicator appears at the bottom while fetching.
- No "Load more" button — scroll-triggered only.

---

## Step 4 — Frontend: Loading Indicator Component

### File: `drana-app/src/components/LoadingDots.tsx` (new)

A minimal three-dot pulsing animation:

```tsx
export function LoadingDots() {
  return (
    <div style={{ textAlign: 'center', padding: '16px 0' }}>
      <span className="loading-dots">
        <span>.</span><span>.</span><span>.</span>
      </span>
      <style>{`
        .loading-dots span {
          font-size: 24px; color: var(--accent); opacity: 0.3;
          animation: dot-pulse 1.4s infinite;
        }
        .loading-dots span:nth-child(2) { animation-delay: 0.2s; }
        .loading-dots span:nth-child(3) { animation-delay: 0.4s; }
        @keyframes dot-pulse {
          0%, 80%, 100% { opacity: 0.3; }
          40% { opacity: 1; }
        }
      `}</style>
    </div>
  );
}
```

### Acceptance Criteria

- Three amber dots pulse at the bottom of the feed while loading.
- Matches the dark theme.

---

## Step 5 — Frontend: Fix getFeed to Always Send Pagination Params

### File: `drana-app/src/api/indexerApi.ts`

The current `getFeed` only sends `page` and `pageSize` if they're truthy. Since `page=1` is truthy this works, but `pageSize` is never sent (defaults to server's 20). Make it explicit:

```typescript
export function getFeed(params: { strategy?: string; channel?: string; page?: number; pageSize?: number } = {}) {
  const q = new URLSearchParams();
  if (params.strategy) q.set('strategy', params.strategy);
  if (params.channel) q.set('channel', params.channel);
  q.set('page', String(params.page || 1));
  q.set('pageSize', String(params.pageSize || 20));
  return get<FeedResponse>(`/v1/feed?${q}`);
}
```

### Acceptance Criteria

- Every feed request includes explicit `page` and `pageSize` params.

---

## Files Modified Summary

| Step | File | Change |
|------|------|--------|
| 1 | `internal/indexer/ranking.go` | Replace fetch-all-then-slice with SQL pagination for top/new/controversial; time-windowed Go scoring for trending; delete sortRankedPosts |
| 2 | `internal/indexer/db.go` | Add `idx_posts_boost_count` index in Migrate |
| 3 | `drana-app/src/pages/Feed.tsx` | Replace useQuery with useInfiniteQuery; add IntersectionObserver sentinel; remove page state and "Load more" button |
| 4 | `drana-app/src/components/LoadingDots.tsx` | New: three-dot amber pulse loading indicator |
| 5 | `drana-app/src/api/indexerApi.ts` | Always send page and pageSize in getFeed |

---

## Testing Notes

- Existing indexer unit tests in `ranking_test.go` seed 4 posts. These tests will still pass because both approaches produce the same results for small datasets. The SQL ORDER BY matches the Go scoring logic.
- The `trending` test may need a small adjustment: the 72-hour cutoff means test posts with `createdAtTime` in the distant past may be excluded. Seed data already uses `time.Now()` offsets, so this should be fine.
- Frontend testing: open the app, create 25+ posts across channels, scroll down, verify posts accumulate. Change strategy — feed resets. Change channel — feed resets. Navigate away and back — cached posts restore.

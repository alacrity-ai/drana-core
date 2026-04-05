# SCROLL_DESIGN.md

## Feed Pagination and Infinite Scroll

### Current State

**Backend:** Both the indexer (`ranking.go`) and node RPC (`server.go`) fetch ALL matching posts into Go memory, score/sort them, then slice `[start:end]` for the requested page. This works for 100 posts. At 10,000 posts it's slow. At 100,000 it's a memory bomb.

**Frontend:** The Feed has a "Load more" button that fetches page N — but it **replaces** the current page instead of appending. Clicking "Load more" discards posts you already scrolled through. The UX is: see 20 posts, click, see a *different* 20 posts, lose the first 20.

Both need fixing.

---

### Solution

#### Backend: SQL-Level Pagination for Non-Score Rankings

For `top`, `new`, and `controversial`, the ranking formulas can be computed in SQL. Only `trending` requires Go-side computation (it uses `time.Now()` which changes every request).

**`top`:** `ORDER BY total_committed DESC LIMIT ? OFFSET ?` — pure SQL.

**`new`:** `ORDER BY created_at_height DESC LIMIT ? OFFSET ?` — pure SQL.

**`controversial`:** `ORDER BY (boost_count * LOG(1 + total_committed)) DESC` — Postgres has `LOG()`. SQLite has it via extension, or we precompute a `controversy_score` column on insert/boost update.

**`trending`:** This one genuinely needs Go-side computation because the score depends on `time.Now()`. But we can optimize: instead of fetching ALL posts, fetch only posts from the last N hours (a configurable window). Posts older than, say, 72 hours have near-zero trending scores — no need to load them. The SQL becomes:

```sql
WHERE created_at_time > ? AND parent_post_id = ''
```

Then score, sort, and slice the (much smaller) working set in Go.

**Implementation:**

Update `rankedPostsQuery` in `ranking.go`:

```go
func (d *DB) rankedPostsQuery(...) {
    switch strategy {
    case RankByTopAllTime:
        // Pure SQL: ORDER BY total_committed DESC LIMIT pageSize OFFSET offset
        return d.sqlPaginatedQuery("total_committed DESC", author, channel, page, pageSize)
    case RankByNew:
        return d.sqlPaginatedQuery("created_at_height DESC", author, channel, page, pageSize)
    case RankByTrending:
        return d.trendingQuery(author, channel, page, pageSize, nowUnix)
    case RankByControversial:
        return d.sqlPaginatedQuery("(boost_count * 1.0) * total_committed DESC", author, channel, page, pageSize)
    }
}

func (d *DB) sqlPaginatedQuery(orderBy, author, channel string, page, pageSize int) ([]RankedPost, int, error) {
    // COUNT(*) for totalCount
    // SELECT ... ORDER BY {orderBy} LIMIT pageSize OFFSET (page-1)*pageSize
    // Score is just the ORDER BY value
}

func (d *DB) trendingQuery(...) {
    // Fetch only posts from last 72 hours
    // Score in Go, sort, slice
}
```

This means:
- `top` and `new` with 100K posts: fast (SQL does the work, only 20 rows transferred)
- `trending` with 100K posts: fetches ~last-72-hours subset (maybe 500 posts), scores in Go (fast), slices
- `controversial`: SQL-level sort on a computed expression

#### Frontend: Infinite Scroll with Accumulated Pages

Replace the "Load more" button with automatic infinite scroll that **appends** pages as you scroll down.

**Mechanism:**

Use TanStack Query's `useInfiniteQuery` instead of `useQuery`:

```typescript
const feed = useInfiniteQuery({
  queryKey: ['feed', strategy, channel],
  queryFn: ({ pageParam = 1 }) => getFeed({ strategy, channel, page: pageParam }),
  getNextPageParam: (lastPage, allPages) => {
    const loaded = allPages.length * 20;
    return loaded < lastPage.totalCount ? allPages.length + 1 : undefined;
  },
});
```

This gives us:
- `feed.data.pages` — array of all fetched pages
- `feed.fetchNextPage()` — loads the next page and appends
- `feed.hasNextPage` — true if more pages exist
- `feed.isFetchingNextPage` — true while loading

**Scroll detection:**

Use an `IntersectionObserver` on a sentinel element at the bottom of the feed. When the sentinel enters the viewport, trigger `fetchNextPage()`:

```typescript
const sentinelRef = useRef<HTMLDivElement>(null);

useEffect(() => {
  if (!sentinelRef.current || !feed.hasNextPage) return;
  const observer = new IntersectionObserver(([entry]) => {
    if (entry.isIntersecting && !feed.isFetchingNextPage) {
      feed.fetchNextPage();
    }
  }, { rootMargin: '200px' }); // trigger 200px before reaching bottom
  observer.observe(sentinelRef.current);
  return () => observer.disconnect();
}, [feed.hasNextPage, feed.isFetchingNextPage]);
```

The `rootMargin: '200px'` means the next page starts loading 200px before you reach the bottom — seamless.

**Render:**

```tsx
{feed.data?.pages.flatMap(page => page.posts).map(post => (
  <PostCard key={post.postId} post={post} ... />
))}
{feed.hasNextPage && (
  <div ref={sentinelRef} style={{ height: 1 }} />
)}
{feed.isFetchingNextPage && <LoadingSpinner />}
```

**Cache behavior:**

When changing strategy or channel, the query key changes, which resets the infinite query. First page loads fresh. Previous pages for a different strategy/channel are cached independently by TanStack Query.

**Scroll position:** When navigating away and back, TanStack Query restores cached pages instantly (staleTime: 5s). The user sees the feed they left, no re-fetch. If the data is stale, it refetches in the background and updates seamlessly.

---

### Implementation Steps

#### Step 1 — Backend: SQL-level pagination for top/new/controversial

**File:** `internal/indexer/ranking.go`

- Add `sqlPaginatedQuery` that builds `SELECT ... ORDER BY ... LIMIT ? OFFSET ?`
- Add `trendingQuery` that filters to last 72 hours before scoring in Go
- `RankedPosts` dispatches to the right method by strategy
- Count query uses the same WHERE clause but `SELECT COUNT(*)`

#### Step 2 — Frontend: Switch to useInfiniteQuery

**File:** `drana-app/src/pages/Feed.tsx`

- Replace `useQuery` with `useInfiniteQuery`
- Remove `page` state (managed by TanStack)
- Flatten `pages` array for rendering
- Add sentinel div with IntersectionObserver
- Remove "Load more" button
- Show a small loading indicator at the bottom while fetching next page

#### Step 3 — Frontend: Loading indicator

**File:** `drana-app/src/components/LoadingDots.tsx` (new)

A subtle three-dot pulsing indicator shown at the bottom of the feed while the next page loads. No spinner — dots match the minimalist aesthetic.

### Acceptance Criteria

1. Scrolling to the bottom of the feed automatically loads the next 20 posts.
2. Previously loaded posts stay visible (accumulated, not replaced).
3. Changing strategy or channel resets the feed to page 1.
4. With 10,000 posts in `top` or `new` ranking, the indexer responds in <50ms (SQL pagination, not Go-side).
5. With 10,000 posts in `trending`, the indexer loads only the recent window and responds in <200ms.
6. The feed feels seamless — no flicker, no jump, no lost scroll position.

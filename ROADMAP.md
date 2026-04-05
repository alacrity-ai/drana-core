# ROADMAP

Future features and improvements for DRANA. Each item includes a minimal description, acceptance criteria, and high-level implementation notes.

---

## 1. Share Post Link

### Problem

There's no way to share a direct link to a specific post outside the app. Users can't copy a URL and paste it on Twitter, Discord, etc. This is table stakes for any social platform — without it, organic growth is blocked.

### MVP

A small share button on each post card and post detail view. Tapping it copies a direct URL (e.g. `https://drana.io/#post/a1b2c3...`) to the clipboard. A brief "Link copied!" toast confirms the action.

### Acceptance Criteria

- Share button visible on every post card (feed) and post detail page.
- Clicking it copies the full URL (including post ID hash fragment) to clipboard.
- Visual feedback ("Copied!") shown for ~2 seconds.
- The copied URL, when opened in a browser, navigates directly to that post's detail view.
- Works on mobile (uses `navigator.clipboard` with fallback).

### Proposed Implementation

**Frontend only — no backend changes.**

- Add a share/link icon button to `PostCard.tsx` and `PostDetail.tsx`.
- On click: `navigator.clipboard.writeText(window.location.origin + '/#post/' + postId)`.
- Show a transient "Copied!" label next to the button (local state, auto-clear after 2s).
- Optionally use `navigator.share()` on mobile for native share sheets (progressive enhancement — fall back to clipboard copy if unavailable).

---

## 2. Remove Wallet (Standalone Access)

### Problem

The "Remove wallet" option currently only appears inside the "Forgot password?" flow of the Unlock modal. A user who is logged in and wants to remove their wallet (e.g. on a shared computer) has no obvious way to do it. It should be accessible from a top-level settings or wallet management area.

### MVP

An explicit "Remove Wallet" option accessible from the wallet dropdown or a wallet settings section. Requires confirmation before deletion.

### Acceptance Criteria

- "Remove Wallet" option accessible while the wallet is unlocked (not buried behind "Forgot password?").
- Confirmation prompt warns that this is irreversible without a private key backup.
- User must type their address (or a confirmation phrase) to confirm.
- After removal, app resets to the no-wallet state (or switches to another wallet if multi-wallet).
- The existing remove flow in the Forgot Password section continues to work as-is.

### Proposed Implementation

**Frontend only — no backend changes.**

- Add a "Remove Wallet" button to the wallet dropdown in `TopBar.tsx` (or a new wallet settings modal).
- Reuse the existing confirmation UI from `UnlockWalletModal` (type-address-to-confirm pattern).
- Call `removeWallet(address)` from `WalletContext` on confirmation.
- If the user has multiple wallets, switch to the next one. If it was the last wallet, show the welcome/create-wallet state.

---

## 3. Keyword Search

### Problem

Users cannot search for posts by content. In a channel with hundreds of posts, there's no way to find a specific post without scrolling through everything. This is especially important as the network grows.

### Caution

Full-text search is expensive. Naive approaches (LIKE '%keyword%' on every query, or indexing every word) can become a performance bottleneck and a DoS vector. The indexer is the right place for this — it's off-chain and can be scaled independently — but it still needs to be engineered carefully.

### MVP

A search bar in the feed header. User types a keyword, hits enter, and sees posts whose text contains that keyword. Results are paginated. Scoped to a channel if one is selected, otherwise searches all posts.

### Acceptance Criteria

- Search bar visible in the feed UI (above the post list).
- Typing a query and pressing Enter returns matching posts, ranked by relevance or recency.
- Supports channel-scoped search (if a channel is selected, search within it).
- Pagination works on search results.
- Empty query returns the normal feed.
- Queries are rate-limited or debounced to prevent abuse.
- Response time is acceptable (< 500ms for typical queries on a 100K-post database).

### Proposed Implementation

**Backend (indexer):**

- **SQLite**: Use FTS5 (Full-Text Search). Create a virtual table `posts_fts` backed by `posts.text`. FTS5 is built into SQLite, fast, and supports ranked results. Query with `MATCH` operator.
- **Postgres**: Use `tsvector`/`tsquery` with a GIN index on `posts.text`. Native full-text search with ranking via `ts_rank`.
- New query method: `SearchPosts(query string, channel string, page, pageSize int) ([]RankedPost, int, error)`.
- New API endpoint: `GET /v1/feed/search?q=keyword&channel=gaming&page=1&pageSize=20`.
- Rate limiting: track queries per IP with a token bucket (e.g. 10 queries/minute). Return 429 if exceeded. Alternatively, enforce a minimum query length (3+ chars) and debounce on the frontend.
- The FTS index is populated incrementally as posts are indexed (add to `InsertPost`).

**Frontend:**

- Add a search input to the feed header (above strategy tabs or inline).
- Debounce input (300ms) before firing the API call.
- Use the existing `FeedResponse` format for results (search endpoint returns the same shape).
- Clear search on channel change or explicit "X" button.
- Show "No results for '...'" empty state.

**Performance considerations:**

- FTS5/GIN indexes are write-cheap and read-fast for keyword lookups.
- Avoid `LIKE '%keyword%'` — it requires a full table scan.
- Consider limiting search to top-level posts (exclude replies) to reduce result set.
- If the dataset grows very large, consider a dedicated search service (e.g. Meilisearch) behind the indexer API, but this is premature for MVP.

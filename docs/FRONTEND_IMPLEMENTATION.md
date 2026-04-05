# FRONTEND_IMPLEMENTATION.md

## DRANA Web Frontend — Implementation Steps

### Overview

A React SPA in `./drana-app/` that serves as the first-party client for the DRANA attention market. Built with React 19, TypeScript, Vite, and Node 24. Runs in Docker alongside the blockchain via docker-compose.

Reference: `FRONTEND_DESIGN.md` for pages, modals, wallet architecture, and visual design.

---

## Step 1 — Project Scaffold

### Files

```
drana-app/
  package.json
  tsconfig.json
  tsconfig.node.json
  vite.config.ts
  index.html
  Makefile
  Dockerfile
  .dockerignore
  .env.example
  src/
    main.tsx
    App.tsx
    styles/
      global.css
      variables.css
```

**`package.json`**

Dependencies:
- `react`, `react-dom` (^19)
- `@tanstack/react-query` — data fetching and cache
- `@noble/ed25519` — Ed25519 signing
- `@noble/hashes` — SHA-256 for transaction hashing and PostID derivation

Dev dependencies:
- `typescript` (^5.5)
- `vite` (^6)
- `@vitejs/plugin-react`
- `@types/react`, `@types/react-dom`

No CSS framework. No router library — hash routing via a minimal custom hook (4 pages doesn't justify react-router).

**`vite.config.ts`**

Standard Vite config with React plugin. Proxy `/api/node` to `http://localhost:26657` and `/api/indexer` to `http://localhost:26680` in dev mode to avoid CORS issues during development.

**`index.html`**

Loads Google Fonts (Inter 400/500/600, JetBrains Mono 500) and mounts `<div id="root">`.

**`src/styles/variables.css`**

All design tokens from FRONTEND_DESIGN.md: colors, spacing, font stacks, transitions.

**`src/styles/global.css`**

Reset, body styles (dark background, Inter font), scrollbar styling, base element styles.

**`src/App.tsx`**

Shell: renders `<TopBar />` and the current page based on hash route. Wraps everything in `<QueryClientProvider>` and `<WalletProvider>`.

**`src/main.tsx`**

`createRoot(document.getElementById('root')).render(<App />)`.

**`Makefile`**

```makefile
dev:       npm run dev
build:     npm run build
preview:   npm run preview
lint:      npx tsc --noEmit
docker:    docker build -t drana-app .
```

**`Dockerfile`**

Multi-stage: Node 24 build stage runs `npm ci && npm run build`, nginx stage serves the `dist/` directory. Nginx config proxies `/api/node/` to the node RPC and `/api/indexer/` to the indexer.

**`.env.example`**

```
VITE_NODE_RPC=http://localhost:26657
VITE_INDEXER_API=http://localhost:26680
```

### Acceptance Criteria

- `npm install && npm run dev` starts a dev server with hot reload.
- `npm run build` produces a production build in `dist/`.
- The page loads in a browser showing a dark background with "DRANA" text.
- Docker build succeeds and serves the static files via nginx.

---

## Step 2 — API Client Layer

### Files

```
src/api/
  types.ts
  nodeRpc.ts
  indexerApi.ts
  config.ts
```

**`src/api/types.ts`**

TypeScript interfaces mirroring every Go response type:

- `NodeInfo` — chain ID, height, epoch, supply stats
- `Block` — height, hash, proposer, timestamp, txCount, transactions
- `Account` — address, balance, nonce, name, stakedBalance
- `UnbondingResponse` — address, entries, total
- `Post` — postId, author, text, channel, parentPostId, createdAtHeight, createdAtTime, totalCommitted, boostCount
- `PostList` — posts, totalCount, page, pageSize
- `RankedPost` — extends Post with score, authorCommitted, thirdPartyCommitted, uniqueBoosterCount, replyCount
- `FeedResponse` — posts (RankedPost[]), totalCount, page, pageSize, strategy
- `BoostHistoryResponse` — boosts, totalCount, page, pageSize
- `IndexedBoost` — postId, booster, amount, blockHeight, blockTime, txHash
- `AuthorProfile` — address, postCount, totalCommitted, totalReceived, uniqueBoosterCount
- `ChannelInfo` — channel, postCount
- `StatsResponse` — latestHeight, totalPosts, totalBoosts, totalBurned, totalIssued, circulatingSupply
- `LeaderboardEntry` — address, totalReceived, postCount, boostCount
- `SubmitTxRequest` — type, sender, recipient, postId, parentPostId, text, channel, amount, nonce, signature, pubKey
- `SubmitTxResponse` — accepted, txHash, error
- `TxStatus` — hash, status, blockHeight
- `Validator` — address, name, pubKey, stakedBalance

**`src/api/config.ts`**

```typescript
export const NODE_RPC = import.meta.env.VITE_NODE_RPC || '/api/node';
export const INDEXER_API = import.meta.env.VITE_INDEXER_API || '/api/indexer';
```

**`src/api/nodeRpc.ts`**

Typed fetch wrappers for every node RPC endpoint:

```typescript
export async function getNodeInfo(): Promise<NodeInfo>
export async function getAccount(address: string): Promise<Account>
export async function getUnbonding(address: string): Promise<UnbondingResponse>
export async function getAccountByName(name: string): Promise<Account>
export async function submitTransaction(req: SubmitTxRequest): Promise<SubmitTxResponse>
export async function getTransaction(hash: string): Promise<TransactionResponse>
export async function getTxStatus(hash: string): Promise<TxStatus>
export async function getValidators(): Promise<Validator[]>
```

Each function: `fetch(url)`, check status, parse JSON, throw on error.

**`src/api/indexerApi.ts`**

```typescript
export async function getFeed(params: { strategy?: string; channel?: string; page?: number; pageSize?: number }): Promise<FeedResponse>
export async function getFeedByAuthor(address: string, params: { strategy?: string; page?: number }): Promise<FeedResponse>
export async function getChannels(): Promise<ChannelInfo[]>
export async function getPost(id: string): Promise<RankedPost>
export async function getPostBoosts(id: string, page?: number): Promise<BoostHistoryResponse>
export async function getPostReplies(id: string, page?: number): Promise<{ replies: RankedPost[]; totalCount: number }>
export async function getAuthorProfile(address: string): Promise<AuthorProfile>
export async function getStats(): Promise<StatsResponse>
export async function getLeaderboard(page?: number): Promise<{ authors: LeaderboardEntry[]; totalCount: number }>
```

### Acceptance Criteria

- All types match the Go response structs exactly.
- Each function is typed end-to-end (input params → Promise<ResponseType>).
- API calls work against a running local testnet (`make docker-up`).

---

## Step 3 — Wallet: Crypto and Key Management

### Files

```
src/wallet/
  crypto.ts
  signableBytes.ts
  storage.ts
```

**`src/wallet/crypto.ts`**

Core cryptographic operations using `@noble/ed25519` and Web Crypto API:

```typescript
export function generateKeyPair(): { publicKey: Uint8Array; privateKey: Uint8Array }
export function sign(privateKey: Uint8Array, message: Uint8Array): Uint8Array
export function deriveAddress(publicKey: Uint8Array): string  // returns "drana1..." with checksum

export async function encryptPrivateKey(privateKey: Uint8Array, password: string): EncryptedKey
export async function decryptPrivateKey(encrypted: EncryptedKey, password: string): Uint8Array
```

`EncryptedKey` type:
```typescript
type EncryptedKey = {
  ciphertext: string;  // base64
  salt: string;        // base64
  iv: string;          // base64
}
```

Encryption uses PBKDF2 (100k iterations, SHA-256) → AES-GCM (256-bit), all via `window.crypto.subtle`.

Address derivation must exactly match the Go implementation:
1. `pubKeyHash = SHA-256(publicKey)`
2. `body = pubKeyHash.slice(0, 20)`
3. `checksum = SHA-256(body).slice(0, 4)`
4. `address = "drana1" + hex(body + checksum)`

**`src/wallet/signableBytes.ts`**

The most critical file. Produces identical bytes to Go's `Transaction.SignableBytes()`.

```typescript
export function computeSignableBytes(tx: UnsignedTransaction): Uint8Array
export function computeTxHash(tx: SignedTransaction): string  // hex
export function derivePostID(authorAddress: string, nonce: number): string  // hex
```

`computeSignableBytes` serialization — must match Go exactly:
1. WriteUint32(type)
2. WriteBytes(sender, 24 bytes)
3. WriteBytes(recipient, 24 bytes)
4. WriteBytes(postId, 32 bytes)
5. WriteString(text) — 8-byte big-endian length prefix + UTF-8 bytes
6. WriteString(channel) — same encoding
7. WriteUint64(amount)
8. WriteUint64(nonce)
9. WriteBytes(pubKey, 32 bytes)

Each `WriteBytes` is: 8-byte big-endian length + data.
Each `WriteString` is: 8-byte big-endian length + UTF-8 bytes.
Each `WriteUint64` is: 8-byte big-endian.

This must have a **dedicated test** that compares output against known Go-generated test vectors. Without this, no transaction will ever be accepted by the chain.

**`src/wallet/storage.ts`**

LocalStorage wrapper:

```typescript
export function saveWallet(wallet: StoredWallet): void
export function loadWallet(): StoredWallet | null
export function clearWallet(): void

type StoredWallet = {
  address: string;
  publicKey: string;      // hex
  encryptedKey: EncryptedKey;
  name: string;           // cached, may be empty
}
```

### Acceptance Criteria

- `generateKeyPair` produces valid Ed25519 keys.
- `deriveAddress` output matches `drana-cli keygen` for the same key.
- `encryptPrivateKey` → `decryptPrivateKey` round-trip recovers the original key.
- Wrong password throws on decrypt.
- **`computeSignableBytes` produces byte-for-byte identical output to Go's `SignableBytes()`** — verified against test vectors generated by `drana-cli`.
- `derivePostID` matches Go's `DerivePostID` for the same inputs.

---

## Step 4 — Wallet: React Context and Modals

### Files

```
src/wallet/
  WalletContext.tsx
  useWallet.ts
  CreateWalletModal.tsx
  UnlockWalletModal.tsx
  ImportWalletModal.tsx
  WalletDropdown.tsx
```

**`src/wallet/WalletContext.tsx`**

React context providing wallet state to the entire app:

```typescript
type WalletState = {
  // Persistent (localStorage)
  address: string | null;
  publicKey: string | null;  // hex
  name: string | null;

  // Ephemeral (memory only)
  isUnlocked: boolean;
  balance: number;           // microdrana, refreshed periodically

  // Actions
  createWallet: (password: string) => Promise<void>;
  unlock: (password: string) => Promise<void>;
  lock: () => void;
  importWallet: (privateKeyHex: string, password: string) => Promise<void>;
  signAndSubmit: (tx: UnsignedTransaction) => Promise<SubmitTxResponse>;
}
```

`signAndSubmit`:
1. Queries current nonce from node RPC
2. Sets `tx.nonce = queriedNonce + 1`
3. Signs with the in-memory decrypted key
4. Submits via `nodeRpc.submitTransaction()`
5. Returns result

On `lock()` or page `beforeunload`: wipes decrypted key from state.

**`src/wallet/CreateWalletModal.tsx`**

Password input (+ confirm), calls `createWallet`, shows address on success.

**`src/wallet/UnlockWalletModal.tsx`**

Password input, calls `unlock`, closes on success.

**`src/wallet/ImportWalletModal.tsx`**

Private key hex input + password, calls `importWallet`.

**`src/wallet/WalletDropdown.tsx`**

Dropdown menu: Register Name, Export Key, Import, Lock, Faucet. Each item opens the appropriate modal or action.

### Acceptance Criteria

- Create wallet → wallet stored in localStorage, auto-unlocked.
- Lock → unlock with correct password restores state.
- Lock → unlock with wrong password shows error.
- `signAndSubmit` produces a transaction the chain accepts.
- Page refresh preserves wallet (locked state), requires password to unlock.
- Closing browser tab wipes decrypted key from memory.

---

## Step 5 — Shared Components

### Files

```
src/components/
  TopBar.tsx
  PostCard.tsx
  ReplyCard.tsx
  Modal.tsx
  AmountInput.tsx
  ChannelPills.tsx
  EmptyState.tsx
  Skeleton.tsx
  DranaAmount.tsx
  TimeAgo.tsx
  TruncatedAddress.tsx
```

**`TopBar.tsx`** — 56px bar with DRANA logo, nav links, wallet indicator. Uses `useWallet()` hook for state.

**`PostCard.tsx`** — Single post row. Props: `post: RankedPost`, `onBoost`, `onReply`. Shows amber amount, author (clickable → profile), channel tag, text (truncated), reply/booster counts, boost/reply buttons. High-value posts get the left amber border.

**`ReplyCard.tsx`** — Compact reply row with purple left border. Same structure as PostCard but smaller, no reply button (flat replies).

**`Modal.tsx`** — Reusable modal shell: dark overlay with backdrop blur, centered panel, close button. Children are the content.

**`AmountInput.tsx`** — Number input styled for DRANA amounts. Shows both microdrana input and DRANA display. Enforces min values.

**`ChannelPills.tsx`** — Horizontal scrollable row of channel filter pills. Active pill is amber.

**`EmptyState.tsx`** — "No posts yet" / "Connect wallet to post" centered messages.

**`Skeleton.tsx`** — Loading placeholder cards with pulse animation.

**`DranaAmount.tsx`** — Renders a DRANA amount in amber JetBrains Mono. Handles microdrana → DRANA conversion.

**`TimeAgo.tsx`** — Renders relative timestamps ("2h ago", "3d ago").

**`TruncatedAddress.tsx`** — Shows `drana1abc...def` with copy-on-click.

### Acceptance Criteria

- All components render correctly in isolation.
- PostCard shows the amber left border for high-value posts.
- Modal handles Escape key and overlay click to close.
- DranaAmount displays "42.50 DRANA" for input 42500000.
- TimeAgo updates automatically.

---

## Step 6 — Feed Page

### Files

```
src/pages/
  Feed.tsx
src/components/
  NewPostModal.tsx
  BoostModal.tsx
  ReplyModal.tsx
  SendModal.tsx
```

**`Feed.tsx`**

The home page. Uses TanStack Query to fetch:
- `getFeed({ strategy, channel, page })` — the post list
- `getChannels()` — for the channel pill bar

State: `strategy` (default "trending"), `channel` (default ""), `page` (default 1).

Renders: channel pills, sort dropdown, post list (PostCard for each), "Load more" button.

**`NewPostModal.tsx`**

Textarea (280 char limit with live counter), channel dropdown, amount input (min 1 DRANA), submit button. On submit: normalize text, call `wallet.signAndSubmit(createPostTx)`, show success with post ID, invalidate feed query.

**`BoostModal.tsx`**

Shows post preview, amount input (min 0.1 DRANA), submit button. On submit: `wallet.signAndSubmit(boostPostTx)`, invalidate feed query.

**`ReplyModal.tsx`**

Shows "Replying to: {parent text}" header, textarea, amount input, submit button. On submit: `wallet.signAndSubmit(createPostTxWithParentId)`, invalidate post detail query.

**`SendModal.tsx`**

"To" field (accepts `drana1...` address or registered name), amount input, send button. If a name is entered, resolves via `nodeRpc.getAccountByName(name)` and shows the resolved address for confirmation before sending. On submit: `wallet.signAndSubmit(transferTx)`, show success with tx hash, refresh balance. Accessible from the wallet dropdown menu.

### Acceptance Criteria

- Feed loads and displays posts from the indexer.
- Changing strategy re-sorts the feed.
- Changing channel filters the feed.
- Pagination works (load more).
- New Post modal creates a transaction the chain accepts.
- Boost modal increases a post's committed amount.
- Reply modal creates a reply visible on the post detail page.
- Send modal transfers DRANA to another address, balance updates after confirmation.
- Send modal resolves registered names to addresses.
- All modals require an unlocked wallet; if locked, show the unlock modal first.

---

## Step 7 — Post Detail Page

### Files

```
src/pages/
  PostDetail.tsx
```

**`PostDetail.tsx`**

Fetches:
- `getPost(id)` — the post with all derived fields
- `getPostReplies(id, page)` — reply thread
- `getPostBoosts(id, page)` — boost history

Renders:
- Full post (large amount, full text, author/channel/block/time metadata)
- Author/third-party committed breakdown
- Unique booster count
- "Boost this post" button
- Reply list (ReplyCard for each, sorted by committed value)
- "Write a reply" button
- Boost history table (compact, monospace)
- Pagination for replies and boosts

### Acceptance Criteria

- Post detail shows all enriched fields from the indexer.
- Replies are sorted by committed value.
- Boost history shows each booster, amount, and block.
- Boost and reply actions work from this page.

---

## Step 8 — Channels and Profile Pages

### Files

```
src/pages/
  Channels.tsx
  Profile.tsx
```

**`Channels.tsx`**

Fetches `getChannels()`. Renders a list of channels with post counts. Clicking a channel navigates to `/#/?channel=gaming`.

**`Profile.tsx`**

Route: `/#/profile/{address}`. Fetches:
- `getAccount(address)` — balance, stake, name
- `getFeedByAuthor(address)` — their posts
- `getAuthorProfile(address)` — aggregate stats

Renders: name (or address), balance, staked balance, post list, aggregate stats (total committed, boosts received, unique boosters).

### Acceptance Criteria

- Channels page lists all channels with counts.
- Clicking a channel navigates to the feed filtered by that channel.
- Profile page shows the correct user's posts and stats.
- Clicking an author name on any post card navigates to their profile.

---

## Step 9 — Hash Router

### Files

```
src/router.ts
```

**`src/router.ts`**

Minimal hash router (no library):

```typescript
export function useRoute(): { page: string; params: Record<string, string> }
export function navigate(path: string): void
```

Routes:
- `#/` → Feed
- `#/post/{id}` → PostDetail
- `#/channels` → Channels
- `#/profile/{address}` → Profile

`useRoute` parses `window.location.hash` and returns the current page + params. Listens to `hashchange` events.

`navigate` sets `window.location.hash`.

### Acceptance Criteria

- Navigation between all 4 pages works via hash URLs.
- Browser back/forward buttons work.
- Direct URL entry works (e.g., `http://localhost:5173/#/post/abc123`).

---

## Step 10 — Docker and Integration with Blockchain

### Files

```
drana-app/
  Dockerfile
  nginx.conf
```

Update root:
```
docker-compose.yml    (add web service)
```

**`drana-app/Dockerfile`**

```dockerfile
FROM node:24-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
```

**`drana-app/nginx.conf`**

```nginx
server {
    listen 3000;
    root /usr/share/nginx/html;
    index index.html;

    # SPA fallback
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Proxy node RPC
    location /api/node/ {
        proxy_pass http://validator-1:26657/;
    }

    # Proxy indexer API
    location /api/indexer/ {
        proxy_pass http://indexer:26680/;
    }
}
```

**Root `docker-compose.yml`** — add:

```yaml
  web:
    build: ./drana-app
    container_name: drana-web
    ports:
      - "3000:3000"
    depends_on:
      - validator-1
      - indexer
    networks:
      - drana
```

**Root `Makefile`** — add targets:

```makefile
web-dev:     cd drana-app && npm run dev        ## Start frontend dev server
web-build:   cd drana-app && npm run build      ## Build frontend for production
web-install: cd drana-app && npm install        ## Install frontend dependencies
```

### Acceptance Criteria

- `docker compose up --build` starts validators + indexer + Postgres + web frontend.
- `http://localhost:3000` serves the SPA.
- API calls from the browser are proxied to the node RPC and indexer via nginx.
- No CORS issues.

---

## Step 11 — Test Vectors and Signing Verification

### Files

```
drana-app/
  test/
    signableBytes.test.ts
    crypto.test.ts
    address.test.ts
```

**Generate Go test vectors:**

Create a Go test helper (or script) that outputs known transactions with their:
- Serialized signable bytes (hex)
- Transaction hash (hex)
- PostID derivation results

Save as `drana-app/test/vectors.json`.

**`signableBytes.test.ts`**

For each test vector:
1. Construct the same transaction in TypeScript
2. Call `computeSignableBytes(tx)`
3. Assert the output matches the Go hex exactly

**`crypto.test.ts`**

- Generate keypair, sign a message, verify.
- Encrypt/decrypt round-trip.

**`address.test.ts`**

- Derive address from a known public key, assert it matches Go output.
- Parse a `drana1...` address, verify checksum.

### Acceptance Criteria

- All test vectors pass.
- A transaction signed in the browser is accepted by a running Go node.
- Address derivation matches `drana-cli keygen` for the same key.

---

## Phase Summary

| Step | What | Key files |
|------|------|-----------|
| 1 | Project scaffold | package.json, vite.config.ts, tsconfig, Dockerfile, Makefile, global CSS with design tokens |
| 2 | API client layer | types.ts (mirrors Go), nodeRpc.ts, indexerApi.ts |
| 3 | Wallet crypto | crypto.ts (Ed25519, AES-GCM), signableBytes.ts (must match Go), storage.ts |
| 4 | Wallet React layer | WalletContext, Create/Unlock/Import modals, dropdown |
| 5 | Shared components | TopBar, PostCard, ReplyCard, Modal, AmountInput, ChannelPills, DranaAmount, TimeAgo |
| 6 | Feed page | Feed.tsx, NewPostModal, BoostModal, ReplyModal |
| 7 | Post detail page | PostDetail.tsx with replies + boosts |
| 8 | Channels + Profile | Channels.tsx, Profile.tsx |
| 9 | Hash router | Minimal custom router, 4 routes |
| 10 | Docker + compose | Dockerfile, nginx.conf, docker-compose web service |
| 11 | Test vectors | Go-generated vectors, TypeScript signing tests |

### Exit Criteria

1. `docker compose up --build` starts the full stack: 3 validators, Postgres, indexer, web frontend.
2. `http://localhost:3000` serves the SPA with the DRANA visual design.
3. A user can create a wallet, see their address, and see a zero balance.
4. After receiving DRANA (faucet or transfer), the balance updates.
5. The user can create a post with a channel, see it appear in the feed.
6. The user can boost any post, see the committed amount increase.
7. The user can reply to a post, see the reply in the post detail thread.
8. The user can register a name, see it reflected in the top bar and on their posts.
9. Sorting and channel filtering work in the feed.
10. Clicking an author name navigates to their profile.
11. All signing tests pass against Go test vectors.
12. The wallet locks on page close and requires password to unlock.

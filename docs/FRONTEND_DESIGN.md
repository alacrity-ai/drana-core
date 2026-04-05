# FRONTEND_DESIGN.md

## DRANA Web Frontend — Design Proposal

### What This Is

A single-page React app that lets anyone browse the DRANA attention market, create a wallet, get free DRANA from a faucet, post messages, boost posts, and reply — all from a browser. No external wallet software required.

### Tech Stack

- **Node 24**, **React 19**, **TypeScript**, **Vite**
- **TanStack Query** for API data fetching and caching
- **Ed25519 signing in-browser** via `@noble/ed25519` (audited, pure JS, no WASM)
- **AES-GCM encryption** for key-at-rest via Web Crypto API (native browser, no library)
- No CSS framework — minimal custom CSS. The product is text and numbers; it should feel like a terminal, not a marketing site.

---

## Pages

The app has **4 pages** plus a persistent top bar. It's a SPA with hash routing (no server-side rendering needed).

### Top Bar (persistent)

Always visible. Shows:

```
┌─────────────────────────────────────────────────────────────────────────┐
│  DRANA                [Feed]  [Channels]           [wallet indicator]   │
│                                                    satoshi · 42.5 DRANA │
└─────────────────────────────────────────────────────────────────────────┘
```

- **DRANA** logo/text — links to home (feed)
- **Feed** / **Channels** nav links
- **Wallet indicator** (right side):
  - No wallet: shows **[Create Wallet]** button
  - Wallet exists but locked: shows **[Unlock]** button
  - Wallet unlocked: shows **name** (or truncated address) + **balance** + small dropdown for wallet actions (lock, export, copy address)

### Page 1: Feed (home page, `/`)

The main view. A ranked list of top-level posts.

```
┌─────────────────────────────────────────────────────────────────────────┐
│  [New Post]                                                             │
│                                                                         │
│  Sort: [Trending ▼]  Channel: [All ▼]                                  │
│  ───────────────────────────────────────────────────────────────────── │
│                                                                         │
│  🔥 42.5 DRANA                                              2 hours ago │
│  satoshi · #general                                                     │
│  "The empire of relevance belongs to the highest bidder."               │
│  💬 12 replies · 👥 8 boosters                          [Boost] [Reply] │
│  ───────────────────────────────────────────────────────────────────── │
│                                                                         │
│  🔥 18.2 DRANA                                              5 hours ago │
│  alice · #politics                                                      │
│  "Attention is the only honest currency."                               │
│  💬 3 replies · 👥 2 boosters                           [Boost] [Reply] │
│  ───────────────────────────────────────────────────────────────────── │
│                                                                         │
│  [Load more]                                                            │
└─────────────────────────────────────────────────────────────────────────┘
```

**Data source:** Indexer `GET /v1/feed?strategy=trending&channel=&page=1`

**Controls:**
- **Sort dropdown:** trending, top, new, controversial
- **Channel dropdown:** "All" + list from `GET /v1/channels`
- **[New Post]** button → opens New Post modal (requires unlocked wallet)
- **[Boost]** button on each post → opens Boost modal
- **[Reply]** button → opens Reply modal

Each post row shows:
- Total committed (in DRANA, human-readable)
- Author name (or truncated address if no name)
- Channel tag
- Post text (truncated to ~200 chars with "show more")
- Reply count, unique booster count
- Relative time ("2 hours ago")

### Page 2: Post Detail (`/post/{id}`)

Full view of a single post with its reply thread.

```
┌─────────────────────────────────────────────────────────────────────────┐
│  ← Back to feed                                                         │
│                                                                         │
│  🔥 42.5 DRANA committed (35.0 by author, 7.5 by others)               │
│  satoshi · #general · Block 142 · 2 hours ago                          │
│                                                                         │
│  "The empire of relevance belongs to the highest bidder."               │
│                                                                         │
│  👥 8 unique boosters · 12 boosts total              [Boost this post]  │
│                                                                         │
│  ─── Replies (12) sorted by: [Top ▼] ──────────────────────────────── │
│                                                                         │
│  🔥 3.2 DRANA · alice · 1 hour ago                                      │
│  "Bold claim. But is it true?"                                 [Boost]  │
│                                                                         │
│  🔥 1.0 DRANA · bob · 45 min ago                                        │
│  "It's already true. Look at this board."                      [Boost]  │
│                                                                         │
│  [Write a reply]                                                        │
│                                                                         │
│  ─── Boost History ─────────────────────────────────────────────────── │
│  bob boosted 3.0 DRANA at block 155                                     │
│  carol boosted 2.5 DRANA at block 148                                   │
│  satoshi boosted 2.0 DRANA at block 145                                 │
│  [Show more]                                                            │
└─────────────────────────────────────────────────────────────────────────┘
```

**Data sources:**
- Indexer `GET /v1/posts/{id}` (enriched post)
- Indexer `GET /v1/posts/{id}/replies?page=1`
- Indexer `GET /v1/posts/{id}/boosts?page=1`

### Page 3: Channels (`/channels`)

Directory of all channels with post counts.

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Channels                                                               │
│  ───────────────────────────────────────────────────────────────────── │
│  #general          42 posts    🔥 850 DRANA burned                      │
│  #politics         15 posts    🔥 320 DRANA burned                      │
│  #gaming            8 posts    🔥 45 DRANA burned                       │
│  #crypto            5 posts    🔥 120 DRANA burned                      │
└─────────────────────────────────────────────────────────────────────────┘
```

Clicking a channel navigates to the feed with that channel pre-selected.

**Data source:** Indexer `GET /v1/channels`

### Page 4: Profile (`/profile/{address}`)

User profile — their posts, stats, and staking info.

```
┌─────────────────────────────────────────────────────────────────────────┐
│  satoshi                                                                │
│  drana1abc123...def456                                                  │
│                                                                         │
│  Balance: 42.5 DRANA    Staked: 1,000 DRANA    Nonce: 17                │
│                                                                         │
│  ─── Posts (12) ────────────────────────────────────────────────────── │
│  [list of their posts, same card format as feed]                        │
│                                                                         │
│  ─── Stats ─────────────────────────────────────────────────────────── │
│  Total committed: 85 DRANA                                              │
│  Boosts received: 23 DRANA from 7 unique boosters                       │
└─────────────────────────────────────────────────────────────────────────┘
```

**Data sources:**
- Node RPC `GET /v1/accounts/{address}` (balance, stake, name)
- Indexer `GET /v1/feed/author/{address}` (their posts)
- Indexer `GET /v1/authors/{address}` (aggregate stats)

---

## Modals

### Create Wallet Modal

Triggered by **[Create Wallet]** in the top bar.

```
┌──────────────────────────────────────┐
│  Create Wallet                       │
│                                      │
│  Choose a password to encrypt your   │
│  private key.                        │
│                                      │
│  Password:     [••••••••••]          │
│  Confirm:      [••••••••••]          │
│                                      │
│  [Create Wallet]                     │
│                                      │
│  ⚠ Your key is stored encrypted in  │
│  this browser. Export a backup.      │
└──────────────────────────────────────┘
```

On create:
1. Generate Ed25519 keypair
2. Encrypt private key with AES-GCM using password-derived key (PBKDF2)
3. Store encrypted key + salt + address in localStorage
4. Auto-unlock the wallet
5. Show the address and offer to copy it

### Unlock Wallet Modal

```
┌──────────────────────────────────────┐
│  Unlock Wallet                       │
│                                      │
│  drana1abc...def                      │
│                                      │
│  Password:     [••••••••••]          │
│                                      │
│  [Unlock]                            │
└──────────────────────────────────────┘
```

On unlock: decrypt private key, hold in memory (React state, not localStorage). The key is wiped on lock or page close.

### New Post Modal

```
┌──────────────────────────────────────┐
│  New Post                            │
│                                      │
│  Channel: [general ▼]               │
│                                      │
│  ┌──────────────────────────────┐   │
│  │ Your message here...         │   │
│  │                              │   │
│  └──────────────────────────────┘   │
│  142 / 280 characters               │
│                                      │
│  Commit: [1.0     ] DRANA           │
│  (min 1.0 DRANA)                     │
│                                      │
│  [Post]                              │
│                                      │
│  This burns 1.0 DRANA permanently.   │
└──────────────────────────────────────┘
```

On post:
1. Normalize text (NFC, trim, collapse whitespace)
2. Query nonce from RPC
3. Construct CreatePost transaction
4. Sign with in-memory private key
5. Submit to node RPC `POST /v1/transactions`
6. Show success with tx hash and post ID
7. Invalidate feed query cache (TanStack Query)

### Boost Modal

```
┌──────────────────────────────────────┐
│  Boost Post                          │
│                                      │
│  "The empire of relevance..."        │
│  by satoshi · currently 42.5 DRANA   │
│                                      │
│  Amount: [0.5     ] DRANA            │
│  (min 0.1 DRANA)                     │
│                                      │
│  [Boost]                             │
│                                      │
│  This burns 0.5 DRANA permanently.   │
└──────────────────────────────────────┘
```

### Reply Modal

Same as New Post modal, but with a "Replying to:" header showing the parent post text, and no channel selector (replies inherit the parent's context).

### Send DRANA Modal

```
┌──────────────────────────────────────┐
│  Send DRANA                          │
│                                      │
│  To:       [drana1... or name   ]    │
│                                      │
│  Amount:   [10.0     ] DRANA         │
│                                      │
│  [Send]                              │
│                                      │
│  Your balance: 42.5 DRANA            │
└──────────────────────────────────────┘
```

The "To" field accepts either a `drana1...` address or a registered name. If a name is entered, the frontend resolves it via `GET /v1/accounts/name/{name}` before constructing the transaction. Shows the resolved address below the input for confirmation.

On send:
1. Resolve name to address if needed
2. Query nonce
3. Construct Transfer transaction
4. Sign and submit
5. Show success with tx hash

### Wallet Actions Dropdown

```
┌──────────────────────────────┐
│  Send DRANA                  │
│  Register Name               │
│  Export Private Key           │
│  Import Wallet                │
│  Lock Wallet                  │
│  ──────────────────────────  │
│  Faucet (get free DRANA)     │
└──────────────────────────────┘
```

**Send DRANA** → opens Send DRANA modal.
**Register Name** → modal with name input, submits RegisterName tx.
**Export** → shows encrypted key (or raw after password confirm) for backup.
**Import** → paste a private key hex, encrypt with password, replace current wallet.
**Lock** → wipes decrypted key from memory, returns to locked state.
**Faucet** → calls faucet endpoint (when built) or shows instructions.

---

## Wallet Architecture

### Key Storage

```
localStorage["drana_wallet"] = JSON.stringify({
  address: "drana1...",
  publicKey: "hex...",
  encryptedKey: "base64...",   // AES-GCM encrypted private key
  salt: "base64...",           // PBKDF2 salt
  iv: "base64...",             // AES-GCM IV
  name: "satoshi"              // cached, updated on RegisterName
})
```

### Encryption Flow

**Encrypt (on create/import):**
1. `salt = crypto.getRandomValues(16 bytes)`
2. `keyMaterial = PBKDF2(password, salt, 100000 iterations, SHA-256)`
3. `aesKey = deriveKey(keyMaterial, AES-GCM, 256)`
4. `iv = crypto.getRandomValues(12 bytes)`
5. `encryptedKey = AES-GCM.encrypt(aesKey, iv, privateKeyBytes)`

**Decrypt (on unlock):**
1. Read salt, iv, encryptedKey from localStorage
2. Derive AES key from password + salt (same PBKDF2 params)
3. `privateKeyBytes = AES-GCM.decrypt(aesKey, iv, encryptedKey)`
4. Store decrypted key in React context (memory only)

**On lock / page unload:**
- Clear the decrypted key from React state
- The encrypted version remains in localStorage

### Transaction Signing

All signing happens in the browser:

```typescript
import { ed25519 } from '@noble/ed25519';

function signTransaction(tx: UnsignedTx, privateKey: Uint8Array): SignedTx {
  const signableBytes = computeSignableBytes(tx);  // same algorithm as Go
  const signature = ed25519.sign(signableBytes, privateKey.slice(0, 32));
  return { ...tx, signature, pubKey: privateKey.slice(32) };
}
```

The `computeSignableBytes` function must produce **identical bytes** to the Go `SignableBytes()` method. This means:
- Same field order
- Same length-prefix encoding (8-byte big-endian length + data)
- Same uint64 big-endian encoding

This is the most critical piece — if the bytes differ by one bit, the signature is invalid and the node rejects the transaction.

---

## API Connectivity

The frontend talks to two backends:

```
Frontend (browser)
  ├── Node RPC (port 26657)
  │   ├── GET /v1/accounts/{addr}     — balance, nonce
  │   ├── POST /v1/transactions        — submit signed tx
  │   └── GET /v1/node/info            — chain status
  │
  └── Indexer API (port 26680)
      ├── GET /v1/feed                  — ranked posts
      ├── GET /v1/channels              — channel list
      ├── GET /v1/posts/{id}            — post detail
      ├── GET /v1/posts/{id}/replies    — reply thread
      ├── GET /v1/posts/{id}/boosts     — boost history
      ├── GET /v1/authors/{addr}        — profile stats
      └── GET /v1/feed/author/{addr}    — user's posts
```

The frontend needs the RPC and indexer URLs configured. In production these would be:
- `https://genesis-validator.drana.io:26657` (or behind a reverse proxy)
- `https://indexer.drana.io:26680`

For local dev: `http://localhost:26657` and `http://localhost:26680`.

Both servers need CORS headers (`Access-Control-Allow-Origin: *`) added to their responses for browser requests to work. This is a small change to `rpc/server.go` and `indexer/api.go`.

---

## Directory Structure

```
web/
  package.json
  vite.config.ts
  tsconfig.json
  index.html
  src/
    main.tsx
    App.tsx
    api/
      nodeRpc.ts          — typed fetch wrappers for node RPC
      indexerApi.ts        — typed fetch wrappers for indexer API
      types.ts             — API response types (mirrors Go structs)
    wallet/
      crypto.ts            — Ed25519 signing, AES encrypt/decrypt
      signableBytes.ts     — transaction serialization (must match Go)
      WalletContext.tsx     — React context for wallet state
      WalletModals.tsx     — Create, Unlock, Export, Import modals
    pages/
      Feed.tsx             — main feed with sort/channel controls
      PostDetail.tsx       — single post + replies + boosts
      Channels.tsx         — channel directory
      Profile.tsx          — user profile
    components/
      TopBar.tsx           — persistent nav + wallet indicator
      PostCard.tsx         — single post row in the feed
      ReplyCard.tsx        — reply row in a thread
      BoostModal.tsx       — boost amount input + confirm
      NewPostModal.tsx     — text + channel + amount + submit
      ReplyModal.tsx       — reply text + amount + submit
    styles/
      global.css           — minimal, dark theme, monospace feel
```

---

## Visual Design

### Identity

DRANA's visual identity communicates: **precision, transparency, weight.** Every DRANA burned is visible. Every commitment is public. The UI makes this feel consequential, not casual.

The brand color is **electric amber** (`#F59E0B`) — the color of molten metal, of value being forged. It represents burn: capital entering the attention furnace.

### Color Palette

```
Background (primary):    #0A0A0F     — near-black with a blue undertone
Background (surface):    #12121A     — card/panel backgrounds
Background (elevated):   #1A1A26     — modals, dropdowns, hover states
Border:                  #2A2A3A     — subtle separators

Text (primary):          #E8E8ED     — high-contrast for post text
Text (secondary):        #8888A0     — metadata, timestamps, labels
Text (muted):            #555566     — disabled states, placeholders

Accent (amber):          #F59E0B     — committed DRANA amounts, primary actions
Accent (amber glow):     #F59E0B20   — subtle background glow behind high-value posts
Accent (amber hover):    #FBBF24     — button hover states

Success:                 #10B981     — confirmed transactions
Error:                   #EF4444     — rejected transactions, validation errors
Info:                    #6366F1     — epoch info, network stats

Channel tag:             #3B82F6     — blue, stands out against amber
Reply indicator:         #8B5CF6     — purple, visually distinct from posts
```

### Typography

```
Headings:       Inter, 600 weight    — clean, professional sans-serif
Body text:      Inter, 400 weight    — readable at density
Post text:      Inter, 500 weight    — slightly heavier to stand out as content
Numbers:        JetBrains Mono       — monospace for DRANA amounts, addresses, hashes
Small labels:   Inter, 500 weight, uppercase, letter-spacing 0.05em — "TRENDING", "REPLIES"
```

Load from Google Fonts: `Inter:400,500,600` and `JetBrains+Mono:500`.

### Layout Principles

- **Max width 720px**, centered. Posts are text — they don't need a wide canvas. A narrow column creates focus and density, like a terminal or a Bloomberg feed.
- **No sidebar.** Everything is vertical. Channel navigation is a horizontal pill bar above the feed, not a side panel.
- **Cards have no rounded corners.** Straight edges, 1px borders in `#2A2A3A`. This isn't playful — it's a ledger.
- **Generous vertical padding within cards** (16px), but **tight spacing between cards** (2px gap). The feed feels like a continuous stream with hairline separators.
- **The committed amount is the visual anchor.** It's the first thing on each card, left-aligned, in amber monospace, larger than the post text. The eye scans the numbers first, the text second. This is by design — it's an attention *market*, and the price is the primary signal.

### Component Specifications

#### Top Bar

```
Height: 56px
Background: #0A0A0F with bottom border #2A2A3A
Left: "DRANA" in amber (#F59E0B), Inter 600, 18px, letter-spacing 0.1em
Center: Nav links — "Feed" "Channels" in #8888A0, active link in #E8E8ED with amber underline
Right: Wallet pill — rounded-full, background #1A1A26, border #2A2A3A
  Locked: "Connect Wallet" in amber
  Unlocked: name + balance in amber monospace, 13px
```

#### Post Card

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  42.50 DRANA                                            2h ago      │
│                                                                     │
│  satoshi                                           #general         │
│  "The empire of relevance belongs to the highest bidder."           │
│                                                                     │
│  💬 12    👥 8                                  [Boost]  [Reply]    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

Amount:       JetBrains Mono 500, 20px, #F59E0B
Timestamp:    Inter 400, 13px, #555566, right-aligned on same line as amount
Author:       Inter 500, 14px, #E8E8ED, clickable (links to profile)
Channel:      Inter 500, 12px, #3B82F6, right-aligned on same line as author
Post text:    Inter 500, 15px, #E8E8ED, max 3 lines with ellipsis
Metrics:      Inter 400, 13px, #8888A0
Buttons:      Inter 500, 13px, #F59E0B, no background, subtle border on hover
Card:         Background #12121A, border-bottom 1px #2A2A3A, padding 16px 20px
```

For high-value posts (top 10% by committed amount in the current feed), add a **left amber border** (3px solid #F59E0B) and a faint amber glow background (`#F59E0B08`). This makes whales visually distinct without being garish.

#### Post Detail Page

The post itself is displayed larger:

```
Amount:       JetBrains Mono 500, 28px, #F59E0B
              Below: "35.0 by author · 7.5 by 8 others" in 13px #8888A0
Post text:    Inter 500, 18px, #E8E8ED, full text (no truncation)
```

Replies section has a small label: `REPLIES (12)` in uppercase, 12px, #8888A0, with a purple left-border accent on each reply card.

Boost history is a compact table:

```
  bob          3.00 DRANA     block 155    1h ago
  carol        2.50 DRANA     block 148    3h ago
```

JetBrains Mono, 13px, #8888A0 for names/times, #F59E0B for amounts.

#### Modals

```
Overlay:        #0A0A0F at 80% opacity, backdrop-blur 4px
Modal:          Background #12121A, border 1px #2A2A3A
                Max-width 420px, centered vertically and horizontally
                Padding 32px
Title:          Inter 600, 18px, #E8E8ED
Inputs:         Background #0A0A0F, border 1px #2A2A3A, padding 12px
                Text: Inter 400, 15px, #E8E8ED
                Placeholder: #555566
                Focus: border-color #F59E0B
Primary button: Background #F59E0B, text #0A0A0F, Inter 600, 14px
                Hover: #FBBF24
                Full width, height 44px, no border-radius
Warning text:   Inter 400, 13px, #EF4444 (for "this burns X DRANA permanently")
```

The "burns permanently" warning is always visible on post/boost modals. It's not a checkbox — it's a statement. The amber button against the dark background creates enough visual gravity.

#### Channel Pills

Horizontal scrollable row above the feed:

```
[All]  [general]  [politics]  [gaming]  [crypto]  [memes]

Active:   Background #F59E0B, text #0A0A0F
Inactive: Background #1A1A26, text #8888A0, border 1px #2A2A3A
Size:     Inter 500, 13px, padding 6px 14px, no border-radius
```

#### Empty States

When the feed is empty (fresh chain, new channel):

```
Center-aligned, 40% down the viewport:
"No posts yet." in Inter 400, 16px, #555566
[Create the first post] button in amber
```

When wallet is needed for an action:

```
"Connect your wallet to post." in #8888A0
[Create Wallet] in amber
```

#### Loading States

Skeleton cards: same dimensions as real cards, background animated between `#12121A` and `#1A1A26` (subtle pulse). No spinners. The feed should feel like it's *resolving*, not *loading*.

### Responsive Behavior

- **Desktop (>768px):** 720px max-width, centered. This is the primary experience.
- **Mobile (<768px):** Full width with 12px horizontal padding. Top bar collapses: nav links become a hamburger, wallet pill shrinks to just the balance. Post cards stack normally — the single-column layout works naturally on mobile.
- **No tablet-specific layout.** The 720px column works fine on tablets.

### Dark Mode Only

There is no light mode. The product is a dark terminal for attention trading. A light mode would undermine the aesthetic identity. This is a deliberate choice, not laziness.

### Animation

Minimal:
- Modal open: fade in 150ms + translate Y from 8px to 0
- Modal close: fade out 100ms
- Button hover: background transition 100ms
- New post appearing in feed: subtle fade in 200ms
- Skeleton pulse: 1.5s ease-in-out infinite

No page transitions. No parallax. No confetti on successful transactions. The UI should feel **immediate and consequential**, like a trading terminal.

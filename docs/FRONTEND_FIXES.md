# FRONTEND_FIXES.md

## Wallet UX Improvements

Three issues with the current wallet experience:

1. **Can't create a second wallet** — once a wallet exists, the only option is "Unlock". No way to create another wallet without clearing localStorage.
2. **Locked out with no escape** — if you forget your password, you're stuck. No "use a different wallet" or "start over" option.
3. **No username integration** — the wallet has no awareness of on-chain name registration. You can't register a name from the UI, and posts show truncated addresses.

---

## Fix 1: Multi-Wallet Support

### Problem

`storage.ts` stores a single wallet under one localStorage key (`drana_wallet`). Creating a new wallet overwrites the old one.

### Solution

Change the storage model from a single wallet to a wallet list with an active wallet pointer.

**`storage.ts` — new model:**

```typescript
const WALLETS_KEY = 'drana_wallets';       // array of StoredWallet
const ACTIVE_KEY = 'drana_active_wallet';  // address of the active wallet

export function saveWallet(wallet: StoredWallet): void
  // Appends to the array if new address, replaces if existing address.

export function loadAllWallets(): StoredWallet[]
  // Returns all stored wallets.

export function loadActiveWallet(): StoredWallet | null
  // Returns the wallet whose address matches ACTIVE_KEY.

export function setActiveWallet(address: string): void
  // Sets which wallet is active.

export function removeWallet(address: string): void
  // Removes a specific wallet from the array.
```

**Migration:** On first load, if the old `drana_wallet` key exists, migrate it to the new `drana_wallets` array format and set it as active.

### UI Changes

**TopBar locked state** — currently shows just "Unlock". Change to show the truncated address with a dropdown:

```
┌──────────────────────────────────┐
│  drana1abc...def   [Unlock ▼]    │
└──────────────────────────────────┘
        │
        ├── Unlock this wallet
        ├── Switch wallet →
        │     ├── drana1xyz... (alice)
        │     ├── drana1qrs... (bob)
        │     └── + Create new wallet
        ├── Import wallet
        └── Remove this wallet
```

**TopBar no-wallet state** — currently shows "Create Wallet". Change to:

```
┌────────────────────────────────────┐
│  [Connect ▼]                       │
└────────────────────────────────────┘
        │
        ├── Create new wallet
        ├── Import wallet
        └── (list of existing wallets if any)
```

This way you always have access to create, import, and switch — regardless of wallet state.

### WalletContext changes

```typescript
type WalletContextType = {
  // ... existing fields ...
  allWallets: StoredWallet[];
  switchWallet: (address: string) => void;
  removeWallet: (address: string) => void;
}
```

`switchWallet` locks the current wallet, sets the new one as active, and clears the decrypted key. The user must enter the password for the new wallet.

`removeWallet` removes from storage. If it's the active wallet, clears active state.

### Files changed

- `src/wallet/storage.ts` — rewrite from single-wallet to wallet-list model
- `src/wallet/WalletContext.tsx` — add `allWallets`, `switchWallet`, `removeWallet`
- `src/components/TopBar.tsx` — dropdown in all states (no wallet, locked, unlocked)
- `src/wallet/WalletModals.tsx` — add `SwitchWalletModal` with wallet list, add "Remove wallet" confirmation

---

## Fix 2: Password Recovery / Reset Path

### Problem

If you forget your password, the encrypted key is unrecoverable. The only option is to manually clear localStorage in browser dev tools.

### Solution

There is no way to recover the key — that's by design (the password is the encryption key). But the UX should make it obvious what your options are and not trap you.

**Add to the Unlock modal:**

Below the password field, add a "Forgot password?" link that expands to:

```
⚠ Your password cannot be recovered. Without it, this wallet's
  private key is inaccessible.

  If you have a backup of your private key (exported hex), you can
  import it into a new wallet with a new password.

  [Import from backup]    [Create a different wallet]    [Remove this wallet]
```

**"Remove this wallet"** shows a confirmation: "This will permanently remove this wallet from your browser. If you haven't exported the private key, the funds in this wallet will be lost forever. Type the address to confirm: [________]"

This gives the user a clear escape path without hiding the consequences.

### Files changed

- `src/wallet/WalletModals.tsx` — add forgot-password section to `UnlockWalletModal`
- `src/wallet/WalletModals.tsx` — add `RemoveWalletModal` with address-confirmation

---

## Fix 3: Username Registration from the UI

### Problem

On-chain names exist (`RegisterName` tx type), the CLI supports them (`drana-cli register-name`), and the backend returns them in account responses. But the web frontend has no way to register a name. Posts show truncated hex addresses, even though the author may want a human-readable name.

### Solution

**Add "Register Name" to the wallet dropdown** (both locked and unlocked states, but only actionable when unlocked):

```
Wallet dropdown:
  ├── Send DRANA
  ├── Register Name          ← new
  ├── Export Private Key
  ├── Import Wallet
  ├── Lock Wallet
  └── Faucet
```

**RegisterNameModal:**

```
┌──────────────────────────────────────┐
│  Register Name                       │
│                                      │
│  Choose a permanent username for     │
│  your account. This cannot be        │
│  changed later.                      │
│                                      │
│  Name: [satoshi          ]           │
│  3-20 chars, a-z, 0-9, underscore    │
│                                      │
│  [Register]                          │
│                                      │
│  ⚠ Names are permanent and unique.  │
│  Choose carefully.                   │
└──────────────────────────────────────┘
```

On submit: construct a `RegisterName` tx (type 4, amount 0, text = name), sign, submit. On success, update the cached `name` in storage so the TopBar immediately shows the new name without waiting for the next balance poll.

**Client-side validation** (before submitting):
- 3-20 characters
- Only `[a-z0-9_]`
- No leading/trailing underscore
- No consecutive underscores
- Same rules as Go's `ValidateName()`

**Post display enhancement:**

When rendering post authors, the indexer and node RPC already return the author address. The frontend should:
1. Check if the author address has a name (from the account query).
2. If yes, display the name. If no, display the truncated address.

Since querying every author on every post would be expensive, use a simple name cache:
- When a post is displayed, if the author isn't in the cache, fetch `getAccount(author)` once and cache the name.
- The cache lives in React context or a simple Map — no persistence needed.
- The PostCard component shows the name if available, truncated address if not.

### Files changed

- `src/components/TopBar.tsx` — add "Register Name" to dropdown
- `src/wallet/WalletModals.tsx` — add `RegisterNameModal`
- `src/wallet/WalletContext.tsx` — add `registerName` action, name cache
- `src/components/PostCard.tsx` — display resolved name instead of truncated address
- `src/pages/PostDetail.tsx` — same name resolution for post author and reply authors

---

## Implementation Order

1. **Fix 1 (multi-wallet)** first — it changes the storage model, which the other fixes depend on.
2. **Fix 2 (password recovery UX)** second — builds on the multi-wallet dropdown.
3. **Fix 3 (username)** third — independent of storage model but uses the wallet context.

All three are frontend-only changes. No backend modifications needed.

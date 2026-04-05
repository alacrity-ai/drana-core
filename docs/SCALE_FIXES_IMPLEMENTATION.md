# SCALE_FIXES_IMPLEMENTATION.md

## Two Problems at Scale

### Problem 1: JavaScript Number Precision

JavaScript's `Number` type is a 64-bit IEEE 754 float. It can only represent integers exactly up to `2^53 - 1` (9,007,199,254,740,991). All microdrana amounts come from Go as `uint64` and are serialized as JSON numbers. When JavaScript parses them, any value above ~9 million DRANA silently loses precision.

This affects:
- Displaying balances, stakes, and rewards
- Signing transactions (the `writeUint64` helper in `signableBytes.ts` produces wrong bytes above 2^53, causing signature verification to fail)
- Transaction submission (the `amount` field sent to the node RPC is a JSON number)

### Problem 2: Intermediate Multiplication Overflow in Go

Several places compute `a * b / c` where all three are `uint64`. If `a * b` exceeds `2^64`, it silently wraps to zero or a wrong value. The division happens too late — the intermediate product is already corrupted.

Affected code (executor is consensus-critical; indexer mirrors it):

| File | Line | Expression | Overflows when |
|------|------|------------|----------------|
| `executor.go` | 77 | `tx.Amount * feePercent / 100` | amount > 1.8e18 (~1.8T DRANA) |
| `executor.go` | 127-129 | `tx.Amount * burnPct / 100` (×3) | amount > 1.8e18 |
| `executor.go` | 154 | `stakerReward * s.Amount / post.TotalStaked` | reward × stake > 2^64 |
| `executor.go` | 279 | `totalAtRisk * slashFraction / 100` | risk > 1.8e18 |
| `proposer.go` | 48 | `(total * 2) / 3 + 1` | total stake > 2^63 |
| `follower.go` | 116, 148-150, 184 | Same formulas as executor | Same thresholds |

---

## Step 1 — Safe Math Library

### File: `internal/types/safemath.go` (new)

Add a `MulDiv` function that computes `a * b / c` without intermediate overflow using 128-bit arithmetic via `math/bits.Mul64`:

```go
package types

import "math/bits"

// MulDiv computes (a * b) / c without intermediate overflow.
// Uses 128-bit intermediate product via math/bits.Mul64.
// Panics if c is zero.
func MulDiv(a, b, c uint64) uint64 {
    hi, lo := bits.Mul64(a, b)
    if hi == 0 {
        return lo / c
    }
    // 128-bit division: (hi:lo) / c
    // Use the standard shift-subtract algorithm.
    return div128by64(hi, lo, c)
}

// div128by64 divides a 128-bit number (hi:lo) by a 64-bit divisor.
func div128by64(hi, lo, d uint64) uint64 {
    // If hi >= d, the result overflows uint64.
    // In our use case this shouldn't happen (we're computing percentages
    // and pro-rata shares), but clamp to max uint64 as a safety net.
    if hi >= d {
        return ^uint64(0) // MaxUint64
    }
    // Use bits.Div64 which computes (hi:lo) / d when hi < d.
    q, _ := bits.Div64(hi, lo, d)
    return q
}
```

### Acceptance Criteria

- `MulDiv(3_000_000_000, 3, 100)` returns `90_000_000` (no overflow).
- `MulDiv(maxUint64, 1, 1)` returns `maxUint64`.
- `MulDiv(1e18, 1e18, 1e18)` returns `1e18` (not 0 from wraparound).
- Unit tests cover edge cases: zero amounts, large products, rounding.

---

## Step 2 — Replace Unsafe Arithmetic in Executor

### File: `internal/state/executor.go`

Replace all `a * b / c` patterns with `types.MulDiv(a, b, c)`.

**applyCreatePost (line 77):**

```go
// Before:
fee := tx.Amount * feePercent / 100

// After:
fee := types.MulDiv(tx.Amount, feePercent, 100)
```

**applyBoostPost (lines 127-129, 154):**

```go
// Before:
burnAmount := tx.Amount * burnPct / 100
authorReward := tx.Amount * authorPct / 100
stakerReward := tx.Amount * stakerPct / 100
...
share := stakerReward * s.Amount / post.TotalStaked

// After:
burnAmount := types.MulDiv(tx.Amount, burnPct, 100)
authorReward := types.MulDiv(tx.Amount, authorPct, 100)
stakerReward := types.MulDiv(tx.Amount, stakerPct, 100)
...
share := types.MulDiv(stakerReward, s.Amount, post.TotalStaked)
```

**applySlash (line 279):**

```go
// Before:
slashAmount := totalAtRisk * e.Params.SlashFractionDoubleSign / 100

// After:
slashAmount := types.MulDiv(totalAtRisk, e.Params.SlashFractionDoubleSign, 100)
```

### File: `internal/consensus/proposer.go`

**QuorumThreshold (line 48):**

```go
// Before:
return (total*2)/3 + 1

// After:
return types.MulDiv(total, 2, 3) + 1
```

### Acceptance Criteria

- All `a * b / c` patterns in executor and consensus use `MulDiv`.
- Existing tests continue to pass (results are identical for small values).
- A 10 trillion DRANA boost computes correct fee breakdown without overflow.

---

## Step 3 — Replace Unsafe Arithmetic in Indexer

### File: `internal/indexer/follower.go`

Mirror the executor changes. The indexer must produce the same results.

```go
// Before:
fee := tx.Amount * f.postFeePercent / 100
burnAmount := tx.Amount * f.boostBurnPct / 100
authorReward := tx.Amount * f.boostAuthorPct / 100
stakerReward := tx.Amount * f.boostStakerPct / 100
share := stakerReward * s.Amount / totalStaked

// After:
fee := types.MulDiv(tx.Amount, f.postFeePercent, 100)
burnAmount := types.MulDiv(tx.Amount, f.boostBurnPct, 100)
authorReward := types.MulDiv(tx.Amount, f.boostAuthorPct, 100)
stakerReward := types.MulDiv(tx.Amount, f.boostStakerPct, 100)
share := types.MulDiv(stakerReward, s.Amount, totalStaked)
```

### Acceptance Criteria

- Indexer produces identical results to executor for the same inputs.
- No `a * b / c` patterns remain in follower.go.

---

## Step 4 — Serialize uint64 as JSON Strings

Go's `json:",string"` struct tag causes `encoding/json` to serialize a number as a quoted string (`"3000000000"` instead of `3000000000`). JavaScript can parse these as strings and convert to `BigInt` without precision loss.

### File: `internal/rpc/types.go`

Add `json:",string"` to every `uint64` field in the RPC response types.

Example for `AccountResponse`:

```go
// Before:
type AccountResponse struct {
    Address          string `json:"address"`
    Balance          uint64 `json:"balance"`
    Nonce            uint64 `json:"nonce"`
    ...
}

// After:
type AccountResponse struct {
    Address          string `json:"address"`
    Balance          uint64 `json:"balance,string"`
    Nonce            uint64 `json:"nonce,string"`
    ...
}
```

Apply to all response types: `NodeInfoResponse`, `BlockResponse`, `AccountResponse`, `PostResponse`, `TransactionResponse`, `TxStatusResponse`, `ValidatorResponse`, `UnbondingEntryResponse`, `UnbondingResponse`.

**Also apply to `SubmitTxRequest`** — the frontend sends `amount` and `nonce` as JSON strings, and Go unmarshals them back with `json:",string"`.

### File: `internal/types/json.go`

Update `transactionJSON` and `blockHeaderJSON` to use `json:",string"` on their uint64 fields:

```go
type transactionJSON struct {
    ...
    Amount    uint64 `json:"amount,string"`
    Nonce     uint64 `json:"nonce,string"`
    ...
}

type blockHeaderJSON struct {
    Height    uint64 `json:"height,string"`
    Timestamp int64  `json:"timestamp,string"`
    ...
}
```

### File: `internal/indexer/types.go`

Same treatment: add `json:",string"` to every `uint64` field in `IndexedPost`, `IndexedBoost`, `RewardEvent`, `PostStakeRecord`, `IndexedTransfer`, `ChainStats`, `AuthorProfile`, `LeaderboardEntry`.

### File: `internal/indexer/api.go`

The API handlers that return `map[string]interface{}` with uint64 values (reward endpoints, reply list) also need attention. These inline maps bypass struct tags. Either:
- Define proper response structs with `json:",string"` tags, or
- Wrap uint64 values manually: `strconv.FormatUint(v, 10)` and use string map values.

The cleaner approach: define response structs.

### Acceptance Criteria

- `GET /v1/accounts/{addr}` returns `{"balance":"950000000",...}` (quoted).
- `POST /v1/transactions` accepts `{"amount":"3000000000",...}` (quoted).
- All uint64 fields across both RPC and indexer APIs are string-encoded.
- Existing integration tests are updated to expect strings.

---

## Step 5 — Frontend: Parse String Amounts

### Approach

All amount fields from the API are now JSON strings. The frontend must:
1. Parse them correctly (as `number` if safe, or as `bigint`/string for display).
2. Send them as strings in transaction requests.

Since JavaScript's `Number` is safe up to 9 quadrillion microdrana (~9 billion DRANA), and realistic chain usage won't approach that for a very long time, a pragmatic approach is:
- Parse string amounts into `number` using `Number()` or `parseInt()`.
- Keep all existing display/math logic using `number`.
- Send amounts as strings in API requests.

This means the immediate code change is minimal — we just need to handle the fact that JSON fields are now strings instead of numbers.

If the chain ever approaches billions of DRANA in a single value, a `BigInt` migration would be needed, but that's far future.

### File: `drana-app/src/api/types.ts`

No type changes needed — TypeScript `number` is still the runtime type. The JSON deserializer will coerce `"123"` to `123` automatically when assigned to a `number` field... except it won't. `JSON.parse('{"x":"123"}')` gives `{x: "123"}` (a string), not a number.

We need a transform layer. Two options:

**Option A: Transform in the API layer (recommended)**

Add a response transformer to the `get()` helper that recursively converts string-encoded numbers back to JS numbers:

```typescript
// drana-app/src/api/numericReviver.ts
export function numericReviver(_key: string, value: unknown): unknown {
  if (typeof value === 'string' && /^\d+$/.test(value)) {
    const n = Number(value);
    // Only convert if it's within safe integer range
    if (Number.isSafeInteger(n)) return n;
    // For values above MAX_SAFE_INTEGER, keep as string.
    // Display functions will need to handle string amounts.
    return value;
  }
  return value;
}
```

Then in `indexerApi.ts` and `nodeRpc.ts`, use it in the JSON parse:

```typescript
async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${API}${path}`);
  if (!res.ok) { ... }
  const text = await res.text();
  return JSON.parse(text, numericReviver);
}
```

**Option B: Change type definitions to `string | number`** — more invasive, not recommended for now.

### File: `drana-app/src/api/indexerApi.ts`

Update `get()` to use the reviver.

### File: `drana-app/src/api/nodeRpc.ts`

Update all `fetch` + `json()` calls to use `text()` + `JSON.parse(text, numericReviver)`.

### File: `drana-app/src/wallet/WalletContext.tsx`

Update `signAndSubmit` to send amounts as strings:

```typescript
const resp = await submitTransaction({
  ...
  amount: String(fullTx.amount),  // send as string
  nonce: String(fullTx.nonce),    // send as string
  ...
});
```

### File: `drana-app/src/api/types.ts`

Update `SubmitTxRequest` to use `string` for amount and nonce:

```typescript
export interface SubmitTxRequest {
  ...
  amount: string;  // was: number — now stringified uint64
  nonce: string;   // was: number — now stringified uint64
  ...
}
```

### File: `drana-app/src/wallet/signableBytes.ts`

The `writeUint64` function uses `Math.floor(v / 0x100000000)` which loses precision above 2^53. Replace with `BigInt`-based encoding:

```typescript
function writeUint64(v: number | bigint): Uint8Array {
  const buf = new Uint8Array(8);
  const dv = new DataView(buf.buffer);
  const big = typeof v === 'bigint' ? v : BigInt(v);
  dv.setBigUint64(0, big, false); // big-endian
  return buf;
}
```

This is safe because `DataView.setBigUint64` is widely supported (all modern browsers, Node 10.3+).

### Acceptance Criteria

- Frontend correctly displays balances and amounts from string-encoded JSON.
- Transaction submission sends amount/nonce as JSON strings.
- Signature computation uses `BigInt` for `writeUint64`, producing correct signatures for any uint64 value.
- Values above `MAX_SAFE_INTEGER` are preserved as strings in the UI (formatted for display rather than computed on).

---

## Step 6 — Update OpenAPI Spec and API Reference

### File: `internal/rpc/openapi.yaml`

Change all `format: uint64` integer fields to `type: string` with a description noting they are stringified uint64 values:

```yaml
# Before:
balance: { type: integer, format: uint64 }

# After:
balance: { type: string, description: "uint64 as string" }
```

### File: `internal/indexer/openapi.yaml`

Same treatment for all uint64 fields.

### File: `docs/API_REFERENCE.md`

Update example responses to show quoted numbers:

```json
{
  "balance": "950000000",
  "nonce": "5",
  "stakedBalance": "1000000000"
}
```

Add a note at the top:

> All uint64 values (amounts, heights, timestamps, nonces) are serialized as **JSON strings** to avoid JavaScript floating-point precision loss. Parse them as `BigInt` or numeric strings on the client side.

### Acceptance Criteria

- OpenAPI specs reflect string-encoded uint64 fields.
- API reference examples show quoted numbers.
- Breaking change is documented.

---

## Files Modified Summary

| Step | Files | Change |
|------|-------|--------|
| 1 | `types/safemath.go` (new), `types/safemath_test.go` (new) | MulDiv with 128-bit intermediate |
| 2 | `state/executor.go`, `consensus/proposer.go` | Replace `a*b/c` with `MulDiv` |
| 3 | `indexer/follower.go` | Same as step 2 for indexer |
| 4 | `rpc/types.go`, `types/json.go`, `indexer/types.go`, `indexer/api.go` | `json:",string"` on all uint64 fields |
| 5 | `drana-app/src/api/*.ts`, `wallet/WalletContext.tsx`, `wallet/signableBytes.ts` | String parsing, BigInt writeUint64 |
| 6 | `openapi.yaml` (×2), `docs/API_REFERENCE.md` | Doc updates |

---

## Migration / Breaking Changes

This is a **breaking API change**. All API consumers (frontend, CLI, any third-party integrations) must update to handle string-encoded uint64 values.

- **Frontend:** Updated in Step 5 (deployed together with backend).
- **CLI (`drana-cli`):** Uses Go types directly, not JSON parsing — no change needed.
- **Third-party consumers:** Must update JSON parsing. The OpenAPI spec change (Step 6) documents this.

The Go `MulDiv` change (Steps 1-3) is **consensus-compatible** — it only changes behavior for values that previously overflowed (which would have produced wrong results anyway). For all values that fit in the original `a*b` product, `MulDiv` produces the same result.

---

## Verification

After implementation, verify with a test scenario:

1. Fund an account with 10 billion DRANA (10,000,000,000,000,000 microdrana).
2. Create a post with 5 billion DRANA stake.
3. Boost the post with 5 billion DRANA from another account.
4. Verify: fee breakdown is correct (no overflow), reward events are correct.
5. Verify: frontend displays all values correctly (no truncation, no NaN).
6. Verify: the signing and submission flow works for the large amounts.

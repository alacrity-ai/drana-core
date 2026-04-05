# POS_ENHANCEMENT_IMPLEMENTATION.md

## Proof-of-Stake Implementation Steps

### Overview

This document specifies the numbered implementation steps for transitioning DRANA from permissioned BFT to open proof-of-stake consensus, as designed in `POS_ENHANCEMENT_DESIGN.md`.

The changes span every layer of the stack: types, state, validation, executor, persistence, consensus, P2P, RPC, CLI, and indexer. The steps are ordered by dependency.

### Guiding Principle

The existing consensus flow (propose → vote → QC → finalize) is preserved. What changes is:
- Where the validator set comes from (state instead of genesis)
- How proposer selection works (stake-weighted instead of equal-rotation)
- How quorum is measured (stake-weighted instead of count-based)
- Two new tx types (`Stake`, `Unstake`)
- Epoch-based validator set transitions
- Slash evidence processing

---

## Step 1 — Types: Staking Primitives

### Files modified

**`internal/types/account.go`**

Add `StakedBalance`:

```go
type Account struct {
    Address       crypto.Address
    Balance       uint64  // spendable microdrana
    Nonce         uint64
    Name          string
    StakedBalance uint64  // locked microdrana
}
```

**`internal/types/transaction.go`**

Add tx type constants:

```go
const (
    TxTransfer     TxType = 1
    TxCreatePost   TxType = 2
    TxBoostPost    TxType = 3
    TxRegisterName TxType = 4
    TxStake        TxType = 5
    TxUnstake      TxType = 6
)
```

Both `Stake` and `Unstake` use the `Amount` field. No new fields needed on `Transaction`.

**`internal/types/staking.go`** (new)

```go
type ValidatorStake struct {
    Address       crypto.Address
    PubKey        crypto.PublicKey
    StakedBalance uint64
}

type UnbondingEntry struct {
    Address       crypto.Address
    Amount        uint64
    ReleaseHeight uint64
}

type SlashEvidence struct {
    VoteA types.BlockVote
    VoteB types.BlockVote
}
```

Add `SignableBytes` and verification for `SlashEvidence`:
- Checks that both votes have the same `VoterAddr` and `Height` but different `BlockHash`.
- Verifies both signatures.

**`internal/types/block.go`**

Add `Evidence` field to `Block`:

```go
type Block struct {
    Header       BlockHeader
    Transactions []*Transaction
    Evidence     []SlashEvidence
    QC           *QuorumCertificate
}
```

Evidence is **not** included in `BlockHeader.Hash()` (same principle as QC — it's attached alongside the block, not part of the header that gets voted on). However, evidence **is** included in the state transition, so the resulting `StateRoot` reflects it.

**`internal/types/genesis.go`**

Add PoS parameters to `GenesisConfig`:

```go
type GenesisConfig struct {
    // ... existing fields ...
    MinStake                 uint64  // microdrana (1000 DRANA = 1_000_000_000)
    EpochLength              uint64  // blocks per epoch (30)
    UnbondingPeriod          uint64  // blocks (30)
    SlashFractionDoubleSign  uint64  // percent (5)
}
```

Add `Stake` field to `GenesisValidator`:

```go
type GenesisValidator struct {
    Address crypto.Address
    PubKey  crypto.PublicKey
    Name    string
    Stake   uint64  // initial stake in microdrana
}
```

### Acceptance Criteria

- All types compile.
- `SlashEvidence` verification correctly identifies valid/invalid evidence.
- Existing tests still pass (new fields default to zero).

---

## Step 2 — State: Validator Set, Unbonding Queue, Epochs

### Files modified

**`internal/state/state.go`**

Add to `WorldState`:

```go
type WorldState struct {
    // ... existing fields ...
    activeValidators []types.ValidatorStake    // frozen for current epoch
    unbondingQueue   []types.UnbondingEntry
    slashRecord      map[slashKey]bool         // (addr, height) -> slashed
    currentEpoch     uint64
}
```

New methods:

```go
func (ws *WorldState) GetActiveValidators() []types.ValidatorStake
func (ws *WorldState) SetActiveValidators(vs []types.ValidatorStake)
func (ws *WorldState) GetUnbondingQueue() []types.UnbondingEntry
func (ws *WorldState) AddUnbondingEntry(e types.UnbondingEntry)
func (ws *WorldState) RemoveMaturedUnbonding(currentHeight uint64) []types.UnbondingEntry
func (ws *WorldState) HasBeenSlashed(addr crypto.Address, height uint64) bool
func (ws *WorldState) RecordSlash(addr crypto.Address, height uint64)
func (ws *WorldState) GetCurrentEpoch() uint64
func (ws *WorldState) SetCurrentEpoch(epoch uint64)
func (ws *WorldState) ComputeActiveValidatorSet(minStake uint64) []types.ValidatorStake
func (ws *WorldState) TotalActiveStake() uint64
```

`ComputeActiveValidatorSet` scans all accounts, collects those with `StakedBalance >= minStake`, sorts by address, and returns the new set. This is called at epoch boundaries.

Update `Clone()` to deep-copy all new fields.

**`internal/state/stateroot.go`**

Add active validators, unbonding queue, and epoch to the state root hash:

```go
// After accounts and posts:
hw.WriteUint64(ws.GetCurrentEpoch())
hw.WriteUint64(uint64(len(activeValidators)))
for _, v := range activeValidators {  // already sorted by address
    hw.WriteBytes(v.Address[:])
    hw.WriteBytes(v.PubKey[:])
    hw.WriteUint64(v.StakedBalance)
}
// Unbonding queue (sorted by releaseHeight, then address)
hw.WriteUint64(uint64(len(unbondingQueue)))
for _, u := range unbondingQueue {
    hw.WriteBytes(u.Address[:])
    hw.WriteUint64(u.Amount)
    hw.WriteUint64(u.ReleaseHeight)
}
```

The `Account` hash already includes `StakedBalance` (added in Step 1 via the Account struct change, and the existing hash loop iterates account fields).

### Acceptance Criteria

- WorldState correctly stores and retrieves validators, unbonding entries, and epoch.
- `ComputeActiveValidatorSet` produces a deterministic sorted set.
- Clone is independent — mutating the clone does not affect the original.
- State root changes when any staking-related field changes.

---

## Step 3 — Validation: Stake and Unstake Rules

### Files modified

**`internal/validation/validate.go`**

Add `Stake` and `Unstake` validation to `ValidateTransaction`:

```go
case types.TxStake:
    return validateStake(tx, sr, params)
case types.TxUnstake:
    return validateUnstake(tx, sr, params)
```

**`validateStake`:**

1. Amount > 0.
2. Standard signature, nonce, and balance checks (via `validateCommon`).
3. If this is the sender's first stake (`account.StakedBalance == 0`): `amount >= params.MinStake`.
4. If the sender already has stake: any positive amount is valid (adding to existing).
5. Sender's spendable balance >= amount.

**`validateUnstake`:**

1. Amount > 0.
2. Standard signature and nonce checks.
3. Sender's `StakedBalance >= amount`.
4. After unstaking, remaining `StakedBalance` is either 0 or >= `MinStake` (no partial stake below minimum).
5. Amount field on the tx is 0 for balance-check purposes (unstaking doesn't spend spendable balance). The actual amount to unstake is carried in a new field or reuses `Amount`.

Note: `validateCommon` currently checks `acct.Balance < tx.Amount`. For `Unstake`, the amount comes from staked balance, not spendable balance. The common validation needs a small adjustment: skip the balance-sufficiency check for Unstake and handle it in `validateUnstake` directly.

### Acceptance Criteria

- Valid stake passes, invalid stake (below minimum, insufficient balance) fails.
- Valid unstake passes, invalid unstake (insufficient staked balance, remainder below minimum) fails.
- All existing tx type validations still pass unchanged.

---

## Step 4 — Executor: Stake, Unstake, Unbonding Release, Epoch Transition, Slashing

### Files modified

**`internal/state/executor.go`**

Add to `ApplyTransaction`:

```go
case types.TxStake:
    return e.applyStake(ws, tx)
case types.TxUnstake:
    return e.applyUnstake(ws, tx, blockHeight)
```

**`applyStake`:**

```go
sender.Balance -= tx.Amount
sender.StakedBalance += tx.Amount
sender.Nonce++
ws.SetAccount(sender)
```

**`applyUnstake`:**

```go
sender.StakedBalance -= tx.Amount
sender.Nonce++
ws.SetAccount(sender)
ws.AddUnbondingEntry(types.UnbondingEntry{
    Address:       tx.Sender,
    Amount:        tx.Amount,
    ReleaseHeight: blockHeight + params.UnbondingPeriod,
})
```

**Update `ApplyBlock`:**

The block execution order becomes:

1. **Process unbonding releases** — before transactions, release any matured unbonding entries:
   ```go
   released := clone.RemoveMaturedUnbonding(block.Header.Height)
   for _, entry := range released {
       acct := clone.GetAccount(entry.Address)
       acct.Balance += entry.Amount
       clone.SetAccount(acct)
   }
   ```

2. **Process slash evidence** — before transactions:
   ```go
   for _, ev := range block.Evidence {
       e.applySlashEvidence(clone, &ev)
   }
   ```

3. **Apply transactions** — unchanged.

4. **Issue block reward** — unchanged (proposer gets reward to spendable balance).

5. **Epoch transition** — if this block is an epoch boundary:
   ```go
   if block.Header.Height > 0 && block.Header.Height % params.EpochLength == 0 {
       newSet := clone.ComputeActiveValidatorSet(params.MinStake)
       clone.SetActiveValidators(newSet)
       clone.SetCurrentEpoch(block.Header.Height / params.EpochLength)
   }
   ```

6. **Set chain height** — unchanged.

**`applySlashEvidence`:**

```go
func (e *Executor) applySlashEvidence(ws *WorldState, ev *types.SlashEvidence) {
    // Verify evidence.
    if !ev.IsValid() { return }
    addr := ev.VoteA.VoterAddr
    if ws.HasBeenSlashed(addr, ev.VoteA.Height) { return }

    acct, ok := ws.GetAccount(addr)
    if !ok { return }

    // Compute slash amount: 5% of (staked + unbonding).
    totalAtRisk := acct.StakedBalance + ws.UnbondingBalanceFor(addr)
    slashAmount := totalAtRisk * params.SlashFractionDoubleSign / 100

    // Slash from staked balance first.
    slashedFromStake := min(slashAmount, acct.StakedBalance)
    acct.StakedBalance -= slashedFromStake
    remaining := slashAmount - slashedFromStake

    // Slash from unbonding entries if needed.
    if remaining > 0 {
        ws.SlashUnbonding(addr, remaining)
    }

    ws.SetAccount(acct)
    ws.RecordSlash(addr, ev.VoteA.Height)
    ws.SetBurnedSupply(ws.GetBurnedSupply() + slashAmount)
}
```

### Acceptance Criteria

- `applyStake` moves balance from spendable to staked, increments nonce.
- `applyUnstake` moves balance from staked to unbonding queue.
- Unbonding entries release automatically at the correct height.
- Epoch boundary triggers validator set recomputation.
- Slash evidence burns the correct amount and records the slash.
- Supply conservation holds: `spendable + staked + unbonding = genesis + issued - burned`.
- All existing block execution tests still pass.

---

## Step 5 — Consensus: Stake-Weighted Proposer Selection and Quorum

### Files modified

**`internal/consensus/proposer.go`**

Replace the current equal-weight round-robin with stake-weighted selection:

```go
func ProposerForHeight(validators []types.ValidatorStake, height uint64) types.ValidatorStake
```

Algorithm (from POS_ENHANCEMENT_DESIGN.md section 8):
1. Validators are sorted by address (caller ensures this).
2. Compute `totalStake = sum(v.StakedBalance)`.
3. `slot = height % totalStake`.
4. Walk validators in order, accumulating stake. Return the validator whose cumulative range covers `slot`.

Update `IsProposer` to match the new signature.

**`internal/consensus/validator.go`**

**`ValidateProposedBlock`:** Change the `validators` parameter from `[]types.GenesisValidator` to `[]types.ValidatorStake`. The proposer identity check uses the new `ProposerForHeight`.

**`ValidateQuorumCertificate`:** Change to stake-weighted quorum:

```go
func ValidateQuorumCertificate(
    qc *types.QuorumCertificate,
    blockHash [32]byte,
    validators []types.ValidatorStake,
) error
```

- Build a map of `address -> stake` from the active set.
- Sum the stake behind valid unique votes.
- Require `voteStakeSum >= (totalStake * 2 / 3) + 1`.

**`internal/consensus/engine.go`**

Replace all references to `e.Validators` (type `[]types.GenesisValidator`) with the active validator set from state:

```go
// Instead of e.Validators, use:
activeSet := e.State.GetActiveValidators()
```

This means the engine no longer holds a static validator list. It reads the current active set from world state, which updates at epoch boundaries.

The engine's `Run` loop, `proposeBlock`, `OnProposal`, `OnFinalizedBlock`, and `SyncToNetwork` all need to use `state.GetActiveValidators()` instead of `e.Validators`.

### Acceptance Criteria

- Stake-weighted proposer selection is deterministic.
- A validator with 2x stake proposes ~2x as often over a long sequence.
- Quorum requires 2/3 of total staked weight, not count.
- All consensus validation tests updated and passing.

---

## Step 6 — Persistence: Staking State

### Files modified

**`internal/store/kvstore.go`**

Update `encodeAccount` / `decodeAccount` to include `StakedBalance`.

Add new key prefixes and encode/decode for:
- `meta:active_validators` → serialized `[]ValidatorStake`
- `meta:unbonding_queue` → serialized `[]UnbondingEntry`
- `meta:current_epoch` → uint64
- `meta:slash_record` → serialized set of `(addr, height)` pairs

Update `SaveState` and `LoadState` to persist and restore all staking state.

### Acceptance Criteria

- State with staking data survives restart.
- Active validator set, unbonding queue, and epoch are correctly restored.
- Existing state without staking fields loads cleanly (backward compatible).

---

## Step 7 — Genesis: Initial Stakers

### Files modified

**`internal/genesis/genesis.go`**

Update `genesisValidJSON` to include `Stake`:

```go
type genesisValidJSON struct {
    Address string `json:"address"`
    PubKey  string `json:"pubKey"`
    Name    string `json:"name"`
    Stake   uint64 `json:"stake"`
}
```

Update `InitializeState`:
- For each genesis validator with a `Stake` value:
  - Set `account.StakedBalance = stake`
  - Deduct from `account.Balance` (or the stake is a separate allocation on top of balance)
- Compute the initial active validator set from genesis stakers.
- Set `currentEpoch = 0`.

**`scripts/gen-testnet.sh`**

Add `"stake": 1000000000` (1000 DRANA) to each genesis validator entry. Ensure each validator's genesis balance is enough to cover the stake.

**`internal/genesis/gen_testnet/main.go`**

Same update — include stake in generated genesis.

### Acceptance Criteria

- Genesis with staked validators initializes correctly.
- Initial active validator set matches genesis stakers.
- Supply accounting is correct at genesis (balance + stake = allocated amount).

---

## Step 8 — P2P and Protobuf Updates

### Files modified

**`internal/proto/types.proto`**

Add to `Account`:
```protobuf
uint64 staked_balance = 5;
```

Add new messages:
```protobuf
message ValidatorStake {
    bytes address = 1;
    bytes pub_key = 2;
    uint64 staked_balance = 3;
}

message UnbondingEntry {
    bytes address = 1;
    uint64 amount = 2;
    uint64 release_height = 3;
}

message SlashEvidence {
    BlockVote vote_a = 1;
    BlockVote vote_b = 2;
}
```

Update `Block` to include evidence:
```protobuf
message Block {
    BlockHeader header = 1;
    repeated Transaction transactions = 2;
    QuorumCertificate qc = 3;
    repeated SlashEvidence evidence = 4;
}
```

Add `Stake = 5` and `Unstake = 6` to transaction type documentation.

Add PoS fields to `GenesisConfig`:
```protobuf
uint64 min_stake = 13;
uint64 epoch_length = 14;
uint64 unbonding_period = 15;
uint64 slash_fraction_double_sign = 16;
```

Regenerate protobuf code.

**`internal/p2p/convert.go`**

Add conversion functions for `ValidatorStake`, `UnbondingEntry`, `SlashEvidence`. Update `BlockToProto` / `BlockFromProto` to handle the `Evidence` field.

### Acceptance Criteria

- Protobuf codegen succeeds.
- All conversion round-trips preserve data.
- Blocks with evidence serialize and deserialize correctly.

---

## Step 9 — RPC and CLI

### Files modified

**`internal/rpc/types.go`**

Add `StakedBalance` to `AccountResponse`:
```go
type AccountResponse struct {
    Address       string `json:"address"`
    Balance       uint64 `json:"balance"`
    Nonce         uint64 `json:"nonce"`
    Name          string `json:"name,omitempty"`
    StakedBalance uint64 `json:"stakedBalance"`
}
```

Add new response types:
```go
type ValidatorSetResponse struct {
    Validators []ValidatorStakeResponse `json:"validators"`
    TotalStake uint64                   `json:"totalStake"`
    Epoch      uint64                   `json:"epoch"`
}

type ValidatorStakeResponse struct {
    Address       string `json:"address"`
    StakedBalance uint64 `json:"stakedBalance"`
    Name          string `json:"name,omitempty"`
}
```

**`internal/rpc/server.go`**

Update `handleGetAccount` to include `StakedBalance`.

Update `handleListValidators` to return the **active validator set from state** (not genesis), including stake amounts.

Add `handleSubmitTransaction` support for `"stake"` and `"unstake"` tx types.

Add endpoint:
```
GET /v1/network/epoch
```
Returns current epoch number, epoch length, blocks until next epoch boundary.

**`cmd/drana-cli/commands/`**

Add **`stake.go`**:
```
drana-cli stake --key <hex> --amount <microdrana> [--rpc http://...]
```

Add **`unstake.go`**:
```
drana-cli unstake --key <hex> --amount <microdrana> [--rpc http://...]
```

Update `transfer.go` `txTypeStr` for the new types.

Update `main.go` dispatch and help text.

### Acceptance Criteria

- Account balance response includes staked balance.
- Validator list reflects the live staked set, not genesis.
- `drana-cli stake` and `drana-cli unstake` work end-to-end.
- Epoch info endpoint returns correct data.

---

## Step 10 — Indexer

### Files modified

**`internal/indexer/follower.go`**

Handle `"stake"` and `"unstake"` tx types in the block indexer. These can be logged in the `transfers` table or a new `staking_events` table for analytics.

**`internal/indexer/api.go`**

Add to `/v1/stats`:
- `totalStaked` (sum of all staked balances, queryable from node RPC)
- `activeValidatorCount`

### Acceptance Criteria

- Indexer doesn't crash on blocks containing stake/unstake transactions.
- Stats endpoint includes staking metrics.

---

## Step 11 — Integration Test

### Files

**`test/integration/pos_test.go`**

End-to-end test:

1. Start a 3-validator network where each validator stakes 1,000 DRANA at genesis.
2. Wait for 3 blocks — verify all validators are in the active set.
3. Create a new account (user) and fund it with 2,000 DRANA via transfer.
4. User submits a `Stake` transaction for 1,000 DRANA.
5. Verify: user's spendable balance decreased, staked balance increased.
6. Wait for the next epoch boundary.
7. Verify: user is now in the active validator set (4 validators).
8. Verify: proposer selection now includes the new validator (stake-weighted).
9. User submits an `Unstake` transaction for 500 DRANA.
10. Verify: remaining stake is 500 DRANA, which is below minimum — user is removed from active set at next epoch.
11. Wait for unbonding period (30 blocks).
12. Verify: unstaked amount returned to spendable balance.
13. Supply conservation: `sum(spendable) + sum(staked) + sum(unbonding) = genesis + issued - burned`.
14. **Slashing test:**
    a. Construct two conflicting `BlockVote` objects from the same validator at the same height.
    b. Include the `SlashEvidence` in a block.
    c. Verify: validator's stake is reduced by 5%.
    d. Verify: slashed amount added to burned supply.
    e. Supply conservation still holds.
15. Shut down all nodes cleanly.

### Acceptance Criteria

- Test passes end-to-end.
- A new validator can join by staking.
- A validator can leave by unstaking.
- Epoch transitions correctly update the active set.
- Proposer selection reflects stake weights.
- Quorum is stake-weighted.
- Slashing burns the correct amount.
- Supply conservation holds at every step.
- All prior tests (Phase 1–4, name enhancement) continue to pass.

---

## Files Modified Summary

| File | Change |
|------|--------|
| `internal/types/account.go` | Add `StakedBalance` |
| `internal/types/transaction.go` | Add `TxStake = 5`, `TxUnstake = 6` |
| `internal/types/staking.go` | New: `ValidatorStake`, `UnbondingEntry`, `SlashEvidence` |
| `internal/types/block.go` | Add `Evidence []SlashEvidence` |
| `internal/types/genesis.go` | Add PoS params + `Stake` on `GenesisValidator` |
| `internal/state/state.go` | Add `activeValidators`, `unbondingQueue`, `slashRecord`, `currentEpoch` + methods |
| `internal/state/stateroot.go` | Hash validators, unbonding, epoch, staked balance |
| `internal/state/executor.go` | `applyStake`, `applyUnstake`, unbonding release, epoch transition, slash processing |
| `internal/validation/validate.go` | `validateStake`, `validateUnstake` |
| `internal/store/kvstore.go` | Persist/load staking state |
| `internal/consensus/proposer.go` | Stake-weighted proposer selection |
| `internal/consensus/validator.go` | Stake-weighted quorum, updated signatures |
| `internal/consensus/engine.go` | Use `state.GetActiveValidators()` instead of `e.Validators` |
| `internal/genesis/genesis.go` | Parse PoS params, initialize staked validators |
| `internal/proto/types.proto` | Staking messages, evidence, PoS genesis params |
| `internal/p2p/convert.go` | Conversion for new types |
| `internal/rpc/server.go` + `types.go` | StakedBalance in responses, epoch endpoint, stake/unstake tx support |
| `cmd/drana-cli/commands/stake.go` | New: `stake` command |
| `cmd/drana-cli/commands/unstake.go` | New: `unstake` command |
| `cmd/drana-cli/main.go` | Dispatch new commands |
| `internal/indexer/follower.go` | Handle stake/unstake tx types |
| `internal/indexer/api.go` | Staking metrics in stats |
| `scripts/gen-testnet.sh` | Genesis stake field |
| `test/integration/pos_test.go` | New: full PoS integration test |

---

## Backward Compatibility

- `Account.StakedBalance` defaults to 0 — all existing accounts are unaffected.
- `GenesisConfig` new fields default to 0 — a genesis without PoS params disables staking (the chain falls back to genesis-defined validators with no epoch transitions).
- Existing blocks without `Evidence` field parse cleanly.
- All Phase 1–4 tests must continue to pass without modification (their genesis configs don't include PoS params, so the staking machinery is inert).

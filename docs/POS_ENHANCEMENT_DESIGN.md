# POS_ENHANCEMENT_DESIGN.md

## Proof-of-Stake: Open Validator Participation

### Problem

The current consensus model uses a fixed validator set defined at genesis. No one can join or leave the validator set after chain launch. This makes DRANA a private network, not a public one.

### Goal

Any account holding >= 1,000 DRANA can become a validator by staking. Validators earn block rewards proportional to their stake. Validators can unstake and leave. Misbehaving validators have their stake slashed.

The existing BFT consensus machinery (propose, vote, quorum certificate, instant finality) is preserved. What changes is how the validator set is determined: from a hardcoded genesis list to a dynamic set derived from on-chain stake.

---

## 1. Core Concepts

### 1.1 Staking

A user locks DRANA from their spendable balance into a staked balance. Staked DRANA cannot be spent or transferred. The staked amount determines the validator's voting power.

### 1.2 Active Validator Set

At every **epoch boundary**, the chain computes the active validator set from all accounts with stake >= `MinStake`. This set is used for proposer selection and vote validation for the entire next epoch.

The validator set does **not** change mid-epoch. This keeps consensus stable within each epoch — all validators agree on who the validators are for any given block.

### 1.3 Epochs

An **epoch** is a fixed number of blocks. The epoch length is a genesis parameter.

| Parameter | Value |
|-----------|-------|
| `EpochLength` | 30 blocks |
| At 120-second block intervals | ~60 minutes per epoch |

Epoch number for a block at height `H`:
```
epoch = H / EpochLength
```

Epoch boundary: the last block of an epoch triggers validator set recomputation for the next epoch.

### 1.4 Voting Power

Voting power is proportional to stake:
```
votingPower(validator) = validator.StakedBalance
```

Quorum threshold:
```
quorum = 2/3 of totalStakedAmongActiveValidators
```

A block is finalized when it collects votes whose cumulative stake weight exceeds the quorum threshold.

### 1.5 Proposer Selection

The proposer for a given height is selected deterministically based on stake weight. We use a weighted round-robin:

1. Sort active validators by address (deterministic tie-breaking).
2. Each validator has a selection weight equal to their stake.
3. At each height, accumulate weights and select the validator whose cumulative range covers `(blockHeight % totalStake)`.

This ensures that over time, a validator with 2x the stake proposes ~2x as many blocks. The algorithm is deterministic — all nodes compute the same proposer for each height.

---

## 2. New Transaction Types

### 2.1 Stake

Locks DRANA from spendable balance into staked balance.

**Fields:**
- sender
- amount (microdrana to stake)
- nonce
- signature

**Validation:**
- Amount >= `MinStake` (if this is the sender's first stake) OR amount > 0 (if adding to existing stake)
- Sender has sufficient spendable balance
- Standard signature and nonce checks

**State transition:**
- Debit sender's spendable balance by `amount`
- Credit sender's staked balance by `amount`
- Increment sender nonce
- If sender's total stake now >= `MinStake` and they were not previously a staker, mark them as a pending validator (active at next epoch boundary)

**Note:** Staking does NOT require a separate public key. The validator uses the same Ed25519 key as their account. Their node must be running and reachable for them to actually participate in consensus.

### 2.2 Unstake

Begins the unbonding process to withdraw staked DRANA.

**Fields:**
- sender
- amount (microdrana to unstake)
- nonce
- signature

**Validation:**
- Amount > 0
- Sender has sufficient staked balance
- Standard signature and nonce checks

**State transition:**
- Debit sender's staked balance by `amount`
- Create an unbonding entry: `{address, amount, releaseHeight = currentHeight + UnbondingPeriod}`
- Increment sender nonce
- If sender's remaining stake < `MinStake`, they are removed from the active validator set at the next epoch boundary

**Unbonding period:** 30 blocks (~60 minutes). During unbonding:
- The DRANA is neither spendable nor staked
- The DRANA is still subject to slashing (prevents stake-and-run)
- At `releaseHeight`, the DRANA returns to spendable balance automatically

### 2.3 Unstaking release

Not a transaction — this is an automatic state transition. At each block, the executor checks the unbonding queue and releases any entries where `releaseHeight <= currentHeight`, crediting the spendable balance.

---

## 3. Slashing

### 3.1 Slashable Offense: Double Signing

A validator signs two different blocks at the same height. This is the most serious offense because it threatens consensus safety.

**Evidence:** A `SlashEvidence` structure containing two `BlockVote` objects from the same validator at the same height but with different block hashes.

**Anyone** can submit slash evidence as part of a block (included by the proposer). The evidence is verified:
1. Both votes are from the same `VoterAddr`.
2. Both votes are at the same `Height`.
3. The `BlockHash` values differ.
4. Both signatures are valid.
5. The validator has not already been slashed for this height.

**Penalty:** 5% of the validator's total stake (staked + unbonding) is burned.

**State transition:**
- Compute `slashAmount = (stakedBalance + unbondingBalance) * 5 / 100`
- Debit from staked balance first, then from unbonding entries if staked balance is insufficient
- Burned amount is added to `burnedSupply`
- The validator is forcibly removed from the active set at the next epoch boundary

### 3.2 Non-Slashable: Downtime

For v1, downtime (missed proposals, missed votes) is **not slashed**. The penalty is organic: if you're offline, you miss block rewards. This is sufficient incentive for a v1 network.

### 3.3 Slash Evidence in Blocks

Slash evidence is included in the block body alongside transactions. The proposer may include any valid evidence they know about. Evidence is processed before transactions in block execution.

```go
type Block struct {
    Header       BlockHeader
    Transactions []*Transaction
    Evidence     []SlashEvidence  // new
    QC           *QuorumCertificate
}
```

---

## 4. Block Rewards

### 4.1 Current Model (Preserved)

The block reward is minted and credited to the **proposer** of each block. This remains unchanged.

### 4.2 Why Proposer-Only (Not Distributed)

Distributing rewards across all validators who voted would be fairer per-block, but proposer-only rewards are simpler and achieve the same long-term distribution because proposer selection is stake-weighted. Over many blocks, a validator with 10% of total stake proposes ~10% of blocks and earns ~10% of total rewards.

### 4.3 Reward Destination

The block reward is credited to the proposer's **spendable balance**, not their staked balance. This is intentional — validators should actively choose whether to restake, spend, or burn their earnings.

---

## 5. State Changes

### 5.1 Account

```go
type Account struct {
    Address       crypto.Address
    Balance       uint64  // spendable microdrana
    Nonce         uint64
    Name          string
    StakedBalance uint64  // locked microdrana (new)
}
```

### 5.2 Unbonding Queue

A global ordered list of pending unbonding entries:

```go
type UnbondingEntry struct {
    Address       crypto.Address
    Amount        uint64
    ReleaseHeight uint64
}
```

Stored in world state. Processed at the start of each block: any entry with `ReleaseHeight <= blockHeight` has its `Amount` returned to the account's spendable balance.

### 5.3 Active Validator Set

Stored in world state and recomputed at epoch boundaries:

```go
type ValidatorStake struct {
    Address      crypto.Address
    PubKey       crypto.PublicKey
    StakedBalance uint64
}
```

The active set is sorted by address for deterministic ordering. It is fixed for the duration of an epoch.

### 5.4 Slash Record

A set of `(validatorAddress, height)` pairs to prevent double-processing of slash evidence.

---

## 6. Epoch Transitions

At the last block of each epoch (`height % EpochLength == 0`), after all transactions are applied:

1. **Process unbonding releases** — return matured unbonding entries to spendable balances.
2. **Recompute active validator set** — scan all accounts, collect those with `StakedBalance >= MinStake`, sorted by address.
3. **Store the new active set** in world state.
4. **The new set takes effect starting at the next block.**

Between epoch boundaries, the active set is frozen. Stake and unstake transactions are processed immediately (balances change), but the active validator set used for consensus does not update until the next epoch boundary.

---

## 7. Genesis Changes

### 7.1 New Genesis Parameters

```json
{
  "minStake": 1000000000,
  "epochLength": 30,
  "unbondingPeriod": 30,
  "slashFractionDoubleSign": 5
}
```

- `minStake`: 1,000 DRANA = 1,000,000,000 microdrana
- `epochLength`: 30 blocks per epoch
- `unbondingPeriod`: 30 blocks (~60 minutes)
- `slashFractionDoubleSign`: 5 (percent)

### 7.2 Genesis Validators Become Genesis Stakers

The existing `validators` array in genesis gains a `stake` field:

```json
"validators": [
  {
    "address": "drana1...",
    "pubKey": "...",
    "name": "validator-1",
    "stake": 1000000000
  }
]
```

At genesis initialization:
- Each genesis validator's `StakedBalance` is set to their `stake` value
- Their spendable `Balance` is reduced by the staked amount (or the stake comes from a separate allocation)
- The initial active validator set is computed from these genesis stakers

This means the chain launches with a known validator set (just like today), but new validators can join via `Stake` transactions starting from block 1.

### 7.3 Peer Discovery

With an open validator set, we can no longer assume all validators are known at genesis. The `peerEndpoints` config becomes a list of **seed nodes** — known entry points. A new validator:

1. Connects to seed nodes
2. Syncs the chain
3. Stakes via a `Stake` transaction
4. Announces itself to peers
5. Begins participating in consensus at the next epoch boundary

For v1, we keep the static peer list but document that it functions as a seed list. Full dynamic peer discovery is deferred.

---

## 8. Proposer Selection Algorithm

Weighted round-robin based on stake. Given the active validator set sorted by address:

```
validators = [{addr: A, stake: 3000}, {addr: B, stake: 1000}, {addr: C, stake: 6000}]
totalStake = 10000

For height H:
  slot = H % totalStake
  cumulative = 0
  for each validator in sorted order:
    cumulative += validator.stake
    if slot < cumulative:
      return validator  // this is the proposer
```

Over 10,000 blocks: A proposes ~3,000, B proposes ~1,000, C proposes ~6,000. Proportional to stake.

This algorithm is deterministic — all nodes compute the same proposer for any given height given the same active set.

---

## 9. Quorum Calculation

Current system: `required = (validatorCount * 2 / 3) + 1` (count-based).

New system: stake-weighted.

```
totalActiveStake = sum of all active validators' StakedBalance
quorumThreshold = (totalActiveStake * 2 / 3) + 1
```

A block is finalized when the sum of stake behind its votes exceeds `quorumThreshold`.

When validating a QC:
```
voteStakeSum = 0
for each vote in QC:
  if vote is from an active validator and signature is valid:
    voteStakeSum += validator.StakedBalance
if voteStakeSum >= quorumThreshold:
  QC is valid
```

---

## 10. Delegation (Deferred)

Delegation allows non-validators to stake their DRANA behind a validator they trust, sharing rewards proportionally. This is deferred to a future enhancement because:

- It requires reward splitting logic
- It introduces a delegator/validator relationship model
- It needs an unbonding model for delegated stake
- It's a product feature, not a consensus requirement

**Design stub:** When delegation is added, it will introduce:
- `Delegate(validatorAddr, amount)` transaction
- `Undelegate(validatorAddr, amount)` transaction
- Delegated stake counts toward the validator's voting power
- Rewards are split between validator (commission) and delegators (proportional)
- Delegated stake is subject to slashing if the validator misbehaves

The current staking model is designed so that delegation can be layered on top without breaking changes.

---

## 11. Edge Cases

### What if all validators unstake?

The chain halts. This is by design — a PoS chain with no stake has no security. In practice, block rewards incentivize at least some validators to remain staked.

### What if only one validator remains?

They propose every block and self-finalize (their vote is 100% of stake). The chain continues but with no Byzantine fault tolerance. Others can join by staking.

### What if a validator stakes but doesn't run a node?

They are in the active set but never vote. Blocks proposed by other validators still finalize if quorum is met without the absent validator. The absent validator misses proposal opportunities and earns no rewards. They are not slashed (v1 has no downtime slashing).

### What if someone stakes the minimum and then transfers their remaining balance?

Their spendable balance drops but their staked balance is unaffected. They remain a validator as long as their staked balance >= `MinStake`.

### Can a validator add more stake?

Yes. A second `Stake` transaction adds to their existing staked balance. Their voting power increases at the next epoch boundary.

### Can a validator partially unstake?

Yes, as long as their remaining staked balance >= `MinStake`. If it drops below, they are removed from the active set at the next epoch.

---

## 12. Security Properties

### 12.1 Safety

The 2/3 stake-weighted quorum ensures that no conflicting block can be finalized unless >= 1/3 of total stake is controlled by an adversary. This is the standard BFT safety threshold.

### 12.2 Liveness

The chain produces blocks as long as >= 2/3 of stake is online and participating. If < 2/3 is online, the chain halts until enough validators return.

### 12.3 Slashing Deterrence

Double-signing costs 5% of stake. For a validator with 10,000 DRANA staked, that's 500 DRANA — a meaningful economic penalty that makes equivocation unprofitable.

### 12.4 Unbonding Period

The 30-block unbonding period prevents "stake, misbehave, unstake instantly" attacks. Slash evidence can be submitted during the unbonding period, catching the misbehavior before funds are released.

---

## 13. Supply Accounting Update

The supply conservation invariant becomes:

```
sum(spendable balances) + sum(staked balances) + sum(unbonding amounts)
  = genesis_supply + total_issued - total_burned
```

Slashing removes DRANA from staked/unbonding and adds it to `total_burned`.

---

## 14. Summary of Changes

| What | Before (Permissioned BFT) | After (Proof-of-Stake) |
|------|---------------------------|------------------------|
| Who can validate | Genesis-defined list only | Anyone with >= 1,000 DRANA |
| Validator set changes | Never | Every epoch (30 blocks) |
| Proposer selection | `validators[H % N]` (equal weight) | Stake-weighted deterministic selection |
| Quorum threshold | 2/3 of validator count | 2/3 of total staked weight |
| Block rewards | Proposer gets fixed reward | Same — but proposer selected by stake weight |
| New tx types | None | `Stake`, `Unstake` |
| Slashing | None | 5% for double-signing |
| Joining the network | Impossible after genesis | Stake >= 1,000 DRANA, active next epoch |
| Leaving the network | Impossible | Unstake, wait 30 blocks, funds returned |

---

## 15. Open Questions — Resolved

This section confirms that all design questions have been answered.

| Question | Answer |
|----------|--------|
| Minimum stake? | 1,000 DRANA (1,000,000,000 microdrana) |
| Unbonding period? | 30 blocks (~60 minutes) |
| Slashing? | Yes — 5% for double-signing. No downtime slashing. |
| Delegation? | Deferred. Design stub included. |
| Epoch length? | 30 blocks (~60 minutes) |
| Reward distribution? | Proposer-only (stake-weighted selection ensures proportional earnings) |
| Voting power? | Proportional to stake |
| Quorum? | 2/3 of total active stake |
| Can validators add more stake? | Yes |
| Can validators partially unstake? | Yes (if remaining >= MinStake) |
| Peer discovery? | Static seed list for v1. Dynamic discovery deferred. |
| What happens if all validators leave? | Chain halts. By design. |

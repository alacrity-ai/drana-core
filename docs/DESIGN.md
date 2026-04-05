# DESIGN.md

## DRANA — Design Overview

### Status
Draft v0.1

### Document Purpose
This document defines the high-level architecture, protocol model, core transaction types, data model, consensus approach, invariants, and implementation boundaries for **DRANA**, a purpose-built blockchain for a market-based attention system.

DRANA is **not** a general-purpose smart contract platform. It is a **single-application chain** whose canonical purpose is to record:

- wallet balances,
- wallet-to-wallet transfers,
- immutable short-form text posts,
- irreversible coin commitments attached to posts,
- subsequent irreversible boosts to existing posts.

The chain is designed so that multiple independent clients and websites may render different views of the same canonical state.

---

# 1. Executive Summary

DRANA is a custom blockchain written in **Go**, built specifically to support an **attention market** in which users irreversibly spend native coins to publish and promote short on-chain text messages.

The core product thesis is:

> Attention is scarce and should therefore have an explicit price.

A DRANA post is an immutable on-chain text object to which users may attach native currency. That currency permanently leaves circulation and becomes part of the post’s cumulative committed value. Any user may also contribute additional irreversible value to their own post or to someone else’s post.

This creates a protocol-level primitive for:

- self-promotion,
- patronage,
- coalition formation,
- narrative competition,
- public economic signaling.

The blockchain is intentionally narrow in scope. It does not support arbitrary user-defined contracts, media hosting, or complex generalized execution.

---

# 2. Design Goals

## 2.1 Primary Goals

DRANA is designed to satisfy the following goals:

### 1. Canonical On-Chain Attention Ledger
The chain must be the authoritative source of truth for:
- balances,
- posts,
- boosts,
- irreversible spend history.

### 2. Short Immutable Text On-Chain
Posts must be plain text, immutable, and directly recoverable from chain data.

### 3. Irreversible Economic Commitments
Coins committed to posts must be permanently removed from spendable wallet balances and must never be reclaimable.

### 4. Multiple Client Interpretations
Third parties must be able to build independent websites, explorers, and feed renderers from chain data alone.

### 5. Minimal, Tractable Protocol Surface
The chain must remain sufficiently simple to implement from scratch in Go without requiring a smart contract VM.

### 6. Deterministic Replication
All validators must deterministically converge on identical state given the same block history.

---

# 3. Non-Goals

DRANA v1 explicitly does **not** attempt to solve the following:

- arbitrary smart contracts,
- on-chain images or video,
- general-purpose decentralized compute,
- privacy-preserving transactions,
- anonymous balances,
- permissionless validator onboarding,
- staking/slashing economics,
- cross-chain interoperability,
- on-chain governance,
- censorship-proof frontend hosting,
- protocol-native ranking enforcement.

These may be revisited later, but none are part of the initial design.

---

# 4. Product Thesis

DRANA is best understood as:

> An immutable public ledger of short messages and irreversible capital commitments.

Users do not merely publish messages. They must **pay** to place those messages into the shared attention arena. Others may then choose to amplify those same messages by sacrificing their own capital.

This turns the system into a public market for:
- attention,
- allegiance,
- tribal coordination,
- patronage,
- visible ideological and memetic competition.

---

# 5. Protocol Scope

DRANA is a **single-purpose application chain**.

The protocol supports only a small number of state transition types:

- wallet-to-wallet coin transfers,
- post creation with attached irreversible coin spend,
- boosting an existing post with additional irreversible coin spend.

All ranking, feed rendering, categorization, moderation views, and UI composition are outside the protocol and are handled by clients.

---

# 6. Native Asset

## 6.1 Currency Name
The native currency of the network is **DRANA**.

## 6.2 Unit
For v1, the chain may expose balances as integer base units only. Human-readable decimal display can be added at the client level.

Example:
- `1 DRANA = 1_000_000 base units`

This exact denomination remains configurable for implementation.

## 6.3 Economic Role
DRANA is used for:
- peer-to-peer wallet transfers,
- post creation commitments,
- boost commitments.

## 6.4 Attention Spend Model
When DRANA is committed to a post:
- it leaves the sender’s spendable balance,
- it cannot be withdrawn,
- it cannot be reclaimed by any admin or protocol path,
- it increments that post’s cumulative committed total,
- it increments protocol-wide destroyed or permanently consumed supply.

The v1 design assumes a **burn-style model** at the protocol level.

## 6.5 Block Issuance Model

DRANA is not a fixed-supply asset. New DRANA is minted each block and issued to the block proposer.

### Mechanics

- A fixed `block_reward` amount of DRANA is minted at each block height.
- The reward is credited to the proposer's account as part of block execution.
- This issuance is a protocol-level state transition, not a user-originated transaction.

### Supply Dynamics

```
net_supply_change_per_block = block_reward - burn_in_block
```

At any point in the chain's life, total supply can be:

- **growing** — if issuance exceeds burn (low usage period),
- **shrinking** — if burn exceeds issuance (high usage period),
- **stable** — if issuance and burn are roughly equal.

### Economic Loop

The issuance model creates a circulating attention economy:

1. Validators secure the network and receive DRANA.
2. DRANA enters circulation via validator activity (selling, spending, distributing).
3. Users acquire DRANA and burn it for attention (posts, boosts).
4. Burned DRANA exits circulation permanently.
5. New issuance replaces burned supply, completing the loop.

### Design Intent

DRANA is **fuel for attention, not a scarce store of value**. The protocol does not attempt to create artificial scarcity. Instead, it creates a flow: issuance in, burn out, with the market determining the equilibrium.

### Supply Accounting

The protocol tracks:

- `totalIssuedSupply` — cumulative DRANA minted across all blocks,
- `totalBurnedSupply` — cumulative DRANA committed to posts,
- current circulating supply = genesis supply + totalIssuedSupply - totalBurnedSupply.

This must equal the sum of all account balances at every block height.

---

# 7. Core Entities

## 7.1 Account
A user account is identified by a public key / derived address and maintains:

- address,
- spendable balance,
- nonce.

## 7.2 Post
A post is an immutable on-chain text object with associated irreversible capital commitment.

A post includes:
- unique post ID,
- author address,
- text,
- creation height,
- creation timestamp or block time,
- total committed amount,
- boost count.

## 7.3 Validator
A validator is a network node authorized to:
- receive transactions,
- validate blocks,
- propose blocks when selected,
- sign or approve blocks,
- replicate state.

Validators are permissioned in v1 via genesis configuration.

---

# 8. Transaction Types

DRANA v1 supports three user-facing transaction types.

## 8.1 Transfer
Moves DRANA from one wallet to another.

### Purpose
Supports peer-to-peer coin movement and basic wallet functionality.

### Fields
- sender
- recipient
- amount
- nonce
- signature

### Validation Rules
- sender address is valid,
- recipient address is valid,
- signature is valid,
- nonce matches expected sender nonce,
- sender has sufficient spendable balance,
- amount is greater than zero.

### State Transition
- debit sender balance,
- credit recipient balance,
- increment sender nonce.

---

## 8.2 CreatePost
Creates a new immutable on-chain text post and attaches an initial irreversible DRANA commitment.

### Purpose
Publishes a message into the attention market.

### Fields
- author
- text
- amount
- nonce
- signature

### Validation Rules
- author signature is valid,
- nonce matches expected author nonce,
- author has sufficient spendable balance,
- amount is at or above `minimum_post_commitment`,
- text length is within maximum allowed size,
- text is valid UTF-8,
- text normalization rules are satisfied,
- text is non-empty after normalization.

### State Transition
- debit author balance by `amount`,
- increment author nonce,
- create new post object,
- set post `totalCommitted = amount`,
- increment global burned/consumed supply by `amount`.

### Notes
The text is immutable once accepted into a block.

---

## 8.3 BoostPost
Attaches additional irreversible DRANA to an existing post.

### Purpose
Allows a user to increase the competitive weight of a post over time.

### Fields
- booster
- postId
- amount
- nonce
- signature

### Validation Rules
- booster signature is valid,
- nonce matches expected booster nonce,
- booster has sufficient spendable balance,
- referenced post exists,
- amount is at or above `minimum_boost_commitment`.

### State Transition
- debit booster balance by `amount`,
- increment booster nonce,
- increment post `totalCommitted` by `amount`,
- increment post boost count,
- increment global burned/consumed supply by `amount`.

### Notes
Any wallet may boost any post, including:
- the author boosting their own post,
- another user boosting someone else’s post.

This is a deliberate protocol feature and not an accident.

---

# 9. Post Funding Model

## 9.1 Irreversible Commitments
All coin sent into a post is permanently non-recoverable.

No protocol operation may:
- withdraw committed DRANA,
- refund committed DRANA,
- delete a post and restore committed DRANA,
- transfer “ownership” of committed DRANA out of the post.

## 9.2 Third-Party Boosting
Any user may contribute DRANA to any existing post.

This is a first-class feature because it enables:
- patronage,
- tribal reinforcement,
- coalition formation,
- collective amplification of messages.

## 9.3 Post Ownership
A post has a fixed author but is not a transferable asset in v1.

A post:
- is authored by a single address,
- may receive support from many addresses,
- is not an NFT,
- cannot be sold or reassigned at protocol level.

---

# 10. Text Rules

## 10.1 Text-Only Chain
DRANA v1 supports **short-form text only**.

The protocol does not support:
- images,
- audio,
- video,
- rich media blobs,
- markdown rendering semantics,
- HTML.

## 10.2 Maximum Length
Posts are capped to a strict length limit, expected to be in the range of:
- 140 characters, or
- 280 characters.

Final exact value is an implementation parameter.

## 10.3 Encoding
All post text must be valid UTF-8.

## 10.4 Immutability
Posts are immutable once included in a block.

No edit operation exists in v1.

---

# 11. Consensus Model

## 11.1 Consensus Philosophy
DRANA v1 uses a **small permissioned validator set** suitable for development, local testing, and controlled network operation.

It is not initially designed as a permissionless public validator marketplace.

## 11.2 Validator Admission
Validators are defined in genesis configuration via public keys and network endpoints.

## 11.3 Block Proposal
For v1, a simple rotating proposer model is recommended:

- each height has a deterministic proposer,
- proposer selection is derived from block height and validator set ordering,
- proposer assembles valid pending transactions into a block.

## 11.4 Block Approval
Other validators verify:
- previous block hash,
- proposer identity,
- transaction validity,
- signatures,
- nonces,
- sufficient balances,
- deterministic state transition result.

A block is considered accepted when it reaches the configured validator approval threshold.

For v1 PoC, this may be:
- majority, or
- `2/3 + 1` threshold.

## 11.5 Finality
Finality is effectively immediate once a block reaches the required validator signatures/approvals.

Reorg behavior should be minimal or nonexistent in the intended v1 model.

---

# 12. State Model

## 12.1 Protocol State
The chain maintains deterministic replicated state including:

- account balances,
- account nonces,
- post records,
- global burned/consumed supply,
- global issued supply (cumulative block rewards),
- current chain height,
- validator metadata as needed.

## 12.2 Suggested Account Structure
```text
Account {
  address
  balance
  nonce
}
```

## 12.3 Suggested Post Structure

```
Post {  
  postId  
  author  
  text  
  createdAtHeight  
  createdAtTime  
  totalCommitted  
  boostCount  
}
```

Optional future derived or indexed fields:

- authorCommitted
- thirdPartyCommitted
- uniqueBoosterCount
- lastBoostAtHeight

These do not necessarily need to be consensus-native in v1.

---

# 13. Block Model

A block should include:

- block height,
- previous block hash,
- proposer ID/address,
- timestamp,
- ordered transactions,
- state root or deterministic state hash,
- validator signatures / approvals.

## 13.1 Block Validation Rules

A valid block must satisfy:

- correct parent hash,
- correct proposer for the round/height,
- valid and uniquely ordered transactions,
- all transactions pass validation,
- deterministic post-state hash matches execution result,
- required validator approvals are present.

---

# 14. Mempool

Each node maintains a mempool of pending transactions.

## 14.1 Mempool Responsibilities

- receive transactions via RPC or p2p,
- verify signatures and basic validity,
- reject malformed or obviously invalid transactions,
- hold pending txs until proposed or expired.

## 14.2 Ordering

Transaction ordering inside a block is proposer-controlled but must remain deterministic once included in the block.

Initial proposer logic may be simple:

- FIFO by receive time,
- nonce-consistent inclusion,
- bounded block size.

---

# 15. Ranking Model

## 15.1 Ranking Is Not Consensus State

The protocol does **not** enforce a canonical feed ordering.

This is critical.

The chain records only:

- posts,
- spend,
- boosts,
- timing data.

Clients derive ranking from those canonical inputs.

## 15.2 Why Ranking Is Off-Chain

Keeping ranking outside consensus:

- reduces protocol complexity,
- avoids expensive on-chain sorting,
- allows multiple interpretations,
- enables third-party websites to experiment with different ranking formulas.

## 15.3 Example Client Ranking Function

A client may choose something like:

score = totalCommitted / (1 + ageHours)^alpha

or

score = log(1 + totalCommitted) / (1 + ageHours)^alpha

or another deterministic function.

The protocol does not mandate one ranking formula in v1.

---

# 16. Client Ecosystem Model

Because posts and boosts are canonical chain data, any third party can build:

- a first-party website,
- an explorer,
- a raw feed client,
- a politics-only feed,
- a memes-only feed,
- a moderation-heavy client,
- a minimally filtered client,
- analytics dashboards.

This is a deliberate design property.

The chain is the canonical truth layer.  
Clients are interpretations and presentations of that truth.

---

# 17. RPC Surface

The chain node should expose a simple RPC interface for wallet, explorer, and client development.

## 17.1 Minimum RPC Endpoints

Recommended initial RPC surface:

### Chain / Node

- `GetNodeInfo`
- `GetLatestBlock`
- `GetBlockByHeight`
- `GetBlockByHash`

### Accounts

- `GetBalance(address)`
- `GetNonce(address)`

### Posts

- `GetPost(postId)`
- `ListPosts(...)` (basic paging)
- `ListBoosts(postId)` or block-derived history if later indexed elsewhere

### Transactions

- `SubmitTransaction(tx)`
- `GetTransaction(txHash)`
- `GetTransactionStatus(txHash)`

### Network

- `ListPeers`
- `ListValidators`

RPC transport may be:

- JSON over HTTP for initial ease of debugging, or
- gRPC/protobuf if desired from the outset.

---

# 18. Persistence

## 18.1 Storage Principles

Persistence should separate:

- canonical chain history,
- materialized latest state.

## 18.2 Canonical History

Stores:

- blocks,
- transactions,
- validator approvals/signatures.

This is append-only.

## 18.3 Materialized State

Stores current:

- balances,
- nonces,
- posts,
- global counters.

## 18.4 Suggested Storage Approach

A practical v1 implementation in Go may use:

- append-only block files or structured block store,
- embedded KV store for state.

Examples of possible embedded databases include:

- BadgerDB,
- Pebble,
- BoltDB-derived options.

Final choice is implementation detail.

---

# 19. Security and Validation Invariants

The following invariants must always hold.

## 19.1 Balance Safety

No account balance may become negative.

## 19.2 Nonce Monotonicity

Each transaction from an account must consume exactly the next expected nonce.

## 19.3 Signature Validity

All user-originated transactions must be authenticated by valid signatures.

## 19.4 Post Immutability

Once created, a post’s text must never change.

## 19.5 Commitment Irreversibility

No protocol path may restore committed DRANA to a spendable balance.

## 19.6 Deterministic Execution

Given the same chain history, all validators must compute identical state.

## 19.7 Post Existence for Boosts

A boost must only target an existing post.

## 19.8 Supply Accounting

Any DRANA consumed into posts must be accounted for exactly once.

## 19.9 Supply Conservation

At every block height, the following must hold:

```
sum(all account balances) = genesis_supply + total_issued_supply - total_burned_supply
```

No DRANA may be created or destroyed except through the block reward issuance and post/boost burn mechanisms.

---

# 20. Moderation Boundary

## 20.1 Protocol Layer

The protocol records canonical text and spend history.

## 20.2 Client Layer

Individual websites and feed clients may choose whether and how to display content.

This distinction is intentional.

The chain does not attempt to erase or mutate canonical history. Frontends may:

- filter,
- hide,
- label,
- categorize,
- down-rank,
- refuse to render certain content.

This allows the protocol to remain stable while clients compete on policy.

---

# 21. Network Topology for v1

## 21.1 Initial Deployment Model

The initial proof-of-concept network is expected to run across a small number of validator nodes in a controlled environment such as a home network or local lab.

## 21.2 Static Peer Configuration

For v1, peer discovery may be static:

- validators know each other’s addresses in config,
- no dynamic peer discovery is required,
- no DHT or advanced peer networking required.

This keeps implementation tractable.

---

# 22. High-Level Architecture

## 22.1 Components

### 1. Chain Node (Go)

Responsibilities:

- transaction intake,
- mempool,
- p2p communication,
- block proposal,
- block validation,
- state execution,
- persistence,
- RPC.

### 2. CLI Wallet (Go)

Responsibilities:

- key generation,
- address derivation,
- transaction signing,
- transfer submission,
- post creation submission,
- boost submission,
- balance and post queries.

### 3. Explorer / Indexer

Responsibilities:

- consume chain data,
- provide efficient queries,
- compute derived views,
- expose data for websites and dashboards.

This may be written in Go initially or in TypeScript later.

### 4. First-Party Website

Responsibilities:

- render feed,
- show wallets and balances,
- enable posting and boosting,
- provide ranking views,
- offer the first canonical UX for the network.

This is out of scope for the chain implementation phase.

---

# 23. Example State Transitions

## 23.1 Transfer Example

Alice sends 100 DRANA to Bob.

Before:

- Alice balance: 1000
- Bob balance: 50

After:

- Alice balance: 900
- Bob balance: 150

---

## 23.2 CreatePost Example

Alice creates a post:

> "The empire of relevance belongs to the highest bidder."

with 200 DRANA.

Before:

- Alice balance: 900
- burned supply: 0

After:

- Alice balance: 700
- new post totalCommitted: 200
- burned supply: 200

---

## 23.3 BoostPost Example

Bob boosts Alice’s post by 75 DRANA.

Before:

- Bob balance: 150
- post totalCommitted: 200
- burned supply: 200

After:

- Bob balance: 75
- post totalCommitted: 275
- burned supply: 275

---

# 24. Protocol Parameters

These values should be configurable at genesis or node config level.

Recommended initial parameters include:

- maximum post length,
- minimum post commitment,
- minimum boost commitment,
- maximum block size,
- maximum transactions per block,
- block interval target,
- block reward (microdrana minted per block),
- validator approval threshold,
- genesis supply,
- initial validator set.

---

# 25. Implementation Language

The DRANA chain node is to be implemented in **Go**.

## 25.1 Why Go

Go is selected because it offers:

- strong suitability for network daemons,
- practical concurrency primitives,
- good performance characteristics,
- simple deployment as a single binary,
- good ergonomics for RPC, persistence, and validator services,
- proven precedent in blockchain and distributed systems tooling.

## 25.2 Language Boundary

The chain itself is written in Go.

A later website or explorer may use:

- TypeScript,
- React,
- Vite,
- or other standard web tooling.

---

# 26. Recommended Development Phases

## Phase 1 — Core Chain

Deliver:

- block structure,
- validator configuration,
- proposer rotation,
- transaction signing and validation,
- balances,
- nonces,
- `Transfer`,
- `CreatePost`,
- `BoostPost`,
- state persistence,
- basic RPC.

Success criteria:

- multiple validators converge on identical chain state,
- posts and boosts replicate correctly.

## Phase 2 — CLI and Explorer

Deliver:

- wallet CLI,
- query commands,
- block inspection,
- post inspection,
- derived feed reconstruction.

Success criteria:

- operator can create wallets, submit transactions, and inspect results entirely via terminal.

## Phase 3 — First-Party Client

Deliver:

- web UI,
- post feed,
- ranking logic,
- wallet interaction,
- balance display,
- post creation and boosting UX.

Success criteria:

- users can interact with the attention market through a browser.

---

# 27. Questions

The following are important questions:

- Exact signature scheme and address format
- Exact block interval target
- Exact validator approval mechanism and wire protocol
- Exact post character limit
- Exact burn/accounting denomination
- Whether timestamp comes purely from proposer or is constrained further
- Whether post text is stored directly in block tx payload only, or also materialized in state for convenience
- Whether `ListPosts` belongs in node RPC or only in an external indexer


## Answers

### 1. Exact signature scheme and address format

**Recommendation:**  
Use **Ed25519** for transaction signatures, and derive addresses as a truncated, checksummed hash of the public key.

**Proposed exact scheme:**

- Private/public keys: **Ed25519**
- Public key encoding: 32 bytes
- Address derivation:
    - `pubKeyHash = SHA-256(pubKey)`
    - `addressBody = first 20 bytes of pubKeyHash`
    - `checksum = first 4 bytes of SHA-256(addressBody)`
    - display format: `drana1` + hex(addressBody || checksum)

**Why this is the best choice:**

- **Implementation tractability:** Ed25519 is simple, mature, and well-supported in Go’s standard ecosystem.
- **Deterministic verification:** Excellent for a custom chain with a narrow tx model.
- **Performance:** Fast enough for your workload by a wide margin.
- **Operational simplicity:** Easier than ECDSA/secp256k1 to implement correctly from scratch; fewer footguns.
- **Good fit for a fresh chain:** You do not need Ethereum compatibility, so there is no reason to inherit Ethereum’s signing machinery.

**Why not secp256k1?**

- It would only make sense if you wanted compatibility with existing Ethereum-style wallets and tooling.
- Since you explicitly want a fresh chain, that compatibility is less valuable than implementation simplicity.

**Address rationale:**

- 20-byte addresses are familiar, compact, and sufficient.
- A checksum in the displayed form reduces operator error.
- Prefixing with `drana1` creates clear network identity and avoids ambiguity.

---

### 2. Exact block interval target

**Recommendation:**  
Use a target block interval of **120 seconds (2 minutes)**.

**Why this is the best choice:**

- It is slow enough to keep v1 consensus and validator coordination simple.
- It gives proposers enough time to gather txs and validators enough time to validate and approve without introducing needless timing fragility.
- It reduces churn in a small home-network validator set.
- It is more forgiving while you are debugging consensus and persistence.

**Why 2 minutes is better than something faster for v1:**

- A 5–10 second chain sounds exciting, but it increases:
    - proposer timing sensitivity,
    - validator coordination frequency,
    - mempool race complexity,
    - debugging pain,
    - risk of accidental forks or block misses.
- Your product does not need low-latency DeFi behavior. A feed-based attention market can tolerate slower settlement during v1.

**Refinement:**

- Keep **120 seconds** as the protocol target.
- Permit early proposal within a bounded window only if you later want faster empty-to-full block responsiveness.
- For now, strict 2-minute cadence is cleaner.

---

### 3. Exact validator approval mechanism and wire protocol

**Recommendation:**  
Use a **permissioned round-robin proposer + 2/3 quorum vote finalization** model, with **gRPC + Protobuf** as the wire protocol.

**Proposed exact mechanism:**

- Genesis defines `N` validators.
- At block height `H`, proposer is `validators[H mod N]`.
- Proposer builds block and broadcasts `ProposedBlock`.
- Validators independently validate:
    - parent hash,
    - proposer identity,
    - tx signatures,
    - nonce correctness,
    - balance sufficiency,
    - deterministic state root,
    - timestamp validity.
- Validators return signed `BlockVote`.
- Block finalizes when proposer collects **≥ 2/3 of validator voting power**. In v1, voting power can simply be equal-weight-per-validator.
- Finalized block includes the quorum certificate / vote set.

**Why this is the best choice:**

- It is simple enough to implement without becoming a toy.
- It gives you real multi-validator behavior and explicit finality.
- It avoids proof-of-work and avoids premature proof-of-stake economics.
- It is appropriate for a controlled validator set.

**Why 2/3 instead of simple majority?**

- 2/3 is the more serious and future-compatible threshold for Byzantine-style thinking.
- Even if your home-network PoC is friendly, choosing 2/3 now prevents you from hardcoding weaker assumptions into the protocol.

**Why gRPC + Protobuf?**

- Better typed boundaries than ad hoc JSON.
- More stable for node-to-node communication.
- Better binary efficiency.
- Cleaner evolution path as the protocol matures.

**Suggested message types:**

- `SubmitTx`
- `ProposedBlock`
- `BlockVote`
- `FinalizedBlock`
- `GetBlock`
- `GetStateRoot`
- `GetPeerStatus`

**Nuance:**

- You can still expose **JSON HTTP RPC** for wallets and explorers.
- But **node-to-node traffic** should be gRPC/Protobuf.

So the split should be:

- **P2P / validator wire:** gRPC + Protobuf
- **Client-facing RPC:** JSON over HTTP, at least initially

---

### 4. Exact post character limit

**Recommendation:**  
Use **280 characters**, not 500.

**Why 280 is the better answer:**

- It preserves the aesthetic severity of the system.
- It keeps tx payloads small and bounded.
- It encourages punchy, slogan-like, memetic posting, which fits the product.
- It reduces validator/storage burden over time.
- It limits abuse surface relative to longer payloads.

**Why not 500?**

- 500 is not catastrophic, but it moves the system away from “scarce inscription board” and closer to “microblogging platform.”
- Longer posts weaken the clarity of the primitive.
- Longer text increases:
    - state/history bloat,
    - moderation burden,
    - incentive for essay posting rather than attention combat.

The strongest version of DRANA is **a compressed battlefield of slogans, declarations, and rallying cries**, not longform discourse.

**Best exact answer:**  
Set hard cap to **280 Unicode code points**, with normalization rules defined precisely.

**Implementation note:**  
Do not use raw byte count alone as the user-facing rule. Use:

- UTF-8 validity
- normalized Unicode form
- max **280 code points**
- and optionally a secondary max byte cap, e.g. `1024 bytes`, to prevent pathological multi-byte payload abuse

That gives both UX clarity and implementation safety.

---

### 5. Exact burn/accounting denomination

**Recommendation:**  
Use **integer base units only**, with:

- **1 DRANA = 1,000,000 microdrana**
- all on-chain accounting in `uint64` integer base units

**Why this is the best choice:**

- Integer accounting avoids floating-point nonsense completely.
- `1e6` subunits is plenty of granularity for a chain of this type.
- It keeps UX flexible:
    - minimum boost can be small,
    - post commitments can be expressive,
    - future pricing can be tuned.

**Why not 1e18 like Ethereum?**

- Completely unnecessary for this chain.
- Makes human reasoning worse.
- Adds conceptual baggage with no benefit for your use case.

**Why not whole-unit only?**

- Too coarse. You will likely want:
    - small boosts,
    - faucet/test distributions,
    - flexible minimums.

**Best exact answer:**

- Protocol accounting unit: **microdrana**
- Display denomination: **DRANA**
- Conversion:
    - `1 DRANA = 1_000_000 microdrana`

This is clean and practical.

---

### 6. Whether timestamp comes purely from proposer or is constrained further

**Recommendation:**  
The proposer supplies the block timestamp, **but it must be constrained by deterministic validation rules**.

**Best exact rules:**  
A block timestamp must satisfy:

1. `timestamp > parent.timestamp`
2. `timestamp <= validator_local_time + allowed_future_drift`
3. `timestamp` must be aligned to the expected block slot, or at least not earlier than the current slot start

Suggested `allowed_future_drift`:

- **15 seconds** in v1 home-network deployment

**Why this is the best choice:**

- Pure proposer freedom is too loose and can distort feed timing / age-based ranking.
- Fully externalized time consensus is overkill for v1.
- Constrained proposer timestamps are the standard pragmatic middle ground.

**Most important reason in DRANA specifically:**  
Since clients may derive ranking using age/decay, timestamp manipulation would have real economic and visibility consequences. Therefore, block time cannot be unconstrained.

**Additional recommendation:**  
Base ranking primarily on:

- `createdAtHeight`
- `lastBoostAtHeight`

and treat wall-clock time as derived from block schedule where possible.

That makes ranking less vulnerable to subtle timestamp games.

So the answer is:

> timestamp is proposer-supplied, but strictly bounded by parent time and a small future-drift constraint; it is not purely discretionary.

---

### 7. Whether post text is stored directly in block tx payload only, or also materialized in state for convenience

**Recommendation:**  
Store post text in **both** places:

- canonically in the `CreatePost` transaction payload inside the block history
- and materialized in state for direct lookup convenience

**Why this is the best answer:**

- The tx payload is the canonical historical source.
- Materializing in state makes node RPC and client development much simpler.
- The total payload size is small enough, since posts are short text only.
- This chain is single-purpose; convenience matters more than maximal asceticism.

**Why not tx payload only?**

- You would force every node/client query path to replay or index block history just to answer `GetPost(postId)`.
- That pushes too much burden outward for too little gain.
- It makes the core node less useful.

**Why state materialization is acceptable here:**

- Posts are the central object of the chain.
- Post count and payload size are bounded enough for this to be reasonable in v1.
- You are not storing images or arbitrary blobs.

So the best design is:

- **Historical truth:** block tx payload
- **Operational convenience:** current-state post record includes text

That is the right balance.

---

### 8. Whether `ListPosts` belongs in node RPC or only in an external indexer

**Recommendation:**  
`ListPosts` should exist in **both**, but with different intent:

- **Node RPC:** minimal, basic paginated listing for correctness and operability
- **External indexer:** rich querying, ranking, filtering, search, feed composition

**Why this is the best answer:**  
A node that cannot list posts at all is unnecessarily austere and painful to operate. You will want basic introspection directly from the node for:

- debugging,
- CLI tools,
- validation,
- bootstrap clients,
- network health checks.

But a node should **not** become a full feed engine.

**Best split:**

#### Node RPC should support:

- `GetPost(postId)`
- `ListPosts(page, pageSize, sortBy=creationHeight maybe)`
- perhaps `ListPostsByAuthor(address)`

This is enough for correctness and operability.

#### External indexer should support:

- ranked/trending feeds
- boosting history joins
- unique booster counts
- rich filters
- moderation overlays
- full-text search
- category lenses
- analytics

**Why not indexer-only?**

- It makes the protocol too dependent on auxiliary infrastructure.
- It weakens the “anyone can rebuild from chain truth” story.
- It makes initial development slower and more brittle.

**Why not make node RPC very rich?**

- Because that bloats the node’s responsibility and couples protocol implementation to product query patterns.

So the best answer is:

> **basic `ListPosts` belongs in node RPC; sophisticated listing belongs in an external indexer.**

---

## Final recommended set

- **Signature scheme:** Ed25519
- **Address format:** `drana1` + checksummed 20-byte pubkey hash
- **Block interval:** 120 seconds
- **Validator approval:** round-robin proposer + 2/3 quorum votes
- **Wire protocol:** gRPC + Protobuf between validators; JSON HTTP for client RPC
- **Post limit:** 280 Unicode code points, plus byte cap
- **Denomination:** `1 DRANA = 1,000,000 microdrana`
- **Timestamp:** proposer-supplied but strictly constrained
- **Post storage:** canonical in tx payload and materialized in state
- **ListPosts:** basic listing in node RPC; advanced listing in external indexer
- **Block reward:** configurable per-block issuance to proposer

---

# 28. Final Design Statement

DRANA is a purpose-built Go blockchain for a market-based attention protocol.

It provides:

- a native coin with per-block issuance and burn-on-use economics,
- wallet-to-wallet transfers,
- immutable on-chain short text posts,
- irreversible coin commitments attached to posts,
- subsequent boosts by any wallet to any post,
- deterministic replicated ledger state,
- a circulating attention economy where issuance flows to validators and burn flows from users,
- a foundation upon which many different websites and clients may render the same canonical chain data in different ways.

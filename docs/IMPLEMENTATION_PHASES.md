# IMPLEMENTATION_PHASES.md

## Purpose

This document defines the highest-level implementation phases for DRANA. Each phase listed here will be expanded into its own detailed implementation document (`IMPLEMENTATION_1.md`, `IMPLEMENTATION_2.md`, etc.) containing exact file layouts, struct definitions, interface contracts, and acceptance criteria.

These phases are ordered by dependency — each phase builds on the one before it.

---

## Phase 1 — Primitives and Local State Machine

**What this is:** The foundational types, cryptography, transaction logic, and single-node state execution engine — everything needed to process a chain locally with no networking.

**Scope:**

- Ed25519 key generation, signing, and verification
- Address derivation (`drana1` + checksummed 20-byte pubkey hash)
- Core data structures: `Account`, `Post`, `Transaction`, `Block`
- Transaction types: `Transfer`, `CreatePost`, `BoostPost`
- Transaction validation (signatures, nonces, balances, text rules)
- State machine: deterministic execution of an ordered transaction list against world state
- Block reward issuance: per-block minting credited to proposer, tracked as cumulative issued supply
- Genesis configuration: initial accounts, balances, validator set, protocol parameters (including block reward)
- Persistence: embedded KV store for materialized state, append-only block storage
- Serialization: Protobuf definitions for all on-chain structures

**Does NOT include:** Networking, consensus, RPC, CLI tooling.

**Exit criteria:** A single process can load genesis, execute a hardcoded sequence of transactions (including block reward issuance), persist blocks and state, and produce a deterministic state root. A test suite proves all validation rules and invariants from DESIGN.md sections 19.1–19.9, including the supply conservation invariant.

---

## Phase 2 — Consensus and Networking

**What this is:** The multi-validator layer — proposer rotation, block voting, finalization, and peer-to-peer communication that turns isolated state machines into a replicated ledger.

**Scope:**

- gRPC service definitions for validator-to-validator communication (`ProposedBlock`, `BlockVote`, `FinalizedBlock`, peer status)
- Mempool: transaction intake, deduplication, nonce-aware ordering
- Proposer selection: deterministic round-robin from genesis validator set
- Block proposal: proposer assembles valid pending transactions at the target 120-second interval
- Block validation: each validator independently verifies proposed block and returns signed vote
- Quorum finalization: block accepted at >= 2/3 validator approval
- Timestamp rules: proposer-supplied, constrained by parent time and future-drift bound
- Static peer configuration and connection management
- Chain sync: a joining or restarting validator can catch up from peers

**Does NOT include:** Client-facing RPC, CLI wallet, dynamic peer discovery.

**Exit criteria:** Three validator nodes on a local network converge on identical chain state over a sustained block sequence. A validator restarted mid-sequence catches up and re-joins consensus. Deliberately invalid blocks and transactions are correctly rejected.

---

## Phase 3 — Client RPC and CLI Wallet

**What this is:** The external interface layer — the JSON HTTP API that clients and wallets talk to, and the command-line wallet for operators.

**Scope:**

- JSON HTTP RPC server exposing the endpoints from DESIGN.md section 17:
  - Chain: `GetNodeInfo`, `GetLatestBlock`, `GetBlockByHeight`, `GetBlockByHash`
  - Accounts: `GetBalance`, `GetNonce`
  - Posts: `GetPost`, `ListPosts`, `ListPostsByAuthor`
  - Transactions: `SubmitTransaction`, `GetTransaction`, `GetTransactionStatus`
  - Network: `ListPeers`, `ListValidators`
- CLI wallet (`drana-cli`):
  - Key generation and management (keystore)
  - Address display
  - Balance and nonce queries
  - Transaction construction, signing, and submission for all three tx types
  - Post and block inspection commands

**Does NOT include:** Web frontend, indexer, ranking logic.

**Exit criteria:** An operator can, using only the CLI, generate a wallet, receive genesis funds, transfer coins, create a post, boost a post, and query all resulting state. All RPC endpoints return correct data against a live multi-validator network.

---

## Phase 4 — Indexer and Feed Infrastructure

**What this is:** The read-optimized query layer that consumes canonical chain data and serves the richer queries that feed clients and dashboards need.

**Scope:**

- Chain follower: subscribes to finalized blocks and indexes all transactions, posts, and boosts
- Relational or structured storage for indexed data (SQLite or Postgres for v1)
- Derived fields: `uniqueBoosterCount`, `authorCommitted`, `thirdPartyCommitted`, `lastBoostAtHeight`
- Query API: ranked/trending feeds, boost history, per-author activity, global burn stats
- Example ranking implementations (e.g., `score = log(1 + totalCommitted) / (1 + ageHours)^alpha`)

**Does NOT include:** Web UI, moderation policy, full-text search.

**Exit criteria:** The indexer tracks a live network in real time. A client can query a ranked feed of posts sorted by multiple ranking strategies. Derived counters match canonical chain state exactly.

---

## What Comes After

A **Phase 5 — First-Party Web Client** (web UI, post feed, wallet interaction, ranking views) is the natural next step but is outside the scope of the Go chain implementation. It will use the RPC and indexer APIs built in Phases 3 and 4.

---

## How to Use This Document

Each phase above will be expanded into a dedicated `IMPLEMENTATION_N.md` that specifies:

- exact Go package layout and file list,
- struct and interface definitions,
- function signatures and responsibilities,
- test plan and acceptance criteria,
- dependency boundaries (what it imports, what it exports).

Start with `IMPLEMENTATION_1.md`. Do not begin a phase until the previous phase's exit criteria are met.

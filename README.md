# DRANA

**An uncensorable social platform powered by its own blockchain.**

DRANA is Reddit on a blockchain. Anyone can post, anyone can boost, and no one can take it down. There are no moderators, no algorithms deciding what you see, and no company that can pull the plug. Every post, every vote, every interaction is an on-chain transaction — permanent, transparent, and owned by you.

Instead of upvotes, you put your money where your mouth is. Stake DRANA on a post to push it up the feed. If others stake on posts you're invested in, you earn rewards. The best content rises because people are willing to back it with real value — not because an algorithm decided to promote it.

**Your keys, your voice, your stake.**

<p align="center">
  <img src="images/dranapic1.PNG" width="320" alt="DRANA feed" />
</p>
<p align="center"><em>The feed — posts ranked by stake, filterable by channel. Put skin in the game or just browse.</em></p>

<p align="center">
  <img src="images/dranapic2.PNG" width="320" alt="Post detail view" />
</p>
<p align="center"><em>Post detail — see who staked, how much was burned in fees, and add replies. Unstake anytime.</em></p>

<p align="center">
  <img src="images/dranapic3.PNG" width="320" alt="Rewards page" />
</p>
<p align="center"><em>Rewards — track your stakes, see earnings in real time. Authors and stakers both earn when posts get boosted.</em></p>

---

## Core Concepts

- **Seven transaction types:** Transfer, CreatePost, BoostPost, UnstakePost, RegisterName, Stake, Unstake
- **Post staking:** Creating a post or boosting locks DRANA (6% fee, 94% staked and recoverable). Authors earn 2% of boosts, stakers earn 1%.
- **Proof-of-Stake:** Anyone with >= 1,000 DRANA can stake and become a validator
- **Block reward issuance:** New DRANA is minted each block and paid to the proposer (selected by stake weight)
- **Channels:** Posts can be tagged with a topic channel (e.g., `gaming`, `politics`). Feeds filter by channel.
- **Replies:** Flat one-level comment threads on posts. Replies are posts that reference a parent.
- **Off-chain ranking:** The chain stores posts, spend, and timing. Clients derive their own feeds (trending, top, new, controversial)
- **Name registration:** Accounts can register a unique on-chain name (3-20 chars, immutable)
- **Slashing:** Validators who double-sign lose 5% of their stake

## Architecture

```
drana-node      Validator node: PoS consensus, P2P, state machine, JSON RPC
drana-cli       Wallet: keygen, transfer, post, boost, stake, unstake, queries
drana-indexer   Read-optimized feed service: trending/top/new feeds, analytics
```

## Quick Start

### Prerequisites

- Go 1.22+
- Docker and Docker Compose (for the containerized path)

### Build

```bash
make build
```

This produces three binaries in `bin/`:
- `bin/drana-node`
- `bin/drana-cli`
- `bin/drana-indexer`

### Run a Local Testnet (Docker)

```bash
make docker-up
```

This generates a fresh 3-validator testnet and starts all nodes in containers. RPC endpoints:

- Validator 1: http://localhost:26657
- Validator 2: http://localhost:26658
- Validator 3: http://localhost:26659

Check the chain:

```bash
curl -s http://localhost:26657/v1/node/info | jq
```

Stop:

```bash
make docker-down
```

### Run a Local Testnet (Bare Metal)

```bash
make run-local
```

See [MINERS_GUIDE.md](MINERS_GUIDE.md) for detailed setup instructions.

### CLI Reference

```bash
# --- Key Management ---
drana-cli keygen                            # Generate a new wallet
drana-cli keygen --output my.key            # Save private key to file
drana-cli address --keyfile my.key          # Show address for a key

# --- Account Queries ---
drana-cli balance --address drana1...       # Balance, staked balance, name, nonce
drana-cli nonce --address drana1...         # Just the nonce
drana-cli validators                        # Active validator set with stakes
drana-cli unstake-status --address drana1...# Pending unbonding entries
drana-cli node-info                         # Chain height, epoch, supply stats

# --- Transactions ---
drana-cli transfer --key <hex> --to drana1... --amount 1000000
drana-cli post --key <hex> --text "Hello DRANA" --amount 1000000
drana-cli post --key <hex> --text "Gaming post" --amount 1000000 --channel gaming
drana-cli post --key <hex> --text "Great post!" --amount 100000 --reply-to <post-id-hex>
drana-cli boost --key <hex> --post <id-hex> --amount 500000
drana-cli unstake-post --key <hex> --post <id-hex>    # Unstake from a post (all-or-nothing)
drana-cli register-name --key <hex> --name satoshi
drana-cli stake --key <hex> --amount 1000000000       # Validator stake 1000 DRANA
drana-cli unstake --key <hex> --amount 500000000      # Validator unstake 500 DRANA

# --- Lookups ---
drana-cli get-block --latest               # Latest block
drana-cli get-block --height 42            # Block at height 42
drana-cli get-post --id <hex>              # Post details
drana-cli get-tx --hash <hex>              # Transaction details
```

All transaction commands auto-query the nonce. All commands accept `--rpc http://host:port` (default: `http://localhost:26657`). All commands accept `--key <hex>` or `--keyfile <path>`.

### Run the Indexer

```bash
# SQLite (default, zero config):
bin/drana-indexer -rpc http://localhost:26657 -db indexer.db -listen :26680

# PostgreSQL:
bin/drana-indexer -rpc http://localhost:26657 -db "postgres://user:pass@localhost:5432/drana_indexer?sslmode=disable" -listen :26680
```

Feed API examples:
- http://localhost:26680/v1/feed?strategy=trending
- http://localhost:26680/v1/feed?strategy=top&channel=gaming
- http://localhost:26680/v1/channels
- http://localhost:26680/v1/posts/{id}/replies

## Tests

```bash
make test          # all tests (unit + integration)
make test-unit     # unit tests only
make test-race     # with race detector
```

## Project Structure

```
cmd/
  drana-node/       Validator node entrypoint
  drana-cli/        CLI wallet entrypoint
  drana-indexer/    Indexer entrypoint
internal/
  consensus/        Stake-weighted proposer selection, block validation, consensus engine
  crypto/           Ed25519 keys, drana1-prefixed checksummed addresses
  genesis/          Genesis config loading and state initialization
  indexer/          SQLite-backed chain follower, ranking, feed API
  mempool/          Thread-safe transaction pool with nonce-aware reaping
  node/             Top-level node wiring and config
  p2p/              gRPC server/client, protobuf conversion, peer management
  proto/            Protobuf schemas and generated code
  rpc/              JSON HTTP RPC server (client-facing, 14 endpoints)
  state/            World state, executor, staking, epochs, deterministic state root
  store/            BadgerDB persistence (KV state + block history)
  types/            Core domain types (Account, Post, Transaction, Block, Staking)
  validation/       Transaction validation, text rules, name rules, stake rules
docs/
  DESIGN.md                    Full protocol specification
  API_REFERENCE.md             Complete RPC and indexer API documentation
  POS_ENHANCEMENT_DESIGN.md    Proof-of-stake design
  IMPLEMENTATION_PHASES.md     Development roadmap
  IMPLEMENTATION_PHASE_[1-4].md
  NAME_ENHANCEMENT.md
  POS_ENHANCEMENT_IMPLEMENTATION.md
```

## Documentation

- [DESIGN.md](docs/DESIGN.md) — Full protocol specification
- [API_REFERENCE.md](docs/API_REFERENCE.md) — RPC and indexer endpoint reference
- [MINERS_GUIDE.md](MINERS_GUIDE.md) — Validator setup guide (Docker, bare metal, joining a network)
- [POS_ENHANCEMENT_DESIGN.md](docs/POS_ENHANCEMENT_DESIGN.md) — Proof-of-stake design
- [NETWORK_LAUNCH_GUIDE.md](NETWORK_LAUNCH_GUIDE.md) — How to launch and operate a network
- [TODO.md](TODO.md) — Remaining items before production launch
- [docs/IMPLEMENTATION_PHASES.md](docs/IMPLEMENTATION_PHASES.md) — Development roadmap

## Network

The canonical genesis file is at `networks/mainnet/genesis.json`. Seed nodes are embedded in three places for redundancy:

1. **In the genesis file** (`seedNodes` field) — loaded by every node at startup
2. **Hardcoded in the binary** (`internal/node/seeds.go`) — fallback if genesis has none
3. **In node config** (`peerEndpoints`) — operator-specified seeds

A new validator only needs the genesis file and one reachable seed to join. Peer exchange handles the rest — nodes share their peer lists with each other, so the network self-organizes.

Current seed: `genesis-validator.drana.io:26601` (see [TODO.md](TODO.md) for DNS setup status)

## License

DRANA Core is licensed under the [Business Source License 1.1](LICENSE.md). You may view, fork, and modify the code for non-commercial and non-production purposes. Commercial use requires a separate license from Alacrity AI. The license converts to Apache 2.0 on April 5, 2030.

## Protocol Parameters (Testnet Defaults)

| Parameter | Value |
|-----------|-------|
| Block interval | 120 seconds |
| Block reward | 10 DRANA (10,000,000 microdrana) |
| Min post commitment | 1 DRANA |
| Min boost commitment | 0.1 DRANA |
| Post fee (creation) | 6% burned, 94% staked |
| Boost fee | 3% burned, 2% to author, 1% to stakers, 94% staked |
| Post length limit | 280 Unicode code points |
| Denomination | 1 DRANA = 1,000,000 microdrana |
| Min validator stake | 1,000 DRANA |
| Epoch length | 30 blocks (~60 minutes) |
| Unbonding period | 30 blocks (~60 minutes) |
| Slash (double-sign) | 5% of validator stake |
| Consensus | Stake-weighted BFT, 2/3 quorum |
| Signature scheme | Ed25519 |
| Address format | `drana1` + checksummed 20-byte pubkey hash |

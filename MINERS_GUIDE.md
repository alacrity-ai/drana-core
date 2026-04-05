# Validator Guide

This guide walks you through setting up a DRANA validator node.

## What You're Running

DRANA uses **proof-of-stake** consensus. As a validator, you:

1. **Stake DRANA** — lock >= 1,000 DRANA to join the active validator set
2. **Propose blocks** when it's your turn (selected proportionally to your stake)
3. **Vote on blocks** proposed by other validators
4. **Earn block rewards** — 10 DRANA minted per block, paid to the proposer

Blocks are produced every 120 seconds. A block finalizes when validators representing 2/3 of total stake sign off. The validator set updates every 30 blocks (~60 minutes) at epoch boundaries.

---

## Option A: Docker Testnet (Fastest Start)

Spins up a complete 3-validator testnet in containers. Each genesis validator starts with 1,000 DRANA staked.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- Go 1.22+ (for building the CLI)

### Steps

```bash
# 1. Clone the repo
git clone https://github.com/drana-chain/drana.git
cd drana

# 2. Build binaries and start the network
make docker-up
```

Three validators are now running and producing blocks.

### Verify

```bash
# Chain info (height, epoch, supply)
curl -s http://localhost:26657/v1/node/info | jq

# Active validators with stake
curl -s http://localhost:26657/v1/network/validators | jq

# Watch logs
make docker-logs
```

### Interact

```bash
# See a validator's balance and stake
bin/drana-cli balance --address <validator-address> --rpc http://localhost:26657

# Generate your own wallet
bin/drana-cli keygen

# Transfer DRANA from a genesis validator to your wallet
# (private key is in testnet/validator-1/config.json)
bin/drana-cli transfer \
  --key <validator-privkey> \
  --to <your-address> \
  --amount 100000000 \
  --rpc http://localhost:26657

# Register a name
bin/drana-cli register-name --key <your-privkey> --name satoshi

# Create a post (burns 1 DRANA)
bin/drana-cli post \
  --key <your-privkey> \
  --text "The attention economy is live." \
  --amount 1000000

# Create a post in a channel
bin/drana-cli post \
  --key <your-privkey> \
  --text "Check out this game" \
  --amount 1000000 \
  --channel gaming

# Reply to a post
bin/drana-cli post \
  --key <your-privkey> \
  --text "Great point!" \
  --amount 100000 \
  --reply-to <post-id-hex>
```

### Stop

```bash
make docker-down          # stop containers
make clean && make docker-up  # wipe and restart fresh
```

---

## Option B: Bare Metal Testnet

### Steps

```bash
git clone https://github.com/drana-chain/drana.git
cd drana
make build
make testnet
```

Open three terminals:

```bash
# Terminal 1
bin/drana-node -config testnet/validator-1/config.local.json

# Terminal 2
bin/drana-node -config testnet/validator-2/config.local.json

# Terminal 3
bin/drana-node -config testnet/validator-3/config.local.json
```

Or background all three: `make run-local`

Verify: `curl -s http://127.0.0.1:26657/v1/node/info | jq`

Stop: `make stop-local`

---

## Option C: Join an Existing Network

To join a running DRANA network as a new validator:

### 1. Get the Genesis File

The network operator provides `genesis.json`. This defines the chain ID, initial accounts, and protocol parameters.

### 2. Generate Your Identity

```bash
bin/drana-cli keygen --output my-validator.key
```

This prints your address and public key. **Keep the private key safe.**

### 3. Fund Your Wallet

You need at least **1,000 DRANA** to stake. Get DRANA from:
- An existing validator or user (they transfer to your address)
- The genesis allocation (if the network operator includes you)

Check your balance:

```bash
bin/drana-cli balance --address <your-address> --rpc http://<node>:26657
```

### 4. Create Your Node Config

Create `config.json`:

```json
{
  "genesisPath": "/path/to/genesis.json",
  "dataDir": "/path/to/data",
  "privKeyHex": "<your-64-byte-private-key-hex>",
  "listenAddr": "0.0.0.0:26601",
  "rpcListenAddr": "0.0.0.0:26657",
  "peerEndpoints": {
    "seed-1": "192.168.1.10:26601",
    "seed-2": "192.168.1.11:26602"
  }
}
```

`peerEndpoints` should include at least one existing validator's P2P address.

### 5. Start Your Node

```bash
bin/drana-node -config config.json
```

The node will:
1. Load genesis and initialize (or resume from disk)
2. Connect to peers
3. Sync missed blocks from the network
4. Begin participating in consensus (once staked)

### 6. Stake

Once your node is synced and your wallet is funded:

```bash
bin/drana-cli stake \
  --key <your-privkey> \
  --amount 1000000000 \
  --rpc http://localhost:26657
```

This stakes 1,000 DRANA. At the **next epoch boundary** (up to ~60 minutes), you'll be added to the active validator set and start proposing and voting on blocks.

### 7. Verify Your Validator Status

```bash
# Check your staked balance
bin/drana-cli balance --address <your-address>

# See yourself in the validator list (after next epoch)
bin/drana-cli validators
```

---

## Staking Operations

### Add More Stake

You can increase your stake at any time:

```bash
bin/drana-cli stake --key <privkey> --amount 500000000  # add 500 DRANA
```

The increased stake takes effect at the next epoch boundary. More stake = more block proposals = more rewards.

### Unstake

```bash
bin/drana-cli unstake --key <privkey> --amount 500000000  # unstake 500 DRANA
```

Unstaked DRANA enters a **30-block unbonding period** (~60 minutes). During this time:
- The DRANA is not spendable and not staked
- It is still subject to slashing
- After the unbonding period, it returns to your spendable balance automatically

Check unbonding status:

```bash
bin/drana-cli unstake-status --address <your-address>
```

**Important:** If unstaking would leave your remaining stake below 1,000 DRANA, you must unstake everything. Partial stakes below the minimum are not allowed.

### Slashing

If your validator signs two different blocks at the same height (double-signing), **5% of your total stake** (staked + unbonding) is burned. This is detected automatically and enforced by the protocol.

Don't run two instances of the same validator — that's the most common cause of double-signing.

---

## Running the Indexer (Optional)

The indexer provides ranked feeds and analytics. It's a separate process.

```bash
# SQLite (default):
bin/drana-indexer \
  -rpc http://localhost:26657 \
  -db indexer.db \
  -listen :26680 \
  -poll 5s

# Or with PostgreSQL:
bin/drana-indexer \
  -rpc http://localhost:26657 \
  -db "postgres://user:pass@localhost:5432/drana_indexer?sslmode=disable" \
  -listen :26680
```

Endpoints:

```bash
curl -s http://localhost:26680/v1/feed?strategy=trending | jq            # Trending posts
curl -s http://localhost:26680/v1/feed?strategy=top&channel=gaming | jq  # Top in gaming
curl -s http://localhost:26680/v1/channels | jq                          # All channels
curl -s http://localhost:26680/v1/posts/<id>/replies | jq                # Replies to a post
curl -s http://localhost:26680/v1/stats | jq                             # Global stats
curl -s http://localhost:26680/v1/leaderboard | jq                       # Top authors
```

See [API_REFERENCE.md](docs/API_REFERENCE.md) for the full endpoint list.

---

## Ports Reference

| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| P2P | 26601+ | gRPC | Validator-to-validator consensus |
| RPC | 26657+ | HTTP JSON | Client queries, tx submission |
| Indexer | 26680 | HTTP JSON | Ranked feeds, analytics |

---

## Troubleshooting

**"insufficient vote stake"**
The proposer couldn't collect enough votes to reach quorum (2/3 of total stake). Ensure enough validators are online.

**Blocks not advancing**
Check that validators can reach each other. Verify `peerEndpoints` in configs. Check firewall rules.

**"initial stake below minimum"**
You must stake at least 1,000 DRANA (1,000,000,000 microdrana) on your first stake transaction.

**"remaining stake below minimum"**
When unstaking, you must either keep >= 1,000 DRANA staked or unstake everything.

**Restarting a node**
Just restart with the same config. State loads from disk, missed blocks sync from peers.

**Wiping and starting fresh**
Delete the data directory (or `make clean` for the whole testnet), then regenerate with `make testnet`.

**When do I start earning rewards?**
After your stake transaction is confirmed AND the next epoch boundary is reached. Check `bin/drana-cli validators` to see if you're in the active set.

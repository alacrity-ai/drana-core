# Network Launch Guide

This guide walks you through launching a real DRANA network that anyone can join.

By the end, you'll have:
- A genesis validator running on a cloud provider
- A public DNS name (e.g., `genesis-validator.drana.io`)
- A committed genesis file in the repo
- A clear path for others to join as validators

---

## Part 1: Initialize the Network (You Do This Once)

### 1.1 Generate your validator identity and genesis

```bash
cd drana
./scripts/init-network.sh \
  --chain-id "drana-mainnet-1" \
  --seed-domain "genesis-validator.drana.io" \
  --seed-port 26601 \
  --rpc-port 26657 \
  --initial-balance 100000000000000 \
  --stake 1000000000000
```

This creates:

```
networks/mainnet/
  genesis.json           ← commit this to the repo
  validator.key          ← YOUR private key. DO NOT commit.
  node-config.json       ← config for your validator. DO NOT commit.
  seed-info.txt          ← your public address and seed endpoint
```

The script generates:
- One validator keypair (your genesis validator)
- A genesis file with your validator staked and a large initial balance
- A node config pointed at your domain

### 1.2 Inspect what was generated

```bash
cat networks/mainnet/seed-info.txt
```

Shows your validator address, public key, and the seed endpoint that others will use.

```bash
cat networks/mainnet/genesis.json | jq .
```

Verify the chain ID, your address in the accounts and validators lists, and the protocol parameters.

### 1.3 Secure your keys

```bash
# Copy your private key somewhere safe (password manager, encrypted drive)
cp networks/mainnet/validator.key ~/safe-location/

# The key is in .gitignore, but double-check
cat .gitignore | grep validator.key
```

### 1.4 Commit the genesis to the repo

```bash
git add networks/mainnet/genesis.json networks/mainnet/seed-info.txt
git commit -m "Add mainnet genesis"
git push
```

This is the canonical genesis. Every node on the network must use this exact file. Once committed, it never changes.

---

## Part 2: Deploy Your Genesis Validator

### 2.1 Choose a cloud provider

Any provider that can run a Docker container with a public IP and open ports works. Examples:

| Provider | Product | Cost |
|----------|---------|------|
| Hetzner | Cloud VPS (CX22) | ~$4/month |
| DigitalOcean | Droplet | ~$6/month |
| Fly.io | Machine | ~$5/month |
| Railway | Container | ~$5/month |
| AWS | EC2 t3.micro | ~$8/month |

Requirements:
- 1 CPU, 1GB RAM, 20GB disk (minimal — DRANA is lightweight)
- Two open TCP ports: **26601** (P2P) and **26657** (RPC)
- A public IP address

### 2.2 Set up the server

SSH into your server and clone the repo:

```bash
git clone https://github.com/drana-chain/drana.git
cd drana
```

Copy your private key to the server:

```bash
# From your local machine:
scp ~/safe-location/validator.key you@your-server:~/drana/validator.key
```

### 2.3 Option A: Run with Docker

Build and run:

```bash
# Build the image
docker build -t drana-node .

# Create a data directory
mkdir -p /opt/drana/data

# Copy genesis and config
cp networks/mainnet/genesis.json /opt/drana/
```

Create the node config at `/opt/drana/config.json`:

```json
{
  "genesisPath": "/data/genesis.json",
  "dataDir": "/data/db",
  "privKeyHex": "<paste your private key hex here>",
  "listenAddr": "0.0.0.0:26601",
  "rpcListenAddr": "0.0.0.0:26657",
  "peerEndpoints": {}
}
```

Note: `peerEndpoints` is empty — you're the first node. Others will connect to you.

Run:

```bash
docker run -d \
  --name drana-validator \
  --restart unless-stopped \
  -v /opt/drana/genesis.json:/data/genesis.json:ro \
  -v /opt/drana/config.json:/data/config.json:ro \
  -v /opt/drana/data:/data/db \
  -p 26601:26601 \
  -p 26657:26657 \
  drana-node
```

### 2.3 Option B: Run directly

```bash
make build
bin/drana-node -config networks/mainnet/node-config.json
```

Use `systemd`, `tmux`, or `screen` to keep it running.

### 2.4 Verify it's running

```bash
curl -s http://localhost:26657/v1/node/info | jq
```

You should see your chain ID, height incrementing, and your validator earning block rewards.

From outside the server:

```bash
curl -s http://<your-server-ip>:26657/v1/node/info | jq
```

---

## Part 3: Point DNS

### 3.1 Create a DNS record

In your DNS provider (Cloudflare, Route53, etc.), create an **A record**:

```
genesis-validator.drana.io → <your-server-ip>
```

### 3.2 Verify

```bash
curl -s http://genesis-validator.drana.io:26657/v1/node/info | jq
```

This is now the public entry point for the network.

### 3.3 Optional: Run the indexer

On the same server or a separate one:

```bash
docker run -d \
  --name drana-indexer \
  -v /opt/drana/indexer:/data \
  -p 26680:26680 \
  drana-node \
  drana-indexer -rpc http://genesis-validator.drana.io:26657 -db /data/indexer.db -listen 0.0.0.0:26680
```

Point `indexer.drana.io` at this server (or the same one if co-located):

```
indexer.drana.io → <server-ip>
```

Verify:

```bash
curl -s http://indexer.drana.io:26680/v1/stats | jq
```

---

## Part 4: A New Validator Joins the Network

This is what someone else does when they want to validate. These are the instructions you'd share (or they'd find in the repo's MINERS_GUIDE.md).

### 4.1 Clone and build

```bash
git clone https://github.com/drana-chain/drana.git
cd drana
make build
```

### 4.2 Genesis is already in the repo

```bash
ls networks/mainnet/genesis.json   # already there from your commit
```

### 4.3 Generate a wallet

```bash
bin/drana-cli keygen --output ~/.drana/validator.key
```

Gives them an address like `drana1xyz789...`. They have 0 DRANA.

### 4.4 Get funded

They share their address with you (or someone on the network), and receive >= 1,500 DRANA:

```bash
# You run this (or anyone with funds):
bin/drana-cli transfer \
  --keyfile ~/safe-location/validator.key \
  --to drana1xyz789... \
  --amount 1500000000000 \
  --rpc http://genesis-validator.drana.io:26657
```

They verify:

```bash
bin/drana-cli balance \
  --address drana1xyz789... \
  --rpc http://genesis-validator.drana.io:26657
```

### 4.5 Start their node

Create `~/.drana/config.json`:

```json
{
  "genesisPath": "networks/mainnet/genesis.json",
  "dataDir": "~/.drana/data",
  "privKeyHex": "<their private key hex>",
  "listenAddr": "0.0.0.0:26601",
  "rpcListenAddr": "0.0.0.0:26657",
  "peerEndpoints": {
    "seed": "genesis-validator.drana.io:26601"
  }
}
```

```bash
bin/drana-node -config ~/.drana/config.json
```

Node syncs the chain from the seed, discovers other peers, catches up to current height.

### 4.6 Stake

```bash
bin/drana-cli stake \
  --keyfile ~/.drana/validator.key \
  --amount 1000000000000 \
  --rpc http://localhost:26657
```

### 4.7 Wait for epoch and verify

```bash
bin/drana-cli node-info --rpc http://localhost:26657
# Watch blocksUntilNextEpoch count down

bin/drana-cli validators --rpc http://localhost:26657
# After epoch boundary, they appear in the list
```

They're now validating.

---

## Part 5: Ongoing Operations

### Monitor the network

```bash
# Chain status
curl -s http://genesis-validator.drana.io:26657/v1/node/info | jq

# Active validators
curl -s http://genesis-validator.drana.io:26657/v1/network/validators | jq

# Global stats (via indexer)
curl -s http://indexer.drana.io:26680/v1/stats | jq
```

### Fund new users (manual faucet)

```bash
bin/drana-cli transfer \
  --keyfile ~/safe-location/validator.key \
  --to <new-user-address> \
  --amount 10000000 \
  --rpc http://genesis-validator.drana.io:26657
```

### Upgrade the software

```bash
cd drana && git pull && make build
# Restart the node — it resumes from disk
```

### Back up validator state

The critical files:
- `validator.key` — your identity. Lose this, lose your validator.
- `genesis.json` — shared, but keep a copy.
- `data/` directory — chain state. Can be rebuilt by syncing, but backing up avoids long re-syncs.

---

## Checklist

```
[ ] Run init-network.sh to generate genesis + keys
[ ] Inspect genesis.json — verify chain ID and parameters
[ ] Secure validator.key in a safe location
[ ] Commit genesis.json to repo and push
[ ] Deploy validator to cloud provider
[ ] Open ports 26601 (P2P) and 26657 (RPC)
[ ] Point DNS: genesis-validator.drana.io → server IP
[ ] Verify: curl genesis-validator.drana.io:26657/v1/node/info
[ ] Optional: deploy indexer, point indexer.drana.io
[ ] Share the repo — new validators can now join
```

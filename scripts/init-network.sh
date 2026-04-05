#!/usr/bin/env bash
#
# Initializes a new DRANA network with a single genesis validator.
#
# Creates:
#   networks/mainnet/genesis.json     — commit this
#   networks/mainnet/seed-info.txt    — commit this
#   networks/mainnet/validator.key    — DO NOT commit
#   networks/mainnet/node-config.json — DO NOT commit
#
# Usage:
#   ./scripts/init-network.sh \
#     --chain-id "drana-mainnet-1" \
#     --seed-domain "genesis-validator.drana.io" \
#     --seed-port 26601 \
#     --rpc-port 26657 \
#     --initial-balance 100000000000000 \
#     --stake 1000000000000
#
set -euo pipefail

# Defaults
CHAIN_ID="drana-mainnet-1"
SEED_DOMAIN="localhost"
SEED_PORT=26601
RPC_PORT=26657
INITIAL_BALANCE=100000000000000   # 100M DRANA
STAKE=1000000000000               # 1M DRANA
OUTDIR="networks/mainnet"

while [[ $# -gt 0 ]]; do
    case $1 in
        --chain-id) CHAIN_ID="$2"; shift 2 ;;
        --seed-domain) SEED_DOMAIN="$2"; shift 2 ;;
        --seed-port) SEED_PORT="$2"; shift 2 ;;
        --rpc-port) RPC_PORT="$2"; shift 2 ;;
        --initial-balance) INITIAL_BALANCE="$2"; shift 2 ;;
        --stake) STAKE="$2"; shift 2 ;;
        --output) OUTDIR="$2"; shift 2 ;;
        *) echo "Unknown flag: $1"; exit 1 ;;
    esac
done

# Ensure CLI is built.
if [ ! -f bin/drana-cli ]; then
    echo "Building CLI..."
    go build -o bin/drana-cli ./cmd/drana-cli
fi

echo "Initializing network: $CHAIN_ID"
echo "  Seed endpoint: $SEED_DOMAIN:$SEED_PORT"
echo "  RPC endpoint:  $SEED_DOMAIN:$RPC_PORT"
echo ""

mkdir -p "$OUTDIR"

# Check if genesis already exists.
if [ -f "$OUTDIR/genesis.json" ]; then
    echo "ERROR: $OUTDIR/genesis.json already exists."
    echo "If you want to start over, delete $OUTDIR/ first."
    exit 1
fi

# Generate validator keypair.
KEY_OUTPUT=$(bin/drana-cli keygen 2>&1)
ADDRESS=$(echo "$KEY_OUTPUT" | grep "Address:" | awk '{print $2}')
PUBKEY=$(echo "$KEY_OUTPUT" | grep "Public Key:" | awk '{print $3}')
PRIVKEY=$(echo "$KEY_OUTPUT" | grep "Private Key:" | awk '{print $3}')

echo "Genesis validator:"
echo "  Address: $ADDRESS"
echo ""

# Write private key (NOT to be committed).
echo "$PRIVKEY" > "$OUTDIR/validator.key"
chmod 600 "$OUTDIR/validator.key"

# Write genesis.
cat > "$OUTDIR/genesis.json" <<EOF
{
  "chainId": "$CHAIN_ID",
  "genesisTime": $(date +%s),
  "accounts": [
    {
      "address": "$ADDRESS",
      "balance": $INITIAL_BALANCE
    }
  ],
  "validators": [
    {
      "address": "$ADDRESS",
      "pubKey": "$PUBKEY",
      "name": "genesis-validator",
      "stake": $STAKE
    }
  ],
  "maxPostLength": 280,
  "maxPostBytes": 1024,
  "minPostCommitment": 1000000,
  "minBoostCommitment": 100000,
  "maxTxPerBlock": 100,
  "maxBlockBytes": 1048576,
  "blockIntervalSec": 120,
  "blockReward": 10000000,
  "minStake": 1000000000,
  "epochLength": 30,
  "unbondingPeriod": 30,
  "slashFractionDoubleSign": 5,
  "seedNodes": ["$SEED_DOMAIN:$SEED_PORT"],
  "postFeePercent": 6,
  "boostFeePercent": 6,
  "boostBurnPercent": 3,
  "boostAuthorPercent": 2,
  "boostStakerPercent": 1
}
EOF

# Write node config (NOT to be committed).
cat > "$OUTDIR/node-config.json" <<EOF
{
  "genesisPath": "$OUTDIR/genesis.json",
  "dataDir": "$OUTDIR/data",
  "privKeyHex": "$PRIVKEY",
  "listenAddr": "0.0.0.0:$SEED_PORT",
  "rpcListenAddr": "0.0.0.0:$RPC_PORT",
  "peerEndpoints": {}
}
EOF

# Write seed info (safe to commit — public info only).
cat > "$OUTDIR/seed-info.txt" <<EOF
DRANA Network: $CHAIN_ID
Genesis Validator Address: $ADDRESS
Genesis Validator Public Key: $PUBKEY
Seed P2P Endpoint: $SEED_DOMAIN:$SEED_PORT
Seed RPC Endpoint: $SEED_DOMAIN:$RPC_PORT
EOF

echo ""
echo "Files created:"
echo "  $OUTDIR/genesis.json      ← COMMIT this to the repo"
echo "  $OUTDIR/seed-info.txt     ← COMMIT this to the repo"
echo "  $OUTDIR/validator.key     ← DO NOT commit. Keep safe."
echo "  $OUTDIR/node-config.json  ← DO NOT commit. Deploy to server."
echo ""
echo "Next steps:"
echo "  1. git add $OUTDIR/genesis.json $OUTDIR/seed-info.txt"
echo "  2. git commit -m 'Add $CHAIN_ID genesis'"
echo "  3. git push"
echo "  4. Deploy your validator (see NETWORK_LAUNCH_GUIDE.md Part 2)"
echo "  5. Point DNS: $SEED_DOMAIN → your server IP"

#!/usr/bin/env bash
#
# Generates a complete 3-validator testnet configuration.
# Output: testnet/ directory with genesis.json and per-node config + data dirs.
#
# Usage: ./scripts/gen-testnet.sh [output_dir]
#
set -euo pipefail

OUTDIR="${1:-testnet}"
NUM_VALIDATORS=3
CHAIN_ID="drana-testnet-1"
BASE_P2P_PORT=26600
BASE_RPC_PORT=26657

echo "Generating $NUM_VALIDATORS-validator testnet in $OUTDIR/"
rm -rf "$OUTDIR"
mkdir -p "$OUTDIR"

# Generate keypairs.
declare -a ADDRESSES PUBKEYS PRIVKEYS

for i in $(seq 1 $NUM_VALIDATORS); do
    output=$(go run ./cmd/drana-cli keygen 2>&1)
    addr=$(echo "$output" | grep "Address:" | awk '{print $2}')
    pubkey=$(echo "$output" | grep "Public Key:" | awk '{print $3}')
    privkey=$(echo "$output" | grep "Private Key:" | awk '{print $3}')
    ADDRESSES+=("$addr")
    PUBKEYS+=("$pubkey")
    PRIVKEYS+=("$privkey")
    echo "  Validator $i: $addr"
done

# Build peer endpoints JSON fragment.
PEERS="{"
for i in $(seq 1 $NUM_VALIDATORS); do
    idx=$((i - 1))
    port=$((BASE_P2P_PORT + i))
    if [ "$i" -gt 1 ]; then PEERS+=","; fi
    # Use container hostnames for docker, localhost for bare metal.
    PEERS+="\"validator-$i\":\"validator-$i:$port\""
done
PEERS+="}"

# Build genesis accounts.
ACCOUNTS="["
for i in $(seq 1 $NUM_VALIDATORS); do
    idx=$((i - 1))
    if [ "$i" -gt 1 ]; then ACCOUNTS+=","; fi
    ACCOUNTS+="{\"address\":\"${ADDRESSES[$idx]}\",\"balance\":1000000000000}"
done
ACCOUNTS+="]"

# Build genesis validators.
VALIDATORS="["
for i in $(seq 1 $NUM_VALIDATORS); do
    idx=$((i - 1))
    if [ "$i" -gt 1 ]; then VALIDATORS+=","; fi
    VALIDATORS+="{\"address\":\"${ADDRESSES[$idx]}\",\"pubKey\":\"${PUBKEYS[$idx]}\",\"name\":\"validator-$i\",\"stake\":1000000000}"
done
VALIDATORS+="]"

# Write genesis.json.
cat > "$OUTDIR/genesis.json" <<EOF
{
  "chainId": "$CHAIN_ID",
  "genesisTime": $(date +%s),
  "accounts": $ACCOUNTS,
  "validators": $VALIDATORS,
  "maxPostLength": 280,
  "maxPostBytes": 1024,
  "minPostCommitment": 1000000,
  "minBoostCommitment": 100000,
  "maxTxPerBlock": 100,
  "maxBlockBytes": 1048576,
  "blockIntervalSec": 10,
  "blockReward": 10000000,
  "minStake": 1000000000,
  "epochLength": 30,
  "unbondingPeriod": 30,
  "slashFractionDoubleSign": 5,
  "postFeePercent": 6,
  "boostFeePercent": 6,
  "boostBurnPercent": 3,
  "boostAuthorPercent": 2,
  "boostStakerPercent": 1
}
EOF

echo "  Wrote $OUTDIR/genesis.json"

# Write per-node configs.
for i in $(seq 1 $NUM_VALIDATORS); do
    idx=$((i - 1))
    p2p_port=$((BASE_P2P_PORT + i))
    rpc_port=$((BASE_RPC_PORT + i - 1))
    node_dir="$OUTDIR/validator-$i"
    mkdir -p "$node_dir/data"

    # For node configs, use docker hostnames.
    cat > "$node_dir/config.json" <<EOF
{
  "genesisPath": "/data/genesis.json",
  "dataDir": "/data/db",
  "privKeyHex": "${PRIVKEYS[$idx]}",
  "listenAddr": "0.0.0.0:$p2p_port",
  "advertiseAddr": "validator-$i:$p2p_port",
  "rpcListenAddr": "0.0.0.0:$rpc_port",
  "peerEndpoints": $PEERS
}
EOF

    # Also write a bare-metal config using localhost.
    PEERS_LOCAL="{"
    for j in $(seq 1 $NUM_VALIDATORS); do
        local_port=$((BASE_P2P_PORT + j))
        if [ "$j" -gt 1 ]; then PEERS_LOCAL+=","; fi
        PEERS_LOCAL+="\"validator-$j\":\"127.0.0.1:$local_port\""
    done
    PEERS_LOCAL+="}"

    cat > "$node_dir/config.local.json" <<EOF
{
  "genesisPath": "$OUTDIR/genesis.json",
  "dataDir": "$node_dir/data",
  "privKeyHex": "${PRIVKEYS[$idx]}",
  "listenAddr": "127.0.0.1:$p2p_port",
  "advertiseAddr": "127.0.0.1:$p2p_port",
  "rpcListenAddr": "127.0.0.1:$rpc_port",
  "peerEndpoints": $PEERS_LOCAL
}
EOF

    echo "  Wrote $node_dir/config.json (docker) and config.local.json (bare metal)"
done

echo ""
echo "Testnet generated successfully."
echo ""
echo "To run with Docker:    docker compose up"
echo "To run bare metal:     See MINERS_GUIDE.md"
echo ""
echo "RPC endpoints will be available at:"
for i in $(seq 1 $NUM_VALIDATORS); do
    rpc_port=$((BASE_RPC_PORT + i - 1))
    echo "  validator-$i: http://localhost:$rpc_port/v1/node/info"
done

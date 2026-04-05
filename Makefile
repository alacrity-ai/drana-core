.PHONY: build test clean proto testnet docker docker-up docker-down docker-logs \
       docker-logs-validators docker-logs-indexer run-local run-indexer stop-local \
       init-network mainnet-run mainnet-indexer swagger-ui cli info help

# ─── Build ───────────────────────────────────────────────────────────

build: ## Build all binaries
	go build -o bin/drana-node ./cmd/drana-node
	go build -o bin/drana-cli ./cmd/drana-cli
	go build -o bin/drana-indexer ./cmd/drana-indexer

cli: ## Build only the CLI
	go build -o bin/drana-cli ./cmd/drana-cli

# ─── Test ────────────────────────────────────────────────────────────

test: ## Run all tests
	go test ./... -timeout 180s

test-unit: ## Run unit tests only (no integration)
	go test ./internal/... -timeout 60s

test-race: ## Run all tests with race detector
	go test ./... -race -timeout 240s

# ─── Code Generation ─────────────────────────────────────────────────

swagger-ui: ## Download Swagger UI assets
	./scripts/download-swagger-ui.sh

proto: ## Regenerate protobuf code (requires protoc + plugins)
	protoc --proto_path=internal/proto \
		--go_out=internal/proto/pb --go_opt=paths=source_relative \
		--go-grpc_out=internal/proto/pb --go-grpc_opt=paths=source_relative \
		internal/proto/types.proto internal/proto/consensus.proto

# ─── Local Testnet ───────────────────────────────────────────────────

testnet: build ## Generate a fresh 3-validator testnet config
	./scripts/gen-testnet.sh testnet

# ─── Mainnet ─────────────────────────────────────────────────────────

init-network: build ## Generate mainnet genesis + keys (run once, see NETWORK_LAUNCH_GUIDE.md)
	@if [ -f networks/mainnet/genesis.json ]; then \
		echo "networks/mainnet/genesis.json already exists."; \
		echo "Delete networks/mainnet/ to regenerate."; \
		exit 1; \
	fi
	@echo "Usage:"
	@echo "  ./scripts/init-network.sh --chain-id drana-mainnet-1 --seed-domain genesis-validator.drana.io"

mainnet-run: build ## Run your mainnet validator locally (requires init-network first)
	@if [ ! -f networks/mainnet/node-config.json ]; then \
		echo "Error: networks/mainnet/node-config.json not found."; \
		echo "Run: ./scripts/init-network.sh --chain-id <id> --seed-domain <domain>"; \
		exit 1; \
	fi
	bin/drana-node -config networks/mainnet/node-config.json

mainnet-indexer: build ## Run mainnet indexer locally (SQLite)
	@if [ ! -f networks/mainnet/genesis.json ]; then \
		echo "Error: no mainnet genesis. Run init-network first."; \
		exit 1; \
	fi
	bin/drana-indexer \
		-rpc http://localhost:26657 \
		-db networks/mainnet/indexer.db \
		-listen :26680 \
		-poll 5s

# ─── Docker (Testnet — throwaway keys for local development) ─────────

docker: ## Build Docker image
	docker build -t drana-node .

docker-up: testnet ## Start testnet: 3 validators + indexer + Postgres (local dev)
	docker compose up --build -d
	@echo ""
	@echo "Services running:"
	@echo "  Web App:          http://localhost:3000"
	@echo "  Validator 1 RPC:  http://localhost:26657       Swagger: http://localhost:26657/docs"
	@echo "  Validator 2 RPC:  http://localhost:26658"
	@echo "  Validator 3 RPC:  http://localhost:26659"
	@echo "  Indexer API:      http://localhost:26680       Swagger: http://localhost:26680/docs"
	@echo "  Postgres:         localhost:5432 (drana/drana)"
	@echo ""
	@echo "Try: curl -s http://localhost:26657/v1/node/info | jq"

docker-down: ## Stop all Docker Compose services and remove volumes
	docker compose down -v

docker-logs: ## Tail logs from all services
	docker compose logs -f

docker-logs-validators: ## Tail logs from validators only
	docker compose logs -f validator-1 validator-2 validator-3

docker-logs-indexer: ## Tail logs from indexer only
	docker compose logs -f indexer

# ─── Dev Helpers ──────────────────────────────────────────────────────

test-live: ## Run live test against running testnet (requires make docker-up first)
	go test ./test/live/ -v -timeout 600s

fund: build ## Send DRANA to a wallet. Usage: make fund TO=drana1... AMOUNT=2000
	@if [ -z "$(TO)" ]; then echo "Usage: make fund TO=drana1... AMOUNT=2000"; exit 1; fi
	@AMOUNT_MICRO=$$(( $${AMOUNT:-100} * 1000000 )); \
	KEY=$$(python3 -c "import json; print(json.load(open('testnet/validator-1/config.local.json'))['privKeyHex'])"); \
	bin/drana-cli transfer --key $$KEY --to $(TO) --amount $$AMOUNT_MICRO --rpc http://localhost:26657

# ─── Bare Metal (Testnet) ─────────────────────────────────────────────

run-local: testnet ## Run testnet: 3 validators locally (background)
	@echo "Starting 3 validators..."
	@bin/drana-node -config testnet/validator-1/config.local.json &
	@bin/drana-node -config testnet/validator-2/config.local.json &
	@bin/drana-node -config testnet/validator-3/config.local.json &
	@echo ""
	@echo "Validators started. RPC on ports 26657-26659."
	@echo "Swagger: http://localhost:26657/docs"
	@echo ""
	@echo "Run 'make run-indexer' to start the indexer."
	@echo "Run 'make stop-local' to stop everything."

run-indexer: build ## Run the indexer locally (SQLite, against localhost:26657)
	bin/drana-indexer \
		-rpc http://localhost:26657 \
		-db indexer.db \
		-listen :26680 \
		-poll 5s

stop-local: ## Stop locally running validators and indexer
	@pkill -f "drana-node" 2>/dev/null || true
	@pkill -f "drana-indexer" 2>/dev/null || true
	@echo "Stopped."

# ─── Frontend ─────────────────────────────────────────────────────────

web-install: ## Install frontend dependencies
	cd drana-app && npm install

web-dev: ## Start frontend dev server (hot reload, proxies to local chain)
	cd drana-app && npm run dev

web-build: ## Build frontend for production
	cd drana-app && npm run build

# ─── Clean ───────────────────────────────────────────────────────────

clean: ## Remove build artifacts, testnet data, and indexer DBs
	rm -rf bin/ testnet/ indexer.db
	rm -rf networks/mainnet/data/ networks/mainnet/indexer.db
	go clean

# ─── Info ─────────────────────────────────────────────────────────────

info: ## Show all service URLs and ports
	@echo ""
	@echo "  DRANA Services"
	@echo "  ─────────────────────────────────────────────────────────────"
	@echo ""
	@echo "  Web App            http://localhost:3000"
	@echo ""
	@echo "  Node RPC"
	@echo "    Validator 1      http://localhost:26657          API docs: http://localhost:26657/docs"
	@echo "    Validator 2      http://localhost:26658"
	@echo "    Validator 3      http://localhost:26659"
	@echo ""
	@echo "  Indexer API        http://localhost:26680          API docs: http://localhost:26680/docs"
	@echo ""
	@echo "  PostgreSQL         localhost:5432                  user: drana  pass: drana  db: drana_indexer"
	@echo ""
	@echo "  P2P (gRPC)"
	@echo "    Validator 1      localhost:26601"
	@echo "    Validator 2      localhost:26602"
	@echo "    Validator 3      localhost:26603"
	@echo ""
	@echo "  Web Proxies (via nginx at :3000)"
	@echo "    Node RPC         http://localhost:3000/api/node/v1/..."
	@echo "    Indexer          http://localhost:3000/api/indexer/v1/..."
	@echo ""
	@echo "  Quick checks:"
	@echo "    curl -s http://localhost:26657/v1/node/info | jq"
	@echo "    curl -s http://localhost:26680/v1/feed?strategy=trending | jq"
	@echo "    curl -s http://localhost:3000/api/node/v1/node/info | jq"
	@echo ""

# ─── Help ────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

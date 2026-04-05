# SWAGGER_IMPLEMENTATION.md

## OpenAPI / Swagger Documentation

### Goal

Serve interactive API documentation directly from the running node and indexer. Any developer hitting `http://genesis-validator.drana.io:26657/docs` sees a live Swagger UI they can use to explore and test every endpoint.

### Approach

Go doesn't have a decorator/annotation system like Node's `tsoa` or Python's FastAPI. The two viable approaches are:

1. **Spec-first:** Write an `openapi.yaml` by hand, serve it with embedded Swagger UI. The spec is the source of truth; handlers stay as-is.
2. **Code-first with annotations:** Use `swaggo/swag` to generate OpenAPI from comment annotations on handlers.

**Recommendation: spec-first.** Our handler count is small (14 node + 9 indexer = 23 endpoints), the response types are already well-defined in `rpc/types.go` and `indexer/types.go`, and a hand-written spec avoids a build-time codegen dependency. The spec lives in the repo, is human-readable, and can be edited by anyone.

### Endpoints to Document

**Node RPC (14 endpoints on port 26657):**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/node/info` | Chain status, supply, epoch info |
| GET | `/v1/blocks/latest` | Latest finalized block |
| GET | `/v1/blocks/{height}` | Block by height |
| GET | `/v1/blocks/hash/{hash}` | Block by header hash |
| GET | `/v1/accounts/{address}` | Account balance, stake, name |
| GET | `/v1/accounts/{address}/unbonding` | Pending unbonding entries |
| GET | `/v1/accounts/name/{name}` | Account lookup by registered name |
| GET | `/v1/posts` | Paginated post list (filterable by author, channel) |
| GET | `/v1/posts/{id}` | Single post by ID |
| POST | `/v1/transactions` | Submit a signed transaction |
| GET | `/v1/transactions/{hash}` | Transaction by hash |
| GET | `/v1/transactions/{hash}/status` | Transaction status (confirmed/pending/unknown) |
| GET | `/v1/network/validators` | Active validator set with stakes |
| GET | `/v1/network/peers` | Connected peers |

**Indexer API (9 endpoints on port 26680):**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/feed` | Ranked post feed (trending/top/new/controversial, filterable by channel) |
| GET | `/v1/feed/author/{address}` | Ranked posts by author |
| GET | `/v1/channels` | Channel list with post counts |
| GET | `/v1/posts/{id}` | Enriched post with derived fields |
| GET | `/v1/posts/{id}/boosts` | Boost history for a post |
| GET | `/v1/posts/{id}/replies` | Replies to a post |
| GET | `/v1/authors/{address}` | Author profile |
| GET | `/v1/stats` | Global chain statistics |
| GET | `/v1/leaderboard` | Top authors by boosts received |

---

## Implementation Steps

### Step 1 — Write the OpenAPI Specs

**`docs/openapi/node-rpc.yaml`**

A complete OpenAPI 3.0 spec for the node RPC server. Includes:
- All 14 endpoints with paths, methods, parameters, request bodies, and responses
- Schema definitions for every response type (`NodeInfoResponse`, `BlockResponse`, `AccountResponse`, `PostResponse`, `PostListResponse`, `SubmitTxRequest`, `SubmitTxResponse`, `TransactionResponse`, `TxStatusResponse`, `ValidatorResponse`, `PeerResponse`, `UnbondingResponse`, `ErrorResponse`)
- Enum values for transaction types (`transfer`, `create_post`, `boost_post`, `register_name`, `stake`, `unstake`)
- Parameter descriptions with formats (hex strings noted as 64-char hex, addresses as `drana1...` format)
- Example values in responses

**`docs/openapi/indexer.yaml`**

A complete OpenAPI 3.0 spec for the indexer API. Includes:
- All 9 endpoints
- Schema definitions for indexer-specific types (`FeedResponse`, `RankedPost`, `IndexedPost`, `IndexedBoost`, `BoostHistoryResponse`, `AuthorProfile`, `StatsResponse`, `LeaderboardResponse`, `ChannelInfo`)
- Enum values for ranking strategies (`trending`, `top`, `new`, `controversial`)
- Pagination parameters documented consistently

### Step 2 — Embed Swagger UI

Use Go's `embed` package (Go 1.16+) to bundle the Swagger UI static files directly into the binary. No external file serving needed.

**`internal/rpc/docs.go`** (new)

```go
package rpc

import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed swagger-ui/*
var swaggerUI embed.FS

//go:embed openapi.yaml
var openapiSpec []byte

func (s *Server) registerDocs(mux *http.ServeMux) {
    // Serve the OpenAPI spec.
    mux.HandleFunc("/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/yaml")
        w.Write(openapiSpec)
    })

    // Serve Swagger UI.
    uiFS, _ := fs.Sub(swaggerUI, "swagger-ui")
    mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.FS(uiFS))))

    // Redirect /docs to /docs/
    mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
        http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
    })
}
```

**Same pattern for the indexer** in `internal/indexer/docs.go`.

### Step 3 — Bundle Swagger UI Assets

Download the Swagger UI dist files (HTML, JS, CSS) and place them in the embed directory:

```
internal/rpc/swagger-ui/
    index.html          ← customized to point at /docs/openapi.yaml
    swagger-ui.css
    swagger-ui-bundle.js
    swagger-ui-standalone-preset.js
    favicon-32x32.png
```

The `index.html` is a minimal file that loads Swagger UI and points it at the local spec:

```html
<!DOCTYPE html>
<html>
<head>
    <title>DRANA Node RPC</title>
    <link rel="stylesheet" href="swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="swagger-ui-bundle.js"></script>
    <script src="swagger-ui-standalone-preset.js"></script>
    <script>
        SwaggerUIBundle({
            url: "/docs/openapi.yaml",
            dom_id: '#swagger-ui',
            presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
            layout: "StandaloneLayout"
        });
    </script>
</body>
</html>
```

The same approach for the indexer at its own `/docs` path.

### Step 4 — Wire Into Servers

**`internal/rpc/server.go`**

In `NewServer`, add:

```go
s.registerDocs(mux)
```

This adds the `/docs` and `/docs/openapi.yaml` routes alongside the API routes.

**`internal/indexer/api.go`**

Same: register docs routes in `NewAPIServer`.

### Step 5 — Download and Vendor Swagger UI

Create a `scripts/download-swagger-ui.sh` that:

1. Downloads the latest Swagger UI release from GitHub
2. Extracts the `dist/` files
3. Copies them to `internal/rpc/swagger-ui/` and `internal/indexer/swagger-ui/`
4. Patches `index.html` to point at the local spec URL

Add this to the Makefile:

```makefile
swagger-ui: ## Download Swagger UI assets
    ./scripts/download-swagger-ui.sh
```

The vendored files are committed to the repo so builds don't require internet access.

### Step 6 — Add to Makefile and Dockerfile

**Makefile:**

```makefile
docs: swagger-ui  ## Generate and bundle API docs
```

**Dockerfile:**

No change needed — `go:embed` includes the files at compile time. The Swagger UI is part of the binary.

---

## Result

After implementation:

```
http://genesis-validator.drana.io:26657/docs     → Node RPC Swagger UI
http://genesis-validator.drana.io:26657/docs/openapi.yaml  → Raw spec

http://indexer.drana.io:26680/docs               → Indexer Swagger UI
http://indexer.drana.io:26680/docs/openapi.yaml  → Raw spec
```

Any developer can:
- Browse all endpoints interactively
- See request/response schemas with examples
- Try API calls directly from the browser (Swagger UI "Try it out")
- Download the spec to generate client libraries (`openapi-generator`)

---

## Files Created/Modified

| File | Change |
|------|--------|
| `docs/openapi/node-rpc.yaml` | New: full OpenAPI 3.0 spec for node RPC (14 endpoints) |
| `docs/openapi/indexer.yaml` | New: full OpenAPI 3.0 spec for indexer API (9 endpoints) |
| `internal/rpc/docs.go` | New: embed spec + Swagger UI, register `/docs` routes |
| `internal/rpc/swagger-ui/` | New: vendored Swagger UI static assets |
| `internal/rpc/openapi.yaml` | New: embedded copy of the spec (symlink or copy from docs/) |
| `internal/indexer/docs.go` | New: same pattern for indexer |
| `internal/indexer/swagger-ui/` | New: vendored Swagger UI static assets |
| `internal/indexer/openapi.yaml` | New: embedded copy of the spec |
| `internal/rpc/server.go` | Modified: call `registerDocs(mux)` |
| `internal/indexer/api.go` | Modified: call `registerDocs(mux)` |
| `scripts/download-swagger-ui.sh` | New: downloads and vendors Swagger UI |
| `Makefile` | Modified: add `swagger-ui` and `docs` targets |

---

## Why Not swaggo/swag (Code-First)

`swaggo/swag` generates OpenAPI from Go comment annotations like:

```go
// @Summary Get node info
// @Tags chain
// @Produce json
// @Success 200 {object} NodeInfoResponse
// @Router /v1/node/info [get]
```

Pros: spec stays in sync with code automatically.
Cons:
- Requires a `swag init` build step that parses Go AST
- Annotation syntax is its own DSL to learn
- Generated specs are often messier than hand-written ones
- Adds a build dependency

For 23 endpoints with stable, well-defined types, a hand-written spec is cleaner and faster to ship. If the endpoint count grows past ~50, revisit `swaggo/swag`.

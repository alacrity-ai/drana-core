# Build stage
FROM golang:1.25.0-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/drana-node ./cmd/drana-node
RUN CGO_ENABLED=0 go build -o /bin/drana-cli ./cmd/drana-cli
RUN CGO_ENABLED=0 go build -o /bin/drana-indexer ./cmd/drana-indexer

# Runtime stage
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /bin/drana-node /usr/local/bin/
COPY --from=builder /bin/drana-cli /usr/local/bin/
COPY --from=builder /bin/drana-indexer /usr/local/bin/

VOLUME ["/data"]
EXPOSE 26601 26657

ENTRYPOINT ["drana-node"]
CMD ["-config", "/data/config.json"]

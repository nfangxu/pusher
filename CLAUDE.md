# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build server binary
go build -o server ./cmd/server

# Build stress test binary
go build -o stress ./cmd/stress

# Run server (default: :8080)
./server

# Run tests
go test ./...

# Run a single test
go test ./internal/config/ -run TestLoad

# Stress test (requires running server)
./stress connect --num 1000 --salt your-salt
./stress push --num 100 --salt your-salt --token your-push-token
./stress stability --num 1000 --duration 1m --salt your-salt --token your-push-token
```

## Architecture

Go SSE push notification service using Gin framework. Single binary, sharded in-memory registry, async push queue.

### Request Flow

1. **Client connects** via `GET /sse/connect?token=...` → `internal/handler/sse.go` validates token, registers connection in sharded registry, holds HTTP connection open with SSE headers and heartbeat loop
2. **Backend pushes** via `POST /push` (Bearer auth) → `internal/handler/push.go` enqueues to buffered channel → `internal/push/push.go` worker pool fans out to matched connections

### Key Packages

- **`cmd/server/`** — Entry point, wires config → registry → handler → router, graceful shutdown
- **`cmd/stress/`** — Stress test CLI with `connect`, `push`, `stability` subcommands
- **`internal/config/`** — YAML config with env var overrides via `envconfig`
- **`internal/handler/`** — HTTP handlers: `sse.go` (SSE connect + heartbeat), `push.go` (push API), `health.go`
- **`internal/push/`** — Async push queue with worker pool goroutines
- **`internal/registry/`** — Sharded connection registry (FNV-32a hash on channel name, per-shard RWMutex)
- **`internal/router/`** — Gin route setup with token auth middleware
- **`internal/token/`** — JWT-like token: `base64(channel=...&group=...&uuid=...&ts=...&sign=md5({channel}{group}{uuid}{ts}{salt}))`
- **`internal/log/`** — Zap logger with lumberjack rotation

### Push Target Wildcards

| Pattern | Match |
|---------|-------|
| `*` | All connections |
| `{channel}:*` | All in channel |
| `{channel}:{group}:*` | All in channel+group |
| `{channel}:{group}:{uuid}` | Single connection |

### Connection Identity

Each connection is identified by `UserKey = "{channel}:{group}:{uuid}"`. Reconnecting with the same UserKey replaces the old connection.

## Config

Edit `config.yaml` (or set env vars `PUSHER_SERVER_PORT`, `PUSHER_SERVER_SALT`, etc.). Key fields: `server.salt` (token signing), `server.push_token` (push API auth), `push.worker_num`, `push.shard_num`, `server.rate_limit`.

## Code Conventions

- Go module: `pusher` (Go 1.23)
- Chinese log messages (zap structured logging)
- No comments unless the "why" is non-obvious
- Panic recovery via `defer recover()` on all Write/Flush operations to connections (closed connections panic)
- SSE data lines must not contain newlines — use `json.Compact` on message payloads before embedding
- Heartbeat resets connection timeout timer via channel signal (not a one-shot timer)
- Fan-out: <=100 connections sequential, >100 concurrent (configurable via `fan_out_workers`, default 200)
- json.Compact shared once across all connections in a fan-out batch

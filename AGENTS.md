# AGENTS.md

Instructions for agentic coding agents working in this repository.

## Project Overview

PayLock is a **decentralized video storage infrastructure** for Sui. It manages video uploads to **Walrus** and provides a redirection layer for streaming.

**Current State (v2 Alpha):**

- Video uploads are stored directly on Walrus via the Publisher API.
- Streaming is handled via HTTP 307 redirects to the Walrus Aggregator.
- FFmpeg processing is optional. When disabled, paid uploads are rejected to avoid leaking full videos as previews.

## Build Commands

```bash
make run          # Run dev server (go run ./cmd/paylock)
make build        # Compile to bin/paylock
make test         # Run all tests with race detector and coverage
make lint         # go vet ./...
make clean        # Remove bin/ and temporary build artifacts
```

### Running a Single Test

```bash
# Single test file
go test ./internal/middleware/ -v

# Single test function
go test ./internal/walrus/... -run TestStore -v
```

## Prerequisites

- Go 1.25+
- (Optional) `ffmpeg` and `ffprobe` when enabling server-side preview/thumbnail processing

## Code Style Guidelines

### General

- No external dependencies beyond Go standard library (except essential ecosystem tools like `godotenv`)
- Use `log/slog` for structured logging (not `log`)
- Prefer early returns to reduce nesting
- Keep functions focused and small

### Naming Conventions

- **Packages**: lowercase, single word (e.g., `handler`, `walrus`)
- **Types**: PascalCase (e.g., `VideoStore`, `Upload`, `Client`)
- **Functions**: PascalCase for exported, camelCase for unexported
- **Interfaces**: noun-based, singular (e.g., `Uploader`, not `UploadManager`)
- **Constants**: PascalCase for exported, camelCase for unexported
- **Variables**: camelCase; use short names for local scope

### Imports

Group imports in this order:

1. Standard library (no prefix)
2. Third-party packages (empty line before)
3. Internal packages with full import path (empty line before)

```go
import (
    "context"
    "net/http"

    "github.com/joho/godotenv"

    "github.com/anthropics/paylock/internal/walrus"
)
```

### Error Handling

- Define sentinel errors with `errors.New` or `fmt.Errorf` with `%w`
- Wrap errors with context: `fmt.Errorf("action: %w", err)`
- Return errors early; avoid `else` after error checks

### Structs and Types

- Use struct tags for JSON serialization (e.g., `json:"blob_id"`)
- Use `sync.RWMutex` for concurrent access to in-memory state

### HTTP Handlers

- Implement `http.Handler` interface (ServeHTTP method)
- Set headers before writing status code
- Use `http.Redirect` for external storage hand-off (Walrus Aggregator)

### Context and Concurrency

- Use goroutines for non-blocking I/O (e.g., uploading to Walrus)
- Pass `context.Context` to all network-related functions

## Environment Variables

| Var | Default | Description |
|-----|---------|-------------|
| `PAYLOCK_PORT` | `8080` | HTTP listen port |
| `PAYLOCK_WALRUS_PUBLISHER_URL` | `https://publisher.walrus-testnet.walrus.space` | Walrus Publisher API |
| `PAYLOCK_WALRUS_AGGREGATOR_URL` | `https://aggregator.walrus-testnet.walrus.space` | Walrus Aggregator API |
| `PAYLOCK_WALRUS_EPOCHS` | `5` | Default storage duration in epochs |
| `PAYLOCK_MAX_FILE_SIZE_MB` | `500` | Upload size limit in MB |
| `PAYLOCK_ENABLE_FFMPEG` | `true` | Enable FFmpeg processing for preview/thumbnail |
| `PAYLOCK_GATING_PACKAGE_ID` | `0xec50faf6c1bb5720d7744476282a7b22600254de3ed849808ff9aacef8ba161a` | Deployed gating Move package ID on Sui |

## Directory Structure

```
cmd/paylock/main.go          — Entry point; wires all handlers and clients
internal/config/          — Environment loading and validation
internal/model/           — Data models (Video, VideoStore)
internal/walrus/          — Walrus Publisher/Aggregator client
internal/handler/         — HTTP handlers (upload, status, stream redirect)
internal/middleware/      — APIKey auth + CORS middleware
internal/processor/       — (Legacy/Future) FFmpeg wrappers
```

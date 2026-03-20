# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make run          # Run dev server (go run ./cmd/orca)
make build        # Compile to bin/orca
make test         # Run all tests with race detector and coverage
make lint         # go vet ./...
make clean        # Remove bin/

# Run a single test
go test ./internal/processor/... -run TestValidateMagicBytes -v
```

## Environment Variables

| Var | Default | Description |
|-----|---------|-------------|
| `ORCA_PORT` | `8080` | HTTP listen port |
| `ORCA_WALRUS_PUBLISHER_URL` | `https://publisher.walrus-testnet.walrus.space` | Walrus publisher endpoint |
| `ORCA_WALRUS_AGGREGATOR_URL` | `https://aggregator.walrus-testnet.walrus.space` | Walrus aggregator endpoint |
| `ORCA_WALRUS_EPOCHS` | `5` | Number of storage epochs to pay for |
| `ORCA_MAX_FILE_SIZE_MB` | `500` | Upload size limit in MB |
| `ORCA_PAYWALL_PACKAGE_ID` | *(none)* | Deployed paywall Move package ID on Sui |
| `ORCA_SUI_RPC_URL` | `https://fullnode.testnet.sui.io:443` | Sui JSON-RPC endpoint |

## Architecture

Orca is a video upload service that stores MP4 files on Walrus decentralized storage (Sui). It accepts MP4 uploads, validates them, uploads directly to Walrus, and serves them via the Walrus aggregator.

```
cmd/orca/main.go          — wires all packages; route groups:
                            POST /api/upload, GET /api/status/{id}
                            GET /api/videos, DELETE /api/videos/{id}
                            PUT /api/videos/{id}/sui-object, GET /api/config
                            GET /stream/{id}                        → redirects to Walrus

internal/config/          — env-based config
internal/model/           — in-memory VideoStore (sync.RWMutex); NOT persisted across restarts
internal/walrus/          — Walrus HTTP client (Store, BlobURL)
internal/processor/       — MP4 magic-byte validator + size validator
internal/handler/         — HTTP handlers; upload validates then async uploads to Walrus
internal/middleware/      — CORS middleware
```

### Key design decisions

- **Upload flow is async**: `POST /api/upload` validates the MP4 (magic bytes), returns `202 processing` immediately; a goroutine uploads to Walrus; poll `GET /api/status/{id}` for `ready`/`failed`.
- **No local file storage**: Videos go directly to Walrus. No FFmpeg, no HLS segmentation.
- **VideoStore is in-memory only**: Video metadata (including Walrus blob IDs) is lost on restart.
- **Stream endpoint redirects**: `GET /stream/{id}` returns a 307 redirect to the Walrus aggregator blob URL.
- **Only MP4 input is accepted**: Magic bytes check (`ftyp` at offset 4).
- **Frontend uses native `<video>`**: No HLS.js dependency. The browser plays MP4 directly from the Walrus aggregator URL.

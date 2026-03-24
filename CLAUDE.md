# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make run          # Run dev server (go run ./cmd/paylock)
make build        # Compile to bin/paylock
make test         # Run all tests with race detector and coverage
make lint         # go vet ./...
make clean        # Remove bin/

# Run a single test
go test ./internal/processor/... -run TestValidateMagicBytes -v
```

## Environment Variables

| Var | Default | Description |
|-----|---------|-------------|
| `PAYLOCK_PORT` | `8080` | HTTP listen port |
| `PAYLOCK_WALRUS_PUBLISHER_URL` | `https://publisher.walrus-testnet.walrus.space` | Walrus publisher endpoint |
| `PAYLOCK_WALRUS_AGGREGATOR_URL` | `https://aggregator.walrus-testnet.walrus.space` | Walrus aggregator endpoint |
| `PAYLOCK_WALRUS_EPOCHS` | `5` | Number of storage epochs to pay for |
| `PAYLOCK_DATA_DIR` | `data` | Local directory for persisted video metadata |
| `PAYLOCK_MAX_FILE_SIZE_MB` | `500` | Upload size limit in MB |
| `PAYLOCK_PAYWALL_PACKAGE_ID` | *(none)* | Deployed paywall Move package ID on Sui |
| `PAYLOCK_SUI_RPC_URL` | `https://fullnode.testnet.sui.io:443` | Sui JSON-RPC endpoint |

## Architecture

PayLock is a video upload service that stores MP4 files on Walrus decentralized storage (Sui). It accepts MP4 uploads, validates them, uploads directly to Walrus, and serves them via the Walrus aggregator.

```
cmd/paylock/main.go          — wires all packages; route groups:
                            POST /api/upload, GET /api/status/{id}
                            GET /api/videos, DELETE /api/videos/{id}
                            PUT /api/videos/{id}/sui-object, GET /api/config
                            GET /stream/{id}                        → redirects to Walrus

internal/config/          — env-based config
internal/model/           — VideoStore (sync.RWMutex + JSON file persistence in PAYLOCK_DATA_DIR)
internal/walrus/          — Walrus HTTP client (Store, BlobURL)
internal/processor/       — MP4 magic-byte validator + size validator
internal/handler/         — HTTP handlers; upload validates then async uploads to Walrus
internal/middleware/      — CORS middleware
```

### Key design decisions

- **Upload flow is async**: `POST /api/upload` validates the MP4 (magic bytes), returns `202 processing` immediately; a goroutine uploads to Walrus; poll `GET /api/status/{id}` for `ready`/`failed`.
- **Paid vs free upload split**: Free videos upload both blobs server-side. Paid videos upload only preview server-side; the frontend handles Seal encryption + Walrus upload of the full blob.
- **Seal encryption (Phase 2)**: Full blob of paid videos is encrypted with `@mysten/seal` in the browser. The flow: generate random 32-byte namespace → encrypt with Seal using namespace as prefix → upload encrypted blob to Walrus → create Video on-chain with blob IDs and namespace (single transaction).
- **Purchase flow**: User pays via `purchase_and_transfer` → mints AccessPass → Seal SessionKey + `seal_approve` tx → decrypt encrypted blob in browser → play via blob URL.
- **No local file storage**: Videos go directly to Walrus. No HLS segmentation.
- **VideoStore persists to disk**: Video metadata is saved as `videos.json` in `PAYLOCK_DATA_DIR` (default `data/`). Deleting the file only removes local records; Walrus blobs are unaffected.
- **Stream endpoint redirects**: `GET /stream/{id}` returns a 307 redirect to the Walrus aggregator blob URL.
- **Only MP4 input is accepted**: Magic bytes check (`ftyp` at offset 4).
- **Frontend uses native `<video>`**: No HLS.js dependency. The browser plays MP4 directly from the Walrus aggregator URL (or from decrypted blob URL for paid videos).

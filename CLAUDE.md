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
| `PAYLOCK_MAX_FILE_SIZE_MB` | `500` | Upload size limit in MB (free videos) |
| `PAYLOCK_MAX_PREVIEW_SIZE_MB` | `50` | Preview size limit in MB (paid videos) |
| `PAYLOCK_MAX_PREVIEW_DURATION` | `300` | Max preview duration in seconds (validated via ffprobe if FFmpeg available) |
| `PAYLOCK_ENABLE_FFMPEG` | `true` | Enable FFmpeg processing for free video preview/thumbnail |
| `PAYLOCK_GATING_PACKAGE_ID` | *(none)* | Deployed gating Move package ID on Sui |
| `PAYLOCK_SUI_RPC_URL` | `https://fullnode.testnet.sui.io:443` | Sui JSON-RPC endpoint |
| `PAYLOCK_ADMIN_SECRET` | *(none)* | Bearer token for admin endpoints (e.g. `POST /api/reindex`) |

## Architecture

PayLock is a video upload service that stores video files on Walrus decentralized storage (Sui). It accepts video uploads (MP4, MOV, WebM, MKV, AVI), validates them, uploads directly to Walrus, and serves them via the Walrus aggregator.

```
cmd/paylock/main.go          — wires all packages; route groups:
                            POST /api/upload, GET /api/status/{id}
                            GET /api/videos
                            DELETE /api/videos/{id}
                            GET /api/config
                            POST /api/reindex
                            GET /stream/{id}/preview → canonical: sui_object_id; legacy: paylock_id (307 redirect)
                            GET /stream/{id}/full
                            GET /stream/{id}         → deprecated, redirects to /preview

internal/config/          — env-based config
internal/model/           — VideoStore (sync.RWMutex + JSON file persistence + sui_object_id secondary index)
internal/walrus/          — Walrus HTTP client (Store, BlobURL)
internal/processor/       — MP4/JPEG magic-byte validators + size validator + preview duration validator
internal/handler/         — HTTP handlers; upload validates then async uploads to Walrus
internal/indexer/         — Sui chain reindexer (scans on-chain Video objects via JSON-RPC)
internal/middleware/      — CORS middleware
```

### Key design decisions

- **Upload flow is async**: `POST /api/upload` validates the MP4 (magic bytes), returns `202 processing` immediately; a goroutine uploads to Walrus; poll `GET /api/status/{id}` for `ready`/`failed`.
- **Paid vs free upload split**: Free videos upload both blobs server-side. Paid videos: the frontend generates the preview locally (e.g., via `ffmpeg.wasm`) and sends only the preview clip + optional JPEG thumbnail to the server. The full unencrypted video never reaches the server. The frontend handles Seal encryption + Walrus upload of the full blob. FFmpeg/FFprobe is required on the server for paid uploads to validate preview duration.
- **Seal encryption (Phase 2)**: Full blob of paid videos is encrypted with `@mysten/seal` in the browser. The flow: generate random 32-byte namespace → encrypt with Seal using namespace as prefix → upload encrypted blob to Walrus → create Video on-chain with blob IDs and namespace (single transaction).
- **Purchase flow**: User pays via `purchase_and_transfer` → mints AccessPass → Seal SessionKey + `seal_approve` tx → decrypt encrypted blob in browser → play via blob URL.
- **No local file storage**: Videos go directly to Walrus. No HLS segmentation.
- **VideoStore persists to disk**: Video metadata is saved as `videos.json` in `PAYLOCK_DATA_DIR` (default `data/`). Deleting the file only removes local records; Walrus blobs are unaffected.
- **Canonical ID = sui_object_id**: External discovery and streaming use `sui_object_id`. `paylock_id` is a temporary internal workflow ID used during upload. Stream routes accessed by `paylock_id` redirect to the canonical `sui_object_id` URL when available, with `Deprecation` headers.
- **Chain reindexer**: On startup, the server scans the Sui chain for all `Video` objects created by the gating package and populates the VideoStore. If `videos.json` is missing, the store is rebuilt from chain state. `POST /api/reindex` triggers a manual reindex.
- **Stream endpoint redirects**: `GET /stream/{id}` returns a 307 redirect to the Walrus aggregator blob URL. Supports both `paylock_id` and `sui_object_id` lookups.
- **Supported video formats**: MP4, MOV (ftyp box), WebM, MKV (EBML header), AVI (RIFF/AVI). Validated by magic bytes, not file extension.
- **Frontend uses native `<video>`**: No HLS.js dependency. The browser plays MP4 directly from the Walrus aggregator URL (or from decrypted blob URL for paid videos).

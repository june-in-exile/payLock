# PayLock — Decentralized Video Storage & Paywall for Sui

## Introduction

PayLock is a backend service for decentralized video storage on Sui. It uploads video assets to **Walrus** via the Publisher API, serves playback via **HTTP 307 redirects** to the Walrus Aggregator, and syncs metadata with the **Sui gating contract**. It includes optional FFmpeg-based preview and thumbnail generation plus a simple REST API and embedded web UI.

## Current State (v2 Alpha)

- Video uploads are stored directly on Walrus via the Publisher API.
- Streaming is handled via HTTP 307 redirects to the Walrus Aggregator.
- FFmpeg processing is optional. When disabled, paid uploads are rejected to avoid leaking full videos as previews.

## Features

- Walrus Publisher integration for blob storage
- Walrus Aggregator redirects for playback (`/stream/*`)
- Optional FFmpeg preview + thumbnail extraction and faststart optimization
- Server-Sent Events (SSE) for status updates
- Sui chain watcher and reindexer to sync on-chain video objects
- Embedded frontend UI for uploads and playback

---

## Architecture

### Free Video Flow (price = 0)

1. Client uploads video to `POST /api/upload` with `price=0` or omitted.
2. Server optionally extracts preview/thumbnail with FFmpeg and uploads assets to Walrus.
3. Server marks the video `ready` and returns `preview_blob_id` and `full_blob_id`.
4. Playback via `GET /stream/{id}/preview` or `GET /stream/{id}/full` (307 redirect).

### Paid Video Flow (price > 0)

1. Client uploads full video to `POST /api/upload` with `price>0`.
2. Server extracts preview/thumbnail (FFmpeg required) and uploads preview to Walrus.
3. Client encrypts the full video (Seal) and uploads the encrypted blob to Walrus.
4. Client calls `gating::create_video` on-chain with `title`, `price`, `thumbnail_blob_id`, `preview_blob_id`, `full_blob_id`, and `seal_namespace`.
5. Watcher detects the `VideoCreated` event and links the on-chain object to the local record.
6. Server marks the video `ready`; playback via `/stream/{sui_object_id}/preview` or `/stream/{sui_object_id}/full`.

### Streaming & IDs

- `/stream/{id}/preview` and `/stream/{id}/full` accept both `paylock_id` and `sui_object_id`.
- If a `paylock_id` has an associated on-chain object, the server redirects to the canonical `sui_object_id` path and returns deprecation headers.
- `GET /stream/{id}` is deprecated and redirects to `/stream/{id}/preview`.

---

## On-Chain Contract

Contract source: `contracts/sources/gating.move`.

Key types and events:

- `Video` (shared object): title, price, creator, thumbnail/preview/full blob IDs, seal namespace
- `VideoCreated` event: emitted by `create_video` for backend sync
- `VideoDeleted` event: emitted by `delete_video` for backend cleanup
- `AccessPass` (owned object): proof of purchase for Seal decryption

---

## Quick Integration (Free Video)

```js
const PAYLOCK = 'http://localhost:8080';

// 1. Upload
const form = new FormData();
form.append('video', file);
const { id } = await fetch(`${PAYLOCK}/api/upload`, { method: 'POST', body: form }).then(r => r.json());

// 2. Wait for processing (SSE)
await new Promise((resolve, reject) => {
  const es = new EventSource(`${PAYLOCK}/api/status/${id}/events`);
  es.onmessage = (e) => {
    const v = JSON.parse(e.data);
    if (v.status === 'ready') { es.close(); resolve(); }
    if (v.status === 'failed') { es.close(); reject(new Error(v.error || 'failed')); }
  };
  es.onerror = () => { es.close(); reject(new Error('SSE error')); };
});

// 3. Play — browser auto-follows 307 redirect to Walrus
videoElement.src = `${PAYLOCK}/stream/${id}/preview`;
```

For paid flow details, see `API.md`.

---

## Self-Hosting

### Prerequisites

- Go 1.25+
- FFmpeg + FFprobe (required for paid uploads; recommended for previews/faststart)

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PAYLOCK_PORT` | `8080` | HTTP listen port |
| `PAYLOCK_DATA_DIR` | `data` | Metadata and local cache storage |
| `PAYLOCK_WALRUS_PUBLISHER_URL` | `https://publisher.walrus-testnet.walrus.space` | Walrus Publisher API |
| `PAYLOCK_WALRUS_AGGREGATOR_URL` | `https://aggregator.walrus-testnet.walrus.space` | Walrus Aggregator API |
| `PAYLOCK_WALRUS_EPOCHS` | `5` | Walrus storage duration in epochs |
| `PAYLOCK_MAX_FILE_SIZE_MB` | `500` | Upload size limit in MB |
| `PAYLOCK_MAX_PREVIEW_SIZE_MB` | `50` | Max preview size (MB) |
| `PAYLOCK_MAX_PREVIEW_DURATION` | `300` | Max preview duration (seconds) |
| `PAYLOCK_ENABLE_FFMPEG` | `true` | Enable FFmpeg processing |
| `PAYLOCK_FFMPEG_PATH` | `ffmpeg` | FFmpeg binary path |
| `PAYLOCK_FFPROBE_PATH` | `ffprobe` | FFprobe binary path |
| `PAYLOCK_SUI_RPC_URL` | `https://fullnode.testnet.sui.io:443` | Sui RPC endpoint |
| `PAYLOCK_GATING_PACKAGE_ID` | `0xec50...161a` | Deployed gating package ID |
| `PAYLOCK_ADMIN_SECRET` | (empty) | Bearer token for `/api/reindex` |
| `PAYLOCK_WATCHER_INTERVAL` | `5` | Chain watcher poll interval (seconds) |

### Start the Server

```bash
# 1. Copy and edit the environment variables
cp .env.example .env

# 2. Start the server
make run
```

---

## API Reference

See `API.md` for full API specs and the paid video integration guide.

---

## License

MIT

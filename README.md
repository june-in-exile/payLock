# PayLock — Decentralized Video Paywall SDK for Sui

## Introduction

PayLock is a video paywall SDK built on Walrus + Seal. When a video is uploaded, it is automatically split into a preview clip and a full version. The preview is publicly playable, while the full version is encrypted with Seal and stored on Walrus. After payment, viewers receive an on-chain access credential to decrypt and watch the full content. Developers can add video paywall functionality to any dApp with just a few lines of code.

## Features

- **Auto Thumbnail & Preview**: After uploading an MP4, the backend automatically extracts a thumbnail and an N-second preview, uploading both to Walrus in parallel.
- **Faststart Optimization**: Free videos are automatically processed with `faststart` to optimize streaming playback speed on Walrus.
- **Seal Encryption (Paid Videos)**: Paid videos are encrypted client-side using the Seal SDK, ensuring only AccessPass holders can decrypt and play them.
- **Single-Transaction Flow**: Optimized on-chain publishing — only one transaction is needed to create a Video object and set the encryption namespace (Seal Namespace).
- **Real-Time Status Tracking (SSE)**: Server-Sent Events provide real-time progress updates, from upload to extraction to Walrus storage status.
- **Frontend SPA**: Integrated with Slush Wallet, supporting price configuration, video listing, and player (preview → paywall → decrypted playback).

---

## Architecture

### Video Publishing Flow (Paid Videos)

To balance security and performance, PayLock uses a hybrid flow of "backend preprocessing + frontend encryption":

1. **Frontend Preview Generation + Backend Upload**:
   - Frontend generates a short preview clip (default 10s) using `MediaRecorder` (canvas + `captureStream`). External integrators can alternatively use `ffmpeg.wasm`.
   - Frontend uploads the preview (+ optional thumbnail) to `POST /api/upload`.
   - Backend validates the preview duration via `ffprobe` (if FFmpeg is available) against `PAYLOCK_MAX_PREVIEW_DURATION` (default 30s), then uploads to Walrus.
   - Notifies the frontend via `GET /api/status/{id}/events` (SSE) when the preview is ready.

2. **Frontend Encryption & Upload**:
   - Frontend generates a random `seal_namespace`.
   - Encrypts the original video using the Seal SDK, then uploads the encrypted blob to Walrus.

3. **On-Chain Publishing (TX)**:
   - Frontend initiates a transaction calling `create_video`.
   - Passes `price`, `preview_blob_id`, `full_blob_id` (encrypted), and `seal_namespace`.
   - After the transaction succeeds, calls `PUT /api/videos/{id}` to link the on-chain ID with the backend record.

### System Components

```text
[ Client / Frontend SPA ]
    │
    ▼
[ PayLock Backend (Go) ]
    ├── POST /api/upload           Validate MP4 → extract thumbnail/preview → upload to Walrus
    ├── GET /api/status/{id}/events SSE real-time progress tracking
    ├── GET /api/videos            List videos (with thumbnails and pricing)
    └── PUT /api/videos/{id}       Link on-chain object ID
    │
    ├──── Write ────▶ [ Walrus Publisher ] → [ Walrus Storage ]
    └──── Read  ────▶ [ Walrus Aggregator ] ← (streaming playback)

[ Sui Blockchain ]  ← contracts/sources/gating.move
    Video (Shared Object) / AccessPass (Owned Object) / Seal Policy
```

---

## On-Chain Contract Design

Contract located at `contracts/sources/gating.move`:

```move
module paylock::gating {
    /// Video info, created by the creator (shared object)
    public struct Video has key {
        id: UID,
        price: u64,                // Unlock price (MIST)
        creator: address,          // Payment recipient address
        preview_blob_id: String,   // Preview Walrus Blob ID
        full_blob_id: String,      // Full Walrus Blob ID (encrypted for paid videos)
        seal_namespace: vector<u8>,// Seal encryption namespace
    }

    /// Purchase credential, minted after payment, permanently valid (owned by buyer)
    public struct AccessPass has key, store {
        id: UID,
        video_id: ID,
    }

    /// Creator publishes video (single transaction)
    public fun create_video(
        price: u64,
        preview_blob_id: String,
        full_blob_id: String,
        seal_namespace: vector<u8>,
        ctx: &mut TxContext
    );

    /// User pays → mint AccessPass and transfer payment to creator
    entry fun purchase_and_transfer(video: &Video, payment: Coin<SUI>, ctx: &mut TxContext);

    /// Seal key server verifies decryption permission
    entry fun seal_approve(id: vector<u8>, pass: &AccessPass, video: &Video);
}
```

---

## Quick Integration (External Developers)

PayLock is a **self-hosted backend service** that handles video processing, Walrus storage, and stream routing. Your frontend only needs to call the REST API + a few on-chain transactions to integrate.

**Public Testnet Instance**: `https://paylock.up.railway.app`

```js
// Minimal integration example: upload free video → wait for ready → play
const PAYLOCK = 'https://paylock.up.railway.app';

// 1. Upload
const form = new FormData();
form.append('video', file);
const { id } = await fetch(`${PAYLOCK}/api/upload`, { method: 'POST', body: form }).then(r => r.json());

// 2. Wait for processing (SSE)
await new Promise((resolve) => {
  const es = new EventSource(`${PAYLOCK}/api/status/${id}/events`);
  es.onmessage = (e) => {
    if (JSON.parse(e.data).status === 'ready') { es.close(); resolve(); }
  };
});

// 3. Play — browser auto-follows 307 redirect to Walrus
videoElement.src = `${PAYLOCK}/stream/${id}/preview`;
```

For the full paid video integration flow, see [API.md — Paid Video Integration Guide](./API.md#paid-video-integration-guide).

---

## Self-Hosting

### Prerequisites

- Go 1.25+
- **FFmpeg** (required, checked at startup)

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PAYLOCK_PORT` | `8080` | HTTP listen port |
| `PAYLOCK_DATA_DIR` | `data` | Metadata and local cache storage path |
| `PAYLOCK_WALRUS_PUBLISHER_URL` | `...` | Walrus Publisher |
| `PAYLOCK_WALRUS_AGGREGATOR_URL` | `...` | Walrus Aggregator |
| `PAYLOCK_WALRUS_EPOCHS` | `5` | Walrus storage epochs |
| `PAYLOCK_SUI_RPC_URL` | `...` | Sui RPC (Testnet) |
| `PAYLOCK_GATING_PACKAGE_ID` | _(required)_ | Deployed contract Package ID |

### Start the Server

```bash
# 1. Copy and edit the environment variables
cp .env.example .env

# 2. Start the server
make run
```

---

## API Reference

PayLock provides a complete RESTful API with SSE event streams, enabling developers to integrate video paywall functionality into any video application.

For detailed API specs and integration guide, see: [**API.md — PayLock Infrastructure Specification**](./API.md)

### Core Endpoints Summary

1. **Upload Video**: `POST /api/upload` — Initiate async processing.
2. **Real-Time Tracking**: `GET /api/status/{id}/events` — Get processing progress via SSE.
3. **Link On-Chain Object**: `PUT /api/videos/{id}` — Sync on-chain object ID to backend.
4. **Preview Playback**: `GET /stream/{id}/preview` — 307 redirect to preview.
5. **System Config**: `GET /api/config` — Get contract and Walrus endpoint info.

---

## License

MIT

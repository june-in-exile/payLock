# PayLock — Decentralized Video Paywall SDK for Sui

## Introduction

PayLock is a video paywall SDK built on Walrus + Seal. When a video is uploaded, it is automatically split into a preview clip and a full version. The preview is publicly playable, while the full version is encrypted with Seal and stored on Walrus. After payment, viewers receive an on-chain access credential to decrypt and watch the full content. Developers can add video paywall functionality to any dApp with just a few lines of code.

## Features

- **Auto Thumbnail & Preview**: After uploading an MP4, the backend automatically extracts a thumbnail and an N-second preview, uploading both to Walrus in parallel.
- **Faststart Optimization**: Free videos are automatically processed with `faststart` to optimize streaming playback speed on Walrus.
- **Seal Encryption (Paid Videos)**: Paid videos are encrypted client-side using the Seal SDK, ensuring only AccessPass holders can decrypt and play them.
- **Automatic On-Chain Sync (Watcher)**: A background service (Watcher) automatically detects `VideoCreated` events on-chain and links them to local video records, eliminating the need for manual write-backs.
- **Real-Time Status Tracking (SSE)**: Server-Sent Events provide real-time progress updates, from upload to extraction to Walrus storage status, including automatic sync detection.
- **Frontend SPA**: Integrated with Slush Wallet, supporting price configuration, video listing, and player (preview → paywall → decrypted playback).

---

## Architecture

### Video Publishing Flow (Paid Videos)

To balance security and performance, PayLock uses a hybrid flow of "backend preprocessing + frontend encryption":

1. **Backend Preview Generation + Upload**:
   - Frontend uploads the **full video** to `POST /api/upload` with `price > 0`.
   - Backend **requires FFmpeg** to extract the preview clip + thumbnail, then uploads the preview to Walrus.
   - Notifies the frontend via `GET /api/status/{id}/events` (SSE) when `preview_blob_id` is ready.

2. **Frontend Encryption & Upload**:
   - Frontend generates a random `seal_namespace`.
   - Encrypts the original video using the Seal SDK, then uploads the encrypted blob to Walrus.

3. **On-Chain Publishing & Automatic Sync**:
   - Frontend initiates a transaction calling `create_video`, which emits a `VideoCreated` event.
   - Passes `price`, `preview_blob_id`, `full_blob_id` (encrypted), and `seal_namespace`.
   - **Watcher Service**: The PayLock backend polls for `VideoCreated` events. Once detected, it automatically links the `sui_object_id` to the local record using the `preview_blob_id` as a key.
   - Frontend simply waits for the video status to transition to `ready`.

### Wallet Signing Flow

Paid video uploads now require only **1 mandatory wallet interaction**:

| # | Type | When | Purpose |
|---|------|------|---------|
| 1 | **Sign & Execute Transaction** | After encryption & Walrus upload | On-chain publishing — calls `gating::create_video` to write video metadata and emit a `VideoCreated` event. |

Free video uploads require **no wallet signatures** (no on-chain interaction needed).

Identity verification is handled on-chain. The backend Watcher verifies the creator address from the emitted event before updating the local store.

### System Components

```text
[ Client / Frontend SPA ]
    │
    ▼
[ PayLock Backend (Go) ]
    ├── POST /api/upload           Validate preview → upload to Walrus
    ├── GET /api/status/{id}/events SSE real-time progress tracking
    ├── GET /api/videos            List videos
    └── [ Watcher Service ]        Polls Sui for VideoCreated events → Auto-link
    │
    ├──── Write ────▶ [ Walrus Publisher ] → [ Walrus Storage ]
    ├──── Read  ────▶ [ Walrus Aggregator ] ← (streaming playback)
    └──── Poll  ────▶ [ Sui Blockchain ]    ← (detect VideoCreated events)
```

---

## On-Chain Contract Design

Contract located at `contracts/sources/gating.move`:

```move
module paylock::gating {
    /// Video info, created by the creator (shared object)
    public struct Video has key {
        id: UID,
        price: u64,
        creator: address,
        preview_blob_id: String,
        full_blob_id: String,
        seal_namespace: vector<u8>,
    }

    /// Event emitted when a new video is published
    public struct VideoCreated has copy, drop {
        video_id: ID,
        creator: address,
        preview_blob_id: String,
        full_blob_id: String,
    }

    /// Purchase credential (owned by buyer)
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
2. **Real-Time Tracking**: `GET /api/status/{id}/events` — Get progress (including Watcher sync) via SSE.
3. **Preview Playback**: `GET /stream/{id}/preview` — 307 redirect to preview.
4. **System Config**: `GET /api/config` — Get contract, Walrus, and Watcher info.

---

## License

MIT

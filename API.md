# PayLock API Reference

This document describes the PayLock backend API for Walrus-backed video storage and Sui paywall integration.

## Base URL

- Self-hosted: `http://localhost:8080` (default)
- Deployed: use your own host/port

All endpoint paths are relative to the Base URL, for example `POST /api/upload`.

---

## Flow Overview

### Free Videos (price = 0)

```
POST /api/upload (price=0)
  -> Server extracts preview/thumbnail (if FFmpeg enabled)
  -> Upload preview/full to Walrus
  -> status=ready
  -> GET /stream/{id}/preview (307 redirect)
```

### Paid Videos (price > 0)

```
POST /api/upload (price>0)
  -> Server extracts preview/thumbnail (FFmpeg required) and uploads preview
  -> status=processing
  -> Client encrypts full video (Seal), uploads to Walrus
  -> Client calls gating::create_video on-chain
  -> Watcher detects VideoCreated and links sui_object_id
  -> status=ready
  -> GET /stream/{sui_object_id}/preview or /full (307 redirect)
```

---

## Common Formats

### Video Status

| Value | Description |
|-------|-------------|
| `processing` | Upload received and background processing pending or on-chain linking pending |
| `ready` | Preview/full blobs available and linked |
| `failed` | Processing failed; see `error` |

### Video Object Fields

```json
{
  "id": "a1b2c3d4e5f6g7h8",
  "title": "My Video",
  "status": "ready",
  "price": 1000000000,
  "creator": "0xabc...",
  "thumbnail_blob_id": "...",
  "thumbnail_blob_url": "https://aggregator.../v1/blobs/...",
  "preview_blob_id": "...",
  "preview_blob_url": "https://aggregator.../v1/blobs/...",
  "full_blob_id": "...",
  "full_blob_url": "https://aggregator.../v1/blobs/...",
  "encrypted": true,
  "sui_object_id": "0x789...abc",
  "created_at": "2024-03-24T12:00:00Z",
  "deleted": false,
  "deleted_at": "",
  "error": ""
}
```

Notes:

- `encrypted` is `true` when `price > 0`.
- Paid uploads may show `preview_blob_id` while still `status=processing`; `full_blob_id` arrives after on-chain linking.
- Fields with `omitempty` are omitted when empty.

### Error Response Format

```json
{ "error": "description message" }
```

---

## Authentication

PayLock uses Sui wallet signatures for creator operations. The signature is optional on upload and required for delete when the video has an on-chain object.

### Wallet Signature Headers

| Header | Description |
|--------|-------------|
| `X-Wallet-Address` | Signer's Sui address (`0x`-prefixed) |
| `X-Wallet-Sig` | Base64-encoded Sui Ed25519 serialized signature (97 bytes) |
| `X-Wallet-Timestamp` | Unix timestamp (seconds); server allows ±60s drift |

### Signature Message

```
paylock:<action>:<resourceId>:<timestamp>
```

- `action` is `upload` or `delete`.
- `resourceId` is empty for upload and the `{id}` for delete.

### Auth Rules

- `POST /api/upload`: signature is optional. If valid, the server stores `creator` for the new video.
- `DELETE /api/videos/{id}`: signature required only when the video has both `creator` and `sui_object_id` set.

---

## CORS

Only `/stream/*` endpoints have CORS enabled. `/api/*` does not set CORS headers; use a proxy or reverse proxy if needed.

---

## API Endpoints

### 1. Upload Video

**`POST /api/upload`**

Initiate an async upload. The server validates the file and starts background processing.

- **Content-Type**: `multipart/form-data`
- **Supported formats**: MP4, MOV, WebM, MKV, AVI (validated by magic bytes)

**Parameters**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `video` | Yes | Video file (max `PAYLOCK_MAX_FILE_SIZE_MB`, default 500 MB) |
| `title` | No | Video title (defaults to generated ID) |
| `price` | No | Price in MIST (uint64). Omit or `0` for free videos |
| `preview_duration` | No | Preview length in seconds. Defaults to `preview_duration_default` from `/api/config` and must be between min/max |

**Success Response** (`202 Accepted`)

```json
{
  "id": "a1b2c3d4e5f6g7h8",
  "status": "processing"
}
```

**Error Responses**

| Status | Reason |
|--------|--------|
| `400` | Invalid price, preview duration, or file format |
| `413` | File exceeds size limit |

---

### 2. Get Video Status

**`GET /api/status/{id}`**

Returns the full Video object. `{id}` can be either `paylock_id` or `sui_object_id`. The server resolves by `paylock_id` first, then falls back to `sui_object_id`.

**Error Responses**

| Status | Reason |
|--------|--------|
| `400` | Missing video id |
| `404` | Video not found |

---

### 3. Real-Time Status Tracking (SSE)

**`GET /api/status/{id}/events`**

SSE stream that immediately sends the current Video object and then sends the next update (ready/failed or preview-upload update for paid videos). If the connection closes while the video is still `processing`, reconnect or poll.

Example SSE events:

```
data: {"id":"...","status":"processing","title":"My Video"}

data: {"id":"...","status":"ready","preview_blob_id":"...","full_blob_id":"..."}
```

---

### 4. List Videos

**`GET /api/videos`**

Returns videos sorted by `created_at` descending.

**Query Parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `creator` | (none) | Filter by creator address |
| `page` | `1` | Page number (1-based) |
| `per_page` | `20` | Items per page (max 100) |

**Success Response**

```json
{
  "videos": [ { "id": "...", "status": "ready" } ],
  "total": 42,
  "page": 1,
  "per_page": 20
}
```

---

### 5. Delete Video

**`DELETE /api/videos/{id}`**

Deletes the video record from the backend store. This does not delete Walrus blobs or on-chain objects.

**Auth**

- Required only if the video has both `creator` and `sui_object_id` set.

**Success Response**

```json
{ "id": "...", "status": "deleted" }
```

**Error Responses**

| Status | Reason |
|--------|--------|
| `401` | Missing or invalid wallet signature (when required) |
| `403` | Wallet address does not match creator |
| `404` | Video not found |

---

### 6. Preview Stream

**`GET /stream/{id}/preview`**

307 Redirect to the preview blob URL. Available to anyone.

Notes:

- If a `paylock_id` has a linked `sui_object_id`, the server redirects to `/stream/{sui_object_id}/preview`.
- When accessed by `paylock_id`, the server sets deprecation headers with a `Sunset` date of `2026-06-23`.
- `GET /stream/{id}` is deprecated and redirects to `/stream/{id}/preview` with a `Sunset` date of `2026-09-23`.

---

### 7. Full Stream

**`GET /stream/{id}/full`**

307 Redirect to the full blob URL. For paid videos this is typically an encrypted blob that the client decrypts with Seal.

---

### 8. System Config

**`GET /api/config`**

Returns configuration for clients.

```json
{
  "gating_package_id": "0x...",
  "sui_network": "testnet",
  "walrus_publisher_url": "https://publisher.walrus-testnet.walrus.space",
  "walrus_aggregator_url": "https://aggregator.walrus-testnet.walrus.space",
  "preview_duration": 10,
  "preview_duration_default": 10,
  "preview_duration_min": 10,
  "preview_duration_max": 300,
  "watcher_interval": 5000
}
```

---

### 9. Manual Reindex

**`POST /api/reindex`**

Triggers a full chain reindex. Requires `Authorization: Bearer <PAYLOCK_ADMIN_SECRET>`.

**Success Response**

```json
{
  "status": "ok",
  "chain_total": 120,
  "new_entries": 3,
  "pruned": 2
}
```

---

## Paid Video Integration Guide

This section outlines the paid flow at a high level. Client-side Seal usage and on-chain calls are required for paid videos.

### 1. Upload and Wait for Preview

```js
const form = new FormData();
form.append('video', file);
form.append('title', 'My Paid Video');
form.append('price', '1000000000');
const res = await fetch(`${PAYLOCK}/api/upload`, { method: 'POST', body: form });
const { id: paylockId } = await res.json();

const es = new EventSource(`${PAYLOCK}/api/status/${paylockId}/events`);
es.onmessage = (e) => {
  const v = JSON.parse(e.data);
  if (v.preview_blob_id) {
    es.close();
    // proceed to encrypt/upload full and call create_video
  }
};
```

### 2. Encrypt Full Video and Upload to Walrus

Use `@mysten/seal` to encrypt, then upload the encrypted bytes to the Walrus Publisher API to obtain `full_blob_id`.

### 3. Create On-Chain Video

Call the Move function with all blob IDs:

```js
import { Transaction } from '@mysten/sui/transactions';

const tx = new Transaction();
tx.moveCall({
  target: `${config.gating_package_id}::gating::create_video`,
  arguments: [
    tx.pure.string(title),
    tx.pure.u64(priceMist),
    tx.pure.string(thumbnailBlobId),
    tx.pure.string(previewBlobId),
    tx.pure.string(fullBlobId),
    tx.pure.vector('u8', Array.from(sealNamespace)),
  ],
});
```

### 4. Wait for Watcher Sync

The watcher polls Sui and links the on-chain object to the local entry. Continue polling or re-opening the SSE stream until `status === 'ready'` and `sui_object_id` is set.

---

## Move Contract Reference (`paylock::gating`)

Key items from `contracts/sources/gating.move`:

- `create_video(title, price, thumbnail_blob_id, preview_blob_id, full_blob_id, seal_namespace)`
- `purchase_and_transfer(video, payment)`
- `seal_approve(id, pass, video)`
- `VideoCreated` event
- `VideoDeleted` event

---

## FAQ / Troubleshooting

### ID Resolution Logic

`/api/status/{id}` and `/stream/{id}/*` resolve by `paylock_id` first, then by `sui_object_id`. If you store both IDs, prefer the `sui_object_id` for canonical links.

### Upload Stuck on `processing`

- Free videos: check FFmpeg availability if you expect previews/thumbnails.
- Paid videos: you must call `create_video` on-chain; otherwise the watcher will never link and the status stays `processing`.

### Wallet Signature Errors

- If delete is unauthorized, ensure the timestamp is within ±60 seconds and the signature matches the `creator` address.

### Walrus Blob Expiry

Walrus storage is epoch-based. Once a blob expires it cannot be streamed. Renewals are not implemented yet.


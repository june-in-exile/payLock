# PayLock API Reference

## Base URL

Default: `http://localhost:8080`. All paths below are relative to the base URL.

---

## Authentication

Some endpoints accept or require Sui wallet signatures via request headers.

| Header | Description |
|--------|-------------|
| `X-Wallet-Address` | Signer's Sui address (`0x`-prefixed) |
| `X-Wallet-Sig` | Base64-encoded Sui Ed25519 serialized signature (97 bytes) |
| `X-Wallet-Timestamp` | Unix timestamp (seconds); server allows ±60 s drift |

The signed message format:

```
paylock:<action>:<resourceId>:<timestamp>
```

- `action`: `upload` or `delete`
- `resourceId`: empty for upload, the video `{id}` for delete

| Endpoint | Auth |
|----------|------|
| `POST /api/upload` | Optional — if valid, `creator` is recorded |
| `DELETE /api/videos/{id}` | Required only when the video has both `creator` and `sui_object_id` |

---

## Common Types

### Video Status

| Value | Meaning |
|-------|---------|
| `processing` | Background upload or on-chain linking in progress |
| `ready` | All blobs available |
| `failed` | Processing error; see `error` field |

### Video Object

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

- `encrypted` is `true` when `price > 0`.
- Paid videos may have `preview_blob_id` while still `processing`; `full_blob_id` is set after the chain watcher links the on-chain object.
- Fields with `omitempty` are omitted when empty.

### Error Response

```json
{ "error": "description" }
```

---

## CORS

Only `/stream/*` endpoints return CORS headers (allows `GET`, exposes `Content-Range` and `Content-Length`). `/api/*` does not — use a reverse proxy if cross-origin access is needed.

---

## Endpoints

### `POST /api/upload`

Start an async video upload. The server validates the file and begins background processing.

**Content-Type**: `multipart/form-data`
**Supported formats**: MP4, MOV, WebM, MKV, AVI (validated by magic bytes, not extension)

| Parameter | Required | Description |
|-----------|----------|-------------|
| `video` | Yes | Video file (max `PAYLOCK_MAX_FILE_SIZE_MB`, default 500 MB) |
| `title` | No | Title (defaults to generated ID) |
| `price` | No | Price in MIST (uint64). Omit or `0` for free |
| `preview_duration` | No | Preview clip length in seconds (default 10, min 10, max 300) |

**`202 Accepted`**

```json
{ "id": "a1b2c3d4e5f6g7h8", "status": "processing" }
```

| Error | Reason |
|-------|--------|
| `400` | Invalid price, duration, or file format. Paid upload with FFmpeg disabled. |
| `413` | File exceeds size limit |

---

### `GET /api/status/{id}`

<<<<<<< HEAD
Returns the full Video object. `{id}` can be `paylock_id` or `sui_object_id` (resolved in that order).
=======
Returns the full Video object. `{id}` can be `paylock_id` or `sui_object_id` (resolved in that order — `paylock_id` first because callers typically poll with it right after upload, before a `sui_object_id` exists).
>>>>>>> 8b0d4ce (chore: remove reindex endpoint and legacy stream path)

| Error | Reason |
|-------|--------|
| `400` | Missing id |
| `404` | Not found |

---

### `GET /api/status/{id}/events`

SSE stream. Immediately sends the current Video object, then pushes updates on state changes (preview uploaded, ready, failed). Reconnect if the connection drops while `processing`.

```
data: {"id":"...","status":"processing","title":"My Video"}

data: {"id":"...","status":"ready","preview_blob_id":"...","full_blob_id":"..."}
```

---

### `GET /api/videos`

List videos, sorted by `created_at` descending.

| Parameter | Default | Description |
|-----------|---------|-------------|
| `creator` | *(none)* | Filter by creator address |
| `page` | `1` | Page number (1-based) |
| `per_page` | `20` | Items per page (max 100) |

```json
{
  "videos": [ ... ],
  "total": 42,
  "page": 1,
  "per_page": 20
}
```

---

### `DELETE /api/videos/{id}`

Delete a video record from the local store. Does **not** delete Walrus blobs or on-chain objects.

| Error | Reason |
|-------|--------|
| `401` | Missing or invalid wallet signature |
| `403` | Address does not match creator |
| `404` | Not found |

**`200 OK`**

```json
{ "id": "...", "status": "deleted" }
```

---

### `GET /stream/{id}/preview`

307 redirect to the Walrus aggregator URL for the **preview** blob (short clip or thumbnail used for browsing).

- Accepts both `paylock_id` and `sui_object_id`.
- If accessed by `paylock_id` that has a linked `sui_object_id`, redirects to the canonical path with `Deprecation` and `Sunset: 2026-06-23` headers.
- The preview blob is always **unencrypted** — anyone can view it without purchasing.

---

### `GET /stream/{id}/full`

307 redirect to the Walrus aggregator URL for the **full** video blob.

- Accepts both `paylock_id` and `sui_object_id`.
- If accessed by `paylock_id` that has a linked `sui_object_id`, redirects to the canonical path with `Deprecation` and `Sunset: 2026-06-23` headers.
- **Free videos**: the full blob is unencrypted and directly playable.
- **Paid videos**: the full blob is Seal-encrypted — the client must hold a valid `AccessPass` and decrypt it with Seal before playback.

---

### `GET /api/config`

Client configuration.

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

## Paid Video Integration

### 1. Upload and wait for preview

```js
const form = new FormData();
form.append('video', file);
form.append('title', 'My Paid Video');
form.append('price', '1000000000');

const { id } = await fetch(`${PAYLOCK}/api/upload`, {
  method: 'POST',
  body: form,
}).then(r => r.json());

// Wait for server to extract and upload the preview
const preview = await new Promise((resolve, reject) => {
  const es = new EventSource(`${PAYLOCK}/api/status/${id}/events`);
  es.onmessage = (e) => {
    const v = JSON.parse(e.data);
    if (v.preview_blob_id) { es.close(); resolve(v); }
    if (v.status === 'failed') { es.close(); reject(new Error(v.error)); }
  };
});
```

### 2. Encrypt full video and upload to Walrus

Use `@mysten/seal` to encrypt the original video, then upload the encrypted bytes directly to the Walrus Publisher API to obtain a `full_blob_id`.

### 3. Create on-chain Video

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

### 4. Wait for watcher sync

The chain watcher polls Sui for `VideoCreated` events and links the on-chain object to the local record. Poll `GET /api/status/{id}` or reconnect to the SSE stream until `status === 'ready'` and `sui_object_id` is set.

---

## Move Contract Reference (`paylock::gating`)

From `contracts/sources/gating.move`:

| Function / Event | Description |
|------------------|-------------|
| `create_video(title, price, thumbnail_blob_id, preview_blob_id, full_blob_id, seal_namespace)` | Register a video on-chain |
| `purchase_and_transfer(video, payment)` | Pay and mint an `AccessPass` |
| `seal_approve(id, pass, video)` | Authorize Seal decryption |
| `VideoCreated` event | Emitted on creation for backend sync |
| `VideoDeleted` event | Emitted on deletion for backend cleanup |

---

## Troubleshooting

**Upload stuck on `processing`**
<<<<<<< HEAD
- Free videos: check server logs for FFmpeg or Walrus errors.
- Paid videos: you must call `create_video` on-chain; otherwise the watcher has nothing to link.

**Wallet signature rejected**
- Ensure the timestamp is within ±60 seconds of the server clock.
- Ensure the signing address matches the video's `creator`.

**Walrus blob not loading**
- Walrus storage is epoch-based. Expired blobs cannot be streamed. Renewals are not yet implemented.

**ID resolution**
=======

- Free videos: check server logs for FFmpeg or Walrus errors.
- Paid videos: you must call `create_video` on-chain; otherwise the watcher has nothing to link.

**Wallet signature rejected**

- Ensure the timestamp is within ±60 seconds of the server clock.
- Ensure the signing address matches the video's `creator`.

**Walrus blob not loading**

- Walrus storage is epoch-based. Expired blobs cannot be streamed. Renewals are not yet implemented.

**ID resolution**

>>>>>>> 8b0d4ce (chore: remove reindex endpoint and legacy stream path)
- All endpoints that accept `{id}` resolve by `paylock_id` first, then by `sui_object_id`. Prefer `sui_object_id` for stable links.

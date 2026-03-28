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

## Endpoints

### `POST /api/upload`

Initiates an asynchronous video upload. The server validates the request and the file, creates a local record, and then processes the video in the background (extracting previews, thumbnails, and uploading to Walrus).

**Content-Type**: `multipart/form-data`

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `video` | file | Yes | The video file. Max size depends on `PAYLOCK_MAX_FILE_SIZE_MB` (default 500MB). Supports MP4, MOV, WebM, MKV, AVI. |
| `title` | string | No | Video title. Defaults to the generated unique ID. |
| `price` | string | No | Price in MIST (1 Sui = 10^9 MIST). If `0` or omitted, it's a **Free Video**. |
| `preview_duration` | string | No | Desired length of the preview clip in seconds. Must be between `min` and `max` defined in `/api/config`. |

**Authentication:**
Optional. If `X-Wallet-Address` and `X-Wallet-Sig` are provided and valid, the `creator` field will be populated.

**Behavioral Notes:**

- **Free Videos**: The server handles the entire pipeline: extracts a preview (if FFmpeg is enabled), optimizes for fast-start, and uploads both to Walrus.
- **Paid Videos**: The server extracts a preview and thumbnail, then waits for the client to encrypt and upload the full video on-chain. FFmpeg must be enabled for paid uploads.
- **Asynchronous**: Returns immediately with `202 Accepted`. Use the returned `id` to poll `/api/status/{id}`.

**Response `202 Accepted`**

```json
{ "id": "a1b2c3d4e5f6g7h8", "status": "processing" }
```

**Errors:**

- `400 Bad Request`: Invalid price/duration, unsupported file format, or FFmpeg missing for paid uploads.
- `413 Payload Too Large`: File exceeds the maximum allowed size.
- `500 Internal Server Error`: Temporary storage or processing failures.

---

### `GET /api/videos/{id}`

Retrieves the metadata for a specific video.

**Path Parameters:**

- `id`: Can be either the internal `paylock_id` (8-byte hex) or the `sui_object_id` (0x-prefixed). The server resolves `paylock_id` first.

**Response `200 OK`**
Returns a [Video Object](#video-object).

**Errors:**

- `404 Not Found`: No video matches the provided ID.

---

### `GET /api/status/{id}`

A Server-Sent Events (SSE) endpoint that streams real-time updates for a specific video's processing state.

**Path Parameters:**

- `id`: The internal `paylock_id`.

**Event Stream:**

- Immediately sends the current state.
- Pushes a new `data:` packet whenever the `status` or blob IDs change (e.g., when a preview becomes available).
- The client should close the connection once `status` is `ready` or `failed`.

**Example Event:**

```
data: {"id":"...","status":"ready","preview_blob_id":"...","full_blob_id":"..."}
```

---

### `GET /api/videos`

Lists videos with filtering, sorting, and pagination.

**Query Parameters:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `creator` | *(none)* | Filter by the creator's Sui wallet address. |
| `page` | `1` | Page number (1-based). |
| `per_page` | `20` | Number of items per page (min 1, max 100). |

**Sorting:**
Always sorted by `created_at` in descending order (newest videos first).

**Response `200 OK`**

```json
{
  "videos": [ ... ],
  "total": 150,
  "page": 1,
  "per_page": 20
}
```

---

### `DELETE /api/videos/{id}`

Deletes a video record from the PayLock database.

**Path Parameters:**

- `id`: The internal `paylock_id` or `sui_object_id`.

**Authentication Requirements:**

- **Unpublished Videos**: If the video has no `sui_object_id` (e.g., a failed or pending upload), it can be deleted without authentication.
- **Published Videos**: Requires a valid Sui wallet signature. The `X-Wallet-Address` must match the video's `creator`.

**Behavioral Notes:**

- This **only** removes the metadata from the PayLock server.
- It **does not** delete the blobs from Walrus (which are immutable for their duration).
- It **does not** delete the on-chain Sui object.

**Response `200 OK`**

```json
{ "id": "...", "status": "deleted" }
```

**Errors:**

- `401 Unauthorized`: Signature missing or invalid.
- `403 Forbidden`: Authenticated address does not match the video creator.
- `404 Not Found`: Video not found.

---

### `PATCH /api/videos/{id}/link`

Links an on-chain Sui object to an existing video record. Called by the client after `create_video` succeeds on-chain, so the server can immediately transition the video to `ready` without waiting for the chain watcher poll.

**Content-Type**: `application/json`

**Path Parameters:**

- `id`: The internal `paylock_id` returned by `POST /api/upload`.

**Request Body:**

| Field            | Type   | Required | Description                                                                                                        |
|------------------|--------|----------|--------------------------------------------------------------------------------------------------------------------|
| `sui_object_id`  | string | Yes      | The `0x`-prefixed object ID returned by the on-chain `create_video` transaction.                                   |
| `full_blob_id`   | string | No       | The Walrus blob ID of the full (possibly encrypted) video. If provided, `full_blob_url` is derived automatically.  |

**Response `200 OK`**
Returns the updated [Video Object](#video-object) with `status: "ready"` and `sui_object_id` set.

**Errors:**

- `400 Bad Request`: Missing `sui_object_id` or invalid JSON body.
- `404 Not Found`: No video matches the provided `id`.

**Example:**

```js
await fetch(`${PAYLOCK}/api/videos/${id}/link`, {
  method: 'PATCH',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    sui_object_id: '0x789...abc',
    full_blob_id: 'blobId123',
  }),
});
```

---

### `GET /api/config`

Returns the global configuration required for the frontend to interact with Sui, Walrus, and the PayLock backend. This is typically the first endpoint called by the client to initialize its state.

**Response Body:**

| Field | Type | Description |
|-------|------|-------------|
| `gating_package_id` | `string` | The 0x-prefixed ID of the deployed Sui Move package. Used to build `Transaction` calls for `create_video` and `purchase_and_transfer`. |
| `sui_network` | `string` | The target Sui network (e.g., `testnet`, `mainnet`). Helps the client choose the correct RPC node. |
| `walrus_publisher_url` | `string` | The endpoint for uploading blobs (e.g., encrypted videos) directly to Walrus. |
| `walrus_aggregator_url` | `string` | The base URL used to fetch/stream blobs from Walrus. |
| `preview_duration` | `number` | The current active default duration (seconds) for video previews. |
| `preview_duration_default`| `number` | The system-wide default preview duration (seconds). |
| `preview_duration_min` | `number` | The minimum allowed preview duration (seconds) for the `POST /api/upload` endpoint. |
| `preview_duration_max` | `number` | The maximum allowed preview duration (seconds) for the `POST /api/upload` endpoint. |
| `watcher_interval` | `number` | The server's chain-polling frequency in milliseconds. Useful for the client to estimate when an on-chain event might be synced. |

**Example:**

```json
{
  "gating_package_id": "0x5608c0...807e3",
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
  const es = new EventSource(`${PAYLOCK}/api/status/${id}`);
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

### 4. Link on-chain object to server

After the on-chain transaction succeeds, call `PATCH /api/videos/{id}/link` to immediately link the `sui_object_id` and transition the video to `ready`. This bypasses the chain watcher poll delay.

```js
await fetch(`${PAYLOCK}/api/videos/${id}/link`, {
  method: 'PATCH',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ sui_object_id: suiObjectId, full_blob_id: fullBlobId }),
});
```

The chain watcher still runs as a fallback — if the PATCH call fails (e.g., network error), the watcher will eventually link the video automatically.

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

- Free videos: check server logs for FFmpeg or Walrus errors.
- Paid videos: you must call `create_video` on-chain; otherwise the watcher has nothing to link.

**Wallet signature rejected**

- Ensure the timestamp is within ±60 seconds of the server clock.
- Ensure the signing address matches the video's `creator`.

**Walrus blob not loading**

- Walrus storage is epoch-based. Expired blobs cannot be streamed. Renewals are not yet implemented.

**ID resolution**

- All endpoints that accept `{id}` resolve by `paylock_id` first, then by `sui_object_id`. Prefer `sui_object_id` for stable links.

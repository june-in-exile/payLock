# PayLock API Reference

This document provides the complete API specification for the PayLock backend service, designed for developers who want to integrate video paywall functionality into their dApps.

## Base URL

| Environment | URL |
|-------------|-----|
| Testnet (public instance) | `https://paylock.up.railway.app` |
| Self-hosted | Determined by `PAYLOCK_PORT`, default `http://localhost:8080` |

All endpoint paths are relative to the Base URL, e.g. `POST https://paylock.up.railway.app/api/upload`.

---

## Table of Contents

- [PayLock API Reference](#paylock-api-reference)
  - [Base URL](#base-url)
  - [Table of Contents](#table-of-contents)
  - [Flow Overview](#flow-overview)
    - [Free Videos](#free-videos)
    - [Paid Videos](#paid-videos)
  - [Common Formats](#common-formats)
    - [Video Status](#video-status)
    - [Video Object Fields](#video-object-fields)
    - [Error Response Format](#error-response-format)
    - [Authentication](#authentication)
      - [1. Wallet Signature (Creator Operations)](#1-wallet-signature-creator-operations)
      - [2. Admin Secret (Admin Operations)](#2-admin-secret-admin-operations)
      - [Authentication Overview](#authentication-overview)
    - [CORS](#cors)
  - [API Endpoints](#api-endpoints)
    - [1. Upload Video](#1-upload-video)
    - [2. Get Video Status](#2-get-video-status)
    - [3. Real-Time Status Tracking (SSE)](#3-real-time-status-tracking-sse)
    - [4. Link On-Chain Object](#4-link-on-chain-object)
    - [5. List All Videos](#5-list-all-videos)
    - [6. Delete Video](#6-delete-video)
    - [7. Preview Stream](#7-preview-stream)
    - [8. Full Stream](#8-full-stream)
    - [9. System Config](#9-system-config)
    - [10. Manual Reindex](#10-manual-reindex)
  - [Paid Video Integration Guide](#paid-video-integration-guide)
    - [Prerequisites](#prerequisites)
      - [Install Dependencies](#install-dependencies)
      - [Initialize SDKs](#initialize-sdks)
    - [Creator Flow](#creator-flow)
      - [Step 1: Generate Preview \& Upload](#step-1-generate-preview--upload)
      - [Step 2: Wait for Server Processing](#step-2-wait-for-server-processing)
      - [Step 3: Encrypt Full Video \& Publish On-Chain](#step-3-encrypt-full-video--publish-on-chain)
      - [Step 4: Write Back to API](#step-4-write-back-to-api)
    - [Viewer Flow](#viewer-flow)
      - [Step 5: Browse \& Discover Videos](#step-5-browse--discover-videos)
      - [Step 6: Preview Playback](#step-6-preview-playback)
      - [Step 7: Purchase \& Decrypt Playback](#step-7-purchase--decrypt-playback)
    - [Integration Summary](#integration-summary)
    - [Move Contract Reference (`paylock::gating`)](#move-contract-reference-paylockgating)
  - [FAQ / Troubleshooting](#faq--troubleshooting)
    - [ID Detection Logic](#id-detection-logic)
    - [What Happens When Walrus Blobs Expire?](#what-happens-when-walrus-blobs-expire)
    - [CORS Errors When Calling `/api/*` from Frontend](#cors-errors-when-calling-api-from-frontend)
    - [Wallet Signature Verification Failed (403)](#wallet-signature-verification-failed-403)
    - [Upload Stuck on `processing`](#upload-stuck-on-processing)
    - [How to Check if a User Has Already Purchased a Video?](#how-to-check-if-a-user-has-already-purchased-a-video)
    - [Related External Documentation](#related-external-documentation)

---

## Flow Overview

### Free Videos

```text
POST /api/upload (price=0)
    → Server processes preview / thumbnail / full video and uploads to Walrus
    → GET /api/status/{id} poll until status=ready
    → GET /stream/{id} play
```

### Paid Videos

```text
Frontend generates preview (ffmpeg.wasm) + thumbnail from original video
POST /api/upload (price>0, preview + optional thumbnail)
    → Server validates and uploads preview / thumbnail to Walrus
    → GET /api/status/{id}/events (SSE) wait for status=ready
    → Frontend encrypts original video with Seal SDK → uploads encrypted blob to Walrus → gets full_blob_id
    → Frontend sends Sui transaction calling gating::create_video (with price, preview_blob_id, full_blob_id, seal_namespace)
    → PUT /api/videos/{id} (write back sui_object_id + full_blob_id)
    → Buyer: purchase_and_transfer → seal_approve → Seal decrypt → play
```

> **Security**: The full unencrypted video never leaves the user's device. The server only receives the short preview clip.

---

## Common Formats

### Video Status

| Value | Description |
|-------|-------------|
| `processing` | Upload received, background processing in progress |
| `ready` | Preview / full video uploaded to Walrus, ready to stream |
| `failed` | Processing failed, `error` field contains the reason |

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
  "error": ""
}
```

- `encrypted`: `true` for paid videos, indicating `full_blob_id` points to a Seal-encrypted blob.
- `error`: Only present when `status=failed`.
- Fields with `omitempty` are omitted from the response when empty.

### Error Response Format

All errors return a unified format:

```json
{ "error": "description message" }
```

### Authentication

PayLock uses two authentication mechanisms, depending on the endpoint:

#### 1. Wallet Signature (Creator Operations)

Protects endpoints requiring creator identity verification (paid uploads, updates, deletes). Uses Sui wallet Ed25519 signatures to verify the requester's identity.

**Required Headers:**

| Header | Description |
|--------|-------------|
| `X-Wallet-Address` | Signer's Sui address (`0x`-prefixed) |
| `X-Wallet-Sig` | Base64-encoded Sui Ed25519 serialized signature (97 bytes: 1 flag + 64 sig + 32 pubkey) |
| `X-Wallet-Timestamp` | Unix timestamp of the request (seconds), server allows ±60 second drift |

**Signature Generation:**

```js
import { Ed25519Keypair } from '@mysten/sui/keypairs/ed25519';

const timestamp = Math.floor(Date.now() / 1000);
// Format: paylock:<action>:<resourceId>:<timestamp>
// action: "upload" | "update" | "delete"
// resourceId: video ID (empty string for upload)
const message = `paylock:update:${videoId}:${timestamp}`;
const msgBytes = new TextEncoder().encode(message);

const { signature } = await keypair.signPersonalMessage(msgBytes);

// Attach to request headers
headers['X-Wallet-Address'] = keypair.getPublicKey().toSuiAddress();
headers['X-Wallet-Sig'] = signature;
headers['X-Wallet-Timestamp'] = timestamp.toString();
```

**Applicable Endpoints:**

| Endpoint | Action | Resource ID |
|----------|--------|-------------|
| `POST /api/upload` (price > 0) | `upload` | (empty string) |
| `PUT /api/videos/{id}` | `update` | `{id}` |
| `DELETE /api/videos/{id}` | `delete` | `{id}` |

> The server automatically verifies that the signature address matches the video's `creator` field. Returns `403` if they don't match.

#### 2. Admin Secret (Admin Operations)

Used only for `POST /api/reindex`, authenticated via Bearer token.

```http
Authorization: Bearer <PAYLOCK_ADMIN_SECRET>
```

If `PAYLOCK_ADMIN_SECRET` is not set, this endpoint always returns `401`.

#### Authentication Overview

| Endpoint | Auth Method | Notes |
|----------|-------------|-------|
| `POST /api/upload` (free) | None | — |
| `POST /api/upload` (paid) | Wallet Signature | `action=upload` |
| `GET /api/status/{id}` | None | — |
| `GET /api/status/{id}/events` | None | — |
| `GET /api/videos` | None | — |
| `PUT /api/videos/{id}` | Wallet Signature | `action=update` |
| `DELETE /api/videos/{id}` | Wallet Signature | `action=delete` |
| `GET /stream/{id}/preview` | None | CORS enabled |
| `GET /stream/{id}/full` | None | CORS enabled (returns encrypted blob for paid videos) |
| `GET /api/config` | None | — |
| `POST /api/reindex` | Admin Secret | Bearer token |

### CORS

Stream endpoints (`/stream/*`) have CORS enabled, allowing cross-origin frontends to use `<video>` tags directly:

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, OPTIONS`
- `Access-Control-Allow-Headers: Range`
- `Access-Control-Expose-Headers: Content-Range, Content-Length`

API endpoints (`/api/*`) do not have CORS enabled. If your frontend needs to call the API directly, use your own backend proxy or configure a reverse proxy when self-hosting.

---

## API Endpoints

### 1. Upload Video

**`POST /api/upload`**

Initiate an async upload. The server validates the file and starts background processing.

- **Content-Type**: `multipart/form-data`
- **Supported formats**: MP4 (`.mp4`), MOV (`.mov`), WebM (`.webm`), MKV (`.mkv`), AVI (`.avi`). Validated by magic bytes, not file extension.

**Free uploads** (`price=0` or omitted):

| Parameter | Required | Description |
|-----------|----------|-------------|
| `video` | Yes | Video file (max `PAYLOCK_MAX_FILE_SIZE_MB`, default 500 MB) |
| `title` | No | Video title, auto-generated if not provided |
| `price` | No | `0` or omitted = free video |

**Paid uploads** (`price > 0`):

| Parameter | Required | Description |
|-----------|----------|-------------|
| `preview` | Yes | Short preview video clip generated client-side (max `PAYLOCK_MAX_PREVIEW_SIZE_MB`, default 50 MB) |
| `thumbnail` | No | JPEG thumbnail image generated client-side |
| `title` | No | Video title, auto-generated if not provided |
| `price` | Yes | Price in MIST (uint64, must be > 0) |

> **Paid uploads require Wallet Signature authentication** (`action=upload`). See [Authentication](#authentication).
>
> **Preview generation is client-side**: The full unencrypted video is never sent to the server. The frontend must generate a short preview clip and send only the preview. The built-in SPA uses `MediaRecorder` (canvas + `captureStream`) to capture the first N seconds (default 10s, exposed at `GET /api/config` as `preview_duration`). External integrators can also use `ffmpeg.wasm`. If FFmpeg is available on the server, the preview duration is validated against `PAYLOCK_MAX_PREVIEW_DURATION` (default 30 seconds).

**Success Response** (`202 Accepted`):

```json
{
  "id": "a1b2c3d4e5f6g7h8",
  "status": "processing"
}
```

**Error Responses**:

| Status | Reason |
|--------|--------|
| `400` | Cannot read file / unsupported format / price not a positive integer / missing `preview` field for paid upload / invalid thumbnail format |
| `401` | Missing or invalid wallet signature (paid uploads) |
| `413` | File exceeds size limit |

---

### 2. Get Video Status

**`GET /api/status/{id}`**

Get complete metadata for a specific video. `{id}` can be either `paylock_id` or `sui_object_id` — the system auto-detects based on format.

**Success Response** (`200 OK`): Returns the complete Video object (see field definitions above).

**Error Responses**:

| Status | Reason |
|--------|--------|
| `400` | Missing video id |
| `404` | Video not found |

---

### 3. Real-Time Status Tracking (SSE)

**`GET /api/status/{id}/events`**

Server-Sent Events stream that pushes the complete Video object whenever its status changes. Immediately pushes the current state upon connection. Ideal for waiting for processing to complete after upload.

```text
data: {"id":"...","status":"processing","title":"My Video","price":0,"created_at":"..."}

data: {"id":"...","status":"ready","preview_blob_id":"...","preview_blob_url":"...","full_blob_id":"...","full_blob_url":"...","created_at":"..."}
```

The connection is closed by the server after `status` becomes `ready` or `failed`.

---

### 4. Link On-Chain Object

**`PUT /api/videos/{id}`**

After completing the on-chain `create_video` transaction, write back the Sui object ID and encrypted full blob ID to the backend.

- **Authentication required**: Wallet Signature (`action=update`). See [Authentication](#authentication).

**Request Body** (`application/json`):

```json
{
  "sui_object_id": "0x789...abc",
  "full_blob_id": "blobId123"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `sui_object_id` | Yes | On-chain Video shared object ID |
| `full_blob_id` | No | Encrypted full blob's Walrus blob ID (should be provided for paid videos) |

**Success Response** (`200 OK`):

```json
{
  "status": "ok",
  "sui_object_id": "0x789...abc"
}
```

**Error Responses**:

| Status | Reason |
|--------|--------|
| `400` | Missing video id / invalid request body / `sui_object_id` is empty |
| `403` | Signature address does not match the video's creator |
| `404` | Video not found |
| `409` | Video is already linked to a different `sui_object_id` |

---

### 5. List All Videos

**`GET /api/videos`**

Get a list of videos sorted by `created_at` in descending order (newest first). Supports filtering and pagination.

**Query Parameters**:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `creator` | *(none)* | Filter by creator's Sui address |
| `page` | `1` | Page number (1-based) |
| `per_page` | `20` | Items per page (max 100) |

**Success Response** (`200 OK`):

```json
{
  "videos": [
    { "id": "...", "title": "...", "status": "ready", "price": 0, "thumbnail_blob_url": "...", "created_at": "..." },
    { "id": "...", "title": "...", "status": "ready", "price": 1000000000, "encrypted": true, "sui_object_id": "0x...", "created_at": "..." }
  ],
  "total": 42,
  "page": 1,
  "per_page": 20
}
```

---

### 6. Delete Video

**`DELETE /api/videos/{id}`**

Delete the video record from the backend metadata store.

- **Authentication required**: Wallet Signature (`action=delete`). See [Authentication](#authentication).

> **Note**: This does not delete the blob on Walrus or the on-chain Video object.

**Success Response** (`200 OK`):

```json
{ "id": "...", "status": "deleted" }
```

**Error Responses**:

| Status | Reason |
|--------|--------|
| `403` | Signature address does not match the video's creator |
| `404` | Video not found |

---

### 7. Preview Stream

**`GET /stream/{id}/preview`**

307 Redirect to the preview's public URL on Walrus. Accessible by anyone.

```html
<video src="https://your-paylock-host/stream/{id}/preview"></video>
```

> **Deprecated path**: `GET /stream/{id}` still works and 307 redirects to `/stream/{id}/preview` with a `Deprecation` header. Scheduled for removal on 2026-09-23.

**Error Responses**:

| Status | Reason |
|--------|--------|
| `400` | Missing video id |
| `404` | Video not found |
| `500` | Video has no preview blob URL |
| `503` | Video not ready yet (status != ready) |

---

### 8. Full Stream

**`GET /stream/{id}/full`**

307 Redirect to the full blob URL. For paid videos, this returns the encrypted blob which requires frontend Seal decryption.

**Error Responses**: Same as Preview Stream.

---

### 9. System Config

**`GET /api/config`**

Get backend environment configuration. Integrators should use this API to get contract and Walrus endpoints instead of hardcoding.

**Success Response** (`200 OK`):

```json
{
  "gating_package_id": "0x...",
  "sui_network": "testnet",
  "walrus_publisher_url": "https://publisher.walrus-testnet.walrus.space",
  "walrus_aggregator_url": "https://aggregator.walrus-testnet.walrus.space",
  "preview_duration": 10
}
```

---

### 10. Manual Reindex

**`POST /api/reindex`**

Trigger a rescan of all Video objects from the Sui chain, adding missing records to the local VideoStore. The server automatically runs this once at startup; this endpoint is for manual triggering.

- **Authentication required**: `Authorization: Bearer <PAYLOCK_ADMIN_SECRET>` header. If `PAYLOCK_ADMIN_SECRET` is not set, this endpoint always returns `401`.

**Success Response** (`200 OK`):

```json
{
  "status": "ok",
  "chain_total": 120,
  "new_entries": 3
}
```

| Field | Description |
|-------|-------------|
| `chain_total` | Total Video objects found on-chain |
| `new_entries` | Number of new entries added to local store |

**Error Responses**:

| Status | Reason |
|--------|--------|
| `401` | Missing or invalid admin secret |
| `500` | Chain scan failed |

---

## Paid Video Integration Guide

This guide explains how external developers can implement the full paid video flow using the PayLock API. The PayLock server handles video processing, preview generation, Walrus storage, and stream routing — developers only need to call API endpoints to complete most of the integration.

> **On-chain operations**: Creator publishing and viewer purchases involve Sui on-chain transactions, requiring the `@mysten/sui` SDK. Full video encryption for paid videos uses `@mysten/seal`. These are the only parts of the integration that interact directly with the chain.

### Prerequisites

#### Install Dependencies

```bash
npm install @mysten/sui @mysten/seal
```

#### Initialize SDKs

```js
import { SuiClient, getFullnodeUrl } from '@mysten/sui/client';
import { SealClient } from '@mysten/seal';

// 1. Get backend configuration from PayLock API
const PAYLOCK = 'https://paylock.up.railway.app';
const configRes = await fetch(`${PAYLOCK}/api/config`);
const config = await configRes.json();
// → { gating_package_id, sui_network, walrus_publisher_url, walrus_aggregator_url }

// 2. Initialize Sui Client
const suiClient = new SuiClient({ url: getFullnodeUrl(config.sui_network) });

// 3. Initialize Seal Client (required for paid video encryption/decryption)
const sealClient = new SealClient({
  suiClient,
  serverObjectIds: {
    // Seal Testnet key server object IDs
    // See https://docs.mystenlabs.com/seal
  },
  verifyKeyServers: false, // Can set to false for testnet
});

// 4. Wallet (depends on your frontend framework)
// If using @mysten/dapp-kit:
//   const { mutate: signAndExecuteTransaction } = useSignAndExecuteTransaction();
//   const account = useCurrentAccount();
// If using Ed25519Keypair (backend/scripts):
//   import { Ed25519Keypair } from '@mysten/sui/keypairs/ed25519';
//   const keypair = Ed25519Keypair.fromSecretKey(privateKeyBytes);
```

---

### Creator Flow

#### Step 1: Generate Preview & Upload

For paid videos, the frontend generates a short preview clip and optional thumbnail locally, then uploads only those to the server. The full unencrypted video never leaves the user's device.

> **Paid videos require Wallet Signature authentication**. See [Authentication](#authentication).

```js
// 1a. Get preview duration from server config
const configRes = await fetch(`${PAYLOCK}/api/config`);
const config = await configRes.json();
const previewDuration = config.preview_duration || 10; // seconds

// 1b. Generate preview clip — Option A: MediaRecorder (no dependencies, built-in SPA approach)
//     Records the first N seconds via canvas.captureStream() → outputs WebM.
//     See upload-section.js generatePreview() for full implementation.

// 1b. Generate preview clip — Option B: ffmpeg.wasm (outputs MP4, better codec control)
import { FFmpeg } from '@ffmpeg/ffmpeg';
import { fetchFile } from '@ffmpeg/util';

const ffmpeg = new FFmpeg();
await ffmpeg.load();
await ffmpeg.writeFile('input.mp4', await fetchFile(videoFile));
await ffmpeg.exec(['-i', 'input.mp4', '-t', String(previewDuration), '-c', 'copy', '-movflags', '+faststart', 'preview.mp4']);
const previewData = await ffmpeg.readFile('preview.mp4');
const previewBlob = new Blob([previewData], { type: 'video/mp4' });

// 1b. (Optional) Generate thumbnail
await ffmpeg.exec(['-i', 'input.mp4', '-vframes', '1', '-q:v', '2', 'thumb.jpg']);
const thumbData = await ffmpeg.readFile('thumb.jpg');
const thumbBlob = new Blob([thumbData], { type: 'image/jpeg' });

// 1c. Upload preview + thumbnail to PayLock
const form = new FormData();
form.append('preview', previewBlob, 'preview.mp4');
form.append('thumbnail', thumbBlob, 'thumb.jpg');
form.append('title', 'My Paid Video');
form.append('price', '1000000000');  // 1 SUI = 10^9 MIST

const timestamp = Math.floor(Date.now() / 1000);
const message = `paylock:upload::${timestamp}`;
const msgBytes = new TextEncoder().encode(message);
const { signature } = await keypair.signPersonalMessage(msgBytes);

const res = await fetch(`${PAYLOCK}/api/upload`, {
  method: 'POST',
  headers: {
    'X-Wallet-Address': creatorAddress,
    'X-Wallet-Sig': signature,
    'X-Wallet-Timestamp': timestamp.toString(),
  },
  body: form,
});
const { id: paylockId } = await res.json();
// → 202 Accepted, { id: "abc123", status: "processing" }
```

> **Free videos** (`price=0`): No wallet signature needed. Send the full video in the `video` field instead of `preview`. The server processes preview, thumbnail, and full video. Once complete, you can stream directly — Steps 2 & 3 are not needed.

#### Step 2: Wait for Server Processing

Use SSE for real-time tracking, or poll the status endpoint:

```js
// Option A: SSE (recommended)
const events = new EventSource(`/api/status/${paylockId}/events`);
events.onmessage = (e) => {
  const video = JSON.parse(e.data);
  if (video.status === 'ready') {
    events.close();
    // video.preview_blob_id is now available, proceed to Step 3
  }
};

// Option B: Polling
const statusRes = await fetch(`/api/status/${paylockId}`);
const video = await statusRes.json();
// Repeat until video.status === 'ready'
```

Once server processing is complete, the response contains `preview_blob_id` and `preview_blob_url`, which are needed in subsequent steps.

#### Step 3: Encrypt Full Video & Publish On-Chain

This is the only step in the integration that requires direct use of the Seal and Walrus SDKs. The server has already processed the preview — the developer needs to encrypt the original video on the frontend, upload the encrypted blob, and create the on-chain Video object.

```js
import { SealClient } from '@mysten/seal';
import { Transaction } from '@mysten/sui/transactions';

// 3a. Encrypt the original video
const namespace = crypto.getRandomValues(new Uint8Array(32));
const nonce = crypto.getRandomValues(new Uint8Array(5));
const id = toHex(new Uint8Array([...namespace, ...nonce]));

const { encryptedObject } = await sealClient.encrypt({
  threshold: 1,
  packageId: config.gating_package_id,
  id,
  data: new Uint8Array(fileData),
});

// 3b. Upload encrypted blob to Walrus
const walrusRes = await fetch(`${config.walrus_publisher_url}/v1/blobs?epochs=5`, {
  method: 'PUT',
  body: encryptedObject,
});
const walrusData = await walrusRes.json();
const fullBlobId =
  walrusData.newlyCreated?.blobObject?.blobId ??
  walrusData.alreadyCertified?.blobId;

// 3c. Create on-chain Video object
const tx = new Transaction();
tx.moveCall({
  target: `${config.gating_package_id}::gating::create_video`,
  arguments: [
    tx.pure.u64(priceMist),
    tx.pure.string(video.preview_blob_id),       // From Step 2 API response
    tx.pure.string(fullBlobId),
    tx.pure.vector('u8', Array.from(namespace)),
  ],
});
// Sign and execute, obtain suiObjectId
```

#### Step 4: Write Back to API

Write back the on-chain object ID to PayLock, completing the `paylock_id` → `sui_object_id` association. After this, all stream endpoints can be accessed via `sui_object_id`.

```js
// Generate wallet signature (action=update)
const timestamp = Math.floor(Date.now() / 1000);
const message = `paylock:update:${paylockId}:${timestamp}`;
const msgBytes = new TextEncoder().encode(message);
const { signature } = await keypair.signPersonalMessage(msgBytes);

await fetch(`${PAYLOCK}/api/videos/${paylockId}`, {
  method: 'PUT',
  headers: {
    'Content-Type': 'application/json',
    'X-Wallet-Address': creatorAddress,
    'X-Wallet-Sig': signature,
    'X-Wallet-Timestamp': timestamp.toString(),
  },
  body: JSON.stringify({
    sui_object_id: suiObjectId,
    full_blob_id: fullBlobId,
  }),
});
// → 200 OK, { status: "ok", sui_object_id: "0x..." }
```

At this point, the video is available for anyone to preview via `GET /stream/{sui_object_id}/preview`.

---

### Viewer Flow

#### Step 5: Browse & Discover Videos

Discover available videos through the PayLock API:

```js
// List all videos (supports pagination and creator filtering)
const listRes = await fetch('/api/videos?page=1&per_page=20');
const { videos, total } = await listRes.json();

// Query a single video by on-chain object ID (paylock_id also works)
const videoRes = await fetch(`/api/status/${suiObjectId}`);
const video = await videoRes.json();
// → { price, encrypted, preview_blob_url, full_blob_url, sui_object_id, ... }
```

#### Step 6: Preview Playback

Preview streaming requires no authentication or purchase:

```js
// Browser auto-follows 307 redirect to Walrus blob URL
videoElement.src = `/stream/${video.sui_object_id}/preview`;
videoElement.play();
```

#### Step 7: Purchase & Decrypt Playback

After purchasing, the viewer receives an AccessPass and can decrypt the full version via Seal. This step requires on-chain transactions.

```js
import { Transaction } from '@mysten/sui/transactions';
import { SealClient, SessionKey, EncryptedObject } from '@mysten/seal';

// 7a. Check if user already owns an AccessPass (avoid duplicate purchases)
const { data: ownedObjects } = await suiClient.getOwnedObjects({
  owner: buyerAddress,
  filter: {
    StructType: `${config.gating_package_id}::gating::AccessPass`,
  },
  options: { showContent: true },
});
const existingPass = ownedObjects.find((obj) => {
  const fields = obj.data?.content?.fields;
  return fields?.video_id === video.sui_object_id;
});

let accessPassId;
if (existingPass) {
  // Already purchased, use existing AccessPass
  accessPassId = existingPass.data.objectId;
} else {
  // 7b. Purchase — mint AccessPass
  const tx = new Transaction();
  const [coin] = tx.splitCoins(tx.gas, [tx.pure.u64(video.price)]);
  tx.moveCall({
    target: `${config.gating_package_id}::gating::purchase_and_transfer`,
    arguments: [
      tx.object(video.sui_object_id),
      coin,
    ],
  });
  const result = await signAndExecuteTransaction({ transaction: tx });

  // Get the newly created AccessPass object ID from transaction result
  const created = result.effects?.created;
  accessPassId = created?.find(
    (obj) => obj.owner?.AddressOwner === buyerAddress
  )?.reference?.objectId;
}

// 7c. Get encrypted blob URL via PayLock API
const fullUrl = `${PAYLOCK}/stream/${video.sui_object_id}/full`;
// 307 redirects to the encrypted blob on Walrus aggregator

// 7d. Download encrypted blob and decrypt with Seal
const encryptedRes = await fetch(fullUrl);
const encryptedData = new Uint8Array(await encryptedRes.arrayBuffer());

// Create SessionKey (valid for 10 minutes, must recreate after expiry)
const sessionKey = await SessionKey.create({
  address: buyerAddress,
  packageId: config.gating_package_id,
  ttlMin: 10,
  suiClient,
});
const personalMessage = sessionKey.getPersonalMessage();
const { signature } = await wallet.signPersonalMessage({ message: personalMessage });
sessionKey.setPersonalMessageSignature(signature);

// Build seal_approve transaction for Seal key server verification
const parsed = EncryptedObject.parse(encryptedData);
const approveTx = new Transaction();
approveTx.moveCall({
  target: `${config.gating_package_id}::gating::seal_approve`,
  arguments: [
    approveTx.pure.vector('u8', fromHex(parsed.id)),
    approveTx.object(accessPassId),
    approveTx.object(video.sui_object_id),
  ],
});
const txBytes = await approveTx.build({ client: suiClient, onlyTransactionKind: true });

const decryptedBytes = await sealClient.decrypt({
  data: encryptedData,
  sessionKey,
  txBytes,
});

// 7e. Play
const blob = new Blob([decryptedBytes], { type: 'video/mp4' });
videoElement.src = URL.createObjectURL(blob);
videoElement.play();
```

> **Error Handling Tips**:
>
> - `purchase_and_transfer` automatically refunds excess SUI, but the transaction will fail if the amount is insufficient. Compare `video.price` with the user's balance first.
> - `SessionKey` expires after the TTL (default 10 minutes) and must be recreated and re-signed.
> - Seal decryption failures usually indicate an invalid AccessPass or expired SessionKey.

---

### Integration Summary

| Step | Role | PayLock API | On-Chain Transaction |
|------|------|-------------|----------------------|
| 1. Upload Video | Creator | `POST /api/upload` | — |
| 2. Wait for Processing | Creator | `GET /api/status/{id}/events` | — |
| 3. Encrypt & Publish | Creator | — | `create_video` |
| 4. Write Back | Creator | `PUT /api/videos/{id}` | — |
| 5. Browse Videos | Viewer | `GET /api/videos` | — |
| 6. Preview Playback | Viewer | `GET /stream/{id}/preview` | — |
| 7. Purchase & Decrypt | Viewer | `GET /stream/{id}/full` | `purchase_and_transfer` + `seal_approve` |

> 5 out of 7 steps only require calling the PayLock API — no direct Walrus or Seal interaction needed.

### Move Contract Reference (`paylock::gating`)

On-chain functions used in Step 3 and Step 7:

| Function | Type | Description |
|----------|------|-------------|
| `create_video(price, preview_blob_id, full_blob_id, seal_namespace, ctx)` | public | Creates a Video shared object. `seal_namespace` must not be empty when price > 0 |
| `purchase_and_transfer(video, payment, ctx)` | entry | Purchases a video, mints AccessPass and transfers to buyer, automatically refunds excess SUI |
| `seal_approve(id, pass, video)` | entry | Verifies AccessPass + Seal ID prefix, authorizes decryption for Seal key server |
| `seal_approve_owner(id, video, ctx)` | entry | Creator self-decrypts (no AccessPass needed) |

**Key Structs**:

```move
struct Video has key {
    id: UID,
    price: u64,
    creator: address,
    preview_blob_id: String,
    full_blob_id: String,
    seal_namespace: vector<u8>,
}

struct AccessPass has key, store {
    id: UID,
    video_id: ID,
}
```

---

## FAQ / Troubleshooting

### ID Detection Logic

`GET /api/status/{id}` and `/stream/{id}/*` both support `paylock_id` and `sui_object_id`. Detection rule: IDs starting with `0x` are treated as `sui_object_id`, otherwise as `paylock_id`. The two formats are different and will not collide.

### What Happens When Walrus Blobs Expire?

Walrus storage is paid per epoch (default 5 epochs). After a blob expires, it can no longer be played, and currently **cannot be renewed**. Recommendations:

- Paid videos should inform users about the storage validity period
- Future versions will support epoch renewal

### CORS Errors When Calling `/api/*` from Frontend

`/api/*` endpoints do not have CORS enabled. Solutions:

1. **Recommended**: Proxy PayLock API requests through your own backend
2. Self-host PayLock and add a reverse proxy (e.g. nginx) with CORS headers

`/stream/*` endpoints have CORS enabled — frontends can use `<video>` tags directly.

### Wallet Signature Verification Failed (403)

Common causes:

- **Timestamp expired**: Signature timestamp differs from server time by more than 60 seconds. Ensure the client clock is accurate.
- **Action mismatch**: The canonical message action must match the endpoint (`upload` / `update` / `delete`).
- **Address mismatch**: Signature address doesn't match the video's `creator` field (comparison is case-insensitive).

### Upload Stuck on `processing`

- **Free videos**: Verify that the server has FFmpeg enabled (`PAYLOCK_ENABLE_FFMPEG=true`)
- **Paid videos**: FFmpeg is not required on the server (preview is generated client-side). If FFmpeg is available, the server validates the preview duration — check the `error` field for "preview too long" messages
- Check that the Walrus Publisher is reachable
- Use SSE (`/api/status/{id}/events`) to monitor — if `status=failed` is received, the `error` field explains the reason

### How to Check if a User Has Already Purchased a Video?

Query the user's owned AccessPass objects:

```js
const { data } = await suiClient.getOwnedObjects({
  owner: userAddress,
  filter: {
    StructType: `${config.gating_package_id}::gating::AccessPass`,
  },
  options: { showContent: true },
});
const hasPurchased = data.some(
  (obj) => obj.data?.content?.fields?.video_id === videoSuiObjectId
);
```

### Related External Documentation

- **Seal SDK**: [https://docs.mystenlabs.com/seal](https://docs.mystenlabs.com/seal)
- **Walrus**: [https://docs.walrus.site](https://docs.walrus.site)
- **Sui TypeScript SDK**: [https://sdk.mystenlabs.com/typescript](https://sdk.mystenlabs.com/typescript)
- **Move contract source**: `contracts/sources/gating.move`

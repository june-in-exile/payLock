# PayLock API Reference

本文檔提供 PayLock 後端服務的完整 API 規格，專為希望將「影片付費解鎖」功能整合進其 dApp 的開發者設計。

---

## 目錄

- [PayLock API Reference](#paylock-api-reference)
  - [目錄](#目錄)
  - [核心流程總覽](#核心流程總覽)
    - [免費影片](#免費影片)
    - [付費影片](#付費影片)
  - [共用格式](#共用格式)
    - [影片狀態 (Status)](#影片狀態-status)
    - [Video 物件完整欄位](#video-物件完整欄位)
    - [錯誤回應格式](#錯誤回應格式)
  - [API 端點](#api-端點)
    - [1. 上傳影片](#1-上傳影片)
    - [2. 查詢影片狀態](#2-查詢影片狀態)
    - [3. 即時狀態追蹤 (SSE)](#3-即時狀態追蹤-sse)
    - [4. 關聯鏈上物件](#4-關聯鏈上物件)
    - [5. 以鏈上物件 ID 查詢影片](#5-以鏈上物件-id-查詢影片)
    - [6. 列出所有影片](#6-列出所有影片)
    - [7. 刪除影片](#7-刪除影片)
    - [8. 預覽串流](#8-預覽串流)
    - [9. 完整版串流](#9-完整版串流)
    - [10. 系統配置](#10-系統配置)
    - [11. 手動重新索引](#11-手動重新索引)
  - [付費解鎖整合指南](#付費解鎖整合指南)
    - [前置準備](#前置準備)
    - [創作者端流程](#創作者端流程)
      - [Step 1: 上傳影片](#step-1-上傳影片)
      - [Step 2: 等待伺服器處理完成](#step-2-等待伺服器處理完成)
      - [Step 3: 加密完整版並發布上鏈](#step-3-加密完整版並發布上鏈)
      - [Step 4: 回寫 API 完成關聯](#step-4-回寫-api-完成關聯)
    - [觀眾端流程](#觀眾端流程)
      - [Step 5: 瀏覽與探索影片](#step-5-瀏覽與探索影片)
      - [Step 6: 預覽播放](#step-6-預覽播放)
      - [Step 7: 購買與解密播放](#step-7-購買與解密播放)
    - [整合摘要](#整合摘要)
    - [Move 合約參考 (`paylock::gating`)](#move-合約參考-paylockgating)

---

## 核心流程總覽

### 免費影片

```
POST /api/upload (price=0)
    → 伺服器處理預覽 / 縮圖 / 完整版並上傳至 Walrus
    → GET /api/status/{id} 輪詢至 status=ready
    → GET /stream/{id} 播放
```

### 付費影片

```
POST /api/upload (price>0)
    → 伺服器處理預覽 / 縮圖並上傳至 Walrus
    → GET /api/status/{id}/events (SSE) 等待 status=ready
    → 前端以 Seal SDK 加密原始影片 → 上傳加密 Blob 至 Walrus → 取得 full_blob_id
    → 前端發 Sui 交易呼叫 gating::create_video (帶 price, preview_blob_id, full_blob_id, seal_namespace)
    → PUT /api/videos/{id} (回寫 sui_object_id + full_blob_id)
    → 購買者: purchase_and_transfer → seal_approve → Seal 解密 → 播放
```

---

## 共用格式

### 影片狀態 (Status)

| 值 | 說明 |
|----|------|
| `processing` | 上傳已接收，背景處理中 |
| `ready` | 預覽 / 完整版已上傳至 Walrus，可串流 |
| `failed` | 處理失敗，`error` 欄位包含原因 |

### Video 物件完整欄位

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

- `encrypted`: 付費影片為 `true`，表示 `full_blob_id` 指向 Seal 加密後的 Blob。
- `error`: 僅在 `status=failed` 時出現。
- 帶 `omitempty` 的欄位在值為空時不會出現在 response 中。

### 錯誤回應格式

所有錯誤統一回傳：

```json
{ "error": "描述訊息" }
```

---

## API 端點

### 1. 上傳影片

**`POST /api/upload`**

發起非同步上傳。伺服器驗證檔案後開始背景處理。

- **Content-Type**: `multipart/form-data`
- **大小上限**: 由 `PAYLOCK_MAX_FILE_SIZE_MB` 控制（預設 500 MB），超過回傳 `413`。
- **支援格式**: MP4 (`.mp4`), MOV (`.mov`), WebM (`.webm`), MKV (`.mkv`), AVI (`.avi`)。以 magic bytes 驗證，非副檔名。

| 參數 | 必填 | 說明 |
|------|------|------|
| `video` | 是 | 影片檔案 |
| `title` | 否 | 影片標題，未提供則自動產生 |
| `price` | 否 | 價格 (MIST, uint64)。`0` 或未提供 = 免費影片 |
| `creator` | 條件必填 | 創作者的 Sui 地址。`price > 0` 時必填 |

> **付費上傳限制**: `price > 0` 時，必須提供 `creator` 且伺服器必須啟用 FFmpeg (`PAYLOCK_ENABLE_FFMPEG=true`)，否則回傳 `400`。

**成功回應** (`202 Accepted`):

```json
{
  "id": "a1b2c3d4e5f6g7h8",
  "status": "processing"
}
```

**錯誤回應**:

| Status | 原因 |
|--------|------|
| `400` | 無法讀取檔案 / 格式不支援 / price 非正整數 / 付費上傳缺少 creator / 付費上傳但 FFmpeg 未啟用 |
| `413` | 檔案超過大小上限 |

---

### 2. 查詢影片狀態

**`GET /api/status/{id}`**

取得特定影片的完整 Metadata。

**成功回應** (`200 OK`): 回傳完整 Video 物件（見上方欄位定義）。

**錯誤回應**:

| Status | 原因 |
|--------|------|
| `400` | 缺少 video id |
| `404` | 影片不存在 |

---

### 3. 即時狀態追蹤 (SSE)

**`GET /api/status/{id}/events`**

Server-Sent Events 串流，每當影片狀態變更時推送完整 Video 物件。連線後立即推送一次目前狀態。適合用於上傳後等待處理完成。

```text
data: {"id":"...","status":"processing","title":"My Video","price":0,"created_at":"..."}

data: {"id":"...","status":"ready","preview_blob_id":"...","preview_blob_url":"...","full_blob_id":"...","full_blob_url":"...","created_at":"..."}
```

連線在 `status` 變為 `ready` 或 `failed` 後由伺服器關閉。

---

### 4. 關聯鏈上物件

**`PUT /api/videos/{id}`**

前端完成鏈上 `create_video` 交易後，將 Sui 物件 ID 與加密完整 Blob ID 寫回後端。

- **需要認證**: 須附帶 `X-Creator` header，值為影片的創作者 Sui 地址。

**Request Body** (`application/json`):

```json
{
  "sui_object_id": "0x789...abc",
  "full_blob_id": "blobId123"
}
```

| 欄位 | 必填 | 說明 |
|------|------|------|
| `sui_object_id` | 是 | 鏈上 Video shared object 的 ID |
| `full_blob_id` | 否 | 加密後完整 Blob 的 Walrus blob ID（付費影片應提供） |

**成功回應** (`200 OK`):

```json
{
  "status": "ok",
  "sui_object_id": "0x789...abc"
}
```

**錯誤回應**:

| Status | 原因 |
|--------|------|
| `400` | 缺少 video id / request body 無效 / `sui_object_id` 為空 |
| `403` | `X-Creator` 不符合影片的 creator |
| `404` | 影片不存在 |
| `409` | 該影片已綁定不同的 `sui_object_id` |

---

### 5. 以鏈上物件 ID 查詢影片

**`GET /api/videos/by-object/{object_id}`**

以 Sui 鏈上的 `sui_object_id` 查詢對應的影片 Metadata。

**成功回應** (`200 OK`): 回傳完整 Video 物件（見上方欄位定義）。

**錯誤回應**:

| Status | 原因            |
|--------|-----------------|
| `400`  | 缺少 object_id  |
| `404`  | 影片不存在      |

---

### 6. 列出所有影片

**`GET /api/videos`**

取得影片列表，按 `created_at` 降序排列（最新在前）。支援篩選與分頁。

**Query Parameters**:

| 參數 | 預設 | 說明 |
|------|------|------|
| `creator` | *(無)* | 按創作者 Sui 地址篩選 |
| `page` | `1` | 頁碼（從 1 開始） |
| `per_page` | `20` | 每頁筆數（上限 100） |

**成功回應** (`200 OK`):

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

### 7. 刪除影片

**`DELETE /api/videos/{id}`**

從後端 Metadata Store 中刪除該影片記錄。

- **需要認證**: 須附帶 `X-Creator` header，值為影片的創作者 Sui 地址。

> **注意**: 這不會刪除 Walrus 上的 Blob 或鏈上的 Video 物件。

**成功回應** (`200 OK`):

```json
{ "id": "...", "status": "deleted" }
```

**錯誤回應**:

| Status | 原因 |
|--------|------|
| `403` | `X-Creator` 不符合影片的 creator |
| `404` | 影片不存在 |

---

### 8. 預覽串流

**`GET /stream/{id}/preview`**

307 Redirect 至預覽版在 Walrus 上的公開 URL。任何人皆可存取。

```html
<video src="https://your-paylock-host/stream/{id}/preview"></video>
```

> **已棄用路徑**: `GET /stream/{id}` 仍可使用，會 307 Redirect 至 `/stream/{id}/preview` 並附帶 `Deprecation` header。預計 2026-09-23 移除。

**錯誤回應**:

| Status | 原因 |
|--------|------|
| `400` | 缺少 video id |
| `404` | 影片不存在 |
| `500` | 影片無 preview blob URL |
| `503` | 影片尚未就緒 (status != ready) |

---

### 9. 完整版串流

**`GET /stream/{id}/full`**

307 Redirect 至完整版 Blob URL。付費影片回傳的是加密後的 Blob，需前端 Seal 解密。

**錯誤回應**: 同預覽串流。

---

### 10. 系統配置

**`GET /api/config`**

取得後端環境配置。整合者應透過此 API 取得合約與 Walrus 端點，而非硬編碼。

**成功回應** (`200 OK`):

```json
{
  "gating_package_id": "0x...",
  "sui_network": "testnet",
  "walrus_publisher_url": "https://publisher.walrus-testnet.walrus.space",
  "walrus_aggregator_url": "https://aggregator.walrus-testnet.walrus.space"
}
```

---

### 11. 手動重新索引

**`POST /api/reindex`**

觸發從 Sui 鏈上重新掃描所有 Video 物件，將缺少的記錄補入本地 VideoStore。啟動時伺服器會自動執行一次，此端點供手動觸發。

- **需要認證**: 須附帶 `Authorization: Bearer <PAYLOCK_ADMIN_SECRET>` header。若未設定 `PAYLOCK_ADMIN_SECRET` 環境變數，此端點永遠回傳 `401`。

**成功回應** (`200 OK`):

```json
{
  "status": "ok",
  "chain_total": 120,
  "new_entries": 3
}
```

| 欄位           | 說明                         |
|----------------|------------------------------|
| `chain_total`  | 鏈上掃描到的 Video 物件總數  |
| `new_entries`  | 本次新增至本地 Store 的筆數  |

**錯誤回應**:

| Status | 原因                         |
|--------|------------------------------|
| `401`  | 缺少或無效的 admin secret    |
| `500`  | 鏈上掃描失敗                 |

---

## 付費解鎖整合指南

本指南說明外部開發者如何透過 PayLock API 實作付費影片的完整流程。PayLock 伺服器負責影片處理、預覽產生、Walrus 儲存及串流導向，開發者只需呼叫 API 端點即可完成大部分整合。

> **鏈上操作**：創作者發布與觀眾購買涉及 Sui 鏈上交易，須使用 `@mysten/sui` SDK。付費影片的完整版加密使用 `@mysten/seal`。這些是整合中唯一需要直接與鏈互動的部分。

### 前置準備

```js
// 從 PayLock API 取得所有必要設定
const configRes = await fetch('https://your-paylock-server/api/config');
const config = await configRes.json();
// → { gating_package_id, sui_network, walrus_publisher_url, walrus_aggregator_url }
```

---

### 創作者端流程

#### Step 1: 上傳影片

透過 `POST /api/upload` 上傳影片，伺服器自動處理預覽與縮圖並上傳至 Walrus。

```js
const form = new FormData();
form.append('video', videoFile);
form.append('title', 'My Paid Video');
form.append('price', '1000000000');  // 1 SUI = 10^9 MIST
form.append('creator', creatorAddress);

const res = await fetch('/api/upload', { method: 'POST', body: form });
const { id: paylockId } = await res.json();
// → 202 Accepted, { id: "abc123", status: "processing" }
```

> **免費影片** (`price=0`): 伺服器會處理預覽與完整版，上傳完成後即可直接串流，不需要 Step 2 & 3。

#### Step 2: 等待伺服器處理完成

使用 SSE 即時追蹤，或輪詢 status 端點：

```js
// 方式 A: SSE（推薦）
const events = new EventSource(`/api/status/${paylockId}/events`);
events.onmessage = (e) => {
  const video = JSON.parse(e.data);
  if (video.status === 'ready') {
    events.close();
    // video.preview_blob_id 已可用，進入 Step 3
  }
};

// 方式 B: 輪詢
const statusRes = await fetch(`/api/status/${paylockId}`);
const video = await statusRes.json();
// 重複直到 video.status === 'ready'
```

伺服器處理完成後，回應中包含 `preview_blob_id` 和 `preview_blob_url`，後續步驟會用到。

#### Step 3: 加密完整版並發布上鏈

這是整合中唯一需要直接使用 Seal 和 Walrus SDK 的步驟。伺服器已處理好預覽，開發者需在前端加密原始影片、上傳加密 Blob、並建立鏈上 Video 物件。

```js
import { SealClient } from '@mysten/seal';
import { Transaction } from '@mysten/sui/transactions';

// 3a. 加密原始影片
const namespace = crypto.getRandomValues(new Uint8Array(32));
const nonce = crypto.getRandomValues(new Uint8Array(5));
const id = toHex(new Uint8Array([...namespace, ...nonce]));

const { encryptedObject } = await sealClient.encrypt({
  threshold: 1,
  packageId: config.gating_package_id,
  id,
  data: new Uint8Array(fileData),
});

// 3b. 上傳加密 Blob 至 Walrus
const walrusRes = await fetch(`${config.walrus_publisher_url}/v1/blobs?epochs=5`, {
  method: 'PUT',
  body: encryptedObject,
});
const walrusData = await walrusRes.json();
const fullBlobId =
  walrusData.newlyCreated?.blobObject?.blobId ??
  walrusData.alreadyCertified?.blobId;

// 3c. 建立鏈上 Video 物件
const tx = new Transaction();
tx.moveCall({
  target: `${config.gating_package_id}::gating::create_video`,
  arguments: [
    tx.pure.u64(priceMist),
    tx.pure.string(video.preview_blob_id),       // 來自 Step 2 的 API 回應
    tx.pure.string(fullBlobId),
    tx.pure.vector('u8', Array.from(namespace)),
  ],
});
// 簽署並執行，取得 suiObjectId
```

#### Step 4: 回寫 API 完成關聯

將鏈上物件 ID 回寫至 PayLock，完成 `paylock_id` → `sui_object_id` 的關聯。此後所有串流端點均可透過 `sui_object_id` 存取。

```js
await fetch(`/api/videos/${paylockId}`, {
  method: 'PUT',
  headers: {
    'Content-Type': 'application/json',
    'X-Creator': creatorAddress,
  },
  body: JSON.stringify({
    sui_object_id: suiObjectId,
    full_blob_id: fullBlobId,
  }),
});
// → 200 OK, { status: "ok", sui_object_id: "0x..." }
```

至此，影片已可透過 `GET /stream/{sui_object_id}/preview` 供所有人預覽。

---

### 觀眾端流程

#### Step 5: 瀏覽與探索影片

透過 PayLock API 發現可用影片：

```js
// 列出所有影片（支援分頁與創作者篩選）
const listRes = await fetch('/api/videos?page=1&per_page=20');
const { videos, total } = await listRes.json();

// 以鏈上物件 ID 查詢單一影片
const videoRes = await fetch(`/api/videos/by-object/${suiObjectId}`);
const video = await videoRes.json();
// → { price, encrypted, preview_blob_url, full_blob_url, sui_object_id, ... }
```

#### Step 6: 預覽播放

預覽串流無需任何認證或購買，直接使用：

```js
// 瀏覽器會自動跟隨 307 重導至 Walrus blob URL
videoElement.src = `/stream/${video.sui_object_id}/preview`;
videoElement.play();
```

#### Step 7: 購買與解密播放

觀眾購買後取得 AccessPass，再透過 Seal 解密完整版。此步驟需要鏈上交易。

```js
import { Transaction } from '@mysten/sui/transactions';
import { SealClient, SessionKey, EncryptedObject } from '@mysten/seal';

// 7a. 購買（若尚未持有 AccessPass）
const tx = new Transaction();
const [coin] = tx.splitCoins(tx.gas, [tx.pure.u64(video.price)]);
tx.moveCall({
  target: `${config.gating_package_id}::gating::purchase_and_transfer`,
  arguments: [
    tx.object(video.sui_object_id),
    coin,
  ],
});
// 簽署並執行，取得 AccessPass object ID

// 7b. 透過 PayLock API 取得加密 blob URL
const fullUrl = `/stream/${video.sui_object_id}/full`;
// 307 重導至 Walrus aggregator 上的加密 blob

// 7c. 下載加密 Blob 並以 Seal 解密
const encryptedRes = await fetch(fullUrl);
const encryptedData = new Uint8Array(await encryptedRes.arrayBuffer());

const sessionKey = await SessionKey.create({
  address: buyerAddress,
  packageId: config.gating_package_id,
  ttlMin: 10,
  suiClient,
});
const message = sessionKey.getPersonalMessage();
const { signature } = await wallet.signPersonalMessage({ message });
sessionKey.setPersonalMessageSignature(signature);

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

// 7d. 播放
const blob = new Blob([decryptedBytes], { type: 'video/mp4' });
videoElement.src = URL.createObjectURL(blob);
videoElement.play();
```

---

### 整合摘要

| 步驟 | 角色 | PayLock API | 鏈上交易 |
|------|------|-------------|----------|
| 1. 上傳影片 | 創作者 | `POST /api/upload` | — |
| 2. 等待處理 | 創作者 | `GET /api/status/{id}/events` | — |
| 3. 加密 & 發布 | 創作者 | — | `create_video` |
| 4. 回寫關聯 | 創作者 | `PUT /api/videos/{id}` | — |
| 5. 瀏覽影片 | 觀眾 | `GET /api/videos` | — |
| 6. 預覽播放 | 觀眾 | `GET /stream/{id}/preview` | — |
| 7. 購買 & 解密 | 觀眾 | `GET /stream/{id}/full` | `purchase_and_transfer` + `seal_approve` |

> 7 個步驟中有 5 個只需呼叫 PayLock API，無需直接操作 Walrus 或 Seal。

### Move 合約參考 (`paylock::gating`)

以下為 Step 3 和 Step 7 中使用的鏈上函式：

| Function | 類型 | 說明 |
|----------|------|------|
| `create_video(price, preview_blob_id, full_blob_id, seal_namespace, ctx)` | public | 建立 Video shared object。price > 0 時 seal_namespace 不可為空 |
| `purchase_and_transfer(video, payment, ctx)` | entry | 購買影片，鑄造 AccessPass 並轉移給買家，自動退還多餘 SUI |
| `seal_approve(id, pass, video)` | entry | 驗證 AccessPass + Seal ID prefix，供 Seal key server 授權解密 |
| `seal_approve_owner(id, video, ctx)` | entry | 創作者自行解密（無需 AccessPass） |

**關鍵 Struct**:

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

# Orca — Decentralized Video Paywall SDK for Sui

## 簡介

Orca 是建立在 Walrus + Seal 之上的影片付費解鎖 SDK。影片上傳時自動拆為預覽片段與完整版，預覽公開播放，完整版透過 Seal 加密存儲於 Walrus。觀眾付費後取得鏈上存取憑證，解密後即可觀看完整內容。開發者只需幾行程式碼即可為任何 dApp 加入影片付費牆功能。

## 現有功能 (Phase 1)

- **雙 Blob 上傳**：上傳 MP4 後，FFmpeg 自動擷取前 N 秒作為預覽，preview + full 並行上傳至 Walrus Testnet。
- **預覽 / 完整版分離**：免費影片直接播完整版；付費影片播放預覽，結束時顯示 paywall overlay。
- **Move 合約 (`orca::paywall`)**：Video / AccessPass / purchase / seal_approve 已實作（待部署）。
- **非同步處理**：上傳後立即回傳 `202`，背景執行 FFmpeg 擷取 + Walrus 上傳，透過 polling 追蹤狀態。
- **前端 SPA**：上傳（含價格設定）、影片列表（價格 badge）、播放器（preview → paywall → full）、Slush Wallet 連接。

---

## 核心架構

### 上傳流程（Phase 1 — 明文雙 Blob）

```
上傳時：
  MP4 → FFmpeg 擷取前 N 秒 → 預覽 MP4（明文）→ 存 Walrus (Blob A)  ─┐
  MP4 → 完整版（明文，Phase 2 加入 Seal 加密）→ 存 Walrus (Blob B)  ─┤ 並行
  → 後端記錄兩個 Blob ID + 價格 + creator                            ─┘

播放時：
  免費影片 → 直接播放 Blob B（完整版）
  付費影片 → 播放 Blob A（預覽）→ 預覽結束 → 顯示 paywall → Play Full → 播放 Blob B
```

### 付費解鎖流程（Phase 2 目標）

```
  用戶進入 → 播放 Blob A（預覽，任何人可看）
  → 預覽結束 → 顯示付費牆
  → 付費 SUI → 鏈上 mint AccessPass
  → Seal 驗證 AccessPass → 解密 Blob B → 播放完整版
```

### 系統組件

```
[ 用戶端 / 前端 SPA ]
    │
    ▼
[ Orca Backend (Go) ]
    ├── POST /api/upload     驗證 MP4 → FFmpeg 擷取預覽 → 並行上傳雙 Blob
    ├── GET /api/status/{id} 查詢上傳狀態與 Blob IDs
    ├── GET /stream/{id}     307 redirect → 預覽 Blob URL
    └── GET /stream/{id}/full 307 redirect → 完整版 Blob URL
    │
    ├──── 寫入 ────▶ [ Walrus Publisher ] → [ Walrus Storage ]
    └──── 讀取 ────▶ [ Walrus Aggregator ] ← (Range Request 串流)

[ Sui 區塊鏈 ]  ← contracts/orca/sources/paywall.move
    Video / AccessPass / Seal Policy
```

---

## 鏈上合約設計

合約位於 `contracts/orca/sources/paywall.move`：

```move
module orca::paywall {
    /// 影片資訊，creator 上傳時建立（shared object）
    public struct Video has key {
        id: UID,
        price: u64,                // 解鎖價格（MIST）
        creator: address,          // 收款地址
        preview_blob_id: String,   // 預覽版 Walrus Blob ID
        full_blob_id: String,      // 完整版 Walrus Blob ID
    }

    /// 購買憑證，付費後 mint，永久有效（owned by buyer）
    public struct AccessPass has key {
        id: UID,
        video_id: ID,
    }

    /// Creator 發布影片
    public fun create_video(price, preview_blob_id, full_blob_id, ctx);

    /// 用戶付費 → mint AccessPass
    public fun purchase(video: &Video, payment: Coin<SUI>, ctx): AccessPass;

    /// Seal key server 驗證解密權限
    entry fun seal_approve(id: vector<u8>, pass: &AccessPass, video: &Video);
}
```

---

## 快速開始

### 前置需求

- Go 1.21+
- **FFmpeg**（必要，啟動時檢查）

### 環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `ORCA_PORT` | `8080` | HTTP 監聽埠 |
| `ORCA_WALRUS_PUBLISHER_URL` | `https://publisher.walrus-testnet.walrus.space` | Walrus Publisher |
| `ORCA_WALRUS_AGGREGATOR_URL` | `https://aggregator.walrus-testnet.walrus.space` | Walrus Aggregator |
| `ORCA_WALRUS_EPOCHS` | `5` | 儲存期數 |
| `ORCA_MAX_FILE_SIZE_MB` | `500` | 上傳大小限制 (MB) |
| `ORCA_FFMPEG_PATH` | `ffmpeg` | FFmpeg 路徑 |
| `ORCA_FFPROBE_PATH` | `ffprobe` | ffprobe 路徑 |
| `ORCA_PREVIEW_DURATION` | `10` | 預覽片段秒數 |
| `ORCA_SUI_RPC_URL` | `https://fullnode.testnet.sui.io:443` | Sui RPC |
| `ORCA_PAYWALL_PACKAGE_ID` | _(空)_ | 部署後的合約 Package ID |

### 啟動服務

```bash
make run
```

### 常用指令

```bash
make build        # 編譯至 bin/orca
make test         # 執行所有測試（含 race detector）
make lint         # go vet
make clean        # 清除 bin/
```

---

## API 參考

### 1. 上傳影片

`POST /api/upload`

- **Body**: `multipart/form-data`
  - `video` (必要) — MP4 檔案
  - `title` (選填) — 影片標題
  - `price` (選填) — 解鎖價格（MIST，1 SUI = 1,000,000,000 MIST）
  - `creator` (選填) — Creator 的 Sui address
- **Response** `202`:

  ```json
  { "id": "a1b2c3d4e5f6g7h8", "status": "processing" }
  ```

### 2. 查詢狀態

`GET /api/status/{id}`

- **Response**:

  ```json
  {
    "id": "...",
    "title": "My Video",
    "status": "ready",
    "price": 100000000,
    "creator": "0x...",
    "preview_blob_id": "...",
    "preview_blob_url": "https://aggregator.../v1/blobs/...",
    "full_blob_id": "...",
    "full_blob_url": "https://aggregator.../v1/blobs/..."
  }
  ```

### 3. 串流播放（預覽）

`GET /stream/{id}` — 307 Redirect 至預覽 Blob URL

### 4. 串流播放（完整版）

`GET /stream/{id}/full` — 307 Redirect 至完整版 Blob URL

### 5. 列出影片

`GET /api/videos` — 返回所有影片（按建立時間降序）

### 6. 刪除影片

`DELETE /api/videos/{id}` — 從記憶體中刪除影片記錄

---

## 發展路線 (Roadmap)

### Phase 1：Move 合約 + 雙 Blob 上傳 ✅

- [x] `orca::paywall` 合約（Video / AccessPass / purchase / seal_approve）
- [x] FFmpeg 預覽擷取（必要依賴，啟動時檢查）
- [x] 雙 Blob 並行上傳（preview + full → Walrus）
- [x] Video model 擴充（price, creator, preview/full blob IDs）
- [x] 前端 paywall UI（價格輸入、價格 badge、preview → paywall overlay → play full）
- [x] 部署合約至 Sui Testnet (`0xe1a529343f1c8f78af6d92c302524eab3731b13897a0c9ba98d7548ffeb90a90`)
- [x] 前端呼叫 `create_video` 建立鏈上 Video object

### Phase 2：付費解鎖 + Seal 加密

- 上傳時前端以 Seal SDK 加密完整版（`@mysten/seal` 是 browser-only）
- 前端付費牆：預覽播完 → 顯示價格 + 購買按鈕
- 錢包連接 → 呼叫 `purchase` → mint AccessPass
- Seal SDK 驗證 AccessPass → 取得解密金鑰 → 解密 Blob B
- 已購買用戶重新進入時自動偵測 AccessPass，直接播完整版

### Phase 3：Orca SDK 封裝

- `@orca/sdk`：付費 + 解密，與播放器完全解耦

  ```typescript
  import { Orca } from '@orca/sdk';

  const orca = new Orca({ network: 'mainnet' });
  const hasAccess = await orca.checkAccess(videoId, wallet);
  const videoUrl = await orca.unlock(videoId, wallet);
  videoElement.src = videoUrl;
  ```

- `@orca/uploader`：上傳 SDK（自動擷取預覽 + 加密 + 存 Walrus + 建鏈上 object）
- 文件 + 範例 dApp

### 之後逐步加入

- **Creator Dashboard**：上傳管理、收益統計、價格調整
- **批量訂閱**：一次付費解鎖 creator 所有影片
- **分潤機制**：嵌入者 / 推薦者可獲得銷售分成（鏈上自動分潤）
- **HLS 切片模式**：可選的進階模式，支援逐 segment 加密和多解析度

---

## 參考專案

| 專案 | 角色 | 備註 |
|---|---|---|
| [Walrus](https://docs.wal.app) | 去中心化儲存 + 原生串流 | Range Request 支援 MP4 串流播放 |
| [Seal](https://seal.mystenlabs.com) | 加密 + 存取控制 | Identity-based encryption + 鏈上 policy |
| [Seal Examples](https://github.com/MystenLabs/seal/tree/main/examples) | 參考實作 | Subscription pattern |
| [@mysten/seal](https://www.npmjs.com/package/@mysten/seal) | Seal TS SDK | 前端解密 |

---

## License

MIT

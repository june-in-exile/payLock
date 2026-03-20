# 🐋 Orca — Video Paywall Infrastructure for Sui

## 簡介

Orca 是建立在 Walrus + Seal 之上的影片付費解鎖基礎設施。影片上傳時自動切片，前段內容公開播放作為預覽，剩餘部分透過 Seal 加密存儲於 Walrus。觀眾付費後取得鏈上存取憑證，由 Seal key server 驗證後解密播放。整套流程——HLS 切片、部分加密、鏈上定價、付費驗證、播放器解密——封裝為 SDK，開發者只需幾行程式碼即可為任何 dApp 加入影片付費牆功能。

---

## ⚠️ v2 遷移說明

Orca v1 是影片感知的中間層（Video-Native Middleware），提供 Orca Gateway 代理串流和 Orca vs Walrus 效能對比功能。

**v2 移除了以下元件：**

| 移除項目 | 原因 |
|---------|------|
| Orca Gateway 串流代理 (`stream.go`) | 不再需要中間代理層，直接用 Walrus aggregator |
| Compare View（左右對比） | 不再比較 Orca vs Walrus |
| Walrus 代理 handler (`walrus.go`, `walrus_set.go`) | compare 專用，已移除 |
| `StorageBackend` 切換邏輯 | 統一用 Walrus，本地模式僅限開發 |

**保留的元件：**

| 保留項目 | 用途 |
|---------|------|
| FFmpeg 切片 (`processor/ffmpeg.go`) | 加上分段邏輯（明文/加密） |
| 上傳 handler (`handler/upload.go`) | 改為上傳到 Walrus + Seal 加密 |
| Walrus HTTP client (`walrus/walrus.go`) | 繼續用於 blob 上傳/讀取 |
| 影片 model + 驗證 | 擴充價格、預覽長度等欄位 |
| hls.js 播放器 | 替換 loader 為 SealLoader |

> 詳細遷移步驟見 [MIGRATION.md](./MIGRATION.md)

---

## 架構概覽

### 上傳流程

```
原始影片
  → FFmpeg 切成 HLS segments
  → 前 N 個 segments（預覽段）→ 明文存 Walrus
  → 剩餘 segments → Seal 加密後存 Walrus
  → 鏈上建立 Video object（metadata + 價格 + blob IDs）
```

### 播放流程

```
播放器載入 manifest
  → 播放免費預覽段（明文 segments，任何人可看）
  → 預覽結束，提示付費
  → 用戶付 SUI → 鏈上 mint AccessPass
  → hls.js custom loader 偵測到加密 segment
  → Seal SDK 驗證 AccessPass → 取得解密 key
  → Client-side 解密 → 繼續播放
```

### 系統組件

```
┌─────────────────────────────────────────────────┐
│                    dApp 開發者                    │
│         (import orca-sdk, 幾行程式碼整合)          │
├─────────────────────────────────────────────────┤
│  Orca SDK                                       │
│  ├── orca-player     hls.js + Seal 解密 loader   │
│  ├── orca-uploader   切片 + 部分加密 + 上傳       │
│  └── orca-contracts  Move 合約（定價/購買/驗證）   │
├─────────────────────────────────────────────────┤
│  Seal            │  Walrus                       │
│  加密 + 存取控制  │  去中心化儲存                   │
├─────────────────────────────────────────────────┤
│  Sui 區塊鏈                                      │
│  Video object / AccessPass / 付款                 │
└─────────────────────────────────────────────────┘
```

---

## 核心概念

### 部分加密策略

一部影片的 HLS segments 分為兩類：

```
seg000.ts  ← 明文（預覽）
seg001.ts  ← 明文（預覽）
seg002.ts  ← 🔒 Seal 加密
seg003.ts  ← 🔒 Seal 加密
seg004.ts  ← 🔒 Seal 加密
...
```

- **預覽段**：前 N 秒（預設 10 秒，可配置），任何人透過標準 HLS 播放
- **付費段**：用 Seal identity-based encryption 加密，只有持有 AccessPass 的用戶才能解密

### 為什麼用 Seal 而不是傳統加密？

| | 傳統加密（AES + 自建 key server） | Seal |
|---|---|---|
| Key 管理 | 自己管，單點故障 | 去中心化 threshold key server |
| 存取控制 | 後端邏輯，不透明 | 鏈上 Move 合約，可驗證 |
| 付費驗證 | 需要可信中間人 | 鏈上狀態即權限，無需信任 |
| 開發者負擔 | 自建 key server + 驗證邏輯 | `seal_approve` 一個函數搞定 |

### HLS 串流基礎

HLS 把影片切成多個 segment（通常 2-6 秒），加上一個 manifest（`.m3u8`）作為目錄：

```
/storage/{videoId}/
  ├── index.m3u8      ← Manifest（目錄）
  ├── seg000.ts       ← 明文預覽段
  ├── seg001.ts       ← 明文預覽段
  ├── seg002.ts       ← 🔒 加密段
  ├── seg003.ts       ← 🔒 加密段
  └── ...
```

播放器透過 manifest 知道要拉哪些 segments，Orca 的 custom loader 負責判斷是否需要解密。

---

## 鏈上合約設計

### 資料結構

```move
module orca::paywall;

/// 影片資訊，creator 上傳時建立
public struct Video has key {
    id: UID,
    price: u64,                // 解鎖價格（MIST）
    creator: address,          // 收款地址
    preview_blob_id: String,   // 預覽段 manifest blob ID
    full_blob_id: String,      // 完整 manifest blob ID（含加密段）
    preview_duration: u64,     // 預覽秒數
}

/// 購買憑證，付費後 mint 給用戶，永久有效
public struct AccessPass has key {
    id: UID,
    video_id: ID,
}

/// Creator 管理用
public struct CreatorCap has key {
    id: UID,
    video_id: ID,
}
```

### 核心函數

```move
/// 上傳影片 → 建立 Video object + CreatorCap
public fun create_video(
    price: u64,
    preview_blob_id: String,
    full_blob_id: String,
    preview_duration: u64,
    ctx: &mut TxContext,
): CreatorCap;

/// 用戶付費 → mint AccessPass
public fun purchase(
    video: &Video,
    payment: Coin<SUI>,
    ctx: &mut TxContext,
): AccessPass;

/// Seal key server 呼叫 → 驗證用戶是否有權解密
entry fun seal_approve(
    id: vector<u8>,
    pass: &AccessPass,
    video: &Video,
);

/// Creator 更新價格
public fun update_price(
    video: &mut Video,
    cap: &CreatorCap,
    new_price: u64,
);
```

---

## 技術選型

| 項目 | 選擇 | 原因 |
|---|---|---|
| Gateway | **Go** | Goroutine 適合 I/O 密集串流；既有程式碼可沿用 |
| 串流格式 | **HLS** | 最廣泛支援，所有瀏覽器 + 播放器都能播 |
| 轉碼 | **FFmpeg** | 成熟穩定，一行指令切片 |
| 底層儲存 | **Walrus** | Sui 生態原生，去中心化 blob 儲存 |
| 加密 | **Seal** | 鏈上 access control + threshold decryption |
| 播放器 | **hls.js + custom loader** | Client-side 解密，不需要中間代理 |
| 前端 SDK | **TypeScript** | 配合 Seal TS SDK + @mysten/sui |
| 合約 | **Sui Move** | Video object + AccessPass + seal_approve |

---

## Milestone 規劃

### Milestone 1 ✅：核心 HLS 串流（已完成）

Go gateway 實現基礎影片上傳、FFmpeg 切片、HLS 串流播放。

```
Go service
  ├── POST /upload      → 接收 MP4
  ├── FFmpeg            → 切成 HLS segments
  ├── 本地磁碟儲存
  └── GET /stream/{id}  → 回傳 .m3u8 + .ts
```

### Milestone 2 ✅：前端播放器 + Walrus 對比（已完成）

- hls.js 播放器 + 上傳頁面
- Walrus Aggregator 串接
- Orca vs Walrus 效能對比 demo

---

### Milestone 3（2 週）：Move 合約 — 付費解鎖核心

**目標：** 在 Sui testnet 部署 Orca paywall 合約，實現 Video 建立 → 付費 → AccessPass 發放的完整流程。

**具體工作：**

1. **合約開發**
   - `Video` object：價格、creator、blob IDs、預覽長度
   - `AccessPass` NFT：付費後 mint，永久有效
   - `seal_approve`：Seal key server 的驗證入口
   - `CreatorCap`：creator 管理（改價、下架）

2. **合約測試**
   - Move unit tests 覆蓋所有路徑（付費成功、金額不足、重複購買、權限檢查）
   - Sui testnet 部署 + 手動驗證

3. **CLI 驗證工具**
   - 用 Sui CLI 建立 Video、模擬購買、確認 AccessPass

**驗證方式：**

```bash
sui client call --function create_video --args 1000000 "preview_blob" "full_blob" 10
sui client call --function purchase --args $VIDEO_ID --gas-budget 10000000
# 確認 AccessPass 出現在用戶的 owned objects 中
```

**不做：** 前端整合、實際加密、Walrus 上傳

---

### Milestone 4（2 週）：部分加密上傳流程

**目標：** 上傳影片時自動將前 N 秒保留為明文，剩餘 segments 用 Seal 加密後存入 Walrus。

**具體工作：**

1. **切片 + 分類**
   - FFmpeg 切片後，根據預覽秒數將 segments 分為明文組和加密組
   - 可配置預覽長度（預設 10 秒）

2. **Seal 加密整合**
   - 加密組的每個 segment 用 Seal TypeScript SDK 加密
   - 加密 identity 綁定到 Video object ID（確保 `seal_approve` 能驗證）

3. **Walrus 上傳**
   - 明文 segments → 直接上傳 Walrus
   - 加密 segments → 加密後上傳 Walrus
   - 生成兩份 manifest：preview manifest（只含明文段）+ full manifest（含所有段）

4. **Manifest 管理**
   - Preview manifest：segment URL 指向 Walrus aggregator（公開可讀）
   - Full manifest：加密段標記為需要解密，URL 帶上加密 metadata

5. **鏈上註冊**
   - 上傳完成後呼叫 `create_video`，將 blob IDs + 價格寫入鏈上

**驗證方式：**

```bash
# 上傳影片，指定預覽 10 秒、價格 0.1 SUI
orca upload --file video.mp4 --preview 10 --price 100000000

# 預覽 manifest 可正常播放前 10 秒
curl https://aggregator.walrus.site/v1/blobs/$PREVIEW_BLOB_ID
# 加密段無法直接播放（密文）
curl https://aggregator.walrus.site/v1/blobs/$ENCRYPTED_SEGMENT_BLOB_ID
```

---

### Milestone 5（2 週）：播放器 Seal 解密 — Custom hls.js Loader

**目標：** 實作 hls.js custom loader，實現「預覽免費播 → 付費 → 解密繼續播」的完整體驗。

**具體工作：**

1. **SealLoader 實作**

   ```typescript
   class SealLoader extends Hls.DefaultConfig.loader {
     load(context, config, callbacks) {
       fetch(context.url)
         .then(res => res.arrayBuffer())
         .then(buf => {
           if (isEncryptedSegment(context.url)) {
             return sealDecrypt(buf, this.sealClient);
           }
           return buf;
         })
         .then(decrypted => callbacks.onSuccess({ data: decrypted }, context));
     }
   }
   ```

2. **付費牆 UI 流程**
   - 播放器播完預覽段 → 暫停 → 顯示付費提示（價格 + 購買按鈕）
   - 用戶連接錢包 → 呼叫 `purchase` → 取得 AccessPass
   - 自動切換到 full manifest → SealLoader 解密後續 segments

3. **Seal SDK 整合**
   - `@mysten/seal` TypeScript SDK
   - SealClient 配置 key server 列表
   - 解密時自動帶上 AccessPass 作為 `seal_approve` 參數

4. **錯誤處理**
   - 未付費嘗試播加密段 → 友善提示（不是 crash）
   - Seal key server 不可用 → 重試 + fallback
   - AccessPass 驗證失敗 → 提示重新購買

**驗證方式：**

- 未付費：播放前 10 秒後暫停，顯示付費牆
- 付費後：無縫繼續播放，console 無錯誤
- 重新整理頁面：已購買用戶直接播完整版（AccessPass 仍在鏈上）

---

### Milestone 6（2 週）：SDK 封裝 — 開發者可用的 npm package

**目標：** 將 Orca 封裝為 SDK，讓第三方 dApp 開發者用幾行程式碼加入影片付費牆。

**具體工作：**

1. **`@orca/contracts` — Move 合約 package**
   - 可直接 `sui move build` 部署
   - 或透過 Orca 已部署的共享合約（開發者不需要自己部署）

2. **`@orca/uploader` — 上傳 SDK**

   ```typescript
   import { OrcaUploader } from '@orca/uploader';

   const uploader = new OrcaUploader({ network: 'testnet' });
   const video = await uploader.upload({
     file: videoFile,
     previewSeconds: 10,
     price: 100_000_000, // 0.1 SUI
   });
   // video.id, video.previewManifestUrl, video.fullManifestUrl
   ```

3. **`@orca/player` — 播放器元件**

   ```typescript
   import { OrcaPlayer } from '@orca/player';

   // React component
   <OrcaPlayer
     videoId={videoId}
     onPurchaseRequired={(price) => showPaywall(price)}
     walletAdapter={walletAdapter}
   />
   ```

4. **文件 + 範例**
   - Quick Start Guide
   - API Reference
   - 範例 dApp（Next.js + Orca SDK）

**驗證方式：**

```bash
# 開發者體驗測試
npx create-next-app my-video-app
npm install @orca/player @orca/uploader
# 照文件貼 10 行程式碼 → 影片付費牆可用
```

---

### Milestone 7（2 週）：Creator Dashboard + 收益管理

**目標：** 提供 creator 管理介面，查看收益、管理影片。

**具體工作：**

1. **Creator Dashboard**
   - 影片列表（播放次數、收益、購買人數）
   - 上傳新影片（拖曳上傳 + 設定價格/預覽長度）
   - 修改價格 / 下架影片

2. **收益提領**
   - 合約直接將付款轉給 creator（即時到帳，無需提領）
   - Dashboard 顯示歷史收入記錄（讀取鏈上事件）

3. **數據分析**
   - 哪些影片轉換率高（預覽 → 購買）
   - 觀眾地域分佈（匿名統計）

---

### 之後逐步加入

- **多解析度（ABR）**：多品質轉檔 + master manifest，付費段各解析度皆加密
- **批量訂閱**：一次付費解鎖 creator 的所有影片（類似頻道訂閱）
- **分潤機制**：嵌入者 / 推薦者可獲得銷售分成（鏈上自動分潤）
- **Livepeer 轉碼整合**：去中心化轉碼，不用自己養 FFmpeg 機器
- **Edge cache / CDN**：熱門明文段快取加速

---

## 既有基礎（可沿用）

從先前的 Orca 開發中，以下元件可直接沿用：

| 元件 | 狀態 | 沿用方式 |
|---|---|---|
| Go Gateway | ✅ 已完成 | 繼續作為 HLS 串流 server |
| FFmpeg 切片 | ✅ 已完成 | 加上分類邏輯（明文/加密） |
| hls.js 播放器 | ✅ 已完成 | 替換 loader 為 SealLoader |
| Walrus 上傳 | ✅ 已完成 | 沿用，加上 Seal 加密步驟 |
| 本地快取層 | ✅ 已完成 | 沿用 LRU 策略 |
| Storage Backend 抽象 | ✅ 已完成 | 沿用 interface 設計 |

---

## API 設計

### 上傳（Gateway）

```bash
POST /api/upload
Header: X-API-Key: orca_xxx
Body: multipart/form-data
  - video: MP4 file
  - preview_seconds: 10      # 預覽長度
  - price: 100000000         # 解鎖價格（MIST）
  - creator_address: 0x...   # 收款地址

Response: {
  "video_id": "0x...",
  "status": "processing",
  "preview_manifest_url": "https://aggregator.../v1/blobs/...",
}
```

### 串流播放

```bash
# 預覽 manifest（任何人可存取）
GET /stream/{videoId}/preview.m3u8

# 完整 manifest（需要 AccessPass 才能解密加密段）
GET /stream/{videoId}/full.m3u8

# 明文 segment（直接回傳）
GET /stream/{videoId}/seg000.ts

# 加密 segment（回傳密文，client-side 解密）
GET /stream/{videoId}/seg002.ts.enc
```

### 鏈上查詢

```bash
# 查影片資訊
GET /api/video/{videoId}
Response: { "price": 100000000, "creator": "0x...", "preview_duration": 10, "purchases": 42 }

# 查用戶是否已購買
GET /api/access/{videoId}?wallet=0x...
Response: { "has_access": true, "access_pass_id": "0x..." }
```

---

## 參考專案

| 專案 | 角色 | 備註 |
|---|---|---|
| [Walrus](https://docs.wal.app) | 去中心化儲存 | 明文 + 加密 segments 存放 |
| [Seal](https://seal.mystenlabs.com) | 加密 + 存取控制 | Identity-based encryption + 鏈上 policy |
| [Seal Examples](https://github.com/MystenLabs/seal/tree/main/examples) | 參考實作 | Subscription pattern 可參考 |
| [hls.js](https://github.com/video-dev/hls.js) | 前端播放器 | Custom loader 實現 client-side 解密 |
| [@mysten/seal](https://www.npmjs.com/package/@mysten/seal) | Seal TS SDK | 前端加密/解密 |

---

## 開發指引

```bash
# 建立遷移分支
git checkout -b v2/paywall

# Phase 1：移除舊元件（詳見 MIGRATION.md）
rm internal/handler/walrus.go internal/handler/walrus_set.go internal/handler/stream.go
# ... 簡化 main.go、config、前端（見 MIGRATION.md 完整步驟）

# 確認編譯通過
go build ./cmd/orca/
go test ./...

# Phase 2：建立合約
cd contracts/orca && sui move build

# Phase 3+：逐步整合 Seal（見 MIGRATION.md）
```

> 完整遷移步驟：[MIGRATION.md](./MIGRATION.md)

---

## License

TBD

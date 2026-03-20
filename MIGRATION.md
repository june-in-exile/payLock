# 🔄 Orca v2 遷移計畫：從 Video Middleware → Video Paywall Infra

> 本文件為 Orca 項目方向轉換的具體執行步驟。
> 目標：移除 Orca Gateway 及 compare 功能，保留 Walrus 原生串流，逐步串接 Seal 付費解鎖。

---

## 現有檔案結構

```
orca/
├── .env.example
├── .gitignore
├── AGENTS.md
├── CLAUDE.md
├── GEMINI.md
├── Makefile
├── README.md
├── go.mod
├── go.sum
├── cmd/orca/
│   ├── main.go                    ← 入口，包含 Gateway 路由
│   └── web/
│       └── index.html             ← 前端（含 compare view）
├── internal/
│   ├── config/config.go           ← 設定（含 Orca Gateway 設定）
│   ├── handler/
│   │   ├── delete.go              ← 刪除影片
│   │   ├── status.go              ← 查詢處理狀態
│   │   ├── stream.go              ← HLS 串流（Orca Gateway 代理）
│   │   ├── upload.go              ← 上傳影片
│   │   ├── videos.go              ← 影片清單
│   │   ├── videos_test.go
│   │   ├── walrus.go              ← Walrus 代理（compare 用）
│   │   └── walrus_set.go          ← Walrus 設定（compare 用）
│   ├── middleware/
│   │   ├── apikey.go
│   │   ├── cors.go
│   │   └── middleware_test.go
│   ├── model/video.go
│   ├── model/video_test.go
│   ├── processor/
│   │   ├── ffmpeg.go              ← FFmpeg 切片
│   │   ├── validator.go           ← 影片格式驗證
│   │   └── validator_test.go
│   ├── storage/storage.go         ← 本地儲存
│   └── walrus/walrus.go           ← Walrus HTTP client
```

---

## Phase 0：建立遷移分支（5 分鐘）

```bash
cd orca
git checkout -b v2/paywall
git push -u origin v2/paywall
```

---

## Phase 1：移除不再需要的程式碼

### Step 1.1：移除 compare 相關 handler

這兩個檔案是專門為 Orca vs Walrus 對比 demo 寫的，新版不需要。

```bash
# 刪除 compare 相關的 handler
rm internal/handler/walrus.go
rm internal/handler/walrus_set.go
```

**驗證：** `grep -r "walrus_set\|WalrusSet\|CompareView\|walrusProxy" internal/` 應無結果。

### Step 1.2：移除 Orca Gateway 串流代理

`internal/handler/stream.go` 是 Orca Gateway 的核心——它代理 HLS 請求、做 segment prefetch 等。新版直接用 Walrus aggregator 的 URL，不需要中間代理。

```bash
rm internal/handler/stream.go
```

### Step 1.3：簡化 main.go 路由

打開 `cmd/orca/main.go`，移除以下路由（保留 upload 和 status）：

**移除：**

- `/stream/` 相關路由（Orca Gateway 代理串流）
- `/api/walrus/` 相關路由（compare 用的 Walrus 代理）
- `/api/walrus-settings` 路由
- compare view 相關的前端路由

**保留：**

- `POST /api/upload` — 上傳影片（後續會改為上傳到 Walrus + Seal 加密）
- `GET /api/status/{id}` — 查詢處理狀態
- `GET /api/videos` — 影片清單
- `DELETE /api/videos/{id}` — 刪除影片
- 靜態檔案服務（前端頁面）

### Step 1.4：簡化 config

打開 `internal/config/config.go`，移除 Orca Gateway 特有的設定：

**移除：**

- `StorageBackend` 切換邏輯（不再需要 local vs walrus 切換，統一用 Walrus）
- Orca Gateway 的 port、cache 設定（如果有的話）

**保留：**

- Walrus aggregator / publisher URL
- API key
- 基礎 server 設定

### Step 1.5：簡化前端

`cmd/orca/web/index.html` 目前包含 compare view（左右對比面板）。

**移除：**

- Compare view 的整個 UI（左右對比、延遲測量）
- Orca 面板（不再有獨立的 Orca 串流）

**保留：**

- 上傳表單
- 影片清單
- hls.js 播放器（改為指向 Walrus aggregator URL）

**改為：**

- 單一播放器，直接從 Walrus aggregator 播放
- 後續會在這個播放器上加 Seal 解密 loader

### Step 1.6：清理 storage 層

`internal/storage/storage.go` 包含本地磁碟儲存邏輯。新版統一用 Walrus。

**選擇（二擇一）：**

**方案 A：保留 local storage 作為開發模式**（建議）

- 開發時用 local storage，不需要連 Walrus
- 加環境變數 `ORCA_MODE=dev|prod` 切換

**方案 B：直接移除，全面 Walrus**

- 移除 `internal/storage/storage.go`
- 所有操作都透過 `internal/walrus/walrus.go`

建議選 **方案 A**，開發體驗比較好。

### Step 1.7：清理 handler/delete.go

確認刪除邏輯是否有 Orca Gateway 特有的邏輯（如刪除本地快取的 segments）。如果有，簡化為只刪除 Walrus blob 的映射記錄。

---

## Phase 1 完成後的檔案結構

```
orca/
├── .env.example                   ← 更新：移除 Orca Gateway 設定
├── .gitignore
├── AGENTS.md                      ← 更新：反映新方向
├── CLAUDE.md                      ← 更新：反映新方向
├── GEMINI.md                      ← 更新：反映新方向
├── Makefile                       ← 更新：移除 gateway 相關 target
├── README.md                      ← 全面更新
├── go.mod
├── go.sum
├── cmd/orca/
│   ├── main.go                    ← 簡化：只保留 upload/status/videos 路由
│   └── web/
│       └── index.html             ← 簡化：移除 compare，單一 Walrus 播放器
├── internal/
│   ├── config/config.go           ← 簡化
│   ├── handler/
│   │   ├── delete.go
│   │   ├── status.go
│   │   ├── upload.go              ← 後續改為 Walrus + Seal 上傳
│   │   ├── videos.go
│   │   └── videos_test.go
│   ├── middleware/
│   │   ├── apikey.go
│   │   ├── cors.go
│   │   └── middleware_test.go
│   ├── model/video.go             ← 後續加入 price、preview_duration 等欄位
│   ├── model/video_test.go
│   ├── processor/
│   │   ├── ffmpeg.go              ← 後續加入分段加密邏輯
│   │   ├── validator.go
│   │   └── validator_test.go
│   ├── storage/storage.go         ← 保留作為 dev mode（方案 A）
│   └── walrus/walrus.go           ← 保留：Walrus HTTP client
```

---

## Phase 1 驗證 Checklist

```bash
# 1. 編譯通過
go build ./cmd/orca/

# 2. 測試通過
go test ./...

# 3. 啟動正常
make run
# 確認 http://localhost:8080 可以：
#   - 上傳影片
#   - 看到影片清單
#   - 播放影片（直接從 Walrus 或 local storage）

# 4. 確認移除乾淨
grep -r "compare\|CompareView\|walrusProxy\|WalrusSet" . --include="*.go" --include="*.html"
# 應該沒有結果

# 5. Commit
git add -A
git commit -m "refactor: remove gateway and compare features, prepare for Seal paywall"
git push
```

---

## Phase 2：加入 Move 合約目錄結構

Phase 1 完成後，開始建立合約基礎。

```bash
# 建立 Move 合約目錄
mkdir -p contracts/orca/sources
```

建立 `contracts/orca/Move.toml`：

```toml
[package]
name = "orca"
edition = "2024.beta"

[dependencies]
Sui = { git = "https://github.com/MystenLabs/sui.git", subdir = "crates/sui-framework/packages/sui-framework", rev = "framework/testnet" }

[addresses]
orca = "0x0"
```

建立 `contracts/orca/sources/paywall.move`（初始版本）：

```move
module orca::paywall;

use std::string::String;
use sui::{coin::Coin, sui::SUI};

// === Errors ===
const EInsufficientPayment: u64 = 0;
const ENoAccess: u64 = 1;
const EInvalidCap: u64 = 2;

// === Objects ===

/// 影片資訊
public struct Video has key {
    id: UID,
    price: u64,
    creator: address,
    preview_blob_id: String,
    encrypted_blob_id: String,
    preview_duration: u64,
}

/// 購買憑證（永久有效）
public struct AccessPass has key {
    id: UID,
    video_id: ID,
}

/// Creator 管理權限
public struct CreatorCap has key {
    id: UID,
    video_id: ID,
}

// === Public Functions ===

/// 建立影片
public fun create_video(
    price: u64,
    preview_blob_id: String,
    encrypted_blob_id: String,
    preview_duration: u64,
    ctx: &mut TxContext,
): CreatorCap {
    let video = Video {
        id: object::new(ctx),
        price,
        creator: ctx.sender(),
        preview_blob_id,
        encrypted_blob_id,
        preview_duration,
    };
    let cap = CreatorCap {
        id: object::new(ctx),
        video_id: object::id(&video),
    };
    transfer::share_object(video);
    cap
}

entry fun create_video_entry(
    price: u64,
    preview_blob_id: String,
    encrypted_blob_id: String,
    preview_duration: u64,
    ctx: &mut TxContext,
) {
    transfer::transfer(create_video(price, preview_blob_id, encrypted_blob_id, preview_duration, ctx), ctx.sender());
}

/// 用戶付費購買
public fun purchase(
    video: &Video,
    payment: Coin<SUI>,
    ctx: &mut TxContext,
): AccessPass {
    assert!(payment.value() >= video.price, EInsufficientPayment);
    transfer::public_transfer(payment, video.creator);
    AccessPass {
        id: object::new(ctx),
        video_id: object::id(video),
    }
}

entry fun purchase_entry(
    video: &Video,
    payment: Coin<SUI>,
    ctx: &mut TxContext,
) {
    transfer::transfer(purchase(video, payment, ctx), ctx.sender());
}

/// 更新價格
public fun update_price(video: &mut Video, cap: &CreatorCap, new_price: u64) {
    assert!(cap.video_id == object::id(video), EInvalidCap);
    video.price = new_price;
}

// === Seal Access Control ===

use walrus::utils::is_prefix;

/// Seal key server 呼叫此函數驗證解密權限
entry fun seal_approve(id: vector<u8>, pass: &AccessPass, video: &Video) {
    assert!(pass.video_id == object::id(video), ENoAccess);
    assert!(is_prefix(video.id.to_bytes(), id), ENoAccess);
}
```

```bash
# 驗證合約可編譯（需要先安裝 sui CLI）
cd contracts/orca
sui move build

git add -A
git commit -m "feat: add Move paywall contract skeleton"
git push
```

---

## Phase 2 完成後的檔案結構

```
orca/
├── contracts/                     ← 新增
│   └── orca/
│       ├── Move.toml
│       └── sources/
│           └── paywall.move
├── cmd/orca/
│   ├── main.go
│   └── web/index.html
├── internal/
│   ├── config/config.go
│   ├── handler/
│   │   ├── delete.go
│   │   ├── status.go
│   │   ├── upload.go
│   │   ├── videos.go
│   │   └── videos_test.go
│   ├── middleware/...
│   ├── model/video.go
│   ├── processor/...
│   ├── storage/storage.go
│   └── walrus/walrus.go
├── README.md
└── ...
```

---

## Phase 3：更新 model 和 upload 流程

### Step 3.1：擴充 video model

在 `internal/model/video.go` 加入新欄位：

```go
type Video struct {
    ID                string    `json:"id"`
    Status            string    `json:"status"`
    Duration          float64   `json:"duration"`
    CreatedAt         time.Time `json:"created_at"`
    // --- 新增 ---
    Price             uint64    `json:"price"`               // 解鎖價格（MIST）
    PreviewDuration   int       `json:"preview_duration"`    // 預覽秒數
    PreviewBlobID     string    `json:"preview_blob_id"`     // 明文預覽 manifest
    EncryptedBlobID   string    `json:"encrypted_blob_id"`   // 加密完整 manifest
    CreatorAddress    string    `json:"creator_address"`     // Sui 錢包地址
    SuiVideoObjectID  string    `json:"sui_video_object_id"` // 鏈上 Video object ID
}
```

### Step 3.2：修改 upload handler

修改 `internal/handler/upload.go`，接受新參數：

```
POST /api/upload
Body:
  - video: MP4 file
  - preview_seconds: 10        (新增)
  - price: 100000000           (新增)
  - creator_address: 0x...     (新增)
```

### Step 3.3：修改 FFmpeg 處理流程

修改 `internal/processor/ffmpeg.go`，切片後根據 `preview_seconds` 將 segments 分為兩組：

```go
type ProcessResult struct {
    PreviewSegments  []string  // 前 N 秒的 segment 檔案路徑
    PaywallSegments  []string  // 需要加密的 segment 檔案路徑
    ManifestPath     string    // 完整 manifest 路徑
}
```

---

## Phase 4：Seal 加密整合

### Step 4.1：加入 TypeScript 前端專案

```bash
mkdir -p web
cd web
npm init -y
npm install @mysten/seal @mysten/sui hls.js
npm install -D typescript vite
```

### Step 4.2：實作 SealLoader

在 `web/src/seal-loader.ts` 實作 hls.js custom loader。

### Step 4.3：實作付費牆 UI

在 `web/src/paywall.ts` 實作付費提示 + 錢包連接 + 購買流程。

---

## 執行順序總結

| 順序 | Phase | 預計時間 | 內容 |
|------|-------|---------|------|
| 1 | Phase 0 | 5 min | 建立 `v2/paywall` 分支 |
| 2 | Phase 1 | 1-2 天 | 移除 gateway + compare，確保編譯和現有功能正常 |
| 3 | Phase 2 | 2-3 天 | 建立 Move 合約，在 testnet 部署驗證 |
| 4 | Phase 3 | 3-4 天 | 更新 model + upload 流程，加入部分加密 |
| 5 | Phase 4 | 1 週 | Seal 加密整合 + 前端 SealLoader + 付費牆 UI |

---

## 注意事項

1. **每個 Phase 完成後都要 commit + push**，保持可回溯
2. **Phase 1 完成後應該要能正常跑**（只是少了 compare 和 gateway 代理）
3. **合約的 `is_prefix` 依賴 Seal 官方的 `walrus::utils`**，部署時需要確認 dependency
4. **開發期間用 Sui Testnet**，不要碰 Mainnet
5. **舊的 README 保留在 git history 中**，不需要額外備份

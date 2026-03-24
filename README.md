# PayLock — Decentralized Video Paywall SDK for Sui

## 簡介

PayLock 是建立在 Walrus + Seal 之上的影片付費解鎖 SDK。影片上傳時自動拆為預覽片段與完整版，預覽公開播放，完整版透過 Seal 加密存儲於 Walrus。觀眾付費後取得鏈上存取憑證，解密後即可觀看完整內容。開發者只需幾行程式碼即可為任何 dApp 加入影片付費牆功能。

## 現有功能 (Phase 1 & 2)

- **自動縮圖與預覽**：上傳 MP4 後，後端自動擷取縮圖 (Thumbnail) 與前 N 秒預覽 (Preview)，並行上傳至 Walrus。
- **Faststart 優化**：免費影片自動進行 `faststart` 處理，優化 Walrus 上的串流播放速度。
- **Seal 加密 (Paid Videos)**：付費影片在瀏覽器端使用 Seal SDK 加密，確保只有持有 AccessPass 的用戶可解密播放。
- **單筆交易流程**：優化後的鏈上發布流程，僅需一筆交易即可建立 Video 物件並設定加密命名空間 (Seal Namespace)。
- **即時狀態追蹤 (SSE)**：透過 Server-Sent Events 即時回傳處理進度，從上傳、擷取到 Walrus 儲存狀態一目了然。
- **前端 SPA**：整合 Slush Wallet，支援價格設定、影片列表、播放器（預覽 → 付費牆 → 解密播放）。

---

## 核心架構

### 影片發布流程（付費影片）

為了兼顧安全性與效能，PayLock 採用「後端預處理 + 前端加密」的混合流程：

1. **後端預處理**：
   - 用戶上傳影片至 `POST /api/upload`。
   - 後端產生縮圖與預覽版 MP4，並上傳至 Walrus。
   - 透過 `GET /api/status/{id}/events` (SSE) 通知前端預覽版已準備就緒。

2. **前端加密與上傳**：
   - 前端隨機產生 `seal_namespace`。
   - 使用 Seal SDK 加密原始影片，並上傳加密後的 Blob 至 Walrus。

3. **鏈上發布 (TX)**：
   - 前端發起一筆交易呼叫 `create_video`。
   - 傳入 `price`、`preview_blob_id`、`full_blob_id` (加密後) 與 `seal_namespace`。
   - 交易成功後，呼叫 `PUT /api/videos/{id}/sui-object` 將鏈上 ID 與後端記錄關聯。

### 系統組件

```
[ 用戶端 / 前端 SPA ]
    │
    ▼
[ PayLock Backend (Go) ]
    ├── POST /api/upload           驗證 MP4 → 擷取縮圖/預覽 → 並行上傳 Walrus
    ├── GET /api/status/{id}/events SSE 即時追蹤處理進度
    ├── GET /api/videos            列出影片（含縮圖與價格資訊）
    └── PUT /api/videos/.../sui-object 關聯鏈上物件 ID
    │
    ├──── 寫入 ────▶ [ Walrus Publisher ] → [ Walrus Storage ]
    └──── 讀取 ────▶ [ Walrus Aggregator ] ← (串流播放)

[ Sui 區塊鏈 ]  ← contracts/sources/gating.move
    Video (Shared Object) / AccessPass (Owned Object) / Seal Policy
```

---

## 鏈上合約設計

合約位於 `contracts/sources/gating.move`：

```move
module paylock::gating {
    /// 影片資訊，由創作者建立（shared object）
    public struct Video has key {
        id: UID,
        price: u64,                // 解鎖價格（MIST）
        creator: address,          // 收款地址
        preview_blob_id: String,   // 預覽版 Walrus Blob ID
        full_blob_id: String,      // 完整版 Walrus Blob ID (付費影片為加密後)
        seal_namespace: vector<u8>,// Seal 加密命名空間
    }

    /// 購買憑證，付費後 mint，永久有效（owned by buyer）
    public struct AccessPass has key, store {
        id: UID,
        video_id: ID,
    }

    /// 創作者發布影片（僅需一筆交易）
    public fun create_video(
        price: u64,
        preview_blob_id: String,
        full_blob_id: String,
        seal_namespace: vector<u8>,
        ctx: &mut TxContext
    );

    /// 用戶付費 → mint AccessPass 並轉帳給創作者
    entry fun purchase_and_transfer(video: &Video, payment: Coin<SUI>, ctx: &mut TxContext);

    /// Seal key server 驗證解密權限
    entry fun seal_approve(id: vector<u8>, pass: &AccessPass, video: &Video);
}
```

---

## 快速開始

### 前置需求

- Go 1.25+
- **FFmpeg**（必要，啟動時檢查）

### 環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `PAYLOCK_PORT` | `8080` | HTTP 監聽埠 |
| `PAYLOCK_DATA_DIR` | `data` | Metadata 與本機快取儲存路徑 |
| `PAYLOCK_WALRUS_PUBLISHER_URL` | `...` | Walrus Publisher |
| `PAYLOCK_WALRUS_AGGREGATOR_URL` | `...` | Walrus Aggregator |
| `PAYLOCK_WALRUS_EPOCHS` | `5` | Walrus 儲存期數 |
| `PAYLOCK_PREVIEW_DURATION` | `10` | 預覽片段秒數 |
| `PAYLOCK_SUI_RPC_URL` | `...` | Sui RPC (Testnet) |
| `PAYLOCK_GATING_PACKAGE_ID` | _(必填)_ | 部署後的合約 Package ID |

### 啟動服務

```bash
# 1. 複製環境變數範例並修改
cp .env.example .env

# 2. 啟動服務
make run
```

---

## API 參考

PayLock 提供完整的 RESTful API 與 SSE 事件流，方便開發者將付費解鎖功能整合至任何影片應用中。

詳細 API 規格與整合建議請參閱：[**API.md — PayLock 基礎設施規格書**](./API.md)

### 核心接口摘要

1. **上傳影片**：`POST /api/upload` — 發起非同步處理任務。
2. **即時追蹤**：`GET /api/status/{id}/events` — 透過 SSE 獲取處理進度。
3. **建立關聯**：`PUT /api/videos/{id}/sui-object` — 同步鏈上物件 ID 至後端。
4. **預覽播放**：`GET /stream/{id}` — 307 重導向至預覽版。
5. **系統配置**：`GET /api/config` — 獲取合約與 Walrus 端點資訊。

---

## License

MIT

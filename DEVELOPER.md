# 🐋 Orca 開發者整合指南 (Developer Guide)

Orca 是一個為 Sui 生態系設計的 **影片原生去中心化儲存基礎設施**。它封裝了 Walrus 的複雜操作，並整合了 **Seal (加密控制)** 與 **Sui Move (付費牆)**，為開發者提供簡單的影片分發方案。

## 🚀 核心概念：雙重串流策略 (Dual-Stream Strategy)

Orca 為每個影片提供兩種存取路徑，以平衡「傳播性」與「商業價值」：

| 資源類型 | API 端點 | 說明 | 權限控制 |
| :--- | :--- | :--- | :--- |
| **預覽版 (Preview)** | `GET /api/stream/{id}` | 低畫質、片段或公開版本。 | **公開存取** (無須付費) |
| **完整版 (Full)** | `GET /api/stream-full/{id}` | 加密的高畫質完整版本。 | **受 Seal 保護** (須付費/授權) |

---

## 🛠 整合流程

### 1. 上傳與設定售價

透過 `POST /api/upload` 上傳影片。若 `price > 0`，系統會自動啟動 **Seal 加密流程**。

* **Endpoint**: `POST /api/upload`
* **參數**:
  * `file`: 影片檔案 (MP4)。
  * `price`: 觀看完整版所需的 SUI 金額 (以 MIST 為單位，1 SUI = 10^9 MIST)。
  * `title`: 影片標題。

```bash
# 上傳一個售價 2 SUI 的加密影片
curl -X POST http://localhost:8080/api/upload \
  -F "file=@movie.mp4" \
  -F "price=2000000000" \
  -F "title=My Premium Content"
```

### 2. 連結鏈上 Paywall 物件

上傳後，你需要將 Orca 的影片 ID 與 Sui 鏈上的 Paywall 物件進行綁定，以便處理支付邏輯。

* **Endpoint**: `POST /api/set-sui-object`

```bash
curl -X POST http://localhost:8080/api/set-sui-object \
  -H "Content-Type: application/json" \
  -d '{
    "id": "orca-video-uuid", 
    "suiObjectId": "0xYourPaywallObjectId"
  }'
```

### 3. 處理支付與播放

#### 前端展示

對於付費影片，前端應同時引導用戶觀看預覽版並進行購買：

```html
<!-- 1. 展示免費預覽 -->
<video controls src="https://your-orca.com/api/stream/{id}"></video>

<!-- 2. 購買按鈕 (呼叫 Sui Move 合約) -->
<button onclick="purchaseVideo('0xYourPaywallObjectId')">
  付費解鎖完整版
</button>
```

#### 解鎖完整版 (Seal 解密)

當用戶在鏈上完成 `purchase_video` 交易後：

1. **獲取 Full Blob**: 透過 `GET /api/status/{id}` 取得 `fullBlobUrl`。
2. **密鑰獲取**: 前端透過 Seal SDK 提交支付證明（Transaction Digest），向 Seal 節點請求解密密鑰。
3. **解密播放**: 使用密鑰對從 Walrus Aggregator 抓取的加密流進行即時解密。

---

## 📊 狀態查詢 API

開發者應輪詢此介面以確認 Walrus 上傳進度及獲取 Blob IDs。

* **Endpoint**: `GET /api/status/{id}`

**回應範例**:

```json
{
  "id": "uuid",
  "status": "ready",
  "price": 2000000000,
  "encrypted": true,
  "preview_blob_id": "V-j6...",
  "full_blob_id": "F-k9...",
  "sui_object_id": "0x123..."
}
```

---

## 📦 Go 開發者專用 (SDK)

如果你在 Go 後端中直接整合，可以引入以下套件：

* `internal/walrus`: 直接操作 Walrus Publisher/Aggregator。
* `internal/model`: 定義影片狀態與結構。

```go
// 範例：手動更新影片的 Full Blob 資訊
videoStore.SetFullBlob(videoID, "new-blob-id", "https://aggregator...")
```

---

## 💡 開發建議

* **測試網**: 目前所有功能預設運行於 Walrus Testnet。

* **異步處理**: 影片上傳至 Walrus 是非同步的，請確保 UI 有處理 `status: "processing"` 的邏輯。
* **安全**: `full_blob_id` 指向的是加密後的數據，即使 URL 洩漏，沒有 Seal 授權也無法播放。

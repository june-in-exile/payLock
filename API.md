# PayLock API Reference (Infra Edition)

本文檔提供 PayLock 後端服務的完整 API 規格，專為希望將「影片付費解鎖」功能整合進其 dApp 的開發者設計。

---

## 核心流程總覽 (Infrastructure Flow)

1. **上傳與處理**：`POST /api/upload`。伺服器異步執行 FFmpeg 擷取預覽、縮圖並上傳至 Walrus。
2. **狀態監控**：`GET /api/status/{id}/events` (SSE)。追蹤處理進度直到預覽版就緒。
3. **加密上傳 (僅限付費影片)**：前端使用 Seal SDK 加密原始影片，上傳至 Walrus 並取得 `full_blob_id`。
4. **鏈上共識**：前端發送 Sui 交易建立 `Video` 物件，包含價格與所有 Blob ID。
5. **同步記錄**：`PUT /api/videos/{id}/sui-object`。將鏈上 `Video` 物件 ID 寫回後端 Metadata。
6. **串流存取**：`GET /stream/{id}` (預覽) 或 `GET /stream/{id}/full` (完整/加密版)。

---

## API 清單

### 1. 上傳影片 (Async Upload)

**`POST /api/upload`**

發起一個非同步上傳任務。後端會驗證檔案並開始背景處理（擷取預覽版與縮圖並上傳至 Walrus）。

- **Content-Type**: `multipart/form-data`
- **Body 參數**:
  - `video` (必填): MP4, MOV, WebM 等影片檔案。
  - `title` (選填): 影片標題。
  - `price` (選填): 預期價格 (MIST)。若設為 `0` 則視為免費影片（後端會自動上傳完整版）。
  - `creator` (選填): 創作者的 Sui 地址。
- **Response** (`202 Accepted`):

    ```json
    {
      "id": "a1b2c3d4e5f6g7h8",
      "status": "processing"
    }
    ```

### 2. 即時狀態追蹤 (Status Events)

**`GET /api/status/{id}/events`**

使用 Server-Sent Events (SSE) 追蹤處理進度。這對於需要即時更新 UI 的整合者非常有用。

- **Event Data Structure**: 返回與 `GET /api/status/{id}` 相同的影片物件。
- **範例**:

    ```text
    data: {"id":"...","status":"processing","title":"My Movie"}
    data: {"id":"...","status":"ready","preview_blob_id":"...","thumbnail_blob_url":"..."}
    ```

### 3. 查詢影片狀態 (Status Query)

**`GET /api/status/{id}`**

獲取特定影片的 Metadata 與 Walrus Blob ID。

- **Response** (`200 OK`):

    ```json
    {
      "id": "a1b2c3d4",
      "title": "Video Title",
      "status": "ready",
      "price": 1000000000,
      "creator": "0x...",
      "thumbnail_blob_id": "...",
      "thumbnail_blob_url": "https://aggregator.../v1/blobs/...",
      "preview_blob_id": "...",
      "preview_blob_url": "https://aggregator.../v1/blobs/...",
      "full_blob_id": "...",
      "full_blob_url": "https://aggregator.../v1/blobs/...",
      "created_at": "2024-03-24T12:00:00Z"
    }
    ```

### 4. 關聯鏈上物件 (Set Sui Object)

**`PUT /api/videos/{id}/sui-object`**

當前端完成鏈上 `create_video` 交易後，必須呼叫此 API 將 Sui 物件 ID 告訴後端。

- **Request Body**:

    ```json
    { "sui_object_id": "0x789...abc" }
    ```

- **Response** (`200 OK`):

    ```json
    { "status": "updated" }
    ```

### 5. 列出所有影片 (List Videos)

**`GET /api/videos`**

取得系統中所有已處理的影片列表。

- **Response** (`200 OK`):

    ```json
    {
      "videos": [
        { "id": "...", "title": "...", "price": 0, "status": "ready", "thumbnail_blob_url": "..." },
        ...
      ]
    }
    ```

### 6. 刪除影片 (Delete Video)

**`DELETE /api/videos/{id}`**

從後端 Metadata Store 中刪除該影片記錄（注意：這不會影響 Walrus 上的 Blob 或鏈上的物件）。

- **Response** (`204 No Content`)

### 7. 串流重導向 (Preview Stream)

**`GET /stream/{id}`**

直接 307 Redirect 至預覽版在 Walrus 上的公開 URL。

- **Usage**: `<video src="https://api.paylock.com/stream/{id}" />`

### 8. 完整版串流重導向 (Full Stream)

**`GET /stream/{id}/full`**

直接 307 Redirect 至完整版（付費影片則為加密版）在 Walrus 上的 URL。

- **Usage**: 用於下載加密 Blob 或播放免費影片。

### 9. 獲取系統配置 (App Config)

**`GET /api/config`**

獲取後端的環境配置，包含 Walrus 端點與合約 Package ID。整合者可透過此 API 自動適應不同環境。

- **Response** (`200 OK`):

    ```json
    {
      "gating_package_id": "0x...",
      "walrus_publisher_url": "https://...",
      "walrus_aggregator_url": "https://..."
    }
    ```

---

## 付費解鎖邏輯建議 (For Developers)

作為 Infra 整合者，你應該遵循以下模式：

1. **偵測權限**：在錢包連線後，透過 Sui RPC 檢查用戶是否持有該 `Video` 物件對應的 `AccessPass`。
2. **播放決策**：
    - 若無權限：請求 `/stream/{id}` 播放公開預覽。
    - 若有權限：
        1. 請求 `/stream/{id}/full` 取得加密 Blob 資料。
        2. 使用前端 Seal SDK 解密。
        3. 將解密後的 Blob 轉換為 `ObjectURL` 餵給播放器。
3. **購買流程**：引導用戶簽署 `purchase_and_transfer` 交易。成功後重新載入播放器。

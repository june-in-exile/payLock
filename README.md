# 🐋 Orca — Video-Native Decentralized Storage Protocol

> 讓去中心化影片串流像傳統 CDN 一樣順暢。

---

## 問題

目前所有去中心化儲存（Walrus、Filecoin、Arweave）都把影片當成**不透明的 blob**。

這代表每個想做影片串流的 dApp 都要自己處理：

- 影片切片（Segmentation）
- Manifest 管理
- Segment 關聯
- 串流讀取
- 多解析度管理

**每個 dApp 都在重複造輪子。**

| 層級 | 功能 | 現有方案 | 狀態 |
|---|---|---|---|
| 分發（CDN） | 影片送到觀眾 | Theta Network | ✅ 已有 |
| 轉碼 | 多解析度轉換 | Livepeer | ✅ 已有 |
| **儲存** | **影片存放與串流** | **Walrus / Filecoin** | **⚠️ 通用型，不懂影片** |
| 應用層 | 使用者體驗 | DTube、Odysee 等 | 各自拼湊 |

---

## 解決方案

Orca 是一個**影片感知的中間層（Middleware）**，建立在通用儲存（如 Walrus）之上，把影片特化的邏輯下沉到協議層。

開發者不需要理解 HLS 切片、Manifest 管理、Segment 索引——上傳影片，拿到串流 URL，就能播。

### 與通用儲存的差異

| | 通用儲存（Walrus 等） | Orca |
|---|---|---|
| 資料模型 | 不透明 blob | 理解 manifest + segments 關係 |
| 串流支援 | 無（整個 blob 存/取） | 原生 HLS/DASH 支援 |
| Erasure coding | 所有資料相同策略 | 影片感知策略（I-frame 優先、熱門高冗餘） |
| 讀取路徑 | Fetch whole blob | Byte-range request + sequential prefetch |
| Metadata | 只有基本資訊 | 影片時長、解析度、codec、章節標記 |
| 多解析度 | 上層自行管理 | 協議層原生關聯 |

### 生態定位

```
Livepeer（轉碼）──→ Orca（影片儲存）──→ Theta（分發）──→ 播放器
```

Orca **不是要取代 Walrus**，而是讓 Walrus 上的影片體驗從不可用變成好用。我們是 Sui 生態的影片層。

---

## 核心概念

### 影片串流基礎

影片壓縮的核心是**不重複存一樣的東西**。一個影片由三種 frame 組成：

- **I-frame（Intra-coded）**：完整畫面，可獨立解碼，是播放的「進入點」
- **P-frame（Predicted）**：只存與前一幀的差異
- **B-frame（Bi-directional）**：參考前後幀計算差異，壓縮率最高

```
一個 GOP（Group of Pictures）：
I  B  B  P  B  B  P  B  B  P  B  B  I  ...
│                                       │
└───────────────── 一個 GOP ────────────┘
```

**重要性排序：I-frame > P-frame > B-frame**

- 丟了 I-frame → 整個 GOP 報廢
- 丟了 P-frame → 後續依賴它的 frame 受影響
- 丟了 B-frame → 只有該幀不見

### HLS 串流

HLS（HTTP Live Streaming）把影片切成多個 **segment**（通常 2-6 秒），加上一個 **manifest**（`.m3u8`）作為目錄：

```
/storage/abc123/
  ├── index.m3u8      ← Manifest（幾 KB，影片的目錄）
  ├── seg000.ts       ← Segment 0（幾 MB，前 6 秒）
  ├── seg001.ts       ← Segment 1（6-12 秒）
  ├── seg002.ts       ← Segment 2（12-18 秒）
  └── ...
```

**Manifest 範例：**

```
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6

#EXTINF:6.0,
http://orca-node.com/stream/abc123/seg000.ts
#EXTINF:6.0,
http://orca-node.com/stream/abc123/seg001.ts
#EXTINF:6.0,
http://orca-node.com/stream/abc123/seg002.ts
#EXT-X-ENDLIST
```

### Segment 切片規則

- 每個 segment 的開頭必須是 I-frame（不然播放器無法開始解碼）
- 所有解析度的 segment 切點必須對齊（ABR 切換時才能無縫銜接）
- Frame 數 = segment 時長 × FPS（例如 6 秒 × 30fps = 180 frames）

### Seek（進度條跳轉）

播放器 seek 到 1:30:00 時：

1. 查 manifest → 用二分搜找到 1:30:00 對應的 segment
2. 找到最近的 I-frame（例如 1:29:58）
3. 從 I-frame 開始解碼，快進到目標時間

### Adaptive Bitrate（ABR）

同一部影片轉成多個解析度，播放器根據網速動態切換：

```
Master Manifest
  ├── 1080p (5Mbps) → seg0, seg1, seg2...
  ├── 720p  (3Mbps) → seg0, seg1, seg2...
  └── 480p  (1Mbps) → seg0, seg1, seg2...
```

### Manifest 保護策略

```
Segment（大，幾 MB）→ Erasure coding 切割分散（省空間）
Manifest（小，幾 KB）→ 完整複製多份（不能切割，目錄要完整才有用）
Hash 存鏈上 → 驗證 manifest 是否被竄改
```

---

## 系統架構

### 上傳流程

```
原始影片 → [轉碼（FFmpeg/Livepeer）] → HLS 切片 → 存入 Storage Nodes
                                                        ↓
                                                Blockchain (Sui)
                                                ├── 影片 metadata
                                                ├── Blob/Segment IDs
                                                └── 權限控制
```

### 播放流程

```
播放器 → Orca Gateway → [CDN/Edge Cache] → Storage Nodes
  1. 拿 manifest（快取在記憶體，超快）
  2. 依序拉 segments
  3. 預取下幾個 segments（sequential prefetch）
```

### Storage Backend 抽象

```go
type StorageBackend interface {
    Store(segmentID string, data []byte) error
    Fetch(segmentID string) ([]byte, error)
}

// MVP
type LocalStorage struct { ... }

// Production
type WalrusStorage struct { ... }
```

---

## 技術選型

| 項目 | 選擇 | 原因 |
|---|---|---|
| 語言 | **Go** | Goroutine 適合 I/O 密集的串流場景；infra 生態強（Docker、K8s、IPFS 皆 Go）；開發速度快 |
| 串流格式 | **HLS** | 最廣泛支援，所有瀏覽器 + 播放器都能播 |
| 轉碼（MVP） | **FFmpeg** | 成熟穩定，一行指令搞定切片 |
| 轉碼（Prod） | **Livepeer** | 去中心化轉碼網路，不用自己養機器 |
| 底層儲存 | **Walrus** | Sui 生態原生，現成的節點網路和 erasure coding |
| Control Plane | **Sui 區塊鏈** | 物件模型管理影片 metadata，錢包簽名驗證身份 |
| 分發（Prod） | **Theta / Cloudflare** | CDN 不是 Orca 的範圍，交給專業的 |
| Codec | **Codec-agnostic** | 支援 H.264、H.265、AV1，metadata 記錄 codec 資訊 |

### 為什麼選 Go？

Goroutine 是 Go 的輕量級並發機制：

- 每個 goroutine 只佔 2-8 KB（OS thread 佔 1-8 MB）
- 切換成本極低（不需要進 kernel）
- 1000 人同時串流 = 1000 個 goroutine = 只用幾 MB 記憶體

影片串流是 I/O 密集場景（大部分時間在等網路/磁碟），goroutine 的 M:N 排程模型完美適合。

---

## API 設計

### 上傳

```bash
POST /api/upload
Header: X-API-Key: orca_xxx
Body: multipart/form-data (video file)

Response: { "id": "abc123", "status": "processing" }
```

### 查詢狀態

```bash
GET /api/status/{id}

Response: { "id": "abc123", "status": "ready", "duration": 120 }
```

### 串流播放

```bash
GET /stream/{id}/index.m3u8    → HLS manifest
GET /stream/{id}/seg000.ts     → Video segment
```

### CORS 策略

```
/stream/*  → Access-Control-Allow-Origin: *  （公開串流）
/api/*     → 不開 CORS + API Key 驗證       （管理操作）
```

---

## 權限控制（Roadmap）

### Web3 原生身份驗證

```
上傳：錢包簽名驗證 → 影片綁定到錢包地址
刪除：簽名驗證 + owner 檢查
觀看（公開）：不限制
觀看（私人）：鏈上白名單 / NFT Token Gate
```

### Token Gate 流程

```
觀眾用錢包簽名 → Server 驗簽 → 查鏈上權限
→ 有權限 → 發短期 token（1 小時）→ 用 token 拉 segments
→ 無權限 → 403
```

---

## MVP 規劃

### Milestone 1（2-3 週）：核心串流

```
Go service
  ├── POST /upload      → 接收 MP4
  ├── FFmpeg            → 切成 HLS segments
  ├── 本地磁碟儲存       → 先不搞分散式
  └── GET /stream/{id}  → 回傳 .m3u8 + .ts
```

**驗證方式：**

```bash
curl -X POST http://localhost:8080/upload -F "video=@test.mp4"
vlc http://localhost:8080/stream/abc123/index.m3u8
```

**不做：** 前端 UI、分散式、鏈上整合、erasure coding、多解析度、權限控制

### Milestone 2（+1.5 週）：前端 Demo

- 上傳頁面（簡單 form）
- 播放器（hls.js，不用自己寫）
- 影片清單 + 播放頁面

### Milestone 3（已完成）：Walrus 對比 Demo

- 同一部影片分別用 Orca 和 Walrus 播放
- 左右對比：首播延遲、seek 延遲、開發者體驗
- ⚠️ Walrus 端目前為 mock，尚未串接真實 Aggregator

### Milestone 4（已完成）：真實 Walrus 串接

將 compare view 的 Walrus 面板改為實際串接 Walrus Aggregator，達成真正的效能對比實驗。

**具體工作：**

1. **影片雙軌上傳**
   - 上傳時同時發給 Orca 和 Walrus Publisher API
   - 紀錄 Walrus blob ID
2. **Compare View 實驗模式**
   - Walrus 面板改用 `/api/walrus/{blobId}` 直接串流原始 MP4
   - 測量下載延遲、首播延遲、seek 延遲等指標

**驗證方式：**

```bash
# 啟動後自動使用 Walrus 測試網上傳
make run

# 上傳影片，取得 Walrus blob ID
curl -X POST http://localhost:8080/api/upload -F "video=@test.mp4" -H "X-API-Key: xxx"
# 打開 compare view，測量 Orca vs Walrus 的延遲差異
```

### Milestone 5（+2 週）：Walrus Storage Backend

實作 `WalrusStorage`，讓 segments 和 manifests 實際存到 Walrus 去中心化儲存。

```
StorageBackend interface
  ├── LocalStorage   ← Milestone 1（已完成）
  └── WalrusStorage  ← 本階段
```

**具體工作：**

1. **Walrus HTTP API 整合**
   - 實作 `WalrusStorage` struct，透過 Walrus HTTP Publisher API 上傳/讀取 blob
   - Store：將 segment/manifest 上傳到 Walrus，取得 blob ID
   - Fetch：透過 blob ID 從 Walrus Aggregator 讀取資料

2. **Blob ID 映射管理（安全性與效能強化）**
   - **隱患處理**：放棄原本規劃的 JSON 檔，改用 **SQLite** 或 **BoltDB** 儲存 `video ID → { manifest blob ID, segment blob IDs[] }`
   - 確保在高併發寫入時資料不會毀損，並提供高速的索引查詢效能

3. **Manifest 特殊處理**
   - Manifest 內的 segment URL 需改為 Orca Gateway 的 URL（不能直指 Walrus blob）
   - Gateway 收到 segment 請求時，查映射表 → 從 Walrus 拉取 → 回傳給播放器

4. **本地快取層**
   - 熱門 segments 快取在本地磁碟，避免每次都從 Walrus 拉取
   - LRU 策略，可設定快取大小上限

5. **Storage Backend 切換**
   - 環境變數 `ORCA_STORAGE_BACKEND=walrus` 切換

### Milestone 6（+2 週）：多解析度（ABR）

**具體工作：**

1. **多品質轉檔**
   - FFmpeg 轉出多個解析度（1080p / 720p / 480p）
   - Master manifest 關聯所有解析度
2. **資源調度（工程隱患處理）**
   - 實作簡單的 **Task Queue (工作佇列)**，限制併發轉檔任務數量，防止 CPU/Memory 耗盡導致系統崩潰
3. **播放器整合**
   - 播放器自動根據網速切換解析度

### 之後逐步加入

1. Video-aware erasure coding（I-frame 優先、熱門高冗餘）
2. 鏈上 metadata + 權限控制（Sui Move contract）
3. Edge cache / P2P delivery
4. Token-gating / DRM

---

## 經濟模型（長期願景）

```
Phase 1：自己跑節點，免費使用，累積用戶
Phase 2：付費 API（按儲存量 + 串流頻寬收費）
Phase 3：Token 激勵的去中心化節點網路
         使用者付 token → 節點營運者賺 token → Stake + Slash 保證品質
```

---

## 開發注意事項

### 上傳

- ✅ Streaming 寫入（`io.Copy`，不把整個影片讀進記憶體）
- ✅ 格式驗證（magic bytes + 檔案大小上限 + ffprobe 檢查）
- ✅ 非同步處理（上傳後回傳 processing 狀態，背景切片）
- 🔜 斷點續傳（Resumable Upload）

### 串流

- ✅ HTTP Range Request（206 Partial Content）
- ✅ 正確的 Content-Type（`.m3u8` → `application/vnd.apple.mpegurl`、`.ts` → `video/mp2t`）
- ✅ CORS header
- 🔜 熱門 segment 記憶體快取
- 🔜 Sequential prefetch

---

## 參考專案

| 專案 | 相關性 | 備註 |
|---|---|---|
| [Walrus](https://walrus.xyz) | 底層儲存 | Sui 生態通用儲存，Mysten Labs 開發 |
| [Livepeer](https://livepeer.org) | 轉碼層 | 去中心化轉碼網路 |
| [Theta Network](https://thetatoken.org) | CDN 層 | 去中心化影片分發 |
| [hls.js](https://github.com/video-dev/hls.js) | 前端播放器 | 瀏覽器 HLS 播放 library |

---

## License

TBD

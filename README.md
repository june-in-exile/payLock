# Orca: Video-Native Decentralized Storage Protocol

**建立一個理解影片格式的去中心化儲存協議** — 讓去中心化影片串流像傳統 CDN 一樣順暢。

---

## 問題：現有方案的缺口

目前去中心化影片串流的技術堆疊：

| 層級 | 功能 | 現有方案 | 狀態 |
|---|---|---|---|
| 分發 (CDN) | 影片送到觀眾 | Theta Network | ✅ 已有 |
| 轉碼 | 多解析度轉換 | Livepeer / Theta Edge Node | ✅ 已有 |
| **儲存** | **影片存放** | **Walrus / Filecoin / Arweave** | **⚠️ 通用型，不懂影片** |
| 應用層 | 使用者體驗 | DTube、Odysee 等 | 各自拼湊 |

**核心問題：** 所有去中心化儲存都把影片當成不透明的 blob。上層應用要自己處理切片、manifest 管理、segment 關聯、串流讀取，每個 dApp 都在重複造輪子。

---

## 解決方案：Video-Aware Decentralized Storage

把影片特化的邏輯 **下沉到 storage protocol 層**，讓儲存本身就理解影片結構。

### 與通用儲存的差異

| | 通用儲存 (Walrus 等) | Orca |
|---|---|---|
| 資料模型 | 不透明 blob | 理解 manifest + segments 關係 |
| 串流支援 | 無（整個 blob 存/取） | 原生 HLS/DASH 支援 |
| Erasure coding | 所有資料相同策略 | 影片感知策略（熱門高冗餘、冷門低冗餘、關鍵幀優先恢復） |
| 讀取路徑 | Fetch whole blob | Byte-range request + sequential prefetch |
| Metadata | 只有基本資訊 | 影片時長、解析度、codec、章節標記 |
| 多解析度 | 上層自行管理 | 協議層原生關聯 |

### 系統架構（概念）

```
上傳流程：
  原始影片 → [轉碼層(可選)] → 切片(HLS/DASH) → 存入 Storage Nodes
                                                      ↓
                                              Blockchain (Control Plane)
                                              ├── 影片 metadata
                                              ├── Blob/Segment IDs
                                              └── 權限控制 / DRM

播放流程：
  播放器 → Gateway/Edge Cache → Storage Nodes → 按需拉取 segments
```

---

## 定位與生態

### 不是競爭，是互補

- **vs Walrus/Filecoin**：不做通用儲存，只專注影片場景
- **vs Livepeer**：不做轉碼，可以整合 Livepeer 作為轉碼層
- **vs Theta**：不做 CDN 分發，可以疊在 Theta 上面做 delivery

### 生態定位

```
Livepeer (轉碼) ──→ Orca (影片儲存) ──→ Theta (分發) ──→ 播放器
```

---

## 技術選型

| 項目 | 選擇 | 原因 |
|---|---|---|
| 開發語言 | **Go** | Goroutine 適合網路密集/串流並發；infra 生態強（Docker、K8s、IPFS 皆 Go）；開發速度快，適合構想階段快速驗證 |
| Control Plane | **Sui 區塊鏈** | 與 Walrus 同生態，可借力 Sui 的物件模型管理影片 metadata |
| 參考設計 | **Walrus 論文** | 借鏡 RedStuff erasure coding 演算法，但為影片場景重新設計 |

---

## MVP 規劃

```
[Go service]
  ├── 接收影片上傳
  ├── 切成 HLS segments
  ├── 每個 segment 存到 Storage Nodes
  ├── 生成 manifest（記錄 segment/blob IDs）
  └── 提供串流 API 給播放器
```

之後逐步加入：

1. Video-aware erasure coding
2. 多解析度關聯管理
3. Edge cache / P2P delivery
4. 鏈上 metadata 與權限控制
5. Token-gating / DRM

---

## 參考專案

| 專案 | 相關性 | 備註 |
|---|---|---|
| [Walrus](https://walrus.xyz) | 架構參考 | Sui 生態通用儲存，Mysten Labs 開發 |
| [Livepeer](https://livepeer.org) | 轉碼層夥伴 | 去中心化轉碼網路 |
| [Theta Network](https://thetatoken.org) | CDN 層夥伴 | 去中心化影片分發 |
| [EthStorage](https://ethstorage.io) | ETH 生態對標 | Walrus 在 ETH 側的類似定位 |

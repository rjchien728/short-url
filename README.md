# Short URL Service

一個具備高可擴展性、高效能與精確歸因分析的社群短網址服務。系統採用 Go 實作，並透過 Redis Streams 實現非同步任務處理。

## 快速開始

### 本機開發

需求：Go 1.24+、Docker

```bash
make init         # 複製 .env.example → .env，啟動 pg + redis
make migrate-up   # 執行 DB migration
make dev          # 同時啟動 api + worker（Ctrl+C 停止）
```

其他常用指令：

```bash
make docker-up    # 啟動 infra（db + redis）
make docker-down  # 停止 infra
make docker-logs  # 查看 infra logs
make test         # 跑 unit tests
```

### VM 部署（AMD64，Docker Compose）

需求：Docker、Git

```bash
git clone <repo>
cp .env.dev .env              # 以 VM 範本建立 .env
# 編輯 .env，填入 DB_PASSWORD、APP_ID_OBFUSCATION_SALT、SERVER_BASE_URL 等必填項目
make deploy                   # build image + 啟動全部服務（infra → migrate → api + worker）
```

更新版本：

```bash
git pull
make deploy                   # 重新 build 並重啟有變動的服務
```

其他部署指令：

```bash
make deploy-down  # 停止 app 服務（api、worker），保留 infra（db、redis）
make deploy-logs  # 查看所有服務 logs
```

> **注意**：`.env` 中 DB 與 Redis 的 host 在 VM 上必須使用 Docker Compose service name（`db`、`redis`），而非 `localhost`。詳見 `.env.dev` 內的說明。

---

## 系統架構

系統分為兩個主要執行單元：
- **API Server (`cmd/api`)**: 負責處理短網址的建立與重新導向（Redirect）。
- **Background Worker (`cmd/worker`)**: 負責處理耗時的非同步任務，包括連結預覽抓取與點擊數據分析。

---

## 背景服務 (Background Workers)

背景服務基於 **Redis Streams** 與 **Consumer Groups (消費者組)** 機制實作，確保任務的分散式處理與高可靠性。

### 1. OG Fetch Worker (連結預覽抓取)
此業務線負責在短網址建立後，非同步抓取目標網頁的 Open Graph (OG) 標籤。

- **Stream**: `stream:og-fetch`
- **運作流程**:
    1. 當 API 成功建立短網址後，發送任務至 Stream。
    2. Worker 領取任務並訪問目標網址。
    3. 解析 HTML 中的標題、縮圖與描述（最多重試 3 次）。
    4. **成功**: 更新資料庫中的 `og_metadata`，並主動刪除 Redis 快取，使下次訪問讀取到最新的 OG 資訊。
    5. **失敗**: 若重試耗盡仍無法抓取，則標記 `fetch_failed: true`、更新資料庫，並同樣清除快取。
- **特點**: OG 抓取失敗視為「非致命錯誤」，不應阻塞或卡住任務隊列。快取清除失敗同樣為非致命，TTL 到期後自然失效。

### 2. Click Log Worker (點擊數據分析)
此業務線負責將每次短網址的點擊數據持久化至資料庫，支援精確的行銷歸因分析。

- **Stream**: `stream:click-log`
- **運作流程**:
    1. 每次 Redirect 發生時，API Server 會發送點擊事件。
    2. Worker 採用 **Batch Processing (批次處理)**，一次讀取多筆訊息（如 100 筆）後批次寫入資料庫，以極大化寫入效能。
    3. **PEL (Pending Entries List)**: 訊息讀取後進入 PEL 狀態，直到收到 `XACK` 才會移除。
    4. **可靠性機制**:
        - **XCLAIM**: 若某個 Worker 當機導致訊息卡在 PEL 超過 30 秒，其他 Worker 會重新認領並重試。
        - **DLQ (Dead Letter Queue)**: 若同一訊息重試超過 5 次（毒藥訊息），則移至 `stream:click-dlq` 隔離，並記錄 Error Log 供人工排查。
- **特點**: 點擊數據視為「重要資產」，透過重試與 DLQ 機制確保數據「不遺失」。

---

## 核心技術機制

### 消費者組 (Consumer Groups) 邏輯
系統利用消費者組達成以下目標：
- **負載均衡**: 同一組內的多個 Worker 實例會自動分配任務，一筆訊息只會被一個實例處理。
- **業務隔離**: `og-group` 與 `click-group` 擁有獨立的消費進度，互不干擾。
- **進度追蹤**: Redis 自動記錄每個組的最後消費位置，即使服務重啟也能從中斷處繼續。

### 快取策略 (Cache-Aside)
短網址的重新導向採用 Cache-Aside 模式，以減少資料庫查詢壓力：

1. **讀取**: 優先查 Redis 快取（TTL 24 小時）；未命中時查資料庫並回填快取。
2. **寫入**: 短網址建立時不預先寫入快取，首次訪問才觸發回填。
3. **失效**: OG Worker 寫回 `og_metadata` 後，主動刪除對應快取鍵，確保 Bot 下次訪問能拿到最新 OG 資訊，而非初始的 nil 版本。

### 數據保全策略
1. **At-least-once Delivery**: 透過 `XACK` 確認機制，保證每筆任務至少被成功處理一次。
2. **Fail-safe**: 點擊事件發送至 Redis 失敗時，API Server 僅記錄錯誤日誌，不影響使用者的重新導向體驗。
3. **隔離與觀察**: 透過 `click-dlq` 隔離故障資料，維持系統在異常發生時的持續運作能力。

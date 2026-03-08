# 260308-short-url-system-design-plan

## 目標

建立一個具備全球擴充性、高可用性與精確歸因分析的社群短網址服務。第一階段（POC）以簡化版 Snowflake 演算法生成 ID，無 WorkerID 管理負擔，確保在 Cloud Run Serverless 環境下易於部署且具備未來擴充彈性。

## 現況分析與技術權衡

| 決策項目 | 選擇                                           | 理由                                                            |
| :------- | :--------------------------------------------- | :-------------------------------------------------------------- |
| ID 生成  | 簡化版 Snowflake (41-bit 時間戳 + 12-bit 序號) | 無需 WorkerID 協調，單一節點每毫秒可產生 4096 個 ID，適合 POC   |
| 短碼編碼 | Base58，固定 10 碼                             | Snowflake 53-bit 數值轉 Base58 自然落在 10 碼；易讀、無歧義字元 |
| 基礎設施 | 邊緣快取 (CDN) + 非同步處理 (Redis Streams)    | 讀取遠大於寫入，以快取命中為核心優化路徑                        |
| 資料庫   | PostgreSQL                                     | 強一致性唯一碼約束，本地開發友好                                |
| OG 抓取  | 非同步（Redis Streams）                        | API 建立成功後發送訊息，不阻塞回應                              |

## 設計方案

### 1. 簡化版 Snowflake ID 生成器

**結構（53 bits）：**

```
[41-bit 毫秒時間戳 | Epoch: 2026-01-01T00:00:00Z] [12-bit 毫秒內序號]
```

**容量計算：**

- **時間戳**：2^41 = 2.2 兆毫秒 ÷ 1000 ÷ 60 ÷ 60 ÷ 24 ÷ 365.25 ≈ **69.7 年**（可用至約 2095 年）。
- **序號**：每毫秒最多 4,096 個 ID，超過則 spin-wait 至下一毫秒。

**無 WorkerID 的取捨：**

- 移除 WorkerID 後，每個 Cloud Run 實例都是獨立 process，各自從序號 0 開始遞增。
- **潛在碰撞**：若兩個實例在同一毫秒產生序號相同的 ID，`short_code` 會重複。
- **第一版防護策略**：`short_url.short_code` 欄位設 `UNIQUE` 約束，寫入失敗時 retry 重新生成（概率極低，POC 可接受）。
- **未來改善**：可引入 4-bit WorkerID（Redis INCR 分配，無需心跳）或改用號段模式（Segment Mode）徹底解決。
- **IDGenerator 介面**：`snowflake.Generator` 以 `IDGenerator` interface 注入，POC 階段 `NewGenerator()` 不帶 WorkerID 參數，未來只需改 `cmd/` 組裝層，service 層零修改。

**Base58 編碼：** 將 Snowflake 53-bit int 轉為 Base58 字串，固定補齊至 **10 碼**。

### 2. 重新導向邏輯流程 (Redirect Handler)

當使用者點擊 `/{shortCode}` 時：

1. **快取查詢**：優先從 Redis 讀取映射（`short_code → {long_url, expires_at, creator_id}`）。若 Cache Miss，從 PostgreSQL 讀取並回填快取。
2. **過期檢查**：若 `expires_at` 不為 null 且已過期，回傳 **`HTTP 410 Gone`**；短碼不存在則回傳 **`HTTP 404`**。
3. **分析發送 (Fail-safe)**：擷取 Metadata（IP、UA、Referrer、ref 參數）非同步送入 Redis Streams。若失敗，記錄 Error Log 並繼續跳轉邏輯，**不阻塞回應**。
4. **跳轉**：回傳 `HTTP 302 Found`。

### 3. Bot 感知回應 (Link Preview)

- 解析 `User-Agent`，識別社群平台爬蟲（Facebookbot、Twitterbot 等）。
- 爬蟲請求：回傳包含 OG tags 的 HTML（從 `short_url.og_metadata` 讀取）。
- 一般請求：執行步驟 2 重導向邏輯。

### 4. 非同步 OG 抓取流程

```
POST /v1/urls
    └── 寫入 short_url (og_metadata = null)
    └── 發送 {short_url_id, long_url} 至 Redis Stream: stream:og-fetch
        └── Preview Worker 消費訊息
            └── HTTP GET long_url → 解析 OG/Twitter Card tags（最多 retry 3 次，指數退避 1s/2s/4s）
            ├── 成功 → UPDATE short_url SET og_metadata = {...} WHERE id = ?
            └── 全部失敗 → UPDATE short_url SET og_metadata = {"fetch_failed": true} WHERE id = ?
                          XACK（不卡隊列，OG 失敗為非致命錯誤）
```

**OG 失敗處理原則：** OG 抓取失敗不影響核心重導向功能，以 `fetch_failed` 標記保留可觀測性，未來可透過 admin 功能重新觸發，不依賴 Stream 重試機制。

## 未來改善項目 (Future Improvements)

| 項目          | 說明                                             |
| :------------ | :----------------------------------------------- |
| Rate Limiting | API Gateway 或 Middleware 層加入速率限制         |
| JWT 認證      | `creator_id` 改由 JWT Token 攜帶，服務端解析驗證 |
| 惡意網址檢測  | 整合 Google Safe Browsing API                    |
| 黑名單機制    | Domain/短碼封鎖清單                              |
| Bridge Page   | Pixel/GA 追蹤：`HTTP 200` 中間頁執行 JS 後跳轉   |
| 連結去重      | `hash(long_url + creator_id)` 避免重複建立       |
| 號段模式      | 高併發寫入時替換 Snowflake，提升 ID 分配吞吐量   |
| 短碼縮短      | 調整編碼策略支援 6 碼等更短格式                  |

## 技術選型

| 元件             | 技術                     | 用途                                    |
| :--------------- | :----------------------- | :-------------------------------------- |
| 開發語言         | Go 1.24                  | API Server、Worker                      |
| 核心資料庫       | PostgreSQL 17            | 業務資料儲存（穩定版，EOL 2029-11）     |
| 快取             | Redis 7（DB 0）          | 短網址映射快取                          |
| 訊息隊列         | Redis 7（DB 1, Streams） | 點擊事件、OG 抓取任務                   |
| 分析倉儲（本地） | PostgreSQL `click_log`   | 點擊日誌直接寫入 PG，本地不另起分析引擎 |
| 分析倉儲（生產） | Google BigQuery          | 點擊日誌大數據分析                      |

> Redis Cache（DB 0）與 Streams（DB 1）使用不同 DB 隔離，避免 eviction policy 影響 Stream 資料。

## 資料庫 Schema (PostgreSQL)

### Table: `short_url`

| 欄位        | 型別              | 說明                                                                                         |
| :---------- | :---------------- | :------------------------------------------------------------------------------------------- |
| id          | BIGINT (PK)       | Snowflake ID                                                                                 |
| short_code  | VARCHAR(10) (UIX) | Base58 編碼字串，固定 10 碼                                                                  |
| long_url    | TEXT              | 原始網址                                                                                     |
| creator_id  | VARCHAR(50)       | 創作者識別（未來改由 JWT 解析）                                                              |
| og_metadata | JSONB             | OG/Twitter Card 預覽資訊（非同步填入，初始為 null）；抓取失敗時寫入 `{"fetch_failed": true}` |
| expires_at  | TIMESTAMPTZ       | 短網址過期時間（null 表示永不過期）                                                          |
| created_at  | TIMESTAMPTZ       | 建立時間                                                                                     |

### Table: `click_log`（本地用於儲存點擊數據）

| 欄位         | 型別        | 說明                                 |
| :----------- | :---------- | :----------------------------------- |
| id           | UUID (PK)   | 點擊唯一 ID                          |
| short_url_id | BIGINT      | 關聯到 `short_url.id`                |
| short_code   | VARCHAR(10) | 冗餘存儲，加速查詢                   |
| creator_id   | VARCHAR(50) | 歸因用                               |
| referral_id  | VARCHAR(50) | 從 `?ref=` 擷取的推薦碼              |
| referrer     | TEXT        | HTTP Referer                         |
| user_agent   | TEXT        | 裝置與瀏覽器資訊                     |
| ip_address   | VARCHAR(45) | 原始 IP（IPv4/IPv6，需符合隱私規範） |
| is_bot       | BOOLEAN     | 是否為爬蟲                           |
| created_at   | TIMESTAMPTZ | 點擊時間                             |

## 監控與可觀測性 (Observability)

| 指標           | 說明                                |
| :------------- | :---------------------------------- |
| QPS            | API Server 每秒請求數（分路由統計） |
| P99 重導向延遲 | `GET /{shortCode}` 端到端延遲       |
| 快取命中率     | Redis Cache Hit / Miss 比率         |
| Worker 積壓量  | Redis Streams pending 訊息數量      |
| 錯誤率         | 5xx 錯誤比率                        |

**Alert 條件：**

- P99 延遲 > 200ms 持續 1 分鐘
- 錯誤率 > 1% 持續 30 秒
- Stream pending 訊息 > 10,000 持續 5 分鐘

## 實作步驟

1. 建立專案結構與 Docker Compose（Go, Postgres, Redis）。
2. 實作簡化版 Snowflake 生成器與 Base58 編碼器。
3. 實作 `POST /v1/urls` API（接收 `long_url`、`creator_id`）。
4. 實作 `GET /{shortCode}` 重導向 Handler（含快取、過期檢查、302/404/410）。
5. 實作分析 Worker：從 Redis Stream 讀取點擊事件並寫入 PostgreSQL `click_log`（生產環境替換為 BigQuery）。
6. 實作 Preview Worker：從 Redis Stream 讀取任務，抓取 OG Tags 並更新 `short_url.og_metadata`。

## 注意事項

- **Snowflake Epoch**：固定為 `2026-01-01T00:00:00Z`（Unix ms: 1767225600000）。
- **故障隔離**：分析日誌收集需放在背景 Goroutine，不可阻塞 API 回應。
- **隱私合規**：`ip_address` 儲存需符合 GDPR/CCPA；考慮在分析倉儲寫入時進行去識別化處理。
- **Redis 隔離**：Cache（DB 0）設 `allkeys-lru`；Streams（DB 1）設 `noeviction`，避免事件遺失。
- **Worker Graceful Shutdown**：Worker 收到 SIGINT/SIGTERM 後停止拉取新訊息，等待當前 batch 處理完成（含 XACK）後再退出，避免訊息重複消費或資料不一致。
- **點擊事件 Dead Letter**：Click Consumer 處理失敗時不 XACK，訊息留在 PEL 等待重試；超過 5 次重試（XCLAIM idle > 30s）後移至 `stream:click-dlq` Dead Letter Stream，並記錄 error log 保留可觀測性。
- **RecordClick Fail-safe**：點擊事件發送至 Redis Stream 失敗時，僅記錄 error log 與 counter，不阻塞重導向回應。

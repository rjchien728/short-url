# 260307-short-url-product-spec-design

## 目標

設計一個具備高可用性、低延遲且支援社群分享體驗的短網址服務。除了基礎的網址轉換外，系統需整合連結預覽（Link Preview）、推薦碼機制（Referral Tracking）以及進階的行銷追蹤功能。

## 功能需求 (Functional Requirements)

### 1. 短網址生成與管理

- **編碼機制**：採用簡化版 Snowflake 演算法（41-bit 時間戳 + 12-bit 序號，共 53 bits），再以 **Base58** 字元集（排除 `0, O, I, l`）編碼，產生 **10 碼**短網址。
- **容量評估**：Base58 10 碼可容納 58^10 ≈ 4300 兆組合，每日產生 1 萬個短網址可使用數百億年。
- **創作者綁定**：產生短網址時，記錄 `creator_id`，用於後續流量歸因。
- **過期機制**：支援設定短網址的有效期限（`expires_at`）；過期後訪問回傳 **HTTP 410 Gone**。
- **未來擴充方向**：
  - 短碼長度可依需求調整（加入 URL path 版本號，如 `/v2/{shortCode}`）。
  - 如需縮短長度, 可改用號段模式（Segment Mode）分配 ID，提升高併發寫入能力。

### 2. 重新導向 (Redirection)

- **一般跳轉**：回傳 `HTTP 302 Found`，追求重導向速度。
- **過期或不存在**：短碼不存在回傳 `HTTP 404`；短碼已過期回傳 `HTTP 410 Gone`。
- 未來擴充: 增加 HTTP 200 的跳轉頁用 js 送 ga 資訊

### 3. 社群連結預覽 (Link Preview)

- **非同步抓取**：短網址建立後，API Server 發送訊息至 Message Queue；Preview Worker 非同步抓取目標長網址的 Open Graph (OG) 與 Twitter Cards 資訊並寫回資料庫。API 建立成功不等待抓取完成。
- **Bot 感知回應**：當社群平台爬蟲（如 Facebookbot, Twitterbot）訪問時，回傳包含預覽資訊的 HTML；一般使用者則執行重新導向邏輯。

### 4. 推薦與歸因機制 (Referral & Attribution)

- **推薦碼格式**：支援以參數形式帶入推薦資訊，格式如 `baseurl/{short_code}?ref={referral_id}`。
- **點擊日誌**：記錄每次點擊的 Metadata（IP 原始值、User-Agent、Referrer、歸因對象）。
- **成效歸因**：系統需能將轉換成效歸因至特定推薦者（`referral_id`）或原始創作者（`creator_id`）。

## 非功能需求 (Non-Functional Requirements)

### 1. 高可用性 (High Availability)

- **多區域部署**：服務需部署於全球多個 Region，確保單一機房故障不影響服務。
- **無單點故障**：所有組件（API, Worker, DB）皆需具備備援機制。

### 2. 低延遲與可擴展性 (Latency & Scalability)

- **全球加速**：利用 Global Load Balancer (GCLB) 將使用者導向最近節點。
- **邊緣快取**：啟用 CDN 快取重導向回應（需設定 `Cache-Control`），減輕資料庫負擔。注意快取命中時點擊數據需透過其他方式補齊。
- **秒級擴充**：執行環境需能應付社群媒體帶來的突發流量高峰。

### 3. 監控與可觀測性 (Observability)

- **核心指標**：QPS（每秒請求數）、P99 重導向延遲、快取命中率、Worker 積壓量。
- **告警機制**：P99 延遲超過閾值、錯誤率異常、Worker 消費落後時觸發 Alert。

## 技術選型 (Tech Stack)

### 1. 雲端生產環境 (GCP)

- **運算層**：Google Cloud Run (Serverless, Go 執行環境)。
- **資料庫**：Cloud SQL or AlloyDB。
- **快取層**：Memorystore for Redis。
- **分析倉儲**：BigQuery (local 開發先寫進同一個 pg instance)。
- **訊息隊列**：Cloud Pub/Sub (處理非同步抓取與數據寫入)。

### 2. 本地開發環境 (Docker Compose)

- **API/Worker**：Go (Golang 1.24)。
- **主資料庫**：PostgreSQL 17。
- **快取/隊列**：Redis 7（Cache 與 Redis Streams 分開 DB）。
- **分析倉儲（本地）**：PostgreSQL（點擊日誌直接寫入 `click_log` 表，本地開發不另起分析引擎）。

## 實作步驟

1. 建立專案基礎結構與 Docker Compose 配置。
2. 實作簡化版 Snowflake 生成器與 Base58 編碼器。
3. 實作 `POST /v1/urls` API（接收 `long_url`、`creator_id`、可選 `expires_at`）。
4. 實作 `GET /{shortCode}` 重導向 Handler（含 302、404、410 邏輯）。
5. 建立非同步分析流程，將點擊數據送入訊息佇列。
6. 實作 Link Preview Worker，負責 OG 資訊抓取與更新。
7. 開發歸因分析報表邏輯（基於 PostgreSQL `click_log`，生產環境對應 BigQuery）。

## 改善項目 (Improvements，非第一版範圍)

| 項目          | 說明                                                     |
| :------------ | :------------------------------------------------------- |
| Rate Limiting | 對 `POST /v1/urls` 加入速率限制，防止濫用                |
| 認證機制      | `creator_id` 改由 JWT Token 攜帶，由服務端解析，避免偽冒 |
| 惡意網址檢測  | 整合 Google Safe Browsing API 檢查目標網址               |
| 黑名單機制    | 對已知惡意 domain/短碼加入封鎖清單                       |
| Bridge Page   | Pixel/GA 追蹤模式：回傳 `HTTP 200` 中間頁執行 JS 後跳轉  |
| 連結去重      | `hash(long_url + creator_id)` 避免重複生成相同短網址     |
| 短碼縮短      | 調整編碼策略以支援 6 碼等更短格式                        |

## 注意事項

- **隱私合規**：記錄原始 IP 時，需符合 GDPR/CCPA 等隱私規範（考慮 IP 去識別化或僅儲存至分析倉儲）。
- **防止爬蟲干擾**：分析數據需排除已知爬蟲的點擊數，確保歸因準確性。
- **安全性**：Snowflake Epoch 與系統鹽值（若未來使用 Hashids）需透過環境變數注入，不可洩漏至客戶端。

# 系統設計作業題：社群短網址服務

題目：設計一個高可用的短網址服務（Short URL Service），應用於社群分享場景
請設計一個短網址系統，當使用者在社群平台（如 Facebook, Thread, A-pen）分享內容時，能夠產生短網址並提供豐富的分享體驗。系統需滿足以下核心需求：

## 功能需求（Functional Requirements）

### 1. 短網址生成與導向：將長網址轉換為短網址，並能正確重新導向至原始頁面。

**實作**：Snowflake ID → Base58 編碼 → 10 碼 `short_code`，寫入 PostgreSQL。

- Redirect：Cache-Aside（Redis → PostgreSQL），回傳 `302`

---

### 2. 連結預覽（Link Preview）：短網址在社群平台貼上時，需能正確呈現預覽資訊（標題、摘要、縮圖等），提供良好的分享體驗。

**實作**：User-Agent 偵測爬蟲（bot）：

- 一般使用者：`302` redirect
- Bot：`200` HTML，內含從目標頁面非同步抓取的 OG tags（title、description、image、site_name），存於 `short_url.og_metadata`

---

### 3. 推薦碼機制（Referral Tracking）：使用者分享短網址時，可攜帶個人推薦碼，系統需能識別並記錄推薦來源。

**實作**：支援 `?ref=<referral_id>` query parameter，redirect 時擷取寫入 `click_log.referral_id`，可依此欄位歸因推薦來源。

---

### 4. 成效追蹤與歸因分析（Analytics & Attribution）：記錄每個短網址的點擊數據（點擊次數、來源平台、地區、裝置等），並能將轉換成效歸因至特定推薦者或行銷活動。

**實作**：Redirect 時非同步發布點擊事件，Worker 批次寫入 `click_log`。

- 記錄欄位：`referral_id`、`referrer`、`user_agent`、`ip_address`、`is_bot`（GeoIP 待補）
- 可依 `creator_id`、`short_code`、`referral_id` 做歸因查詢

> **進階**：可在 redirect 前加入跳轉中間頁，埋入 GA / GTM 並帶上 UTM 參數，取得更豐富的受眾與轉換歸因資料，無需自建分析 pipeline。

---

## 非功能需求（Non-Functional Requirements）

### 5. 高可用性（High Availability）：服務需具備高可用性，短網址的導向不可因單點故障而中斷。

**實作**：API 與 Worker 均為 stateless，可水平擴展。

- Redis Consumer Group：多個 Worker 實例不重複消費同一訊息
- PEL + XCLAIM：訊息在 `XACK` 前不消失，Worker 當機後由其他實例重新認領

> **進階**：DB 層可採用 AlloyDB（GCP）或 Aurora Global Database（AWS），在各地區部署 Read Replica，寫入回主地區。短網址服務讀寫比高，各地區副本可就近服務讀取請求，跨地區複製延遲通常在 1 秒內，對查詢場景完全可接受。

---

### 6. 低延遲（Low Latency）：短網址的重新導向以及產生應在極短時間內完成。

**實作**：

- Redirect hot path：Redis Cache 優先，cache miss 才查 DB
- OG 抓取、click log 寫入均非同步，不阻塞 redirect 回應
- Snowflake ID 本地計算，無額外網路 round-trip

> **進階**：可引入 CDN（如 CloudFront）在 edge node 快取跳轉頁，redirect 完全不需打到 origin server，實現極低延遲。此方案下 click 資料改由跳轉頁 JS 上報 GA。需注意 bot 偵測與 OG preview 須透過 Lambda@Edge bypass CDN，否則爬蟲只會拿到快取的跳轉頁而非 OG HTML。

---

### 7. 可擴展性（Scalability）：需能承受社群分享帶來的突發流量高峰。

**實作**：

- API / Worker stateless，透過 container orchestration 水平擴展
- Redis Cache 吸收讀取峰值
- Redis Stream 作為 buffer 削峰，Worker 批次寫入降低 DB 壓力

> **注意**：Redis Stream 與 PostgreSQL 為 POC 階段的簡化選擇。Production 建議 Stream 替換為 Kafka / Cloud Pub/Sub；click log 寫入替換為 BigQuery / ClickHouse 等 OLAP 儲存，以支撐大規模分析查詢。搭配多地區 Read Replica（見需求 5）可進一步分散讀取壓力。

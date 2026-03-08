# 260308-short-url-implementation-plan

## 專案背景與動機

本專案為系統設計作業題的實作練習，目標是從零開始實作一套具備生產水準思維的**社群短網址服務（Short URL Service）**。

### 為什麼要做這個

社群平台分享時，長網址難以傳播、且無法控制連結預覽的呈現內容。本系統解決三個核心問題：

1. **網址縮短**：將長網址轉換為 10 碼短碼（Base58 + Snowflake ID），易於傳播
2. **連結預覽（Link Preview）**：當社群爬蟲（如 Facebookbot）訪問短網址時，回傳含 OG tags 的 HTML，讓貼文有正確縮圖與標題
3. **歸因分析（Attribution Analytics）**：記錄每次點擊的 Metadata（IP、UA、Referrer、推薦碼），支援行銷成效追蹤

### 技術棧速覽

| 元件            | 技術                                 |
| :-------------- | :----------------------------------- |
| 語言            | Go 1.24                              |
| HTTP Framework  | Echo v4                              |
| 資料庫          | PostgreSQL 17                        |
| 快取 / 訊息隊列 | Redis 7（Cache DB 0 / Streams DB 1） |
| Config          | Viper                                |
| DB Client       | pgx v5 + squirrel                    |
| Logger          | slog（stdlib）                       |

### 系統架構概述

兩個執行入口，分層架構：

```
cmd/api      → HTTP API Server（建立短網址、重導向）
cmd/worker   → Background Worker（OG 抓取 + 點擊日誌寫入）

分層（由下而上）：
  infra（DB/Redis 連線）
    → repository / gateway（資料存取實作）
      → service（業務邏輯）
        → handler / consumer（驅動層，接收 HTTP 請求或 Redis Stream 訊息）
          → cmd（組裝層，唯一知道具體型別的地方）
```

中心合約（`internal/domain/`）：定義所有 interface 與 entity，零外部依賴，所有層均依賴此合約。

---

## 設計文件索引

> AI agent 實作各 Phase 前，**必須先閱讀對應的設計文件章節**，不可憑假設實作。

| 文件              | 路徑                                           | 說明                   | 主要內容                                                                                   |
| :---------------- | :--------------------------------------------- | :--------------------- | :----------------------------------------------------------------------------------------- |
| 原始需求          | `spec.md`                                      | 系統設計作業原始題目   | 功能需求、非功能需求                                                                       |
| Product Spec      | `260307-short-url-product-spec-design.md`      | 功能需求細化與改善計劃 | 短網址生成規格、過期機制、Link Preview、推薦碼、技術選型                                   |
| System Design     | `260308-short-url-system-design-plan.md`       | 技術方案設計           | Snowflake ID 結構、重導向流程、OG 抓取流程、DB Schema、Redis 隔離策略、監控指標            |
| Code Architecture | `260308-short-url-code-architecture-design.md` | 程式碼架構設計         | **目錄結構**、分層定義、所有 interface/entity/service 的 Go 程式碼、測試策略、設計決策紀錄 |
| API Spec          | `260308-short-url-api-spec-design.md`          | 外部 API 與事件規範    | HTTP 端點、Request Validation 規則、Error Response 格式、Redis Stream Event Payload        |
| Config Design     | `260308-short-url-config-design.md`            | 設定管理設計           | 環境變數映射表、預設值、viper 載入邏輯、Config struct 設計                                 |

---

## 目標

依照已完成的 system design、code architecture、API spec 與 config design，規劃 Short URL POC 的完整實作順序。以「可在本地端跑起來的可工作系統」為每個 Phase 的交付標準。

## 實作原則

- **由下而上**：先建基礎設施與 domain 合約，再疊服務層，最後接驅動層。
- **每個 Phase 可獨立驗收**：完成後能執行對應的測試或手動驗證。
- **不跳層**：不在 service 尚未完成時寫 handler，避免邊界不清。

---

## 現有產出（實作開始前的狀態）

> AI agent 實作時，以下已存在的檔案**不應重新建立或覆蓋**，除非任務明確要求修改。

| 檔案 / 目錄                                   | 說明                                                                                                       |
| :-------------------------------------------- | :--------------------------------------------------------------------------------------------------------- |
| `go.mod`                                      | Module 為 `github.com/rjchien728/short-url`，Go 1.24。目前只安裝了 echo 與 uuid，其餘依賴需在 Phase 0 補齊 |
| `docker-compose.yml`                          | PostgreSQL 17 + Redis 7，已可使用                                                                          |
| `.env.example`                                | 環境變數範例，包含所有 Config Design 中定義的變數                                                          |
| `.gitignore`                                  | 已包含 `.env`                                                                                              |
| `migrations/000001_create_short_url.up.sql`   | `short_url` 表建立 SQL                                                                                     |
| `migrations/000001_create_short_url.down.sql` | `short_url` 表刪除 SQL                                                                                     |
| `migrations/000002_create_click_log.up.sql`   | `click_log` 表建立 SQL                                                                                     |
| `migrations/000002_create_click_log.down.sql` | `click_log` 表刪除 SQL                                                                                     |
| `internal/pkg/logger/logger.go`               | context-aware slog wrapper，已實作 `Setup`、`WithLogger`、`FromContext`、`Info/Debug/Warn/Error`           |
| `internal/pkg/logger/middleware.go`           | Echo middleware，已實作 `Middleware()`，注入 `request_id` 與 request-scoped logger 至 context              |

---

## Phase 0：專案基礎建設

建立可執行的空殼專案，確認所有工具鏈正常運作。

> **前置條件**：無
>
> **參考文件**：
>
> - `260308-short-url-code-architecture-design.md` → 「目錄結構」章節（完整目錄樹）
> - `260308-short-url-config-design.md` → 「環境變數映射表」
> - `spec.md` → 功能需求（了解系統全貌）

### 任務

1. **初始化 Go module**
   - module 已存在（`github.com/rjchien728/short-url`），確認 `go.mod` 正確
   - 建立 `cmd/api/main.go`、`cmd/worker/main.go`（空的 `main` 函數）

2. **建立目錄結構骨架**
   - 依照 code architecture 文件建立所有目錄（空目錄用 `.gitkeep` 佔位）
   - 目錄清單：`internal/domain/entity/`、`internal/domain/service/`、`internal/domain/repository/`、`internal/domain/gateway/`、`internal/service/url/`、`internal/service/redirect/`、`internal/service/ogworker/`、`internal/service/clickworker/`、`internal/repository/shorturl/`、`internal/repository/clicklog/`、`internal/repository/urlcache/`、`internal/repository/eventpub/`、`internal/gateway/ogfetch/`、`internal/consumer/`、`internal/handler/`、`internal/infra/`、`internal/pkg/snowflake/`、`internal/pkg/base58/`、`internal/pkg/botdetect/`

3. **Docker Compose**
   - `docker-compose.yml` 已存在，確認 PostgreSQL 17（port 5432）與 Redis 7（port 6379）可正常啟動
   - 驗收：`docker compose up -d` 後，`psql` 與 `redis-cli ping` 均可連線

4. **Makefile**
   - 建立 `Makefile`，定義常用指令：`make run-api`、`make run-worker`、`make test`、`make migrate-up`、`make migrate-down`、`make lint`

5. **安裝依賴套件**（補齊 go.mod 缺少的部分）
   - `github.com/jackc/pgx/v5`
   - `github.com/Masterminds/squirrel`
   - `github.com/redis/go-redis/v9`
   - `github.com/spf13/viper`
   - `github.com/golang-migrate/migrate/v4`
   - `golang.org/x/sync`（errgroup）
   - `github.com/testcontainers/testcontainers-go`（Phase 4 整合測試用）
   - `github.com/stretchr/testify`（所有測試用）
   - 已存在：`github.com/labstack/echo/v4`、`github.com/google/uuid`

### 驗收標準

- `go build ./...` 無錯誤
- `docker compose up -d` 服務正常啟動
- `make test` 指令可執行（即使無 test 也不 error）

---

## Phase 1：Config 與基礎設施層

建立 config 載入與 DB/Redis 連線元件，所有後續層均依賴此 Phase。

> **前置條件**：Phase 0 完成
>
> **參考文件**：
>
> - `260308-short-url-config-design.md` → 完整文件（Config struct 設計、環境變數映射、viper 載入邏輯）
> - `260308-short-url-system-design-plan.md` → 「技術選型」章節（Redis DB 隔離策略）
> - `260308-short-url-code-architecture-design.md` → 「組裝層設計」章節（`cmd/api/main.go` 中 `config.Load()` 的使用方式）

### 任務

1. **Config（`internal/infra/config.go`）**
   - 定義 Config struct（App、Server、Database、Redis Cache、Redis Stream）
   - 使用 viper 實作載入邏輯：`.env` → 環境變數
   - 依照 config design 文件設定所有預設值與環境變數映射
   - `.env.example` 已存在，確認 struct 欄位與其對應

2. **PostgreSQL 連線（`internal/infra/postgres.go`）**
   - 實作 `NewPool(cfg DatabaseConfig) (*pgxpool.Pool, error)`
   - 套用 `MaxOpenConns`、`MaxIdleConns` 設定

3. **Redis 連線（`internal/infra/redis.go`）**
   - 實作 `NewClient(cfg RedisConfig) (*redis.Client, error)`
   - DB 號碼由 `RedisConfig` 中的 URL 決定（Cache URL 包含 `/0`，Stream URL 包含 `/1`）

4. **Database Migration（`migrations/`）**
   - Migration SQL 已存在，不需重新建立
   - 在 `Makefile` 中確認 `make migrate-up` / `make migrate-down` 可正常執行
   - 驗收：`make migrate-up` 成功建立兩張 table

### 驗收標準

- `config.Load()` 可正確讀取 `.env` 與環境變數
- `infra.NewPool()` 可連線至本地 PostgreSQL
- `infra.NewClient()` 可連線至本地 Redis
- `make migrate-up` 跑完後，`\d short_url` 與 `\d click_log` 結構正確

---

## Phase 2：工具層（internal/pkg）

實作零或極少外部依賴的共用工具，可獨立開發與測試。

> **前置條件**：Phase 0 完成（目錄結構存在即可，不依賴 Phase 1）
>
> **參考文件**：
>
> - `260308-short-url-system-design-plan.md` → 「簡化版 Snowflake ID 生成器」章節（位元結構、Epoch、序號溢出策略）
> - `260308-short-url-code-architecture-design.md` → 「IDGenerator 介面」章節（interface 定義）、「工具層」表格（各工具職責）

### 任務

1. **Logger（`internal/pkg/logger/`）**
   - `logger.go` 與 `middleware.go` 已實作完畢，勿覆蓋
   - 補齊 unit test（`logger_test.go`）：確認 `WithLogger` / `FromContext` / `Setup` 行為正確
   - `Middleware()` 已是 echo middleware 格式，不需修改

2. **Snowflake ID 生成器（`internal/pkg/snowflake/`）**
   - 定義 `IDGenerator` interface（`Generate() (int64, error)`）
   - 實作 `Generator`（41-bit 時間戳 + 12-bit 序號，Epoch: `2026-01-01T00:00:00Z`，Unix ms: `1767225600000`）
   - 處理同毫秒序號溢出（spin-wait 至下一毫秒）
   - 驗收：unit test 確認生成 ID 單調遞增、無碰撞

3. **Base58 編碼器（`internal/pkg/base58/`）**
   - 實作 `Encode(id int64) string`，固定補齊至 10 碼
   - 實作 `Decode(s string) (int64, error)`
   - 驗收：unit test 確認 encode/decode 互逆，53-bit 數值輸出固定 10 碼

4. **Bot 偵測（`internal/pkg/botdetect/`）**
   - 實作 `IsBot(userAgent string) bool`
   - 涵蓋 Facebookbot、Twitterbot、Googlebot 等常見爬蟲 UA
   - 驗收：unit test 確認常見 bot UA 正確識別

### 驗收標準

- `go test ./internal/pkg/...` 全部通過

---

## Phase 3：Domain 合約層

定義所有 interface 與 entity，零外部依賴，是整個系統的合約中心。

> **前置條件**：Phase 0 完成（目錄結構存在即可，可與 Phase 1、2 平行開發）
>
> **參考文件**：
>
> - `260308-short-url-code-architecture-design.md` → 「介面設計」章節（完整的 Go 程式碼定義，**直接照抄實作**，不可自行推斷介面簽名）
> - `260308-short-url-system-design-plan.md` → 「資料庫 Schema」章節（了解 entity 欄位的業務含義）

### 注意事項

- 所有 interface 與 entity 的**欄位名稱、型別、方法簽名**均在 code architecture 文件中有完整 Go 程式碼定義，請直接參照，不可自行推斷或修改。

### 任務

1. **Entity（`internal/domain/entity/`）**
   - `shorturl.go`：`ShortURL`、`OGMetadata`（含 `FetchFailed bool`）、`OGFetchTask`、`IsExpired()` 方法
   - `clicklog.go`：`ClickLog`（**不含 `IPCountry` 欄位**，POC 不實作 GeoIP）

2. **Service Interface（`internal/domain/service/`）**
   - `url.go`：`URLService`、`CreateURLRequest`
   - `redirect.go`：`RedirectService`
   - `worker.go`：`OGWorkerService`、`ClickWorkerService`

3. **Repository Interface（`internal/domain/repository/`）**
   - `repository.go`：`ShortURLRepository`、`ClickLogRepository`、`URLCache`、`EventPublisher`

4. **Gateway Interface（`internal/domain/gateway/`）**
   - `gateway.go`：`OGFetcher`

### 驗收標準

- `go build ./internal/domain/...` 無錯誤，無任何外部依賴（`go mod tidy` 後 domain 不引入外部套件）

---

## Phase 4：被驅動層（Repository & Gateway）

實作 domain interface 的具體存取邏輯。

> **前置條件**：Phase 1（infra 連線元件可用）、Phase 3（domain interface 已定義）
>
> **參考文件**：
>
> - `260308-short-url-code-architecture-design.md` → 「domain/repository」介面定義（`ShortURLRepository`、`ClickLogRepository`、`URLCache`、`EventPublisher`）、「domain/gateway」介面定義（`OGFetcher`）
> - `260308-short-url-system-design-plan.md` → 「資料庫 Schema」章節（欄位對應）
> - `260308-short-url-api-spec-design.md` → 「內部 Event Payload」章節（Redis Stream 欄位定義）

### 任務

1. **ShortURL Repository（`internal/repository/shorturl/`）**
   - `Create(ctx, *entity.ShortURL) error`：使用 squirrel 組 INSERT
   - `FindByShortCode(ctx, shortCode) (*entity.ShortURL, error)`：查詢含 `og_metadata`
   - `UpdateOGMetadata(ctx, id, *entity.OGMetadata) error`：UPDATE JSONB 欄位
   - 驗收：整合測試（testcontainers-go）

2. **ClickLog Repository（`internal/repository/clicklog/`）**
   - `BatchCreate(ctx, []*entity.ClickLog) error`：使用 pgx `CopyFrom` 或批次 INSERT
   - 驗收：整合測試

3. **URL Cache（`internal/repository/urlcache/`）**
   - `Get`、`Set`（TTL 固定 24h）、`Delete`
   - 驗收：整合測試（miniredis 或 testcontainers）

4. **Event Publisher（`internal/repository/eventpub/`）**
   - `PublishClickEvent`：XADD 至 `stream:click-log`，欄位參照 API spec 的 Event Payload 定義
   - `PublishOGFetchTask`：XADD 至 `stream:og-fetch`
   - 驗收：整合測試

5. **OG Fetcher（`internal/gateway/ogfetch/`）**
   - `Fetch(ctx, url) (*entity.OGMetadata, error)`：HTTP GET + HTML parse OG tags
   - 解析 `og:title`、`og:description`、`og:image`、`og:site_name`
   - 驗收：整合測試（`httptest.NewServer` mock 目標網頁）

### 驗收標準

- `go test ./internal/repository/... ./internal/gateway/...` 整合測試通過

---

## Phase 5：服務層（Service）

實作業務邏輯，依賴 mock interface 做單元測試。

> **前置條件**：Phase 2（snowflake、base58 已實作）、Phase 3（domain interface 已定義）、Phase 4 不需完成（service 使用 mock）
>
> **參考文件**：
>
> - `260308-short-url-code-architecture-design.md` → 「Service 實作設計」章節（`URLService`、`RedirectService`、`OGWorkerService`、`ClickWorkerService` 的完整流程與 struct 定義）
> - `260308-short-url-system-design-plan.md` → 「重新導向邏輯流程」章節、「非同步 OG 抓取流程」章節
> - `260308-short-url-api-spec-design.md` → 「請求驗證規範」章節（了解 service 需處理的輸入）

### 任務

1. **URLService（`internal/service/url/`）**
   - package 名稱：`urlsvc`（避免與 `net/url` 衝突）
   - `Create` 流程：`idGen.Generate()` → `base58.Encode()` → `repo.Create()`（unique violation retry 最多 3 次）→ `publisher.PublishOGFetchTask()` → 回傳
   - 驗收：unit test 覆蓋 happy path 與 retry 邏輯

2. **RedirectService（`internal/service/redirect/`）**
   - `Resolve` 流程：cache get → miss 時 DB 查詢 + cache set → `IsExpired()` 檢查
   - `RecordClick` 流程：`publisher.PublishClickEvent()`，失敗只 log 不阻塞
   - 驗收：unit test 覆蓋 cache hit、cache miss、過期、不存在四種路徑

3. **OGWorkerService（`internal/service/ogworker/`）**
   - `ProcessTask` 流程：`fetcher.Fetch()` retry 3 次（指數退避 1s/2s/4s）→ 成功 `UpdateOGMetadata` / 失敗標記 `FetchFailed`
   - 驗收：unit test 覆蓋成功、部分失敗、全部失敗路徑

4. **ClickWorkerService（`internal/service/clickworker/`）**
   - `ProcessBatch`：`repo.BatchCreate(logs)`
   - 驗收：unit test

### 驗收標準

- `go test ./internal/service/...` 全部通過
- mock 由 `testify/mock` 或手寫 stub 實作

---

## Phase 6：驅動層 — HTTP Handler

接上 service 介面，實作 HTTP 端點。

> **前置條件**：Phase 3（domain interface 已定義）、Phase 5（service 已實作，handler 依賴 service interface）
>
> **參考文件**：
>
> - `260308-short-url-api-spec-design.md` → 「外部 API 接口」章節（端點規範、Request/Response 格式）、「請求驗證規範」章節、「錯誤回應規範」章節
> - `260308-short-url-code-architecture-design.md` → 「組裝層設計（cmd/api/main.go）」章節（handler 如何被注入與掛載）
> - `260308-short-url-system-design-plan.md` → 「Bot 感知回應」章節、「重新導向邏輯流程」章節

### 任務

1. **URL Handler（`internal/handler/url_handler.go`）**
   - `POST /v1/urls`：解析 body → 驗證 `long_url`（scheme 只接受 http/https、長度上限 2048 字元、可通過 `net/url.ParseRequestURI` 驗證）→ 呼叫 `URLService.Create` → 回傳 201
   - Response 不得包含 Snowflake ID（只回傳 `short_url`、`long_url`、`creator_id`、`created_at`）
   - 錯誤映射：validation error → 400 `INVALID_ARGUMENT`、service error → 500 `INTERNAL_ERROR`
   - 驗收：unit test（echo httptest），覆蓋 201、400、500

2. **Redirect Handler（`internal/handler/redirect_handler.go`）**
   - `GET /:shortCode`：呼叫 `botdetect.IsBot()` → bot 回 OG HTML / 一般回 302
   - 組裝 `ClickLog`（含 `request_id`、IP、UA、Referrer、`ref` 參數、`IsBot` 標記）
   - 呼叫 `RedirectService.Resolve()` + `RecordClick()`
   - 錯誤映射：not found → 404 `NOT_FOUND`、expired → 410 `URL_EXPIRED`
   - 驗收：unit test，覆蓋 302、200（bot）、404、410

3. **Health Handler（`internal/handler/health_handler.go`）**
   - `GET /healthz`：永遠回傳 `{"status": "ok"}` 200，不做 DB/Redis 健康探測
   - 驗收：unit test

4. **Route 註冊與 Server 啟動（`cmd/api/main.go`）**
   - 組裝所有元件（infra → repo → service → handler），依照 code architecture 文件「組裝層設計」章節的程式碼骨架實作
   - 掛載 `logger.Middleware()`（位於 `internal/pkg/logger/middleware.go`，已實作）
   - 設定 Graceful Shutdown（SIGINT/SIGTERM → 10s timeout）

### 驗收標準

- `make run-api` 啟動後：
  - `curl -X POST /v1/urls` 回傳 201 與短網址
  - `curl /{shortCode}` 回傳 302 並導向正確目標

---

## Phase 7：驅動層 — Redis Stream Consumer

實作兩個 Worker Consumer，完成異步處理流程。

> **前置條件**：Phase 3（domain interface 已定義）、Phase 5（service 已實作）、Phase 4（Repository 整合測試完成，確認 Redis Stream 行為）
>
> **參考文件**：
>
> - `260308-short-url-code-architecture-design.md` → 「Consumer 設計」章節（`OGConsumer`、`ClickConsumer` struct 定義、錯誤處理流程圖）、「組裝層設計（cmd/worker/main.go）」章節
> - `260308-short-url-system-design-plan.md` → 「注意事項」中的 Worker Graceful Shutdown、點擊事件 Dead Letter 說明
> - `260308-short-url-api-spec-design.md` → 「內部 Event Payload」章節（Stream 欄位格式）

### 任務

1. **OG Consumer（`internal/consumer/og_consumer.go`）**
   - `XREADGROUP` 消費 `stream:og-fetch`，consumer group 名稱由 config 決定
   - 呼叫 `OGWorkerService.ProcessTask()`
   - 成功或失敗（FetchFailed 已標記）均 `XACK`（OG 抓取失敗為非致命錯誤，不卡隊列）
   - 驗收：整合測試（miniredis 或 testcontainers），確認 ACK 行為正確

2. **Click Consumer（`internal/consumer/click_consumer.go`）**
   - `XREADGROUP` 批次消費 `stream:click-log`（batch size 由 config 決定）
   - 成功 → 整批 `XACK`；失敗 → 不 ACK，留在 PEL
   - 定時 `XCLAIM`（idle > config 設定時間，預設 30s）重新認領
   - delivery count > `maxDelivery`（預設 5）→ 移至 `stream:click-dlq` + `XACK` 原訊息 + slog.Error 記錄
   - 驗收：整合測試

3. **Worker 組裝（`cmd/worker/main.go`）**
   - 依照 code architecture 文件「組裝層設計（cmd/worker/main.go）」章節實作
   - 組裝 DB/Redis → repo → service → consumer
   - 使用 `errgroup` 同時啟動兩個 consumer
   - Graceful Shutdown：signal → cancel ctx → 等待當前 batch 完成後退出

### 驗收標準

- `make run-worker` 啟動後：
  - 建立短網址後，`stream:og-fetch` 中的訊息被消費，`og_metadata` 更新至 DB
  - 點擊後，`stream:click-log` 中的訊息被消費，`click_log` table 有對應記錄

---

## Phase 8：端對端驗收（E2E）

確認完整流程串連正確。

> **前置條件**：Phase 6、Phase 7 全部完成
>
> **參考文件**：
>
> - `260308-short-url-api-spec-design.md` → 「外部 API 接口」章節（確認回應格式正確）
> - `260308-short-url-system-design-plan.md` → 「監控與可觀測性」章節（驗收時觀察的指標）

### 手動驗收腳本

```bash
# 1. 啟動基礎設施
docker compose up -d

# 2. 執行 migration
make migrate-up

# 3. 啟動 API Server
make run-api

# 4. 啟動 Worker
make run-worker

# 5. 建立短網址
curl -X POST http://localhost:8080/v1/urls \
  -H "Content-Type: application/json" \
  -d '{"long_url": "https://example.com/path", "creator_id": "user_01"}'
# 預期：201，回傳 short_url（格式：http://localhost:8080/{10碼短碼}）

# 6. 跳轉驗收
curl -v http://localhost:8080/{shortCode}
# 預期：HTTP 302，Location: https://example.com/path

# 7. Bot 請求驗收
curl -v -H "User-Agent: facebookexternalhit/1.1" http://localhost:8080/{shortCode}
# 預期：HTTP 200，HTML 包含 og:title

# 8. 點擊記錄驗收（等待 Worker 消費後）
psql -c "SELECT * FROM click_log LIMIT 1;"
# 預期：有對應記錄

# 9. OG metadata 驗收（等待 Worker 消費後）
psql -c "SELECT og_metadata FROM short_url WHERE short_code = '{shortCode}';"
# 預期：og_metadata 非 null
```

### 驗收標準

- 完整流程無 error log
- `stream:og-fetch` 與 `stream:click-log` pending 數最終歸零

---

## 依賴關係圖

```
Phase 0（基礎建設）
    └── Phase 1（Config & Infra）
            ├── Phase 2（工具層）   ← 可與 Phase 1 平行
            └── Phase 3（Domain）  ← 可與 Phase 1, 2 平行
                    └── Phase 4（Repository & Gateway）
                            └── Phase 5（Service）
                                    ├── Phase 6（HTTP Handler）
                                    └── Phase 7（Consumer）
                                            └── Phase 8（E2E）
```

> Phase 2 與 Phase 3 均無外部依賴，可在 Phase 1 進行中平行開發。

---

## 注意事項

- **不提交 `.env`**：`.env.example` 已提交，`.gitignore` 已設定排除 `.env`，勿手動提交。
- **Migration 不可逆操作需謹慎**：`down.sql` 務必在 `up.sql` 完成後立即撰寫，避免遺漏。
- **整合測試需 Docker**：Phase 4、7 的整合測試依賴 testcontainers-go 或本地 Docker，CI 環境需確認 Docker-in-Docker 可用。
- **Snowflake Epoch 固定**：`2026-01-01T00:00:00Z`（Unix ms: `1767225600000`），硬編碼不可配置。
- **Dead Letter Stream**：`stream:click-dlq` 名稱硬編碼，POC 不需配置化。
- **Logger Middleware 位置**：`internal/pkg/logger/middleware.go`（已實作），POC 不另建 `internal/middleware/` 目錄。code architecture 文件提到的 `internal/middleware/request_logger.go` 為設計草稿，實際已整合至 `internal/pkg/logger/` package，以現有實作為準。
- **`url` package 命名**：`internal/service/url/` 的 package 名稱使用 `urlsvc`，避免與 stdlib `net/url` 在同一檔案 import 時強制 alias。
- **RecordClick Fail-safe**：點擊事件發送至 Redis Stream 失敗時，僅記錄 error log，不阻塞重導向回應。

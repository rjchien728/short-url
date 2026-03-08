# 260308-short-url-implementation-plan

## 目標

依照已完成的 system design、code architecture、API spec 與 config design，規劃 Short URL POC 的完整實作順序。以「可在本地端跑起來的可工作系統」為每個 Phase 的交付標準。

## 實作原則

- **由下而上**：先建基礎設施與 domain 合約，再疊服務層，最後接驅動層。
- **每個 Phase 可獨立驗收**：完成後能執行對應的測試或手動驗證。
- **不跳層**：不在 service 尚未完成時寫 handler，避免邊界不清。

---

## Phase 0：專案基礎建設

建立可執行的空殼專案，確認所有工具鏈正常運作。

### 任務

1. **初始化 Go module**
   - `go mod init github.com/<user>/short-url`
   - 建立 `cmd/api/main.go`、`cmd/worker/main.go`（空的 `main` 函數）

2. **建立目錄結構骨架**
   - 依照 code architecture 文件建立所有目錄（空目錄用 `.gitkeep` 佔位）

3. **Docker Compose**
   - 定義 `docker-compose.yml`，包含：
     - PostgreSQL 17（port 5432）
     - Redis 7（port 6379）
   - 驗收：`docker compose up -d` 後，`psql` 與 `redis-cli ping` 均可連線

4. **Makefile**
   - 定義常用指令：`make run-api`、`make run-worker`、`make test`、`make migrate-up`、`make lint`

5. **安裝依賴套件**
   - `github.com/labstack/echo/v4`
   - `github.com/jackc/pgx/v5`
   - `github.com/Masterminds/squirrel`
   - `github.com/redis/go-redis/v9`
   - `github.com/spf13/viper`
   - `github.com/google/uuid`
   - `github.com/golang-migrate/migrate/v4`
   - `golang.org/x/sync`（errgroup）

### 驗收標準

- `go build ./...` 無錯誤
- `docker compose up -d` 服務正常啟動

---

## Phase 1：Config 與基礎設施層

建立 config 載入與 DB/Redis 連線元件，所有後續層均依賴此 Phase。

### 任務

1. **Config（`internal/infra/config.go`）**
   - 定義 Config struct（App、Server、Database、Redis、Worker）
   - 使用 viper 實作載入邏輯：`.env` → 環境變數
   - 依照 config design 文件設定所有預設值與環境變數映射
   - 建立 `.env.example`（含所有變數，值為範例，提交至 git）
   - 在 `.gitignore` 加入 `.env`

2. **PostgreSQL 連線（`internal/infra/postgres.go`）**
   - 實作 `NewPool(cfg DatabaseConfig) (*pgxpool.Pool, error)`
   - 套用 `MaxOpenConns`、`MaxIdleConns` 設定

3. **Redis 連線（`internal/infra/redis.go`）**
   - 實作 `NewClient(cfg RedisConfig) (*redis.Client, error)`
   - DB 號碼由 `RedisConfig.DB` 決定（Cache=0, Stream=1）

4. **Database Migration（`migrations/`）**
   - 建立 `000001_create_short_url.up.sql` / `.down.sql`
   - 建立 `000002_create_click_log.up.sql` / `.down.sql`
   - Schema 依照 system design 文件定義
   - 驗收：`make migrate-up` 成功建立兩張 table

### 驗收標準

- `config.Load()` 可正確讀取 `.env` 與環境變數
- `infra.NewPool()` 可連線至本地 PostgreSQL
- `infra.NewClient()` 可連線至本地 Redis
- migration 跑完後，`\d short_url` 與 `\d click_log` 結構正確

---

## Phase 2：工具層（internal/pkg）

實作零或極少外部依賴的共用工具，可獨立開發與測試。

### 任務

1. **Logger（`internal/pkg/logger/`）**
   - 遷移現有 `pkg/logger/logger.go` 至 `internal/pkg/logger/`
   - 將 `middleware.go` 從 gin 改為 echo middleware 格式（`echo.MiddlewareFunc`）
   - 驗收：unit test 確認 `WithLogger` / `FromContext` / `Setup` 行為正確

2. **Snowflake ID 生成器（`internal/pkg/snowflake/`）**
   - 定義 `IDGenerator` interface
   - 實作 `Generator`（41-bit 時間戳 + 12-bit 序號，Epoch: `2026-01-01T00:00:00Z`）
   - 處理同毫秒序號溢出（spin-wait）
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

### 任務

1. **Entity（`internal/domain/entity/`）**
   - `shorturl.go`：`ShortURL`、`OGMetadata`、`OGFetchTask`、`IsExpired()` 方法
   - `clicklog.go`：`ClickLog`

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
   - `PublishClickEvent`：XADD 至 `stream:click-log`
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

### 任務

1. **URLService（`internal/service/url/`）**
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

### 任務

1. **URL Handler（`internal/handler/url_handler.go`）**
   - `POST /v1/urls`：解析 body → 驗證 `long_url`（scheme、長度）→ 呼叫 `URLService.Create` → 回傳 201
   - 錯誤映射：validation error → 400、service error → 500
   - 驗收：unit test（echo httptest），覆蓋 201、400、500

2. **Redirect Handler（`internal/handler/redirect_handler.go`）**
   - `GET /:shortCode`：呼叫 `botdetect.IsBot()` → bot 回 OG HTML / 一般回 302
   - 組裝 `ClickLog`（含 `request_id`、IP、UA、Referrer、`ref` 參數）
   - 呼叫 `RedirectService.Resolve()` + `RecordClick()`
   - 錯誤映射：not found → 404、expired → 410
   - 驗收：unit test，覆蓋 302、200（bot）、404、410

3. **Health Handler（`internal/handler/health_handler.go`）**
   - `GET /healthz`：永遠回傳 `{"status": "ok"}` 200
   - 驗收：unit test

4. **Route 註冊（`cmd/api/main.go`）**
   - 組裝所有元件（infra → repo → service → handler）
   - 掛載 logger middleware
   - 設定 Graceful Shutdown（SIGINT/SIGTERM → 10s timeout）

### 驗收標準

- `make run-api` 啟動後：
  - `curl -X POST /v1/urls` 回傳 201 與短網址
  - `curl /{shortCode}` 回傳 302 並導向正確目標

---

## Phase 7：驅動層 — Redis Stream Consumer

實作兩個 Worker Consumer，完成異步處理流程。

### 任務

1. **OG Consumer（`internal/consumer/og_consumer.go`）**
   - `XREADGROUP` 消費 `stream:og-fetch`
   - 呼叫 `OGWorkerService.ProcessTask()`
   - 成功或失敗（FetchFailed 已標記）均 `XACK`
   - 驗收：整合測試（miniredis），確認 ACK 行為正確

2. **Click Consumer（`internal/consumer/click_consumer.go`）**
   - `XREADGROUP` 批次消費 `stream:click-log`（batch size 由 config 決定）
   - 成功 → 整批 `XACK`；失敗 → 不 ACK，留在 PEL
   - 定時 `XCLAIM`（idle > config 設定時間）重新認領
   - delivery count > `maxDelivery`（5）→ 移至 `stream:click-dlq` + `XACK` 原訊息
   - 驗收：整合測試

3. **Worker 組裝（`cmd/worker/main.go`）**
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
# 預期：201，回傳 short_url

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

## 注意事項

- **不提交 `.env.local`**：僅提交 `.env.local.example`，`.gitignore` 需在 Phase 0 完成時設定。
- **Migration 不可逆操作需謹慎**：`down.sql` 務必在 `up.sql` 完成後立即撰寫，避免遺漏。
- **整合測試需 Docker**：Phase 4、7 的整合測試依賴 testcontainers-go 或本地 Docker，CI 環境需確認 Docker-in-Docker 可用。
- **Snowflake Epoch 固定**：`2026-01-01T00:00:00Z`（Unix ms: 1767225600000），硬編碼不可配置。
- **Dead Letter Stream**：`stream:click-dlq` 名稱硬編碼，POC 不需配置化。

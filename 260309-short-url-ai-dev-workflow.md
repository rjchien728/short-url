# 260309-short-url-ai-dev-workflow

## 目的

本文件定義 AI agent 執行 Short URL 各 Phase 實作時的**標準作業程序（SOP）**，補充 `260308-short-url-implementation-plan.md` 未涵蓋的操作細節，使 AI 能夠全自動、無歧義地完成每個 Phase。

---

## 一、AI 全自動實作的差距分析

現有計劃文件已相當完整，但以下幾個方面仍缺乏讓 AI **無需人工介入**即可執行的明確指引：

### 1. 缺乏「Phase 啟動前的環境確認清單」

**問題**：AI 不知道在開始每個 Phase 前應確認哪些先決條件已就緒。
**缺少的內容**：
- 應執行哪些指令來驗證前置條件（例如：Phase 4 開始前如何確認 DB 已在線、interface 已定義）
- `go build ./...` 是否為每個 Phase 完成後的必要步驟
- Docker 服務何時需要啟動

### 2. 缺乏明確的「檔案對應與 package 宣告規則」

**問題**：`internal/service/url/` 的 package 名稱為 `urlsvc`（非目錄名），這種異常命名若未在每個 Phase 前說明，AI 可能會自動使用目錄名。
**缺少的內容**：
- 完整的 package 名稱對應表
- import path 與 package name 不同的情況

### 3. 整合測試的基礎設施細節不足

**問題**：Phase 4 要求整合測試，但未說明 testcontainers-go 的具體使用模式（TestMain 還是 setup/teardown per test），AI 可能寫出不一致的測試。
**缺少的內容**：
- 整合測試的標準結構（TestMain + container setup）
- test build tag 策略（是否需要 `//go:build integration`）
- miniredis vs testcontainers 的選擇規則

### 4. 缺乏「錯誤型別定義」規範

**問題**：Handler 需要將 service error 映射至 HTTP 錯誤碼（`INVALID_ARGUMENT`、`NOT_FOUND`、`URL_EXPIRED`），但 domain layer 中沒有標準錯誤型別定義，AI 可能各自發明不同的錯誤處理方式。
**缺少的內容**：
- domain 層是否應定義 sentinel error（`ErrNotFound`、`ErrExpired`）
- handler 如何 type-switch 或比對 error

### 5. 缺乏「Redis Stream Consumer Group 初始化」流程

**問題**：Redis Stream 的 Consumer Group 必須在第一次消費前建立（`XGROUP CREATE ... MKSTREAM`），計劃文件未說明此步驟由誰負責。
**缺少的內容**：
- Consumer Group 初始化應在哪裡執行（consumer 啟動時？worker main 組裝時？）
- `MKSTREAM` 參數的使用時機

### 6. 缺乏「Dependency 版本鎖定」規範

**問題**：Phase 0 要求安裝多個套件，但未指定版本號，AI 可能安裝到 breaking change 版本。
**缺少的內容**：
- 各套件的明確版本號
- `go mod tidy` 的執行時機

### 7. 缺乏「commit 粒度與 Phase 交付物」定義

**問題**：AI 不知道每個 Phase 完成後應產出什麼（git commit、test report），可能在 Phase 中途停下或過度實作。
**缺少的內容**：
- 每個 Phase 的最小可交付清單（檔案列表）
- AI 何時應停下等待人工確認

---

## 二、AI 開發 SOP

### 2.1 全域原則

1. **閱讀優先**：每個 Phase 開始前，必須先閱讀對應的設計文件章節，不可憑假設實作。
2. **由下而上**：嚴格遵守 Phase 依賴順序，不跳層。
3. **驗收即停**：每個 Phase 完成驗收標準後停止，不預先實作下一個 Phase 的內容。
4. **不覆蓋現有檔案**：「現有產出」表中的檔案不可重新建立或覆蓋，除非 Phase 任務明確要求修改。
5. **測試同步**：每個有測試要求的 Phase，測試必須與實作同步完成，不留待後期補寫。

---

### 2.2 Package 名稱對應表

> AI 在建立每個 package 時，`package` 宣告必須嚴格依此表。

| 目錄路徑 | package 名稱 | 原因 |
| :--- | :--- | :--- |
| `internal/service/url/` | `urlsvc` | 避免與 `net/url` 衝突 |
| `internal/service/redirect/` | `redirect` | 無衝突 |
| `internal/service/ogworker/` | `ogworker` | 無衝突 |
| `internal/service/clickworker/` | `clickworker` | 無衝突 |
| `internal/repository/shorturl/` | `shorturl` | 無衝突 |
| `internal/repository/clicklog/` | `clicklog` | 無衝突 |
| `internal/repository/urlcache/` | `urlcache` | 無衝突 |
| `internal/repository/eventpub/` | `eventpub` | 無衝突 |
| `internal/gateway/ogfetch/` | `ogfetch` | 無衝突 |
| `internal/consumer/` | `consumer` | 無衝突 |
| `internal/handler/` | `handler` | 無衝突 |
| `internal/infra/` | `infra` | 無衝突 |
| `internal/pkg/snowflake/` | `snowflake` | 無衝突 |
| `internal/pkg/base58/` | `base58` | 無衝突 |
| `internal/pkg/botdetect/` | `botdetect` | 無衝突 |
| `internal/domain/entity/` | `entity` | 無衝突 |
| `internal/domain/service/` | `service` | 無衝突 |
| `internal/domain/repository/` | `repository` | 無衝突 |
| `internal/domain/gateway/` | `gateway` | 無衝突 |

---

### 2.3 套件版本鎖定

Phase 0 安裝依賴時，必須使用以下版本（`go get` 時指定）：

| 套件 | 版本 |
| :--- | :--- |
| `github.com/jackc/pgx/v5` | `v5.7.5` |
| `github.com/Masterminds/squirrel` | `v1.5.4` |
| `github.com/redis/go-redis/v9` | `v9.10.0` |
| `github.com/spf13/viper` | `v1.20.1` |
| `github.com/testcontainers/testcontainers-go` | `v0.38.0` |
| `github.com/stretchr/testify` | `v1.10.0` |
| `golang.org/x/sync` | `v0.15.0` |

已存在（不需要重複安裝）：
- `github.com/labstack/echo/v4`
- `github.com/google/uuid`
- `github.com/golang-migrate/migrate/v4`
- `github.com/joho/godotenv`

---

### 2.4 Domain 錯誤型別規範

Phase 3 建立 Domain 層時，必須在 `internal/domain/entity/` 中定義 sentinel errors。

**`internal/domain/entity/errors.go`**（Phase 3 新增）：

```go
package entity

import "errors"

var (
    ErrNotFound = errors.New("short url not found")
    ErrExpired  = errors.New("short url expired")
)
```

**Handler 映射規則**（Phase 6 使用）：

```go
import "errors"

switch {
case errors.Is(err, entity.ErrNotFound):
    // → 404 NOT_FOUND
case errors.Is(err, entity.ErrExpired):
    // → 410 URL_EXPIRED
default:
    // → 500 INTERNAL_ERROR
}
```

**Repository 回傳規則**（Phase 4 實作時）：
- `FindByShortCode` 查無記錄時，必須回傳 `entity.ErrNotFound`（不可回傳 `pgx.ErrNoRows` 或 `nil`）

---

### 2.5 整合測試標準結構

Phase 4 的整合測試採用以下規範：

**Build Tag 策略**：整合測試**不使用** build tag，但檔名以 `_integration_test.go` 結尾，並使用 `TestMain` 管理容器生命週期。

**標準結構（以 shorturl repository 為例）**：

```go
// internal/repository/shorturl/impl_integration_test.go
package shorturl_test

import (
    "context"
    "testing"

    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/suite"
)

type ShortURLRepoSuite struct {
    suite.Suite
    container *postgres.PostgresContainer
    repo      *Repository
}

func (s *ShortURLRepoSuite) SetupSuite() {
    // 啟動 PostgreSQL container
    // 執行 migration
    // 初始化 repo
}

func (s *ShortURLRepoSuite) TearDownSuite() {
    s.container.Terminate(context.Background())
}

func TestShortURLRepo(t *testing.T) {
    suite.Run(t, new(ShortURLRepoSuite))
}
```

**Redis 整合測試**：使用 `github.com/alicebob/miniredis/v2`（不需 testcontainers-go），可在 CI 無 Docker 環境中執行。

> 注意：`miniredis` 需在 Phase 0 的依賴安裝中加入：`github.com/alicebob/miniredis/v2 v2.34.0`

---

### 2.6 Redis Stream Consumer Group 初始化規範

**由誰負責**：Consumer Group 的建立由各 Consumer 的 `New` 建構子負責，在 `Run()` 前執行。

**OGConsumer 初始化流程**：

```go
const (
    ogStream    = "stream:og-fetch"
    clickStream = "stream:click-log"
    clickDLQ    = "stream:click-dlq"
)

func NewOGConsumer(rdb *redis.Client, svc service.OGWorkerService, cfg ConsumerConfig) *OGConsumer {
    // 建立 Consumer Group（忽略 BUSYGROUP 錯誤表示已存在）
    err := rdb.XGroupCreateMkStream(context.Background(), ogStream, cfg.GroupName, "0").Err()
    if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
        panic(fmt.Sprintf("failed to create consumer group: %v", err))
    }
    return &OGConsumer{...}
}
```

**Click Consumer Group 同理**，對 `stream:click-log` 執行相同初始化。

---

### 2.7 每個 Phase 的執行流程

#### Phase 啟動流程（每個 Phase 開始前必須執行）

```
1. 確認前置 Phase 的驗收標準已達成：
   - 執行 `go build ./...` 確認無編譯錯誤
   - 確認對應 Phase 的 go test 通過

2. 閱讀本 Phase 對應的設計文件章節（依 implementation plan 中的「參考文件」）

3. 確認 Docker 服務狀態（Phase 1、4、7 需要）：
   - `docker compose ps` 確認 postgres、redis 均為 running
   - 若未啟動：`docker compose up -d`

4. 開始實作
```

#### Phase 完成流程（每個 Phase 完成後必須執行）

```
1. 執行 `go build ./...`，確認無編譯錯誤

2. 執行對應測試：
   - 工具層：`go test ./internal/pkg/...`
   - Domain：`go build ./internal/domain/...`
   - Repository/Gateway：`go test ./internal/repository/... ./internal/gateway/...`
   - Service：`go test ./internal/service/...`
   - Handler：`go test ./internal/handler/...`

3. 執行 `go mod tidy` 整理依賴

4. 確認驗收標準全部達成後停止
```

---

## 三、各 Phase 補充細節

### Phase 0：專案基礎建設

**補充：目錄骨架建立規則**

- 空目錄使用 `.gitkeep` 佔位，但**不在已有 Go 檔案的目錄中放 `.gitkeep`**
- `internal/middleware/` 目錄**不需建立**（`logger.Middleware()` 已在 `internal/pkg/logger/middleware.go`，不另設 middleware 目錄）

**補充：`cmd/api/main.go` 最小骨架**

```go
package main

func main() {
    // Phase 6 實作
}
```

**補充：`cmd/worker/main.go` 最小骨架**

```go
package main

func main() {
    // Phase 7 實作
}
```

**補充：依賴安裝指令（按順序執行）**

```bash
go get github.com/jackc/pgx/v5@v5.7.5
go get github.com/Masterminds/squirrel@v1.5.4
go get github.com/redis/go-redis/v9@v9.10.0
go get github.com/spf13/viper@v1.20.1
go get github.com/golang-migrate/migrate/v4@v4.19.1
go get golang.org/x/sync@v0.15.0
go get github.com/testcontainers/testcontainers-go@v0.38.0
go get github.com/stretchr/testify@v1.10.0
go get github.com/alicebob/miniredis/v2@v2.34.0
go mod tidy
```

---

### Phase 1：Config 與基礎設施層

**補充：Config struct 完整定義**

依照 `260308-short-url-config-design.md` 的環境變數映射表：

```go
// internal/infra/config.go
package infra

type Config struct {
    App      AppConfig
    Server   ServerConfig
    Database DatabaseConfig
    Cache    RedisConfig  // Redis DB 0
    Stream   RedisConfig  // Redis DB 1
}

type AppConfig struct {
    Env string // APP_ENV, 預設 "development"
}

type ServerConfig struct {
    Port string // SERVER_PORT, 預設 "8080"
}

type DatabaseConfig struct {
    URL          string // DATABASE_URL
    MaxOpenConns int    // DATABASE_MAX_OPEN_CONNS, 預設 10
    MaxIdleConns int    // DATABASE_MAX_IDLE_CONNS, 預設 5
}

type RedisConfig struct {
    URL string // REDIS_CACHE_URL 或 REDIS_STREAM_URL
}
```

**補充：`infra.NewPool()` 中 `MaxConns` 的設定方式**

pgx v5 的 pool config 使用 `pgxpool.Config`：

```go
poolCfg, _ := pgxpool.ParseConfig(cfg.URL)
poolCfg.MaxConns = int32(cfg.MaxOpenConns)
pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
```

> 注意：pgx v5 沒有 `MaxIdleConns`，只有 `MinConns`。`MaxIdleConns` 設定可忽略或對應至 `MinConns`。

---

### Phase 3：Domain 合約層

**補充：errors.go 必須同步建立**（見 2.4 節）

Phase 3 的任務清單中需新增：
- `internal/domain/entity/errors.go`：定義 `ErrNotFound`、`ErrExpired`

---

### Phase 4：被驅動層

**補充：`squirrel` 的 Placeholder 格式**

pgx 使用 `$1, $2, ...` 佔位符（非 `?`），squirrel 需設定：

```go
sq := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
```

**補充：`FindByShortCode` 的 `og_metadata` 處理**

`og_metadata` 在 DB 中為 JSONB，掃描時使用 `pgtype.JSONB` 或直接 `[]byte`：

```go
var ogJSON []byte
// 掃描後
if ogJSON != nil {
    var og entity.OGMetadata
    json.Unmarshal(ogJSON, &og)
    url.OGMetadata = &og
}
```

**補充：`URLCache` 的序列化方式**

Redis value 使用 JSON 序列化 `entity.ShortURL`，固定 TTL 24 小時：

```go
const cacheTTL = 24 * time.Hour

// Set
data, _ := json.Marshal(url)
rdb.Set(ctx, "shorturl:"+shortCode, data, cacheTTL)

// Get
data, err := rdb.Get(ctx, "shorturl:"+shortCode).Bytes()
if errors.Is(err, redis.Nil) {
    return nil, nil  // cache miss，回傳 nil, nil（非 error）
}
```

**補充：`EventPublisher` 的 Stream 欄位格式**

依照 `260308-short-url-api-spec-design.md` 的 Event Payload 定義：

`stream:og-fetch` 的 `XAdd` fields：
```go
map[string]any{
    "short_url_id": strconv.FormatInt(task.ShortURLID, 10),
    "long_url":     task.LongURL,
}
```

`stream:click-log` 的 `XAdd` fields（10 個欄位，參照 API spec）：
```go
map[string]any{
    "id":          log.ID,
    "short_url_id": strconv.FormatInt(log.ShortURLID, 10),
    "short_code":  log.ShortCode,
    "creator_id":  log.CreatorID,
    "referral_id": log.ReferralID,
    "referrer":    log.Referrer,
    "user_agent":  log.UserAgent,
    "ip_address":  log.IPAddress,
    "is_bot":      strconv.FormatBool(log.IsBot),
    "created_at":  log.CreatedAt.UTC().Format(time.RFC3339),
}
```

---

### Phase 5：服務層

**補充：URLService Create 的 unique violation retry 判斷**

```go
import "github.com/jackc/pgx/v5/pgconn"

// 判斷是否為 unique constraint violation
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    // retry
}
```

**補充：OGWorkerService 的指數退避實作**

```go
backoff := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
for i, d := range backoff {
    metadata, err = s.fetcher.Fetch(ctx, task.LongURL)
    if err == nil {
        break
    }
    if i < len(backoff)-1 {
        time.Sleep(d)
    }
}
```

---

### Phase 6：HTTP Handler

**補充：OG HTML 模板**

Redirect Handler 偵測到 Bot 時，回傳包含 OG tags 的 HTML：

```go
const ogHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
<meta property="og:title" content="%s" />
<meta property="og:description" content="%s" />
<meta property="og:image" content="%s" />
<meta property="og:site_name" content="%s" />
<meta http-equiv="refresh" content="0; url=%s" />
</head>
<body></body>
</html>`
```

`og:title` 等欄位若 `OGMetadata` 為 nil 或 `FetchFailed`，則使用 `LongURL` 作為 fallback。

**補充：IP 取得方式**

```go
ipAddress := c.RealIP() // echo 內建，會處理 X-Forwarded-For
```

**補充：`ref` 參數取得方式**

```go
referralID := c.QueryParam("ref") // GET /:shortCode?ref=xxx
```

---

### Phase 7：Redis Stream Consumer

**補充：XREADGROUP 的 count 與 block 設定**

```go
// OG Consumer：每次讀 1 筆，block 5 秒
rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    groupName,
    Consumer: consumerName,
    Streams:  []string{ogStream, ">"},
    Count:    1,
    Block:    5 * time.Second,
})

// Click Consumer：每次讀 batch size 筆，block 5 秒
rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    groupName,
    Consumer: consumerName,
    Streams:  []string{clickStream, ">"},
    Count:    int64(batchSize),
    Block:    5 * time.Second,
})
```

**補充：Click Consumer 的 XCLAIM 排程**

XCLAIM 重認領應在獨立 goroutine 中以固定間隔執行（建議 10 秒），而非在每次 XREADGROUP 後執行。

**補充：delivery count 取得方式**

`XAutoClaim` 或 `XPending` 的結果中包含 delivery count：

```go
// 使用 XPENDING 查詢後再 XCLAIM
pending, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{...}).Result()
for _, p := range pending {
    if p.RetryCount > int64(maxDelivery) {
        // 移至 DLQ
    }
}
```

---

## 四、AI 實作前必讀文件清單

在開始任何 Phase 前，AI 必須確認已閱讀以下文件的指定章節：

| Phase | 必讀章節 |
| :--- | :--- |
| Phase 0 | `260308-short-url-code-architecture-design.md` → 「目錄結構」、`260308-short-url-config-design.md` → 「環境變數映射表」 |
| Phase 1 | `260308-short-url-config-design.md`（全文）、`260308-short-url-code-architecture-design.md` → 「組裝層設計」 |
| Phase 2 | `260308-short-url-system-design-plan.md` → 「簡化版 Snowflake ID 生成器」、`260308-short-url-code-architecture-design.md` → 「工具層」 |
| Phase 3 | `260308-short-url-code-architecture-design.md` → 「介面設計」（**直接照抄，不可推斷**） |
| Phase 4 | `260308-short-url-code-architecture-design.md` → 「domain/repository」介面、`260308-short-url-api-spec-design.md` → 「內部 Event Payload」 |
| Phase 5 | `260308-short-url-code-architecture-design.md` → 「Service 實作設計」（完整流程與 struct 定義） |
| Phase 6 | `260308-short-url-api-spec-design.md` → 「外部 API 接口」、「請求驗證規範」、「錯誤回應規範」 |
| Phase 7 | `260308-short-url-code-architecture-design.md` → 「Consumer 設計」、`260308-short-url-api-spec-design.md` → 「內部 Event Payload」 |
| Phase 8 | `260308-short-url-api-spec-design.md` → 「外部 API 接口」 |

---

## 五、常見陷阱與禁止事項

| 類型 | 禁止行為 | 正確行為 |
| :--- | :--- | :--- |
| 命名 | `package url`（在 `internal/service/url/` 中） | `package urlsvc` |
| 錯誤處理 | 直接回傳 `pgx.ErrNoRows` | 包裝為 `entity.ErrNotFound` |
| Cache miss | 回傳 `redis.Nil` error | 回傳 `nil, nil`（表示 miss，非錯誤） |
| OG 失敗 | 不 XACK，讓訊息重試 | XACK 並標記 `FetchFailed: true` |
| Click 失敗 | 直接丟棄或 XACK | 不 XACK，讓 PEL 重試 |
| ClickLog 欄位 | 包含 `IPCountry` | 不含 `IPCountry`（POC 不實作 GeoIP） |
| Snowflake Epoch | 使用當前時間或其他值 | 固定 `1767225600000`（2026-01-01） |
| Bot HTML | 只回傳 og:title | 包含全部 4 個 og tags + meta refresh |
| 現有檔案 | 覆蓋 `internal/pkg/logger/` 下的任何檔案 | 僅補齊 unit test |
| DLQ 名稱 | 從 config 讀取 | 硬編碼 `stream:click-dlq` |

---

## 六、驗收標準速查表

| Phase | 核心驗收指令 |
| :--- | :--- |
| Phase 0 | `go build ./...` 無錯誤 |
| Phase 1 | `config.Load()` 讀取 `.env` 成功、DB/Redis 連線成功、`make migrate-up` 通過 |
| Phase 2 | `go test ./internal/pkg/...` 全部通過 |
| Phase 3 | `go build ./internal/domain/...` 無錯誤，無外部依賴 |
| Phase 4 | `go test ./internal/repository/... ./internal/gateway/...` 整合測試通過 |
| Phase 5 | `go test ./internal/service/...` 全部通過 |
| Phase 6 | `make run-api` 後 `POST /v1/urls` 回 201、`GET /:shortCode` 回 302 |
| Phase 7 | Worker 消費後 `og_metadata` 更新、`click_log` table 有記錄 |
| Phase 8 | 完整流程無 error log，Stream pending 數歸零 |

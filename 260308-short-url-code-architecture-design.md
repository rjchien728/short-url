# Short URL 程式碼架構設計

## 目標

為 Short URL 系統設計一套可測試性高、分層清晰、複雜度適中的 Go 程式碼架構。系統包含兩個執行入口：API Server 與 Worker（OG 抓取 + 點擊統計）。

## 技術選型

| 元件         | 選擇                       | 理由                               |
| :----------- | :------------------------- | :--------------------------------- |
| HTTP Router  | echo                       | 輕量、middleware 生態成熟          |
| DB Client    | pgx + squirrel             | SQL-first，手動組 query 彈性最大   |
| Redis Client | go-redis/redis v9          | 社群主流，Streams API 支援完整     |
| Config       | viper                      | 支援多格式 config，功能豐富        |
| HTTP Client  | net/http (stdlib)          | OG 抓取需求單純，標準庫足夠        |
| UUID 生成    | github.com/google/uuid     | 生成 UUID v7（時間有序），用於 ClickLog 主鍵；v7 自 v1.6.0 起支援 |
| Logger       | slog (stdlib)              | Go 1.21+ 標準庫，零依賴            |
| DB Migration | golang-migrate             | 獨立 CLI + Go library，支援多種 DB |
| DI 方式      | 手動 Constructor Injection | 清晰透明，適合中型專案             |

## 目錄結構

```
short-url/
├── cmd/
│   ├── api/
│   │   └── main.go                  # API Server 進入點
│   └── worker/
│       └── main.go                  # Worker 進入點（OG 抓取 + 點擊統計）
│
├── internal/
│   ├── domain/                      # 合約中心：介面 + 領域模型（零外部依賴）
│   │   ├── entity/
│   │   │   ├── shorturl.go          # ShortURL, OGMetadata, OGFetchTask
│   │   │   └── clicklog.go          # ClickLog
│   │   ├── service/
│   │   │   ├── url.go               # URLService interface
│   │   │   ├── redirect.go          # RedirectService interface
│   │   │   └── worker.go            # OGWorkerService, ClickWorkerService interfaces
│   │   ├── repository/
│   │   │   └── repository.go        # ShortURLRepository, ClickLogRepository,
│   │   │                            # URLCache, EventPublisher interfaces
│   │   │                            # （廣義：DB 持久化、Cache、Queue 皆屬資料存取層）
│   │   └── gateway/
│   │       └── gateway.go           # OGFetcher interface
│   │
│   ├── service/                     # 服務層實作（每個子系統獨立 package）
│   │   ├── url/                     # package urlsvc（避免與 net/url 衝突）
│   │   │   ├── impl.go              # 實作 domain/service.URLService
│   │   │   └── impl_test.go
│   │   ├── redirect/
│   │   │   ├── impl.go              # 實作 domain/service.RedirectService
│   │   │   └── impl_test.go
│   │   ├── ogworker/
│   │   │   ├── impl.go              # 實作 domain/service.OGWorkerService
│   │   │   └── impl_test.go
│   │   └── clickworker/
│   │       ├── impl.go              # 實作 domain/service.ClickWorkerService
│   │       └── impl_test.go
│   │
│   ├── repository/                  # 資料存取實作
│   │   ├── shorturl/
│   │   │   └── impl.go              # 實作 ShortURLRepository（pgx）
│   │   ├── clicklog/
│   │   │   └── impl.go              # 實作 ClickLogRepository（pgx）
│   │   ├── urlcache/
│   │   │   └── impl.go              # 實作 URLCache（go-redis, DB 0）
│   │   └── eventpub/
│   │       └── impl.go              # 實作 EventPublisher（go-redis Streams, DB 1）
│   │
│   ├── gateway/                     # 外部服務實作
│   │   └── ogfetch/
│   │       └── impl.go              # 實作 OGFetcher（net/http + HTML parse）
│   │
│   ├── consumer/                    # 驅動層：Redis Stream 消費者
│   │   ├── og_consumer.go           # OG 抓取任務消費者
│   │   └── click_consumer.go        # 點擊事件消費者
│   │
│   ├── handler/                     # 驅動層：HTTP Handler
│   │   ├── url_handler.go           # POST /v1/urls
│   │   ├── url_handler_test.go
│   │   ├── redirect_handler.go      # GET /:shortCode
│   │   ├── redirect_handler_test.go
│   │   └── health_handler.go        # GET /healthz
│   │
│   ├── middleware/
│   │   └── request_logger.go        # echo middleware：注入 request_id + slog logger 至 context
│   │                                # 使用 internal/pkg/logger 實作，依賴 echo.Context
│   │
│   ├── infra/                       # 底層連線元件
│   │   ├── postgres.go              # NewPool() → *pgxpool.Pool
│   │   └── redis.go                 # NewClient(cfg RedisConfig) → *redis.Client
│   │                                # DB 號碼由 RedisConfig 結構決定，不 hardcode
│   │
│   └── pkg/                         # 專案內共用工具（不對外暴露）
│       ├── logger/
│       │   ├── logger.go            # Setup()、context-aware slog wrapper（Info/Debug/Error/Warn）
│       │   └── middleware.go        # echo middleware，注入 request_id 至 context
│       ├── snowflake/
│       │   ├── snowflake.go         # 簡化版 Snowflake ID 生成器，實作 IDGenerator 介面
│       │   └── snowflake_test.go
│       ├── base58/
│       │   ├── base58.go            # Base58 編碼/解碼，固定 10 碼
│       │   └── base58_test.go
│       └── botdetect/
│           ├── botdetect.go         # User-Agent bot 偵測
│           └── botdetect_test.go
│
├── migrations/
│   ├── 000001_create_short_url.up.sql
│   ├── 000001_create_short_url.down.sql
│   ├── 000002_create_click_log.up.sql
│   └── 000002_create_click_log.down.sql
│
├── docker-compose.yml
├── Makefile
├── go.mod
└── go.sum
```

## 分層架構

### 分層定義

| 層級       | 目錄                                        | 職責                                                               | 依賴規則                                                                         |
| :--------- | :------------------------------------------ | :----------------------------------------------------------------- | :------------------------------------------------------------------------------- |
| 領域層     | `internal/domain/`                          | 定義 entity（純 struct）與所有介面（service、repository、gateway） | 零外部依賴                                                                       |
| 驅動層     | `internal/handler/`、`internal/consumer/`   | 接收外部輸入（HTTP / Redis Stream），呼叫 service 介面             | 依賴 `domain/service` 介面、`domain/entity`                                      |
| 服務層     | `internal/service/`                         | 業務邏輯實作                                                       | 依賴 `domain/repository`、`domain/gateway` 介面、`domain/entity`、`internal/pkg` |
| 被驅動層   | `internal/repository/`、`internal/gateway/` | 資料存取與外部服務的具體實作                                       | 依賴 `domain/` 介面與 entity、`internal/infra`、外部套件                         |
| 基礎設施層 | `internal/infra/`                           | 底層連線元件（db pool、redis client）                              | 依賴外部套件（pgx、go-redis）                                                    |
| 工具層     | `internal/pkg/`                             | 專案內共用純工具（logger、snowflake、base58、botdetect）；單一 module 應用程式，統一放 `internal/` 語義更精確 | 零或極少外部依賴 |
| 組裝層     | `cmd/`                                      | Composition Root — 建立所有實例、注入依賴、啟動服務                | 依賴所有層（唯一知道具體型別的地方）                                             |

### 依賴流向圖

```
                        cmd/api  cmd/worker
                          │          │
               ┌──────────┘          └──────────┐
               ▼                                 ▼
          handler/                          consumer/
          （HTTP 驅動）                      （Stream 驅動）
               │                                 │
               └──────────┐          ┌───────────┘
                          ▼          ▼
                     domain/service/
                       （服務介面）
                          │ 實作
                          ▼
                    internal/service/
                      （服務實作）
                          │
            ┌─────────────┼─────────────┐
            ▼             ▼             ▼
    domain/repository  domain/gateway  domain/entity
      （介面）           （介面）        （純 struct）
            │             │
            ▼             ▼
  internal/repository  internal/gateway
      （實作）            （實作）
            │             │
            └──────┬──────┘
                   ▼
             internal/infra/
           （底層連線元件）
```

### 依賴反轉（DIP）體現

- `internal/service/` 和 `internal/repository/` 都只依賴 `domain/`（抽象），彼此不直接 import
- `cmd/main.go` 是唯一知道「用 pgx 實作 ShortURLRepository」的地方
- 換 DB（如 MySQL）只需新增 adapter + 改 `cmd/`，`service/` 零修改

## 介面設計

### domain/entity

```go
// entity/shorturl.go
package entity

type ShortURL struct {
    ID         int64
    ShortCode  string
    LongURL    string
    CreatorID  string
    OGMetadata *OGMetadata
    ExpiresAt  *time.Time
    CreatedAt  time.Time
}

func (s *ShortURL) IsExpired() bool {
    if s.ExpiresAt == nil {
        return false
    }
    return time.Now().After(*s.ExpiresAt)
}

type OGMetadata struct {
    Title       string `json:"title"`
    Description string `json:"description"`
    Image       string `json:"image"`
    SiteName    string `json:"site_name"`
    FetchFailed bool   `json:"fetch_failed"` // OG 抓取失敗標記，避免重複觸發
}

// OGFetchTask 為 Worker 任務 DTO，非 domain entity，
// 因規模小暫放於 entity/ 下，未來可移至 domain/task/。
type OGFetchTask struct {
    ShortURLID int64
    LongURL    string
}

// entity/clicklog.go
package entity

type ClickLog struct {
    ID         string // UUID v7（時間有序），由 handler 組裝時以 uuid.NewV7() 產生
    ShortURLID int64
    ShortCode  string
    CreatorID  string
    ReferralID string
    Referrer   string
    UserAgent  string
    IPAddress  string
    IsBot      bool
    CreatedAt  time.Time
}
```

### domain/service

```go
// service/url.go
package service

type URLService interface {
    Create(ctx context.Context, req CreateURLRequest) (*entity.ShortURL, error)
}

type CreateURLRequest struct {
    LongURL   string
    CreatorID string
    ExpiresAt *time.Time
}

// service/redirect.go
type RedirectService interface {
    Resolve(ctx context.Context, shortCode string) (*entity.ShortURL, error)
    RecordClick(ctx context.Context, log *entity.ClickLog) error
}

// service/worker.go
type OGWorkerService interface {
    ProcessTask(ctx context.Context, task *entity.OGFetchTask) error
}

type ClickWorkerService interface {
    ProcessBatch(ctx context.Context, logs []*entity.ClickLog) error
}
```

### domain/repository

```go
// repository/repository.go
package repository

// 廣義資料存取層：DB 持久化、Cache、Queue 皆視為資料存取的一種形式

type ShortURLRepository interface {
    Create(ctx context.Context, url *entity.ShortURL) error
    FindByShortCode(ctx context.Context, shortCode string) (*entity.ShortURL, error)
    UpdateOGMetadata(ctx context.Context, id int64, metadata *entity.OGMetadata) error
}

type ClickLogRepository interface {
    BatchCreate(ctx context.Context, logs []*entity.ClickLog) error
}

type URLCache interface {
    Get(ctx context.Context, shortCode string) (*entity.ShortURL, error)
    // Set 寫入快取；POC 固定 TTL=24h，由實作層決定，不由 caller 傳入
    Set(ctx context.Context, shortCode string, url *entity.ShortURL) error
    // Delete 供未來短網址停用/刪除功能使用，主動清除快取
    Delete(ctx context.Context, shortCode string) error
}

type EventPublisher interface {
    PublishClickEvent(ctx context.Context, event *entity.ClickLog) error
    PublishOGFetchTask(ctx context.Context, task *entity.OGFetchTask) error
}
```

### domain/gateway

```go
// gateway/gateway.go
package gateway

type OGFetcher interface {
    Fetch(ctx context.Context, url string) (*entity.OGMetadata, error)
}
```

## Service 實作設計

### IDGenerator 介面

為提升 `URLService` 的可測試性（尤其是 unique violation retry 邏輯），`snowflake.Generator` 透過介面注入：

```go
// internal/pkg/snowflake/snowflake.go
package snowflake

type IDGenerator interface {
    Generate() (int64, error)
}

type Generator struct { ... }

func (g *Generator) Generate() (int64, error) { ... }
```

### URLService

```go
// internal/service/url/impl.go
package urlsvc  // 避免與 net/url 衝突

type Service struct {
    repo      repository.ShortURLRepository
    cache     repository.URLCache
    publisher repository.EventPublisher
    idGen     snowflake.IDGenerator  // 介面，測試時可 mock 控制 ID 生成
}

func New(
    repo      repository.ShortURLRepository,
    cache     repository.URLCache,
    publisher repository.EventPublisher,
    idGen     snowflake.IDGenerator,
) *Service
```

**流程：**

1. `idGen.Generate()` → `base58.Encode()` → 產生 `short_code`
2. `repo.Create()` — 若 unique violation → retry（最多 3 次，重新生成 ID）
3. `publisher.PublishOGFetchTask()` — 發送非同步 OG 抓取任務
4. 回傳 `*entity.ShortURL`

### RedirectService

```go
// internal/service/redirect/impl.go
package redirect

type Service struct {
    repo      repository.ShortURLRepository
    cache     repository.URLCache
    publisher repository.EventPublisher
}

func New(
    repo      repository.ShortURLRepository,
    cache     repository.URLCache,
    publisher repository.EventPublisher,
) *Service
```

**Resolve 流程：**

1. `cache.Get()` → 若 miss → `repo.FindByShortCode()` → `cache.Set(ttl=24h)`
2. 檢查 `IsExpired()` → 過期回錯誤（由 handler 轉為 HTTP 410）
3. 回傳 `*entity.ShortURL`

> **Cache TTL 策略（POC）**: 固定使用 `24h` TTL，不依賴 `expires_at`。
> POC 階段 `expires_at` 不開放設定，故無需動態對齊過期時間。
> 未來支援 `expires_at` 時，TTL 應改為 `min(24h, expires_at - now())`。

**RecordClick 流程：**

- `publisher.PublishClickEvent()` — 失敗只 log 不阻塞，記錄 error counter

> Bot 偵測（`pkg/botdetect`）與回應決策（回 OG HTML 或 302）由 `handler` 層處理。
> `ClickLog` 的組裝（含 `IsBot`、`IPAddress` 等欄位）由 handler 負責，
> POC 階段接受此做法；未來可重構為 service 接收原始 request 並在內部做 enrichment。

### OGWorkerService

```go
// internal/service/ogworker/impl.go
package ogworker

type Service struct {
    repo    repository.ShortURLRepository
    fetcher gateway.OGFetcher
}

func New(repo repository.ShortURLRepository, fetcher gateway.OGFetcher) *Service
```

**流程：**

1. `fetcher.Fetch(task.LongURL)` — 最多 retry 3 次（指數退避：1s / 2s / 4s）
2. 成功 → `repo.UpdateOGMetadata(task.ShortURLID, metadata)`
3. 全部失敗 → `repo.UpdateOGMetadata(id, &OGMetadata{FetchFailed: true})`（標記失敗，不卡隊列）

### ClickWorkerService

```go
// internal/service/clickworker/impl.go
package clickworker

type Service struct {
    repo repository.ClickLogRepository
}

func New(repo repository.ClickLogRepository) *Service
```

**流程：**

- `repo.BatchCreate(logs)` — 批次寫入 `click_log`

## Consumer 設計

Consumer 屬於驅動層，與 Handler 平行，負責從 Redis Stream 拉取訊息並呼叫 Service 介面。Consumer 不定義 domain 介面（只被 `cmd/worker` 使用，YAGNI 原則）。

> Consumer 直接持有 `*redis.Client` 具體型別，測試策略為整合測試（搭配 testcontainers 或 miniredis），
> 不做 mock 單元測試。

### OG Consumer

```go
// internal/consumer/og_consumer.go
package consumer

type OGConsumer struct {
    rdb       *redis.Client
    ogService service.OGWorkerService
    groupName string
    consumer  string
}

func NewOGConsumer(rdb *redis.Client, svc service.OGWorkerService) *OGConsumer

func (c *OGConsumer) Run(ctx context.Context) error
```

**錯誤處理流程：**

```
XREADGROUP 取出任務
    │
    ▼
ogService.ProcessTask()（內部含 retry 邏輯）
    ├── 成功 → XACK
    └── 失敗（OGFetcher 全部 retry 耗盡）
            → ogService 已將 og_metadata 標記 FetchFailed
            → XACK（不卡隊列，OG 抓取失敗為非致命錯誤）
            → slog.Error 記錄

ctx.Done() → 停止拉新訊息，等待當前任務完成後退出
```

### Click Consumer

```go
// internal/consumer/click_consumer.go
package consumer

type ClickConsumer struct {
    rdb          *redis.Client
    clickService service.ClickWorkerService
    groupName    string
    consumer     string
    maxDelivery  int // 預設 5，超過移至 dead letter stream
}

func NewClickConsumer(rdb *redis.Client, svc service.ClickWorkerService) *ClickConsumer

func (c *ClickConsumer) Run(ctx context.Context) error
```

**錯誤處理流程：**

```
XREADGROUP 取出一批訊息
    │
    ▼
clickService.ProcessBatch()
    ├── 成功 → XACK 整批
    └── 失敗 → 不 XACK（訊息留在 PEL）
              → slog.Error 記錄

定時 XCLAIM（idle > 30s 的 PEL 訊息重新認領重試）
    └── delivery count > maxDelivery（5次）
            → 移至 dead letter stream（stream:click-dlq）
            → XACK 原訊息
            → slog.Error 記錄（可觀測遺失量）

ctx.Done() → 停止拉新訊息，等待當前 batch 完成後退出
```

## 組裝層設計

### cmd/api/main.go

```go
func main() {
    cfg := config.Load()

    // Infrastructure
    dbPool    := infra.NewPostgresPool(cfg.Database)
    cacheRdb  := infra.NewRedisClient(cfg.RedisCache)    // DB 號碼由 cfg.RedisCache.DB 決定
    streamRdb := infra.NewRedisClient(cfg.RedisStream)   // DB 號碼由 cfg.RedisStream.DB 決定

    // Repository & Gateway（實作 domain 介面）
    urlRepo   := shorturl.NewRepository(dbPool)
    urlCache  := urlcache.NewCache(cacheRdb)
    publisher := eventpub.NewPublisher(streamRdb)
    idGen     := snowflake.NewGenerator()  // 實作 snowflake.IDGenerator 介面

    // Service（注入介面）
    urlSvc      := urlsvc.New(urlRepo, urlCache, publisher, idGen)
    redirectSvc := redirect.New(urlRepo, urlCache, publisher)

    // Handler（注入 domain/service 介面與設定參數）
    e := echo.New()
    handler.RegisterURLRoutes(e, urlSvc, cfg.Server.BaseURL)
    handler.RegisterRedirectRoutes(e, redirectSvc)
    handler.RegisterHealthRoutes(e)

    // Graceful Shutdown
    go func() { e.Start(":" + cfg.Server.Port) }()
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    e.Shutdown(ctx)
}
```

### cmd/worker/main.go

```go
func main() {
    cfg := config.Load()

    // Infrastructure
    dbPool    := infra.NewPostgresPool(cfg.Database)
    streamRdb := infra.NewRedisClient(cfg.RedisStream)

    // Repository & Gateway
    urlRepo   := shorturl.NewRepository(dbPool)
    clickRepo := clicklog.NewRepository(dbPool)
    fetcher   := ogfetch.NewFetcher(&http.Client{Timeout: 10 * time.Second})

    // Service
    ogSvc    := ogworker.New(urlRepo, fetcher)
    clickSvc := clickworker.New(clickRepo)

    // Consumer（注入 domain/service 介面）
    ogConsumer    := consumer.NewOGConsumer(streamRdb, ogSvc)
    clickConsumer := consumer.NewClickConsumer(streamRdb, clickSvc)

    // Graceful Shutdown：signal 觸發 ctx cancel，consumer 完成當前 batch 後退出
    ctx, cancel := context.WithCancel(context.Background())
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-quit
        slog.Info("shutting down worker...")
        cancel()
    }()

    // 啟動兩條 worker
    g, ctx := errgroup.WithContext(ctx)
    g.Go(func() error { return ogConsumer.Run(ctx) })
    g.Go(func() error { return clickConsumer.Run(ctx) })

    if err := g.Wait(); err != nil {
        slog.Error("worker stopped", "error", err)
    }
}
```

## 測試策略

| 層級                   | 測試方式                                  | Mock 對象                                                                  |
| :--------------------- | :---------------------------------------- | :------------------------------------------------------------------------- |
| `internal/service/`    | 單元測試 — 核心價值                       | Mock `domain/repository` + `domain/gateway` + `snowflake.IDGenerator` 介面 |
| `internal/handler/`    | 單元測試 — echo httptest                  | Mock `domain/service` 介面                                                 |
| `internal/consumer/`   | 整合測試 — testcontainers-go 或 miniredis | 啟動真實 Redis，驗證 Stream 消費、ACK、錯誤處理流程                        |
| `internal/repository/` | 整合測試 — testcontainers-go              | 啟動真實 PG/Redis container                                                |
| `internal/gateway/`    | 整合測試 — httptest.Server                | Mock HTTP server 模擬目標網頁                                              |
| `internal/pkg/`        | 單元測試 — 純函數                         | 無需 mock                                                                  |

## 設計決策紀錄

| 決策                         | 選擇                                               | 理由                                                            |
| :--------------------------- | :------------------------------------------------- | :-------------------------------------------------------------- |
| 介面歸屬                     | `domain/` 作為合約中心                             | 所有介面集中管理，多個 service 共用介面時不需重複定義           |
| 驅動層分類                   | `handler/` 與 `consumer/` 平行                     | 兩者都是外部輸入驅動，依賴 `domain/service` 介面，層級相同      |
| EventPublisher/URLCache 歸屬 | `domain/repository/`（廣義）                       | DB、Cache、Queue 皆視為資料存取層的一種形式，Service 主動呼叫   |
| EventConsumer 歸屬           | `internal/consumer/`（驅動層）                     | Consumer 呼叫 Service，是驅動層而非被驅動層，不定義 domain 介面 |
| Consumer 測試策略            | 整合測試                                           | Consumer 直接依賴 `*redis.Client`，mock 無意義，以整合測試覆蓋  |
| OG 抓取失敗策略              | retry 3 次後 XACK + 標記 FetchFailed               | OG 失敗為非致命錯誤，不卡隊列；以 FetchFailed 欄位保留可觀測性  |
| 點擊事件失敗策略             | 不 XACK → PEL → XCLAIM → Dead Letter               | 點擊統計需盡力保全，透過 PEL 機制重試，超過上限移至 DLQ         |
| Worker Graceful Shutdown     | signal → cancel ctx → consumer 完成當前 batch 退出 | 避免訊息處理到一半被中斷導致重複消費或資料不一致                |
| IDGenerator 介面化           | `snowflake.IDGenerator` interface                  | URLService 的 retry 邏輯可在測試中控制 ID 生成行為              |
| domain 層扁平 vs 子 package  | 扁平（用檔名區分）                                 | 專案規模小（~4 entity、~9 介面），切 package 過度設計           |
| 實作層按子系統切 package     | 每個 service/repo 獨立 package                     | 測試獨立、職責隔離，避免不同實作混雜                            |
| Handler 依賴                 | `domain/service` 介面                              | 可 mock service 做單元測試，符合 DI 原則                        |
| Consumer 不定義介面          | 直接用具體型別                                     | 只被 `cmd/worker/main.go` 使用，YAGNI 原則                      |
| `url` package 命名           | `urlsvc`                                           | 避免與 stdlib `net/url` 在同一檔案 import 時強制 alias          |
| Logger 實作                  | `internal/pkg/logger`（已實作）                    | context-aware slog wrapper，透過 `WithLogger` / `FromContext` 傳遞 request-scoped logger；middleware 負責注入 `request_id` |
| Logger 放置位置              | `internal/pkg/`（與其他工具統一）                  | 單一 module 應用程式，`internal/` 不阻止 `cmd/` import，語義上更精確表達「不對外暴露」；頂層 `pkg/` 留給真正的 library 專案 |

## 後續優化項目（POC 後處理）

| 項目                        | 說明                                                                           |
| :-------------------------- | :----------------------------------------------------------------------------- |
| Cache Stampede 防護         | `redirect.Resolve()` 高並發 cache miss 時加入 `golang.org/x/sync/singleflight` |
| RecordClick enrichment 重構 | 將 `ClickLog` 組裝責任從 handler 移至 service，handler 只傳原始 request        |
| OGFetchTask 位置            | 規模擴大後移至 `domain/task/`，與 entity 語義分離                              |
| `URLCache.Delete` 觸發時機  | 短網址停用/刪除功能上線時實作，目前介面已預留                                  |

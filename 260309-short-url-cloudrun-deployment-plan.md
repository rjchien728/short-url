# Short URL — Cloud Run 部署規劃

## 目標

將短網址服務從本地開發環境遷移部署至 Google Cloud Run，使用全託管 GCP 服務取代自架基礎設施，達成高可用、易維護的生產環境部署。

### 核心決策

| 項目 | 開發環境 | 生產環境 |
|------|--------|--------|
| 計算 | 本地執行 | Cloud Run (API + Worker 分開部署) |
| 資料庫 | PostgreSQL (Docker) | AlloyDB for PostgreSQL |
| 訊息佇列 | Redis Streams | Cloud Pub/Sub |
| 快取 | Redis (Docker) | Cloud Memorystore for Redis |
| 秘密管理 | `.env` 檔案 | Cloud Secret Manager |
| 容器映像 | 無 | Artifact Registry |

---

## 現況分析

### 目前服務結構

專案包含兩個獨立的執行進程，分別對應不同職責：

- **API Server** (`cmd/api/main.go`): 接收 HTTP 請求，負責短網址建立、重定向解析、OG 標籤讀取
- **Worker** (`cmd/worker/main.go`): 後台消費者，負責 OG Fetch 任務處理與 Click Log 批次寫入

### 訊息佇列現況 (Redis Streams)

```
[API Server]
  eventpub.PublishOGFetchTask()  →  rdb.XAdd("stream:og-fetch", fields)
  eventpub.PublishClickLog()     →  rdb.XAdd("stream:click-log", fields)

[Worker]
  og_consumer.Run()        →  rdb.XReadGroup("stream:og-fetch", ">")
  click_consumer.Run()     →  rdb.XReadGroup("stream:click-log", ">")
  click_consumer.reclaimLoop()  →  rdb.XClaim() // 定期認領逾期 PEL
```

遷移到 Pub/Sub 後，`domain/gateway/eventpub.go` 介面定義不變，僅替換底層實作。

### 配置現況

`internal/infra/config.go` 目前的 `Config` 結構：

```go
type Config struct {
    App      AppConfig
    Server   ServerConfig
    Database DatabaseConfig
    Cache    RedisConfig    // Redis DB 0，URL cache 用
    Stream   RedisConfig    // Redis DB 1，Stream 用 → 遷移後移除
    Consumer ConsumerConfig // Redis Stream consumer 設定 → 遷移後移除
}
```

---

## 設計方案

### 整體架構

```
┌─────────────────────────────────────────────────────────┐
│                     Cloud Run                           │
│                                                         │
│  ┌──────────────────────┐   ┌──────────────────────┐  │
│  │    short-url-api     │   │  short-url-worker    │  │
│  │  (auto-scaling 1-10) │   │  (single instance)   │  │
│  │                      │   │                      │  │
│  │ POST /urls           │   │ og_consumer          │  │
│  │ GET  /:code          │   │ click_consumer       │  │
│  └──────────┬───────────┘   └──────────┬───────────┘  │
└─────────────┼─────────────────────────-┼───────────────┘
              │                          │
              │  VPC Connector           │  VPC Connector
              ↓                          ↓
┌─────────────────────────────────────────────────────────┐
│              GCP 託管服務層                              │
│                                                         │
│  ┌──────────────┐  ┌───────────────┐  ┌─────────────┐ │
│  │   AlloyDB    │  │  Cloud Pub/Sub│  │ Memorystore │ │
│  │ (PostgreSQL) │  │               │  │  (Redis)    │ │
│  │              │  │  og-fetch     │  │             │ │
│  │ short_urls   │  │  click-log    │  │  URL Cache  │ │
│  │ og_metadata  │  │  *-dlq        │  │  (DB 0)     │ │
│  │ click_logs   │  │               │  │             │ │
│  └──────────────┘  └───────────────┘  └─────────────┘ │
└─────────────────────────────────────────────────────────┘
              ↑
┌─────────────────────────────────────────────────────────┐
│              Cloud Secret Manager                       │
│  db-connection-string / redis-url / og-default-image   │
└─────────────────────────────────────────────────────────┘
```

### 資料流

**建立短網址流程:**

```
Client → API → AlloyDB (insert short_urls)
                    → Pub/Sub Publish (og-fetch topic)
                    → Memorystore (cache short URL)
```

**OG Fetch 流程:**

```
Pub/Sub (og-fetch topic)
  → Worker og_consumer.Receive()
  → ogfetch.FetchOGTags(long_url)
  → AlloyDB (upsert og_metadata)
  [失敗] → 重試 (max 5 次) → og-fetch-dlq topic
```

**Click Log 流程:**

```
Client → API → Pub/Sub Publish (click-log topic)

Pub/Sub (click-log topic)
  → Worker click_consumer.Receive() (批次 100 筆)
  → AlloyDB (batch insert click_logs)
  [失敗] → 重試 (max 3 次) → click-log-dlq topic
```

---

## 技術選型

### Cloud Pub/Sub vs Redis Streams

| 功能 | Redis Streams | Cloud Pub/Sub |
|------|--------------|--------------|
| 發佈訊息 | `rdb.XAdd()` | `topic.Publish()` |
| 消費訊息 | `XReadGroup()` 阻塞輪詢 | `subscription.Receive()` push callback |
| ACK 確認 | `XACK` | `msg.Ack()` |
| 死信佇列 | 自行實作 (`XClaim` + DLQ stream) | 原生支援 Dead Letter Topic |
| Consumer Group | 原生支援 | Subscription 自動管理 |
| 與 CloudRun 整合 | 需要 VPC + Memorystore (Stream) | 原生整合，無需 VPC |
| 重試策略 | 需自行實作 (`reclaimLoop`) | 可設定 min/max backoff |

遷移後 `click_consumer` 的 `reclaimLoop()` 邏輯可移除，Pub/Sub 原生處理重試與死信。

### AlloyDB vs Cloud SQL

| 項目 | Cloud SQL (PostgreSQL) | AlloyDB |
|------|----------------------|---------|
| 基本費用 | ~$94/月 (2 vCPU) | ~$194/月 (2 vCPU) |
| 儲存費用 | $0.17/GB/月 | $0.35/GB/月 |
| 讀取效能 | 標準 PostgreSQL | 最高 4x 讀取吞吐量 |
| 寫入效能 | 標準 PostgreSQL | 最高 2x 寫入吞吐量 |
| 高可用 | 需額外設定 | 內建 HA，自動故障轉移 |
| 向量搜尋 | 需安裝 pgvector | 原生整合 |
| 建議場景 | 小流量、成本優先 | 高流量、高可用優先 |

**結論**: 初期若以成本優先可選 Cloud SQL，後期流量增長後再遷移 AlloyDB。

### Pub/Sub 主題設計

```
og-fetch              (訊息保留: 7 天)
  └─ og-fetch-sub     (訂閱, max retry: 5)
  └─ og-fetch-dlq     (死信, 訊息保留: 30 天)

click-log             (訊息保留: 1 天)
  └─ click-log-sub    (訂閱, max retry: 3, batch: 100)
  └─ click-log-dlq    (死信, 訊息保留: 7 天)
```

---

## 實作步驟

### 1. 程式碼修改

#### 1.1 配置層 (`internal/infra/config.go`)

移除 `Stream RedisConfig` 與 `Consumer ConsumerConfig`，新增 `PubSubConfig`:

```go
type PubSubConfig struct {
    ProjectID     string // GCP_PROJECT_ID
    OGTopic       string // PUBSUB_OG_TOPIC, default "og-fetch"
    ClickTopic    string // PUBSUB_CLICK_TOPIC, default "click-log"
    OGDLQTopic    string // PUBSUB_OG_DLQ_TOPIC, default "og-fetch-dlq"
    ClickDLQTopic string // PUBSUB_CLICK_DLQ_TOPIC, default "click-log-dlq"
}

type Config struct {
    App    AppConfig
    Server ServerConfig
    DB     DatabaseConfig
    Cache  RedisConfig   // Memorystore Redis，URL cache 用
    PubSub PubSubConfig  // 新增
}
```

#### 1.2 事件發佈 (`internal/repository/eventpub/impl.go`)

替換 `redis.XAdd()` 為 `pubsub.Topic.Publish()`:

```go
// 目前
err := p.rdb.XAdd(ctx, &redis.XAddArgs{
    Stream: ogStream,
    Values: map[string]any{...},
}).Err()

// 修改後
data, _ := json.Marshal(payload)
result := p.ogTopic.Publish(ctx, &pubsub.Message{Data: data})
_, err := result.Get(ctx)
```

#### 1.3 OG Consumer (`internal/consumer/og_consumer.go`)

替換 `XReadGroup()` 阻塞輪詢為 `subscription.Receive()` callback:

```go
func (c *OGConsumer) Run(ctx context.Context) error {
    return c.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
        if err := c.processMessage(ctx, msg); err != nil {
            msg.Nack() // 觸發重試
            return
        }
        msg.Ack()
    })
}
```

#### 1.4 Click Consumer (`internal/consumer/click_consumer.go`)

- 移除 `reclaimLoop()` — Pub/Sub 原生處理重試
- 替換 `XReadGroup()` 為 `subscription.Receive()` + 批次收集邏輯

#### 1.5 依賴套件

```bash
# 新增
go get cloud.google.com/go/pubsub

# 若 Redis 只保留 cache 用途，go-redis 仍保留
```

#### 1.6 環境變數更新 (`.env.example`)

```env
# App
APP_ENV=production
APP_LOG_LEVEL=info
PORT=8080
SERVER_BASE_URL=https://your-domain.com
OG_DEFAULT_IMAGE=https://example.com/default.jpg

# AlloyDB
DB_DSN=postgres://user:pass@10.0.1.10:5432/shorturl?sslmode=require
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=5

# Memorystore (Redis cache only)
REDIS_CACHE_URL=redis://10.0.0.5:6379/0

# Cloud Pub/Sub
GCP_PROJECT_ID=my-gcp-project
PUBSUB_OG_TOPIC=og-fetch
PUBSUB_CLICK_TOPIC=click-log
PUBSUB_OG_DLQ_TOPIC=og-fetch-dlq
PUBSUB_CLICK_DLQ_TOPIC=click-log-dlq
```

### 2. 容器化

#### 2.1 Dockerfile.api

```dockerfile
# Stage 1: Build
FROM golang:1.24.5-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o api cmd/api/main.go

# Stage 2: Runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/api .
EXPOSE 8080
CMD ["./api"]
```

#### 2.2 Dockerfile.worker

```dockerfile
# Stage 1: Build
FROM golang:1.24.5-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o worker cmd/worker/main.go

# Stage 2: Runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/worker .
CMD ["./worker"]
```

#### 2.3 .dockerignore

```
.git
.gitignore
.env
.env.example
.devcontainer
docker-compose.yml
Makefile
*.md
internal/**/*_test.go
```

### 3. GCP 資源準備

#### 3.1 啟用必要 API

```bash
gcloud services enable \
  run.googleapis.com \
  alloydb.googleapis.com \
  redis.googleapis.com \
  pubsub.googleapis.com \
  secretmanager.googleapis.com \
  artifactregistry.googleapis.com \
  vpcaccess.googleapis.com
```

#### 3.2 VPC Connector (CloudRun → AlloyDB/Memorystore)

AlloyDB 與 Memorystore 僅提供私有 IP，CloudRun 需透過 VPC Connector 連線：

```bash
gcloud compute networks vpc-access connectors create short-url-connector \
  --network default \
  --region asia-east1 \
  --range 10.8.0.0/28
```

#### 3.3 Pub/Sub 主題與訂閱

```bash
# 建立主題
gcloud pubsub topics create og-fetch
gcloud pubsub topics create og-fetch-dlq
gcloud pubsub topics create click-log
gcloud pubsub topics create click-log-dlq

# 建立訂閱 (帶死信設定)
gcloud pubsub subscriptions create og-fetch-sub \
  --topic og-fetch \
  --dead-letter-topic og-fetch-dlq \
  --max-delivery-attempts 5 \
  --ack-deadline 60

gcloud pubsub subscriptions create click-log-sub \
  --topic click-log \
  --dead-letter-topic click-log-dlq \
  --max-delivery-attempts 3 \
  --ack-deadline 30
```

#### 3.4 Secret Manager

```bash
# DB 連線字串 (含帳號密碼)
echo -n "postgres://user:pass@10.0.1.10:5432/shorturl?sslmode=require" | \
  gcloud secrets create db-connection-string --data-file=-

# Redis URL
echo -n "redis://10.0.0.5:6379/0" | \
  gcloud secrets create redis-cache-url --data-file=-

# OG 預設圖片
echo -n "https://example.com/default-og.jpg" | \
  gcloud secrets create og-default-image --data-file=-
```

#### 3.5 Service Account 權限

```bash
# 建立 service account
gcloud iam service-accounts create short-url-runner \
  --display-name "Short URL Cloud Run"

SA="short-url-runner@PROJECT_ID.iam.gserviceaccount.com"

# 賦予必要權限
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:$SA" \
  --role="roles/pubsub.publisher"

gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:$SA" \
  --role="roles/pubsub.subscriber"

gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:$SA" \
  --role="roles/secretmanager.secretAccessor"
```

### 4. 容器映像推送

```bash
# 設定 Artifact Registry
gcloud artifacts repositories create short-url \
  --repository-format=docker \
  --location=asia-east1

# 構建並推送映像
gcloud builds submit --tag asia-east1-docker.pkg.dev/PROJECT_ID/short-url/api:latest \
  --dockerfile Dockerfile.api .

gcloud builds submit --tag asia-east1-docker.pkg.dev/PROJECT_ID/short-url/worker:latest \
  --dockerfile Dockerfile.worker .
```

### 5. Cloud Run 部署

#### 5.1 部署 API Service

```bash
gcloud run deploy short-url-api \
  --image asia-east1-docker.pkg.dev/PROJECT_ID/short-url/api:latest \
  --region asia-east1 \
  --service-account short-url-runner@PROJECT_ID.iam.gserviceaccount.com \
  --vpc-connector short-url-connector \
  --memory 512Mi \
  --cpu 1 \
  --min-instances 1 \
  --max-instances 10 \
  --allow-unauthenticated \
  --set-env-vars "APP_ENV=production,APP_LOG_LEVEL=info,GCP_PROJECT_ID=PROJECT_ID,PUBSUB_OG_TOPIC=og-fetch,PUBSUB_CLICK_TOPIC=click-log" \
  --set-secrets "DB_DSN=db-connection-string:latest,REDIS_CACHE_URL=redis-cache-url:latest,OG_DEFAULT_IMAGE=og-default-image:latest"
```

#### 5.2 部署 Worker Service

```bash
gcloud run deploy short-url-worker \
  --image asia-east1-docker.pkg.dev/PROJECT_ID/short-url/worker:latest \
  --region asia-east1 \
  --service-account short-url-runner@PROJECT_ID.iam.gserviceaccount.com \
  --vpc-connector short-url-connector \
  --memory 1Gi \
  --cpu 2 \
  --min-instances 1 \
  --max-instances 2 \
  --no-allow-unauthenticated \
  --set-env-vars "APP_ENV=production,APP_LOG_LEVEL=info,GCP_PROJECT_ID=PROJECT_ID,PUBSUB_OG_TOPIC=og-fetch,PUBSUB_CLICK_TOPIC=click-log,PUBSUB_CLICK_DLQ_TOPIC=click-log-dlq" \
  --set-secrets "DB_DSN=db-connection-string:latest,REDIS_CACHE_URL=redis-cache-url:latest"
```

### 6. 資料庫初始化

```bash
# 透過 Cloud Run Job 執行 migration
gcloud run jobs create short-url-migrate \
  --image asia-east1-docker.pkg.dev/PROJECT_ID/short-url/api:latest \
  --region asia-east1 \
  --service-account short-url-runner@PROJECT_ID.iam.gserviceaccount.com \
  --vpc-connector short-url-connector \
  --set-secrets "DB_DSN=db-connection-string:latest" \
  --command "./api" \
  --args "migrate,up"

gcloud run jobs execute short-url-migrate
```

---

## 驗證清單

### 部署後檢查

- [ ] API health check: `curl https://SHORT_URL_API/health` 回傳 200
- [ ] 建立短網址: `POST /urls` 成功寫入 AlloyDB
- [ ] 重定向: `GET /:code` 正確重定向並快取至 Memorystore
- [ ] Pub/Sub 流: Cloud Console → Pub/Sub → `og-fetch` 訂閱，確認訊息被消費
- [ ] Worker 日誌: `gcloud run logs short-url-worker` 確認 `og_consumer: started`
- [ ] AlloyDB 資料: 確認 `og_metadata` 表有資料寫入
- [ ] DLQ 監控: `og-fetch-dlq` 和 `click-log-dlq` 主題無異常積壓

### 回滾計畫

若部署後發現問題：

```bash
# 回滾至上一版本
gcloud run services update-traffic short-url-api \
  --to-revisions PREV_REVISION=100

gcloud run services update-traffic short-url-worker \
  --to-revisions PREV_REVISION=100
```

---

## 注意事項

### 程式碼遷移重點

- `click_consumer.reclaimLoop()` 可在遷移後移除，Pub/Sub 原生處理 unack 訊息的重試
- `og_consumer` 和 `click_consumer` 的 `XReadGroup` 阻塞讀取改為 `subscription.Receive()` callback 模式，需注意 context cancellation 的 graceful shutdown 行為
- Worker 的 `ConsumerConfig` (group name, consumer name, batch size) 由 Pub/Sub subscription 設定取代，可從 config 結構中移除

### 網路連線

- AlloyDB 與 Memorystore 均使用**私有 IP**，CloudRun 需設定 VPC Connector 才能連線
- Pub/Sub 為 Google 公共 API，CloudRun 不需要 VPC Connector 即可存取

### Secret Manager 注意

- Secret 版本固定用 `:latest` 便於更新，但須注意 CloudRun 快取 Secret 至多 10 分鐘
- 若需立即生效，需重新部署 (新 revision) 而非只更新 Secret 版本

### Worker 實例數

- Worker 設定 `min-instances: 1` 確保永遠有實例在消費 Pub/Sub
- `max-instances: 2` 避免多個 Worker 實例造成 click log 重複計數問題 (Pub/Sub at-least-once 語義)
- 若需水平擴展 Worker，需在 click_consumer 層實作冪等性保護

### 成本監控

建議設定以下 Cloud Billing 告警：
- 每日預算告警 $30/天
- 月度預算告警 $500/月

主要成本來源排序: AlloyDB > VPC Connector > CloudRun Worker > Memorystore > Pub/Sub

---

## 受影響檔案清單

| 檔案 | 變更類型 | 說明 |
|------|--------|------|
| `internal/infra/config.go` | 修改 | 新增 `PubSubConfig`，移除 `Stream`、`Consumer` |
| `internal/repository/eventpub/impl.go` | 重寫 | Pub/Sub client 替代 `redis.XAdd()` |
| `internal/consumer/og_consumer.go` | 重寫 | `subscription.Receive()` 替代 `XReadGroup()` |
| `internal/consumer/click_consumer.go` | 重寫 | 移除 `reclaimLoop()`，改用 Pub/Sub callback |
| `cmd/worker/main.go` | 修改 | 初始化 Pub/Sub client 替代 stream Redis client |
| `.env.example` | 修改 | 新增 Pub/Sub 相關變數，移除 Stream 變數 |
| `go.mod` | 修改 | 新增 `cloud.google.com/go/pubsub` |
| `Dockerfile.api` | 新增 | API 容器映像構建設定 |
| `Dockerfile.worker` | 新增 | Worker 容器映像構建設定 |
| `.dockerignore` | 新增 | 排除不必要檔案以縮小映像大小 |

# 260308-short-url-config-design

## 目標

設計一套具備靈活性、安全性且符合雲端原生（Cloud Native）規範的設定管理系統，支援本地開發環境（`.env`）與生產環境（環境變數注入）的無縫切換。

## 現況分析

- **開發環境**: 需支援多個開發者使用不同的資料庫與 Redis 連線資訊。
- **生產環境 (GCP Cloud Run)**: 主要透過環境變數注入（如 `PORT`），且需支援 Serverless 的橫向擴展。
- **效能調優**: 針對 Redis Streams 的消費行為（Batch Size, Idle Time），需提供合理的預設值並支援外部覆蓋。

## 設計方案

### 1. 載入優先級 (Priority Logic)

系統將依序嘗試從以下來源讀取設定，後者覆蓋前者：
1. **內建預設值 (Defaults)**: 程式碼中定義的硬編碼預設值。
2. **設定檔 (`.env`)**: 本地開發專用，不進入版本控制。
3. **系統環境變數 (Environment Variables)**: 生產環境（如 Cloud Run / Kubernetes）的主要來源。
4. **命令列參數 (Flags)**: 啟動時手動指定的參數（視需求實作）。

### 2. 設定結構 (Config Structure)

設定將被組織為以下具備命名空間的結構：

- **App**: 基礎環境設定（Env, LogLevel）。
- **Server**: HTTP 服務設定（Port, BaseURL）。
- **Database**: 關聯式資料庫連線（DSN、MaxOpenConns、MaxIdleConns）。
- **Redis**: 快取（DB 0）與 訊息隊列（DB 1）的隔離連線。

### 3. 環境變數映射表 (Environment Mappings)

| 環境變數 | 預設值 | 說明 |
| :--- | :--- | :--- |
| `APP_ENV` | `development` | 運行環境 (production / development) |
| `APP_LOG_LEVEL` | `info` | 日誌層級 (debug / info / error) |
| `PORT` | `8080` | HTTP 服務監聽埠 (映射自 Cloud Run) |
| `SERVER_BASE_URL` | `http://localhost:8080` | 生成短網址時使用的域名 |
| `DB_DSN` | (必填) | PostgreSQL DSN |
| `REDIS_CACHE_URL` | `redis://localhost:6379/0` | Redis Cache 連線字串 |
| `REDIS_STREAM_URL` | `redis://localhost:6379/1` | Redis Streams 連線字串 |
| `DB_MAX_OPEN_CONNS` | `10` | PostgreSQL 最大開放連線數（Cloud Run 建議控制在低值避免超過 PG 限制） |
| `DB_MAX_IDLE_CONNS` | `5` | PostgreSQL 最大閒置連線數 |

## 技術選型

- **庫**: `github.com/spf13/viper`
- **優點**: 支援自動環境變數映射、多格式設定檔、以及強型別結構體（Struct Unmarshal）。

## 實作步驟

1. **定義設定結構體**: 建立 `internal/infra/config.go`。
2. **設定預設值**: 使用 `viper.SetDefault` 初始化效能參數。
3. **讀取設定檔**: 設定 `viper.SetConfigFile(".env")`。
4. **自動映射變數**: 調用 `viper.AutomaticEnv()` 並配合 `mapstructure` tag。
5. **結構化解析**: 使用 `viper.Unmarshal` 將設定注入物件中。

## 注意事項

- **安全性**: 嚴禁將包含密碼或 DSN 的 `.env` 檔案提交至 Git。需在 `.gitignore` 中加入該檔案。
- **Redis 隔離**: 務必確保 Cache 與 Streams 使用不同的 DB 號碼，以避免 Eviction Policy 導致訊息遺失。
- **Serverless 兼容性**: `PORT` 變數必須能夠正確映射至 `Server.Port`，以符合 Google Cloud Run 的規範（啟動時需組合為 `":" + cfg.Server.Port`）。

# 260308-short-url-api-spec-design

## 目標

定義 Short URL 系統的外部 API 接口與內部異步通訊的 Event Payload 規範，確保系統安全性（不洩漏內部 ID）與高效能分析能力。

## 設計方案

### 1. 通用規範

- **Base URL**: 所有的短網址回應將使用 `{base_url}` 作為佔位符。
- **時間格式**: 統一使用 RFC3339 (ISO 8601) 格式，例如 `2026-03-08T10:00:00Z`。
- **編碼**: API Request/Response 統一使用 UTF-8 編碼的 JSON 格式。

### 2. 外部 API 接口 (RESTful)

#### A. 建立短網址 (Create Short URL)

- **Endpoint**: `POST /v1/urls`
- **Request Body**:
  ```json
  {
    "long_url": "https://example.com/very/long/path",
    "creator_id": "user_12345"
  }
  ```
- **Response Body (201 Created)**:
  ```json
  {
    "short_url": "{base_url}/Ab3D5fG7hJ",
    "long_url": "https://example.com/very/long/path",
    "creator_id": "user_12345",
    "created_at": "2026-03-08T10:00:00Z"
  }
  ```
- **安全性說明**: 不在回應中洩漏 Snowflake ID。
- **POC 限制**: `expires_at`（短網址過期時間）於 POC 階段不開放傳入，所有短網址預設永不過期。

#### B. 短網址跳轉 (Redirection)

- **Endpoint**: `GET /:shortCode`
- **Query Parameters**:
  - `ref`: (選填) 推薦碼 (Referral ID)，用於歸因分析。
- **行為規範**:
  - **一般使用者**: 回傳 `HTTP 302 Found`。
  - **社群爬蟲 (Bot)**: 回傳 `HTTP 200 OK`，內容為包含 OG Metadata 的 HTML 預覽頁面。
  - **連結已過期**: 回傳 `HTTP 410 Gone`。
  - **連結不存在**: 回傳 `HTTP 404 Not Found`。

#### C. 健康檢查 (Health Check)

- **Endpoint**: `GET /healthz`
- **用途**: 供 Cloud Run / Load Balancer 探測服務存活狀態。
- **Response Body (200 OK)**:
  ```json
  {
    "status": "ok"
  }
  ```
- **行為規範**: 此端點永遠回傳 `HTTP 200`，不做資料庫或 Redis 連線探測（POC 階段）。

### 3. 請求驗證規範 (Request Validation)

#### `long_url` 驗證規則

| 規則 | 說明 |
| :--- | :--- |
| 必填 | 欄位不可為空 |
| Scheme | 只接受 `http://` 或 `https://`，其他 scheme（如 `javascript:`、`ftp://`）一律拒絕 |
| 長度上限 | 最大 2048 字元（對應 PostgreSQL `TEXT` 欄位與常見瀏覽器限制） |
| 格式 | 必須為合法的 URL 結構（可通過 Go `net/url.ParseRequestURI` 驗證） |

> **POC 說明**: `creator_id` 不做身份驗證，直接信任前端傳入值（非空即可）。JWT 驗證留於 Future Improvement。

### 5. 錯誤回應規範 (Error Response)

- **結構**:
  ```json
  {
    "error": "ERROR_CODE",
    "message": "Human readable message"
  }
  ```
- **常用錯誤碼**:
  - `INVALID_ARGUMENT` (400): 參數缺失或格式錯誤。
  - `UNAUTHORIZED` (401): 身份驗證失敗。
  - `NOT_FOUND` (404): 短碼不存在。
  - `URL_EXPIRED` (410): 連結已超過有效期。
  - `INTERNAL_ERROR` (500): 伺服器內部錯誤。

### 6. 內部 Event Payload (Redis Streams)

#### A. OG 抓取任務 (`stream:og-fetch`)

由 API Server 發送，供 Preview Worker 消費。

- **Fields**:
  - `short_url_id`: Snowflake ID (內部關聯用)
  - `long_url`: 原始網址

#### B. 點擊日誌事件 (`stream:click-log`)

由 Redirect Handler 發送，供 Click Worker 批次寫入 BigQuery (或 PG)。

- **Fields**:
  - `click_id`: UUID v7 (時間有序，優化資料庫索引與分析效能)
  - `short_url_id`: Snowflake ID (內部關聯用)
  - `short_code`: 短碼
  - `creator_id`: 創作者 ID
  - `referral_id`: 推薦碼 (來自 `ref` 參數)
  - `referrer`: HTTP Referer 標頭
  - `user_agent`: User-Agent 標頭
  - `ip_address`: 使用者 IP
  - `is_bot`: 布林值 (API Server 預先識別結果)
  - `created_at`: 事件觸發時間

## 技術選型

- **UUID**: 使用 **UUID v7** 作為點擊日誌主鍵。
- **Redis Streams**: 作為異步任務與數據收集的緩衝。

## 注意事項

- **ID 隱藏**: Snowflake ID 嚴禁透過公開 API 暴露給客戶端。
- **效能優化**: Click Log 事件中包含 `creator_id` 與 `short_code` 冗餘欄位，確保 Worker 寫入分析倉儲時不需二次查詢資料庫。

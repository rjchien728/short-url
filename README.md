# Short URL Service

A URL shortener service built with Go, designed for social sharing with async background processing via Redis Streams.

---

## Architecture

```
                    ┌─────────────────────────────┐
   HTTP Request ──► │         API Server          │
                    │  POST /v1/urls               │
                    │  GET  /:shortCode            │◄──► Redis Cache (db 0)
                    └──────────────┬──────────────┘          │
                                   │ publish                  │ cache miss
                                   ▼                          ▼
                          Redis Streams (db 1)           PostgreSQL
                    ┌──────────────────────────┐
                    │  stream:og-fetch         │
                    │  stream:click-log        │
                    └──────────────┬───────────┘
                                   │ consume
                                   ▼
                    ┌─────────────────────────────┐
                    │      Background Worker      │
                    │  OG Fetch Worker            │──► PostgreSQL
                    │  Click Log Worker           │──► PostgreSQL
                    └─────────────────────────────┘
```

### Processes

| Process | Entry Point | Responsibility |
|---------|-------------|----------------|
| API Server | `cmd/api` | Handle short URL creation and redirects |
| Background Worker | `cmd/worker` | Consume Redis Streams for async task processing |

### Background Workers

| Worker | Stream | Description |
|--------|--------|-------------|
| OG Fetch | `stream:og-fetch` | Scrapes Open Graph metadata from the destination URL after a short link is created |
| Click Log | `stream:click-log` | Persists click events to PostgreSQL in batches. Unprocessed messages stay in PEL for retry; messages exceeding the retry limit are moved to `stream:click-dlq` |

> **Note — Intentional simplifications**
>
> This project prioritises implementation simplicity over production scale:
> - **Message queue**: Redis Streams is used instead of Kafka or Cloud Pub/Sub.
> - **Analytics storage**: Click logs are written to PostgreSQL instead of a dedicated OLAP store (e.g. BigQuery, ClickHouse).

---

## Database Schema

### `short_url`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `BIGINT` | Snowflake ID (primary key) |
| `short_code` | `VARCHAR(10)` | Base58-encoded unique code |
| `long_url` | `TEXT` | Original destination URL |
| `creator_id` | `VARCHAR(50)` | Identifier of the creator |
| `og_metadata` | `JSONB` | Open Graph metadata (title, description, image, site_name, fetch_failed) |
| `expires_at` | `TIMESTAMPTZ` | Optional expiry time |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `click_log`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` | UUID v7 (primary key) |
| `short_url_id` | `BIGINT` | References `short_url.id` |
| `short_code` | `VARCHAR(10)` | Denormalised for query convenience |
| `creator_id` | `VARCHAR(50)` | Copied from the short URL record |
| `referral_id` | `VARCHAR(50)` | Optional referral tracking ID (`?ref=`) |
| `referrer` | `TEXT` | HTTP `Referer` header |
| `user_agent` | `TEXT` | HTTP `User-Agent` header |
| `ip_address` | `VARCHAR(45)` | Client IP (supports IPv6) |
| `is_bot` | `BOOLEAN` | Bot detection result |
| `created_at` | `TIMESTAMPTZ` | Click timestamp |

---

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check |
| `POST` | `/v1/urls` | Create a short URL |
| `GET` | `/:shortCode` | Redirect to the destination URL |

### `POST /v1/urls`

**Request**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `long_url` | string | yes | The destination URL to shorten |
| `creator_id` | string | yes | Identifier of the creator |

```json
{
  "long_url": "https://example.com/some/very/long/path",
  "creator_id": "user_01"
}
```

**Response `201`**

| Field | Type | Description |
|-------|------|-------------|
| `short_url` | string | Full shortened URL (e.g. `http://localhost:8080/aB3xYz1234`) |
| `long_url` | string | Original URL |
| `creator_id` | string | Creator identifier |
| `created_at` | string | ISO 8601 timestamp |

```json
{
  "short_url": "http://localhost:8080/aB3xYz1234",
  "long_url": "https://example.com/some/very/long/path",
  "creator_id": "user_01",
  "created_at": "2026-03-10T00:00:00Z"
}
```

### `GET /:shortCode`

| Scenario | Status | Behaviour |
|----------|--------|-----------|
| Normal browser request | `302` | Redirect to `long_url` |
| Bot / social crawler | `200` | Returns an HTML page with OG meta tags for link previews |
| Not found | `404` | `{"error": "not_found"}` |
| Expired | `410` | `{"error": "expired"}` |

---

## Local Development

**Prerequisites**: Go 1.24+, Docker

```bash
# 1. Initialise environment (copies .env.example → .env, starts infra, runs migrations)
make local-init

# 2. Start API + Worker together (Ctrl+C to stop)
make dev
```

### Commands

| Command | Description |
|---------|-------------|
| `make local-init` | One-shot setup: copy `.env`, start infra, run migrations |
| `make dev` | Start API + Worker (Ctrl+C to stop both) |
| `make run-api` | Start API server only |
| `make run-worker` | Start Worker only |
| `make infra-up` | Start PostgreSQL + Redis containers |
| `make infra-down` | Stop PostgreSQL + Redis containers |
| `make migrate-up` | Run DB migrations |
| `make migrate-down` | Rollback DB migrations |
| `make test` | Unit tests (no external dependencies required) |
| `make test-integration` | Integration tests (requires running infra) |
| `make mock` | Regenerate mock files |
| `make build-linux` | Cross-compile API + Worker binaries for Linux amd64 |

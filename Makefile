# 引入 .env 變數（如果存在的話）
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# --- 基礎變數 ---
BINARY_NAME=short-url-api
MIGRATE_CMD=go run cmd/migrate/main.go

# --- 初始化 ---
.PHONY: init
init: ## 初始化環境：建立 .env 並啟動 Docker
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	fi
	docker compose up -d
	@echo "Infrastructure is up and running."

# --- Docker 管理 ---
.PHONY: docker-up docker-down docker-logs
docker-up: ## 啟動基礎設施
	docker compose up -d

docker-down: ## 停止基礎設施
	docker compose down

docker-logs: ## 查看 Docker 容器日誌
	docker compose logs -f

# --- 資料庫遷移 (Migration) ---
.PHONY: migrate-up migrate-down
migrate-up: ## 執行所有尚未執行的遷移 (建立 Schema)
	$(MIGRATE_CMD) up

migrate-down: ## 撤銷所有遷移 (清空資料庫)
	$(MIGRATE_CMD) down

# --- 開發指令 ---
.PHONY: run-api run-worker build test lint
run-api: ## 啟動 API Server
	go run cmd/api/main.go

run-worker: ## 啟動 Worker
	go run cmd/worker/main.go

build: ## 編譯專案
	go build -o bin/$(BINARY_NAME) cmd/api/main.go

test: ## 執行單元測試
	go test -v ./...

lint: ## 執行代碼檢查 (需安裝 golangci-lint)
	golangci-lint run

# --- 其他 ---
.PHONY: help
help: ## 顯示此幫助訊息
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

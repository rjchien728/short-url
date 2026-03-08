# Load .env variables if present
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# --- Base Variables ---
BINARY_NAME=short-url-api
MIGRATE_CMD=go run cmd/migrate/main.go

# --- Initialization ---
.PHONY: init
init: ## Initialize environment: create .env and start Docker
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	fi
	docker compose up -d
	@echo "Infrastructure is up and running."

# --- Docker Management ---
.PHONY: docker-up docker-down docker-logs
docker-up: ## Start infrastructure
	docker compose up -d

docker-down: ## Stop infrastructure
	docker compose down

docker-logs: ## View Docker container logs
	docker compose logs -f

# --- Database Migration ---
.PHONY: migrate-up migrate-down
migrate-up: ## Run all pending migrations
	$(MIGRATE_CMD) up

migrate-down: ## Rollback all migrations
	$(MIGRATE_CMD) down

# --- Mock Generation ---
.PHONY: mock
mock: ## Generate all mocks via go:generate (requires mockgen in PATH)
	go generate ./internal/domain/... ./internal/pkg/snowflake/...
	@echo "Mocks generated successfully."

# --- Development Commands ---
.PHONY: run-api run-worker dev build test lint
run-api: ## Start API Server
	go run cmd/api/main.go

run-worker: ## Start Worker
	go run cmd/worker/main.go

dev: ## Start API and Worker together (Ctrl+C stops both)
	@go run cmd/api/main.go & API_PID=$$!; \
	go run cmd/worker/main.go & WORKER_PID=$$!; \
	trap "kill $$API_PID $$WORKER_PID 2>/dev/null" INT TERM; \
	wait $$API_PID $$WORKER_PID

build: ## Build the project
	go build -o bin/$(BINARY_NAME) cmd/api/main.go

test: ## Run unit tests only (no DB/Redis required)
	go test -v -count=1 -timeout=30s $(shell go list ./... | grep -v 'repository/shorturl\|repository/clicklog\|internal/consumer')

test-integration: ## Run integration tests against local DB and Redis (requires migrate-up first)
	go test -v -count=1 -timeout=120s ./internal/repository/... ./internal/gateway/... ./internal/consumer/...

lint: ## Run linter (requires golangci-lint)
	golangci-lint run

# --- Miscellaneous ---
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

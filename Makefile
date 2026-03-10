# ==============================================================================
# Main Variables
# ==============================================================================

MIGRATE_CMD  = go run cmd/migrate/main.go
COMPOSE_INFRA = docker compose -f deploy/docker-compose.infra.yml
COMPOSE_APP   = docker compose -f deploy/docker-compose.app.yml

# Load .env variables if present
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# ==============================================================================
# 1. Environment Initialization
# ==============================================================================

.PHONY: local-init
local-init: ## Initialize development environment: .env -> infra-up -> migrate-up
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	fi
	@$(MAKE) infra-up
	@$(MAKE) migrate-up
	@echo "Local environment initialized. You can now run 'make dev'."

# ==============================================================================
# 2. Infrastructure Management (DB, Redis)
# ==============================================================================

.PHONY: infra-up infra-down infra-logs
infra-up: ## Start infrastructure containers (db + redis) and wait for health
	$(COMPOSE_INFRA) up -d --wait

infra-down: ## Stop and remove infrastructure containers
	$(COMPOSE_INFRA) down

infra-logs: ## View infrastructure logs
	$(COMPOSE_INFRA) logs -f

# ==============================================================================
# 3. Local Development (Go Environment)
# ==============================================================================

.PHONY: dev run-api run-worker mock
dev: ## Start API and worker locally (Ctrl+C stops both)
	@go run cmd/api/main.go & API_PID=$$!; \
	go run cmd/worker/main.go & WORKER_PID=$$!; \
	trap "kill $$API_PID $$WORKER_PID 2>/dev/null" INT TERM; \
	wait $$API_PID $$WORKER_PID

run-api: ## Run API server locally
	go run cmd/api/main.go

run-worker: ## Run worker locally
	go run cmd/worker/main.go

mock: ## Generate mocks for development
	go generate ./internal/domain/... ./internal/pkg/snowflake/...
	@echo "Mocks generated."

# ==============================================================================
# 4. Database Migrations (Local)
# ==============================================================================

.PHONY: migrate-up migrate-down
migrate-up: ## Run database migrations locally
	$(MIGRATE_CMD) up

migrate-down: ## Rollback database migrations locally
	$(MIGRATE_CMD) down

# ==============================================================================
# 5. Testing & Quality
# ==============================================================================

.PHONY: test test-integration lint
test: ## Run unit tests (logic only, no external deps)
	go test -v -count=1 -timeout=30s $(shell go list ./... | grep -v 'repository/shorturl\|repository/clicklog\|internal/consumer')

test-integration: ## Run integration tests (against local DB/Redis)
	go test -v -count=1 -timeout=30s ./internal/repository/... ./internal/gateway/... ./internal/consumer/...

lint: ## Run golangci-lint
	golangci-lint run

# ==============================================================================
# 6. Build & Deployment
# ==============================================================================

.PHONY: build-linux build-images app-up app-down app-logs

build-linux: ## Cross-compile API and Worker binaries for Linux amd64
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/api ./cmd/api
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/worker ./cmd/worker
	@echo "Binaries built: bin/api, bin/worker"

build-images: ## Build Docker images for API and Worker
	$(COMPOSE_APP) build

app-up: ## Start API and Worker containers (requires infra-up)
	$(COMPOSE_APP) up -d

app-down: ## Stop API and Worker containers
	$(COMPOSE_APP) down

app-logs: ## View application logs
	$(COMPOSE_APP) logs -f

# ==============================================================================
# 7. Utilities
# ==============================================================================

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

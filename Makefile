# Load .env variables if present
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# --- Base Variables ---
BINARY_NAME=short-url-api
MIGRATE_CMD=go run cmd/migrate/main.go

# --- Compose file shortcuts ---
COMPOSE_INFRA=docker compose -f deploy/docker-compose.infra.yml
COMPOSE_ALL=docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.app.yml

# --- Initialization ---
.PHONY: init
init: ## Initialize environment: create .env and start infra
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	fi
	$(COMPOSE_INFRA) up -d
	@echo "Infrastructure is up and running."

# --- Docker Management (infra only) ---
.PHONY: docker-up docker-down docker-logs
docker-up: ## Start infra (db + redis)
	$(COMPOSE_INFRA) up -d

docker-down: ## Stop infra (db + redis)
	$(COMPOSE_INFRA) down

docker-logs: ## View infra container logs
	$(COMPOSE_INFRA) logs -f

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
run-api: ## Start API server
	go run cmd/api/main.go

run-worker: ## Start worker
	go run cmd/worker/main.go

dev: ## Start API and worker together (Ctrl+C stops both)
	@go run cmd/api/main.go & API_PID=$$!; \
	go run cmd/worker/main.go & WORKER_PID=$$!; \
	trap "kill $$API_PID $$WORKER_PID 2>/dev/null" INT TERM; \
	wait $$API_PID $$WORKER_PID

build: ## Build the API binary
	go build -o bin/$(BINARY_NAME) cmd/api/main.go

test: ## Run unit tests only (no DB/Redis required)
	go test -v -count=1 -timeout=30s $(shell go list ./... | grep -v 'repository/shorturl\|repository/clicklog\|internal/consumer')

test-integration: ## Run integration tests against local DB and Redis (requires migrate-up first)
	go test -v -count=1 -timeout=30s ./internal/repository/... ./internal/gateway/... ./internal/consumer/...

lint: ## Run linter (requires golangci-lint)
	golangci-lint run

# --- Deployment (VM) ---
.PHONY: deploy deploy-down deploy-logs
deploy: ## Build images and start all services: infra + migrate + api + worker
	$(COMPOSE_ALL) up -d --build

deploy-down: ## Stop app services only (api, worker, migrate); keeps infra running
	docker compose -f deploy/docker-compose.app.yml down

deploy-logs: ## View logs for all services
	$(COMPOSE_ALL) logs -f

# --- Miscellaneous ---
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

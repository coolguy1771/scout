# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build parameters
BINARY_NAME=scout
BINARY_PATH=bin/$(BINARY_NAME)
MAIN_PATH=./cmd/scout

# Test parameters
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Default target
.DEFAULT_GOAL := all

.PHONY: help all build install clean test test-race test-coverage vet fmt tidy lint docker-build docker-up docker-down docker-logs migrate-up migrate-down migrate-create run-api run-worker

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

all: build ## Build the project (default target)

build: ## Build the Scout CLI binary
	$(GOBUILD) -o $(BINARY_PATH) $(MAIN_PATH)

install: ## Install the binary to $GOPATH/bin
	$(GOBUILD) -o $$(go env GOPATH)/bin/$(BINARY_NAME) $(MAIN_PATH)

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)

test: ## Run tests
	$(GOTEST) -v ./...

test-race: ## Run tests with race detector
	$(GOTEST) -race -v ./...

test-coverage: ## Run tests with coverage report
	$(GOTEST) -v -coverprofile=$(COVERAGE_FILE) ./...
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)

vet: ## Run go vet
	$(GOVET) ./...

fmt: ## Format code
	$(GOFMT) ./...

tidy: ## Tidy and verify dependencies
	$(GOMOD) tidy
	$(GOMOD) verify

lint: ## Run linter (requires golangci-lint)
	golangci-lint run

# Docker targets
docker-build: ## Build Docker images
	docker compose build

docker-up: ## Start Docker containers
	docker compose up -d

docker-down: ## Stop Docker containers
	docker compose down

docker-logs: ## View Docker logs
	docker compose logs -f

# Migration targets
migrate-up: ## Run database migrations up
	migrate -path migrations -database "postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable" up

migrate-down: ## Run database migrations down
	migrate -path migrations -database "postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable" down

migrate-create: ## Create a new migration (usage: make migrate-create NAME=migration_name)
	migrate create -ext sql -dir migrations -seq $(NAME)

# Run targets
run-api: ## Run the API service
	$(GOCMD) run $(MAIN_PATH) api

run-worker: ## Run the worker service
	$(GOCMD) run $(MAIN_PATH) worker

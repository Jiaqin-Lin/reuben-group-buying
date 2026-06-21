.PHONY: build run test lint clean dev up down build-web docker-build docker-up docker-down monitor-up monitor-down

# Variables
APP_NAME := group-buy
CMD_DIR := ./cmd/server
BUILD_DIR := ./bin

# Build (backend only)
build:
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

# Build web + backend together (production)
build-web:
	cd web && npm run build
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

run: build
	$(BUILD_DIR)/$(APP_NAME)

# Dev - start deps + run with hot reload (requires air)
dev:
	air -c .air.toml

# Docker
up:
	docker compose up -d mysql redis app

down:
	docker compose down

docker-build:
	docker compose build app

docker-up:
	docker compose up -d --build mysql redis app

docker-down:
	docker compose down

# Monitoring (Prometheus + Grafana, requires app already running)
monitor-up:
	docker compose up -d prometheus grafana

monitor-down:
	docker compose stop prometheus grafana
	docker compose rm -f prometheus grafana

# Test
test:
	go test ./... -count=1 -race -short

test-verbose:
	go test ./... -count=1 -race -v

test-cover:
	go test ./... -count=1 -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# DB
migrate:
	mysql -h 127.0.0.1 -u dev -pdev123 group_buy_market < migrations/001_init.sql

# Clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

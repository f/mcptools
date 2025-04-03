# Color definitions
BLUE=\033[0;34m
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

BINARY_NAME=mcp
ALIAS_NAME=mcpt
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Default Makefile step
default: setup
	@echo "$(GREEN)All setup steps completed successfully!$(NC)"

check-go:
	@echo "$(BLUE)Checking Go installation and version...$(NC)"
	chmod +x ./scripts/check_go.bash
	./scripts/check_go.bash

build:
	@echo "$(YELLOW)Building $(BINARY_NAME)...$(NC)"
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/$(BINARY_NAME) ./cmd/mcptools

install:
	@echo "$(YELLOW)Installing $(BINARY_NAME)...$(NC)"
	sudo install bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	sudo ln -s /usr/local/bin/$(BINARY_NAME) /usr/local/bin/$(ALIAS_NAME)

uninstall:
	@echo "$(YELLOW)Uninstalling $(BINARY_NAME)...$(NC)"
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	sudo rm -f /usr/local/bin/$(ALIAS_NAME)

test: check-go
	@echo "$(YELLOW)Running tests...$(NC)"
	go test -v ./...

lint: check-go
	@echo "$(BLUE)Running linter...$(NC)"
	golangci-lint run ./...

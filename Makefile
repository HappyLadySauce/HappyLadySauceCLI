# HappyLadySauceCLI — project Makefile
# Windows: GnuWin32 make (cmd.exe) or Git Bash make (bash).
# Install: choco install make  OR  Git for Windows (Git Bash).
#
# Quick start:
#   copy .env.example .env   (Windows)  /  cp .env.example .env  (Unix)
#   make run

PROJECT  := HAPPLADYSAUCECLI
MODULE   := github.com/HappyLadySauce/HappyLadySauceCLI
CMD      := ./cmd/root.go
BIN_DIR  := bin
GO       := go

ifeq ($(OS),Windows_NT)
	BIN_EXT := .exe
	SHELL := cmd.exe
	.SHELLFLAGS := /c
	IS_WINDOWS := 1
	VERSION := $(shell git describe --tags --always --dirty 2>nul)
	ifeq ($(VERSION),)
		VERSION := dev
	endif
else
	BIN_EXT :=
	SHELL := /usr/bin/env bash
	IS_WINDOWS :=
	VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
endif

BINARY   := $(BIN_DIR)/$(PROJECT)$(BIN_EXT)
LDFLAGS  := -s -w

# Auto-load local secrets; missing .env is OK.
# 自动加载本地密钥；.env 不存在时不报错。
-include .env
export

.PHONY: help deps tidy fmt vet lint check verify build install run run-bin check-env \
        test test-v test-race test-cover test-cover-html test-agents test-commands test-security \
        cross clean clean-cache

.DEFAULT_GOAL := help

## help: Show this help message
help:
ifeq ($(IS_WINDOWS),1)
	@echo HappyLadySauceCLI Makefile (version: $(VERSION))
	@echo Usage: make [target]
	@findstr /R /C:"^## " Makefile
else
	@echo "HappyLadySauceCLI Makefile (version: $(VERSION))"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
endif

## tidy: Tidy go.mod / go.sum
tidy:
	$(GO) mod tidy

## fmt: Format all Go source files
fmt:
	$(GO) fmt ./...

## vet: Run go vet on all packages
vet:
	$(GO) vet ./...

## lint: Run golangci-lint (skipped with warning if not installed)
lint:
ifeq ($(IS_WINDOWS),1)
	@where golangci-lint >nul 2>&1 && golangci-lint run ./... || echo warning: golangci-lint not found in PATH; skipping lint
else
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "warning: golangci-lint not found in PATH; skipping lint"; \
	fi
endif

## check: fmt, vet, lint, and tidy (mutates files — use before commit)
check: fmt vet lint tidy

## verify: Read-only quality gate (vet + lint + tidy check, no fmt)
verify: vet lint
ifeq ($(IS_WINDOWS),1)
	@$(GO) mod tidy -diff 2>nul || $(GO) mod verify
else
	@$(GO) mod tidy -diff 2>/dev/null || $(GO) mod verify
endif

## build: Build binary to bin/HAPPLADYSAUCECLI[.exe]
build:
ifeq ($(IS_WINDOWS),1)
	@if not exist "$(BIN_DIR)" mkdir "$(BIN_DIR)"
	@set CGO_ENABLED=0&& $(GO) build -ldflags "$(LDFLAGS)" -o "$(BINARY)" $(CMD)
	@echo built $(BINARY)
else
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)
	@echo "built $(BINARY)"
endif

## install: Install binary to GOPATH/bin
install:
	$(GO) install $(CMD)

## check-env: Ensure required HAPPLADYSAUCECLI_* variables are set
check-env:
ifeq ($(IS_WINDOWS),1)
	@if "$(HAPPLADYSAUCECLI_BASE_URL)"=="" (echo error: HAPPLADYSAUCECLI_BASE_URL is not set & echo copy .env.example .env and fill HAPPLADYSAUCECLI_* values & exit /b 1)
	@if "$(HAPPLADYSAUCECLI_MODEL)"=="" (echo error: HAPPLADYSAUCECLI_MODEL is not set & echo copy .env.example .env and fill HAPPLADYSAUCECLI_* values & exit /b 1)
else
	@missing=0; \
	if [ -z "$${HAPPLADYSAUCECLI_BASE_URL:-}" ]; then echo "error: HAPPLADYSAUCECLI_BASE_URL is not set"; missing=1; fi; \
	if [ -z "$${HAPPLADYSAUCECLI_MODEL:-}" ]; then echo "error: HAPPLADYSAUCECLI_MODEL is not set"; missing=1; fi; \
	if [ "$$missing" -ne 0 ]; then \
		echo ""; \
		echo "Copy .env.example to .env, fill in HAPPLADYSAUCECLI_* values, then run: make run"; \
		echo "  cp .env.example .env"; \
		exit 1; \
	fi
endif

## run: Run CLI with go run (requires HAPPLADYSAUCECLI_BASE_URL and HAPPLADYSAUCECLI_MODEL)
run: check-env
	$(GO) run $(CMD)

## run-bin: Build and run the compiled binary
run-bin: build check-env
ifeq ($(IS_WINDOWS),1)
	"$(BINARY)"
else
	./$(BINARY)
endif

## test: Run all tests
test:
	$(GO) test ./...

## test-v: Run all tests with verbose output
test-v:
	$(GO) test -v ./...

## test-race: Run tests with race detector (may be limited on Windows)
test-race:
	$(GO) test -race ./...

## test-cover: Run tests with coverage report
test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	@echo HTML report: make test-cover-html

## test-cover-html: Generate coverage.html from coverage.out
test-cover-html: test-cover
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo wrote coverage.html

## cross: Cross-compile for linux/darwin/windows amd64
cross:
ifeq ($(IS_WINDOWS),1)
	@if not exist "$(BIN_DIR)" mkdir "$(BIN_DIR)"
	@set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=amd64&& $(GO) build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(PROJECT)-linux-amd64" $(CMD)
	@set CGO_ENABLED=0&& set GOOS=darwin&& set GOARCH=amd64&& $(GO) build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(PROJECT)-darwin-amd64" $(CMD)
	@set CGO_ENABLED=0&& set GOOS=darwin&& set GOARCH=arm64&& $(GO) build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(PROJECT)-darwin-arm64" $(CMD)
	@set CGO_ENABLED=0&& set GOOS=windows&& set GOARCH=amd64&& $(GO) build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(PROJECT)-windows-amd64.exe" $(CMD)
	@echo cross-build artifacts in $(BIN_DIR)/
else
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(PROJECT)-linux-amd64     $(CMD)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(PROJECT)-darwin-amd64    $(CMD)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(PROJECT)-darwin-arm64    $(CMD)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(PROJECT)-windows-amd64.exe $(CMD)
	@echo "cross-build artifacts in $(BIN_DIR)/"
endif

## clean: Remove build artifacts and coverage files
clean:
ifeq ($(IS_WINDOWS),1)
	@if exist "$(BIN_DIR)" rmdir /s /q "$(BIN_DIR)"
	@if exist coverage.out del /f /q coverage.out
	@if exist coverage.html del /f /q coverage.html
else
	rm -rf $(BIN_DIR) coverage.out coverage.html
endif

## clean-cache: Clear Go build and test caches
clean-cache:
	$(GO) clean -cache -testcache

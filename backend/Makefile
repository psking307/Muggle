.PHONY: dev fmt lint test build

# 当前 WSL 的 Go 安装在 /usr/local/go；允许调用者用 GO=... 覆盖。
GO ?= /usr/local/go/bin/go

dev:
	$(GO) run ./cmd/api

fmt:
	$(GO) fmt ./...

lint:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	mkdir -p bin
	$(GO) build -o bin/api ./cmd/api

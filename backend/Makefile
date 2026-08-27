.PHONY: dev dev-worker fmt lint test build

# 当前 WSL 的 Go 安装在 /usr/local/go；允许调用者用 GO=... 覆盖。
GO ?= /usr/local/go/bin/go

dev:
	$(GO) run ./cmd/api

# 阶段六：运行独立 Worker 进程（消费 Kafka 浏览事件并幂等更新浏览量）。
dev-worker:
	$(GO) run ./cmd/worker

fmt:
	$(GO) fmt ./...

lint:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	mkdir -p bin
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/worker ./cmd/worker
	$(GO) build -o bin/admin ./cmd/admin

.PHONY: dev fmt lint test build

dev:
	go run ./cmd/api

fmt:
	go fmt ./...

lint:
	go vet ./...

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/api ./cmd/api

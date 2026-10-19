.PHONY: help fmt lint test test-integration test-race coverage vulncheck build run worker migrate-up migrate-down sqlc proto arch-check up down new-module dev-env

help:
	@echo "Targets: fmt lint test test-integration test-race coverage vulncheck build run worker migrate-up migrate-down sqlc proto arch-check up down new-module dev-env"

dev-env:
	go run ./tools/devenv

fmt:
	go fmt ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

coverage:
	go run ./tools/coverage

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test-race:
ifeq ($(OS),Windows_NT)
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.26.9 sh -c "go test -race ./..."
else
	go test -race ./...
endif

build:
	go build ./cmd/...

run:
	go run ./cmd/api

worker:
	go run ./cmd/worker

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

sqlc:
	go run ./tools/sqlc

proto:
	buf generate api/proto

arch-check:
	go test -count=1 ./internal/archtest

up:
	docker compose up --build -d

down:
	docker compose down

new-module:
	go run ./tools/newmodule -name $(name)

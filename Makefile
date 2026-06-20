BINARY := trae
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X github.com/bytedance/trae-agent/internal/cli.Version=$(VERSION) -X github.com/bytedance/trae-agent/internal/cli.Commit=$(COMMIT)

.PHONY: build test lint clean vet fmt

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/trae

test:
	go test ./... -v

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

clean:
	rm -rf $(BIN_DIR)

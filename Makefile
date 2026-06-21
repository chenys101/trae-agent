BINARY := trae
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X github.com/bytedance/trae-agent/internal/cli.Version=$(VERSION) -X github.com/bytedance/trae-agent/internal/cli.Commit=$(COMMIT)

.PHONY: build test test-verbose coverage lint clean vet fmt

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/trae

test:
	go test -race ./...

test-verbose:
	go test -race -v ./...

coverage:
	go test -race -cover ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

# lint 在缺工具时输出警告并 exit 0，保持本地友好；
# 注意：CI 环境应使用硬失败（例如直接调用 golangci-lint run），避免静默跳过。
lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

clean:
	rm -rf $(BIN_DIR)

.PHONY: build build-windows build-linux build-all run test lint clean install

# 二进制输出目录
BIN_DIR := bin
# 当前操作系统
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
# 版本号（从 git 获取，无 git 时用 dev）
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)

# 默认目标：编译当前平台的二进制
build:
	@echo "Building trae for $(GOOS)/$(GOARCH)..."
	go build -ldflags "-s -w" -o $(BIN_DIR)/trae ./cmd/trae
	@echo "Done: $(BIN_DIR)/trae"

# 编译 Windows 版本（用于在 Linux/WSL 上交叉编译给 Windows）
build-windows:
	@echo "Building trae.exe for windows/amd64..."
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/trae.exe ./cmd/trae
	@echo "Done: $(BIN_DIR)/trae.exe"

# 编译 Linux 版本
build-linux:
	@echo "Building trae for linux/amd64..."
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o $(BIN_DIR)/trae-linux-amd64 ./cmd/trae
	@echo "Done: $(BIN_DIR)/trae-linux-amd64"

# 编译所有平台
build-all: build-windows build-linux
	@echo "All platforms built."

# 编译并运行（替代 go run，利用 go build 缓存）
run: build
	./$(BIN_DIR)/trae $(ARGS)

# 快速编译不加优化（首次编译更快，调试用）
build-debug:
	go build -o $(BIN_DIR)/trae ./cmd/trae
	@echo "Done: $(BIN_DIR)/trae (debug, no optimization)"

# 运行测试
test:
	go test -race ./...

# 运行 lint
lint:
	bash scripts/lint.sh

# 安装到 GOBIN（可直接全局使用 trae 命令）
install:
	go install ./cmd/trae
	@echo "Installed. Run 'trae' to use."

# 清理
clean:
	rm -rf $(BIN_DIR)

.PHONY: build test lint install clean cross-compile

# 变量
BINARY_NAME=trae
BUILD_DIR=./bin
GO=go
LDFLAGS=-ldflags "-s -w"
GOFLAGS=-trimpath

# 构建
build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/trae/

# 运行测试
test:
	$(GO) test ./... -count=1 -v

# 运行测试（简洁输出）
test-short:
	$(GO) test ./... -count=1

# 安装 golangci-lint 并运行
lint:
	which golangci-lint > /dev/null 2>&1 || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin
	golangci-lint run ./...

# 安装到 GOPATH/bin
install:
	CGO_ENABLED=0 $(GO) install $(GOFLAGS) $(LDFLAGS) ./cmd/trae/

# 清理构建产物
clean:
	rm -rf $(BUILD_DIR)

# 交叉编译
cross-compile:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/trae/
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/trae/
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/trae/
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/trae/

# 运行（需要 ANTHROPIC_API_KEY 或 OPENAI_API_KEY 环境变量）
run: build
	$(BUILD_DIR)/$(BINARY_NAME) run "List all Go source files in the current project"

# 显示工具列表
tools: build
	$(BUILD_DIR)/$(BINARY_NAME) tools

# 显示配置
show-config: build
	$(BUILD_DIR)/$(BINARY_NAME) show-config

# 依赖整理
tidy:
	$(GO) mod tidy

# 验证构建
verify: build test
	@echo "Build and tests passed successfully!"
